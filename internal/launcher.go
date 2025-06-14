package internal

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/manager"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/metrics"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/nat"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/quic"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/s3"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/stun"
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)

// Launcher implements the SessionLauncher interface
type Launcher struct {
	config       *config.Config
	stunClient   stun.Client
	s3Coord      s3.Coordinator
	natTraversal nat.Traversal
	quicServer   *quic.Server
}

// NewLauncher creates a new Launcher instance
func NewLauncher(cfg *config.Config, stunClient stun.Client, s3Coord s3.Coordinator, natTraversal nat.Traversal, quicServer *quic.Server) *Launcher {
	return &Launcher{
		config:       cfg,
		stunClient:   stunClient,
		s3Coord:      s3Coord,
		natTraversal: natTraversal,
		quicServer:   quicServer,
	}
}

// Launch creates a new session by performing the NAT traversal workflow
func (l *Launcher) Launch(ctx context.Context) (*manager.Session, error) {
	log.Println("Launcher: Starting new session launch")
	
	// 1. Discover public IP via STUN
	stunStart := time.Now()
	publicIP, err := l.stunClient.DiscoverPublicIP(ctx, l.config.STUNServer)
	stunLatency := time.Since(stunStart)
	metrics.RecordSTUNLatency(stunLatency)
	
	if err != nil {
		return nil, fmt.Errorf("failed to discover public IP: %w", err)
	}
	log.Printf("Launcher: Public IP: %s", publicIP)
	
	// 2. Create UDP socket for hole punching
	udpConn, localPort, err := l.natTraversal.CreateUDPSocket()
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP socket: %w", err)
	}
	// Note: udpConn ownership will be transferred to QUIC server
	
	// 3. Write coordination to S3 (triggers Lambda)
	sessionID := shared.GenerateSessionID()
	if err := l.s3Coord.WriteCoordination(ctx, sessionID, publicIP, localPort); err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("failed to write coordination to S3: %w", err)
	}
	log.Printf("Launcher: Coordination written for session: %s", sessionID)
	
	// 4. Wait for Lambda response
	lambdaResp, err := l.s3Coord.WaitForLambdaResponse(ctx, sessionID, l.config.LambdaResponseTimeout)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("failed to get Lambda response: %w", err)
	}
	log.Printf("Launcher: Lambda endpoint: %s:%d", lambdaResp.LambdaPublicIP, lambdaResp.LambdaPublicPort)
	
	// 5. Perform NAT hole punching
	lambdaAddr := &net.UDPAddr{
		IP:   net.ParseIP(lambdaResp.LambdaPublicIP),
		Port: lambdaResp.LambdaPublicPort,
	}
	
	natStart := time.Now()
	if err := l.natTraversal.PerformHolePunch(udpConn, sessionID, lambdaAddr, l.config.NATHolePunchTimeout); err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("NAT hole punching failed: %w", err)
	}
	natTraversalTime := time.Since(natStart)
	metrics.RecordNATTraversalTime(natTraversalTime)
	log.Println("Launcher: NAT hole punched successfully!")
	
	// 6. Start QUIC server and wait for Lambda connection
	quicStart := time.Now()
	quicConn, err := l.quicServer.StartAndAccept(ctx, udpConn, l.config)
	if err != nil {
		metrics.RecordQUICConnectionError()
		return nil, fmt.Errorf("failed to start QUIC server: %w", err)
	}
	quicHandshakeTime := time.Since(quicStart)
	metrics.RecordQUICHandshakeTime(quicHandshakeTime)
	
	log.Printf("Launcher: Session %s established with QUIC connection", sessionID)
	
	// Open control stream (stream 0)
	controlStream, err := quicConn.OpenStreamSync(ctx)
	if err != nil {
		metrics.RecordQUICConnectionError()
		quicConn.CloseWithError(0, "failed to open control stream")
		return nil, fmt.Errorf("failed to open control stream: %w", err)
	}
	
	// Record QUIC stream creation
	metrics.IncrementActiveQUICStreams()
	
	// Create the session
	session := &manager.Session{
		ID:            sessionID,
		QuicConn:      quicConn,
		StartedAt:     time.Now(),
		ControlStream: controlStream,
		TTL:           l.config.Rotation.SessionTTL,
		LambdaPublicIP: lambdaResp.LambdaPublicIP,
	}
	session.SetHealthy(true) // Start as healthy
	
	// Start health check loop
	go l.startHealthCheck(ctx, session)
	
	return session, nil
}

// startHealthCheck runs the health check loop for a session
func (l *Launcher) startHealthCheck(ctx context.Context, session *manager.Session) {
	defer func() {
		if r := recover(); r != nil {
			shared.LogErrorf("Panic in health check for session %s: %v", session.ID, r)
			session.SetHealthy(false)
		}
		shared.LogInfof("Health check for session %s stopped", session.ID)
	}()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	defer session.ControlStream.Close()
	
	var nonce uint64
	
	for {
		select {
		case <-ctx.Done():
			shared.LogInfof("Health check for session %s stopping due to context cancellation", session.ID)
			return
		case <-session.QuicConn.Context().Done():
			shared.LogInfof("Health check for session %s stopping due to QUIC connection closure", session.ID)
			return
		case <-ticker.C:
			nonce++
			
			// Record ping start time for RTT calculation
			pingStart := time.Now()
			
			// Check context before sending ping
			select {
			case <-ctx.Done():
				shared.LogInfof("Health check for session %s cancelling during ping", session.ID)
				return
			default:
			}
			
			// Send ping
			metrics.RecordPingSent()
			if err := shared.WritePing(session.ControlStream, nonce); err != nil {
				shared.LogErrorf("Failed to send ping to session %s: %v", session.ID, err)
				session.SetHealthy(false)
				metrics.SetSessionHealthy(false)
				return
			}
			
			// Set read deadline for pong with shorter timeout to be more responsive
			session.ControlStream.SetReadDeadline(time.Now().Add(3 * time.Second))
			
			// Read response with context check
			opcode, receivedNonce, err := shared.ReadControlMessage(session.ControlStream)
			
			// Always clear read deadline first
			session.ControlStream.SetReadDeadline(time.Time{})
			
			// Check context again after read
			select {
			case <-ctx.Done():
				shared.LogInfof("Health check for session %s cancelling after ping response", session.ID)
				return
			default:
			}
			
			if err != nil {
				missedCount := session.IncrementMissedPings()
				metrics.RecordMissedPing()
				shared.LogErrorf("Failed to receive pong from session %s (missed: %d): %v", session.ID, missedCount, err)
				
				if missedCount >= 3 {
					shared.LogErrorf("Session %s marked unhealthy after 3 missed pings", session.ID)
					session.SetHealthy(false)
					metrics.SetSessionHealthy(false)
					return
				}
				continue
			}
			
			if opcode == shared.OpPong && receivedNonce == nonce {
				// Calculate and record RTT
				rtt := time.Since(pingStart)
				metrics.RecordRTT(rtt)
				
				session.ResetMissedPings()
				session.SetHealthy(true)
				metrics.SetSessionHealthy(true)
				
				shared.LogInfof("Session %s health check: RTT %v", session.ID, rtt)
			} else if opcode == shared.OpShutdown {
				// Handle shutdown signal gracefully during health check
				shared.LogInfof("Session %s received shutdown signal during health check", session.ID)
				session.SetHealthy(false)
				metrics.SetSessionHealthy(false)
				return
			} else {
				shared.LogErrorf("Unexpected control message from session %s: opcode=%02x, nonce=%d (expected %d)", 
					session.ID, opcode, receivedNonce, nonce)
			}
		}
	}
}