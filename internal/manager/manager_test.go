package manager

import (
	"testing"
	"time"

	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)

func TestSession_RoleMethods(t *testing.T) {
	session := &Session{
		ID:   "test-session",
		Role: RolePrimary,
	}
	
	if !session.IsPrimary() {
		t.Error("Expected session to be primary")
	}
	if session.IsSecondary() {
		t.Error("Expected session not to be secondary")
	}
	if session.IsDraining() {
		t.Error("Expected session not to be draining")
	}
	
	session.Role = RoleSecondary
	if session.IsPrimary() {
		t.Error("Expected session not to be primary")
	}
	if !session.IsSecondary() {
		t.Error("Expected session to be secondary")
	}
	if session.IsDraining() {
		t.Error("Expected session not to be draining")
	}
	
	session.Role = RoleDraining
	if session.IsPrimary() {
		t.Error("Expected session not to be primary")
	}
	if session.IsSecondary() {
		t.Error("Expected session not to be secondary")
	}
	if !session.IsDraining() {
		t.Error("Expected session to be draining")
	}
}

func TestSession_RemainingTTL(t *testing.T) {
	session := &Session{
		ID:        "test-session",
		StartedAt: time.Now().Add(-2 * time.Minute), // Started 2 minutes ago
		TTL:       5 * time.Minute,
	}
	
	remaining := session.RemainingTTL()
	expected := 3 * time.Minute // 5 minute TTL - 2 minutes elapsed = 3 minutes
	
	// Allow for some timing tolerance
	if remaining < expected-time.Second || remaining > expected+time.Second {
		t.Errorf("Expected remaining TTL around %v, got %v", expected, remaining)
	}
	
	// Test expired session
	expiredSession := &Session{
		ID:        "expired-session",
		StartedAt: time.Now().Add(-6 * time.Minute), // Started 6 minutes ago
		TTL:       5 * time.Minute,
	}
	
	if expiredSession.RemainingTTL() != 0 {
		t.Errorf("Expected expired session to have 0 remaining TTL, got %v", expiredSession.RemainingTTL())
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := shared.GenerateSessionID()
	id2 := shared.GenerateSessionID()
	
	if id1 == id2 {
		t.Error("Expected different session IDs")
	}
	
	if len(id1) == 0 {
		t.Error("Expected non-empty session ID")
	}
}