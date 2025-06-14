package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/metrics"
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
	"github.com/quic-go/quic-go"
)

// SessionLauncher defines the interface for launching new sessions
type SessionLauncher interface {
	Launch(ctx context.Context) (*Session, error)
}

// Session role constants
const (
	RolePrimary   = "primary"
	RoleSecondary = "secondary"
	RoleDraining  = "draining"
)

// Session represents an active QUIC connection session
type Session struct {
	ID            string
	QuicConn      quic.Connection
	Cancel        context.CancelFunc
	StartedAt     time.Time
	ControlStream quic.Stream
	Role          string
	TTL           time.Duration
	healthy       bool
	healthMutex   sync.RWMutex
	missedPings   int
	LambdaPublicIP string
}

// LaunchState tracks the state of session launches to prevent race conditions
type LaunchState struct {
	launchingPrimary   bool
	launchingSecondary bool
	lastLaunchAttempt  time.Time
	failedAttempts     int
	mu                 sync.Mutex // Protects launch state
}

// ConnManager manages the lifecycle of QUIC connection sessions
type ConnManager struct {
	cfg         *config.Config
	launcher    SessionLauncher
	mu          sync.RWMutex
	
	// Resource management
	activeGoroutines sync.WaitGroup
	shutdownOnce     sync.Once
	shutdownCh       chan struct{}
	
	// Resource limits
	maxSessions     int
	maxGoroutines   int
	currentSessions int
	
	sessions    []*Session
	launchState *LaunchState
}

// New creates a new ConnManager instance
func New(cfg *config.Config, launcher SessionLauncher) *ConnManager {
	return &ConnManager{
		cfg:         cfg,
		launcher:    launcher,
		launchState: &LaunchState{},
		
		// Resource management
		shutdownCh:    make(chan struct{}),
		maxSessions:   10, // Configurable limit
		maxGoroutines: 50, // Prevent goroutine explosion
	}
}

// startGoroutine safely starts a goroutine with resource tracking
func (cm *ConnManager) startGoroutine(name string, fn func()) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	// Check if we're shutting down
	select {
	case <-cm.shutdownCh:
		return fmt.Errorf("manager is shutting down, cannot start goroutine %s", name)
	default:
	}
	
	// Check goroutine limit
	// Track goroutine creation for monitoring
	
	cm.activeGoroutines.Add(1)
	go func() {
		defer cm.activeGoroutines.Done()
		defer func() {
			if r := recover(); r != nil {
				shared.LogErrorf("Goroutine %s panicked: %v", name, r)
			}
		}()
		fn()
	}()
	
	return nil
}

// cleanupSession ensures complete cleanup of a session
func (cm *ConnManager) cleanupSession(session *Session) error {
	if session == nil {
		return nil
	}
	
	shared.LogInfof("Cleaning up session %s", session.ID)
	
	// Cancel the session context
	if session.Cancel != nil {
		session.Cancel()
	}
	
	// Close control stream
	if session.ControlStream != nil {
		if err := session.ControlStream.Close(); err != nil {
			shared.LogErrorf("Failed to close control stream for session %s: %v", session.ID, err)
		}
	}
	
	// Close QUIC connection
	if session.QuicConn != nil {
		if err := session.QuicConn.CloseWithError(0, "session cleanup"); err != nil {
			shared.LogErrorf("Failed to close QUIC connection for session %s: %v", session.ID, err)
		}
	}
	
	return nil
}

// Start launches the first session and monitors it, blocking on the provided context
func (cm *ConnManager) Start(ctx context.Context) error {
	shared.LogInfo("ConnManager: Starting session management")
	
	// Launch initial session
	session, err := cm.launchSession(ctx)
	if err != nil {
		return fmt.Errorf("failed to launch initial session: %w", err)
	}
	
	// Set as primary role
	session.Role = RolePrimary
	
	cm.mu.Lock()
	cm.sessions = []*Session{session}
	metrics.SetActiveSessions(len(cm.sessions))
	cm.mu.Unlock()
	
	// Start monitoring in background
	if err := cm.startGoroutine("monitor", func() { cm.monitor(ctx) }); err != nil {
		return fmt.Errorf("failed to start monitor goroutine: %w", err)
	}
	
	// Block until context is cancelled
	<-ctx.Done()
	
	// Begin shutdown process
	return cm.shutdown()
}

