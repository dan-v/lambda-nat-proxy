package stun

import (
	"context"
	"testing"
	"time"
)

func TestDiscoverPublicIP_ContextCancel(t *testing.T) {
	client := New()
	
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	_, err := client.DiscoverPublicIP(ctx, "stun.l.google.com:19302")
	
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
}

func TestDiscoverPublicIP_InvalidServer(t *testing.T) {
	client := New()
	
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	
	_, err := client.DiscoverPublicIP(ctx, "invalid.server:12345")
	
	if err == nil {
		t.Error("Expected error for invalid STUN server")
	}
}

func TestNew(t *testing.T) {
	client := New()
	
	if client == nil {
		t.Error("Expected client to be created")
	}
	
	// Test that it implements the interface
	var _ Client = client
}