// shutdown gracefully shuts down the ConnManager
func (cm *ConnManager) shutdown() error {
	var err error
	
	cm.shutdownOnce.Do(func() {
		shared.LogInfo("ConnManager: Beginning graceful shutdown")
		
		// Signal shutdown to prevent new goroutines
		close(cm.shutdownCh)
		
		// Clean up all sessions
		cm.mu.Lock()
		sessions := make([]*Session, len(cm.sessions))
		copy(sessions, cm.sessions)
		cm.sessions = nil
		cm.mu.Unlock()
		
		// Clean up each session
		for _, session := range sessions {
			if cleanupErr := cm.cleanupSession(session); cleanupErr != nil {
				shared.LogErrorf("Error cleaning up session %s: %v", session.ID, cleanupErr)
				if err == nil {
					err = cleanupErr
				}
			}
		}
		
		// Wait for all goroutines to finish with timeout
		done := make(chan struct{})
		go func() {
			cm.activeGoroutines.Wait()
			close(done)
		}()
		
		select {
		case <-done:
			shared.LogInfo("ConnManager: All goroutines finished cleanly")
		case <-time.After(5 * time.Second):
			shared.LogError("ConnManager: Timeout waiting for goroutines to finish", fmt.Errorf("shutdown timeout"))
		}
		
		shared.LogInfo("ConnManager: Shutdown complete")
	})
	
	return err
}

// monitor watches sessions and handles rotation
func (cm *ConnManager) monitor(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.checkSessions(ctx)
		}
	}
}

// checkSessions examines all sessions and handles rotation/cleanup
func (cm *ConnManager) checkSessions(ctx context.Context) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	// Remove closed or unhealthy sessions
	activeSessions := make([]*Session, 0, len(cm.sessions))
	var primarySession *Session
	
	for _, session := range cm.sessions {
		// Check if session is closed
		select {
		case <-session.QuicConn.Context().Done():
			shared.LogInfof("ConnManager: Session %s (%s) closed", session.ID, session.Role)
			continue
		default:
		}
		
		// Check if session is unhealthy
		if !session.IsHealthy() && !session.IsDraining() {
			shared.LogInfof("ConnManager: Session %s (%s) unhealthy, removing", session.ID, session.Role)
			session.Cancel()
			continue
		}
		
		activeSessions = append(activeSessions, session)
		if session.IsPrimary() {
			primarySession = session
		}
	}
	
	cm.sessions = activeSessions
	metrics.SetActiveSessions(len(cm.sessions))
	
	// If no primary session, launch one (but only if we don't have too many sessions)
	if primarySession == nil {
		if len(activeSessions) < 2 && cm.canLaunchPrimary() {
			shared.LogInfo("ConnManager: No primary session, launching new one")
			go cm.launchPrimarySession(ctx)
		} else {
			shared.LogInfof("ConnManager: No primary session but %d sessions exist, waiting for cleanup", len(activeSessions))
		}
	} else {
		// Check if primary needs rotation based on TTL
		remaining := primarySession.RemainingTTL()
		if remaining <= cm.cfg.Rotation.OverlapWindow {
			// Check if we already have a secondary
			hasSecondary := false
			for _, session := range cm.sessions {
				if session.IsSecondary() {
					hasSecondary = true
					break
				}
			}
			
			// Use atomic launch state check to prevent race conditions
			if !hasSecondary && len(cm.sessions) < 2 && cm.canLaunchSecondary() {
				shared.LogInfof("ConnManager: Primary session %s TTL %v <= overlap window %v, launching secondary", 
					primarySession.ID, remaining, cm.cfg.Rotation.OverlapWindow)
				go cm.launchSecondarySession(ctx)
			}
		}
	}
}

// launchSession creates a new session using the launcher
func (cm *ConnManager) launchSession(ctx context.Context) (*Session, error) {
	sessionCtx, cancel := context.WithCancel(ctx)
	
	session, err := cm.launcher.Launch(sessionCtx)
	if err != nil {
		cancel()
		return nil, err
	}
	
	// Store the cancel function in the session
	session.Cancel = cancel
	
	return session, nil
}

// GetCurrent returns the current primary session
func (cm *ConnManager) GetCurrent() *Session {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	var selectedSession *Session
	
	// First, look for a healthy primary session
	for _, session := range cm.sessions {
		if session.IsPrimary() && session.IsHealthy() {
			selectedSession = session
			break
		}
	}
	
	// If no healthy primary, look for any healthy secondary (during transition)
	if selectedSession == nil {
		for _, session := range cm.sessions {
			if session.IsSecondary() && session.IsHealthy() {
				selectedSession = session
				break
			}
		}
	}
	
	// Last resort: return any healthy session (but not draining)
	if selectedSession == nil {
		for _, session := range cm.sessions {
			if session.IsHealthy() && !session.IsDraining() {
				selectedSession = session
				break
			}
		}
	}
	
	// Only log when no session is found and we have sessions (unusual condition)
	if selectedSession == nil && len(cm.sessions) > 0 {
		shared.LogNetworkf("GetCurrent: No suitable session found among %d sessions", len(cm.sessions))
		for i, session := range cm.sessions {
			shared.LogNetworkf("  Session %d: %s (role: %s, healthy: %v, draining: %v)", 
				i, session.ID, session.Role, session.IsHealthy(), session.IsDraining())
		}
	}
	
	return selectedSession
}

// GetAllSessions returns all sessions for dashboard display
func (cm *ConnManager) GetAllSessions() []*Session {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	// Return a copy of all sessions
	sessionsCopy := make([]*Session, len(cm.sessions))
	copy(sessionsCopy, cm.sessions)
	
	return sessionsCopy
}

// Primary returns the primary session (alias for GetCurrent)
func (cm *ConnManager) Primary() *Session {
	return cm.GetCurrent()
}

// WaitForSession waits until a session is available or context is cancelled
func (cm *ConnManager) WaitForSession(ctx context.Context) (*Session, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if session := cm.GetCurrent(); session != nil {
				return session, nil
			}
		}
	}
}

// IsHealthy returns whether the session is healthy
func (s *Session) IsHealthy() bool {
	s.healthMutex.RLock()
	defer s.healthMutex.RUnlock()
	return s.healthy
}

// SetHealthy sets the health status of the session
func (s *Session) SetHealthy(healthy bool) {
	s.healthMutex.Lock()
	defer s.healthMutex.Unlock()
	s.healthy = healthy
}

// IncrementMissedPings increments the missed ping counter
func (s *Session) IncrementMissedPings() int {
	s.healthMutex.Lock()
	defer s.healthMutex.Unlock()
	s.missedPings++
	return s.missedPings
}

// ResetMissedPings resets the missed ping counter
func (s *Session) ResetMissedPings() {
	s.healthMutex.Lock()
	defer s.healthMutex.Unlock()
	s.missedPings = 0
}

// RemainingTTL returns the remaining time to live for the session
func (s *Session) RemainingTTL() time.Duration {
	elapsed := time.Since(s.StartedAt)
	remaining := s.TTL - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IsPrimary returns true if the session is in primary role
func (s *Session) IsPrimary() bool {
	return s.Role == RolePrimary
}

// IsSecondary returns true if the session is in secondary role
func (s *Session) IsSecondary() bool {
	return s.Role == RoleSecondary
}

// IsDraining returns true if the session is in draining role
func (s *Session) IsDraining() bool {
	return s.Role == RoleDraining
}

// launchPrimarySession launches a new primary session
func (cm *ConnManager) launchPrimarySession(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			shared.LogErrorf("ConnManager: Panic in launchPrimarySession: %v", r)
			cm.clearLaunchState(true, false)
		}
	}()
	defer cm.clearLaunchState(true, false) // Default to failure, update on success
	
	// Check if we already have a primary (race condition guard)
	cm.mu.Lock()
	for _, session := range cm.sessions {
		if session.IsPrimary() {
			cm.mu.Unlock()
			shared.LogInfo("ConnManager: Primary session already exists, skipping launch")
			cm.clearLaunchState(true, true) // Not a failure, just redundant
			return
		}
	}
	cm.mu.Unlock()
	
	session, err := cm.launchSession(ctx)
	if err != nil {
		shared.LogErrorf("ConnManager: Failed to launch primary session: %v", err)
		metrics.RecordSessionFailure()
		return
	}
	
	metrics.RecordSessionLaunch()
	
	session.Role = RolePrimary
	
	cm.mu.Lock()
	// Double-check after acquiring lock (race condition guard)
	for _, existingSession := range cm.sessions {
		if existingSession.IsPrimary() {
			cm.mu.Unlock()
			shared.LogInfof("ConnManager: Primary session already exists, discarding new session %s", session.ID)
			cm.cleanupSession(session) // Use proper cleanup
			cm.clearLaunchState(true, true) // Not a failure, just redundant
			return
		}
	}
	cm.sessions = append(cm.sessions, session)
	cm.mu.Unlock()
	
	cm.clearLaunchState(true, true) // Success
	shared.LogSuccessf("ConnManager: Successfully launched primary session %s", session.ID)
}

// launchSecondarySession launches a new secondary session for rotation
func (cm *ConnManager) launchSecondarySession(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			shared.LogErrorf("ConnManager: Panic in launchSecondarySession: %v", r)
			cm.clearLaunchState(false, false)
		}
	}()
	defer cm.clearLaunchState(false, false) // Default to failure, update on success
	
	// Check if we already have a secondary or are at max sessions (race condition guard)
	cm.mu.Lock()
	hasSecondary := false
	for _, session := range cm.sessions {
		if session.IsSecondary() {
			hasSecondary = true
			break
		}
	}
	if hasSecondary || len(cm.sessions) >= 2 {
		cm.mu.Unlock()
		shared.LogInfof("ConnManager: Secondary already exists or at max sessions (%d), skipping launch", len(cm.sessions))
		cm.clearLaunchState(false, true) // Not a failure, just redundant
		return
	}
	cm.mu.Unlock()
	
	session, err := cm.launchSession(ctx)
	if err != nil {
		shared.LogErrorf("ConnManager: Failed to launch secondary session: %v", err)
		metrics.RecordSessionFailure()
		return
	}
	
	metrics.RecordSessionLaunch()
	
	session.Role = RoleSecondary
	
	cm.mu.Lock()
	// Double-check after acquiring lock (race condition guard)
	hasSecondary = false
	for _, existingSession := range cm.sessions {
		if existingSession.IsSecondary() {
			hasSecondary = true
			break
		}
	}
	if hasSecondary || len(cm.sessions) >= 2 {
		cm.mu.Unlock()
		shared.LogInfof("ConnManager: Secondary already exists or at max sessions (%d), discarding new session %s", len(cm.sessions), session.ID)
		session.Cancel()
		cm.clearLaunchState(false, true) // Not a failure, just redundant
		return
	}
	cm.sessions = append(cm.sessions, session)
	
	// Check if secondary is healthy and promote it to primary
	go cm.checkForPromotion(ctx, session)
	cm.mu.Unlock()
	
	cm.clearLaunchState(false, true) // Success
	shared.LogInfof("ConnManager: Successfully launched secondary session %s", session.ID)
}

// checkForPromotion monitors a secondary session and promotes it when ready
func (cm *ConnManager) checkForPromotion(ctx context.Context, secondary *Session) {
	defer func() {
		if r := recover(); r != nil {
			shared.LogErrorf("ConnManager: Panic in checkForPromotion: %v", r)
		}
	}()
	
	// Wait longer for the secondary to establish health and verify multiple health checks
	healthCheckCount := 0
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	timeout := time.NewTimer(45 * time.Second) // Increased from 20s
	defer timeout.Stop()
	
	for {
		select {
		case <-timeout.C:
			shared.LogInfof("ConnManager: Secondary session %s promotion timeout reached", secondary.ID)
			return
		case <-ctx.Done():
			return
		case <-secondary.QuicConn.Context().Done():
			shared.LogInfof("ConnManager: Secondary session %s closed before promotion", secondary.ID)
			return
		case <-ticker.C:
			if secondary.IsHealthy() {
				healthCheckCount++
				shared.LogInfof("ConnManager: Secondary session %s health check %d/3 passed", secondary.ID, healthCheckCount)
				
				// Require 3 consecutive successful health checks before promotion
				if healthCheckCount >= 3 {
					shared.LogInfof("ConnManager: Promoting secondary session %s to primary", secondary.ID)
					cm.promoteSecondary(secondary)
					return
				}
			} else {
				// Reset counter if health check fails
				if healthCheckCount > 0 {
					healthCheckCount = 0
				}
			}
		}
	}
}

// promoteSecondary promotes a secondary session to primary
func (cm *ConnManager) promoteSecondary(secondary *Session) {
	var oldPrimary *Session
	
	// Critical section: promote sessions atomically
	func() {
		cm.mu.Lock()
		defer cm.mu.Unlock()
		
		// Verify the secondary is still healthy before promotion
		if !secondary.IsHealthy() {
			shared.LogInfof("ConnManager: Secondary session %s no longer healthy, skipping promotion", secondary.ID)
			return
		}
		
		// Find the current primary session
		for _, session := range cm.sessions {
			if session != secondary && session.IsPrimary() {
				oldPrimary = session
				break
			}
		}
		
		// Promote secondary to primary first (atomic operation)
		secondary.Role = RolePrimary
		shared.LogInfof("ConnManager: Session %s promoted to primary", secondary.ID)
		
		// Then demote old primary to draining
		if oldPrimary != nil {
			oldPrimary.Role = RoleDraining
			shared.LogInfof("ConnManager: Session %s demoted to draining", oldPrimary.ID)
		}
	}()
	
	// Start drain cleanup AFTER releasing the lock to avoid deadlock
	if oldPrimary != nil {
		cm.startGoroutine(fmt.Sprintf("drain-cleanup-%s", oldPrimary.ID), func() {
			cm.scheduleDrainCleanup(oldPrimary)
		})
	}
	
	metrics.RecordSessionRotation()
}

// sendShutdownSignal sends a shutdown signal to a session
func (cm *ConnManager) sendShutdownSignal(session *Session) {
	if session.ControlStream == nil {
		shared.LogInfof("ConnManager: No control stream for session %s, cannot send shutdown", session.ID)
		return
	}
	
	shared.LogInfof("ConnManager: Sending SHUTDOWN signal to session %s", session.ID)
	if err := shared.WriteShutdown(session.ControlStream); err != nil {
		shared.LogErrorf("ConnManager: Failed to send SHUTDOWN to session %s: %v", session.ID, err)
		return
	}
	
	shared.LogInfof("ConnManager: SHUTDOWN signal sent to session %s", session.ID)
}

// scheduleDrainCleanup schedules cleanup of a draining session
func (cm *ConnManager) scheduleDrainCleanup(session *Session) {
	shared.LogInfof("ConnManager: Starting drain cleanup for session %s (timeout: %v)", session.ID, cm.cfg.Rotation.DrainTimeout)
	timer := time.NewTimer(cm.cfg.Rotation.DrainTimeout)
	defer timer.Stop()
	
	select {
	case <-timer.C:
		shared.LogInfof("ConnManager: Drain timeout reached for session %s, sending shutdown signal", session.ID)
		// Send shutdown signal to Lambda after drain timeout
		cm.sendShutdownSignal(session)
		// Give Lambda a moment to exit cleanly
		time.Sleep(500 * time.Millisecond)
		// Then cancel the session
		shared.LogInfof("ConnManager: Cancelling draining session %s", session.ID)
		session.Cancel()
	case <-session.QuicConn.Context().Done():
		// Session closed naturally before timeout
		shared.LogInfof("ConnManager: Session %s closed naturally during drain", session.ID)
		return
	}
}

// canLaunchPrimary checks if we can launch a primary session (with cooldown)
func (cm *ConnManager) canLaunchPrimary() bool {
	cm.launchState.mu.Lock()
	defer cm.launchState.mu.Unlock()
	
	// Prevent launching if already launching
	if cm.launchState.launchingPrimary {
		return false
	}
	
	// Add cooldown period to prevent rapid retries
	cooldown := 5 * time.Second
	if cm.launchState.failedAttempts > 2 {
		cooldown = time.Duration(cm.launchState.failedAttempts) * 10 * time.Second
	}
	
	if time.Since(cm.launchState.lastLaunchAttempt) < cooldown {
		return false
	}
	
	// Set launching state
	cm.launchState.launchingPrimary = true
	cm.launchState.lastLaunchAttempt = time.Now()
	return true
}

// canLaunchSecondary checks if we can launch a secondary session (with cooldown)
func (cm *ConnManager) canLaunchSecondary() bool {
	cm.launchState.mu.Lock()
	defer cm.launchState.mu.Unlock()
	
	// Prevent launching if already launching
	if cm.launchState.launchingSecondary {
		return false
	}
	
	// Add cooldown period to prevent rapid retries (shorter for secondary)
	cooldown := 2 * time.Second
	if cm.launchState.failedAttempts > 2 {
		cooldown = time.Duration(cm.launchState.failedAttempts) * 5 * time.Second
	}
	
	if time.Since(cm.launchState.lastLaunchAttempt) < cooldown {
		return false
	}
	
	// Set launching state
	cm.launchState.launchingSecondary = true
	cm.launchState.lastLaunchAttempt = time.Now()
	return true
}

// clearLaunchState clears the launching state flags
func (cm *ConnManager) clearLaunchState(isPrimary bool, success bool) {
	cm.launchState.mu.Lock()
	defer cm.launchState.mu.Unlock()
	
	if isPrimary {
		cm.launchState.launchingPrimary = false
	} else {
		cm.launchState.launchingSecondary = false
	}
	
	if success {
		cm.launchState.failedAttempts = 0
	} else {
		cm.launchState.failedAttempts++
	}
}