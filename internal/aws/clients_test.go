package aws

import (
	"context"
	"testing"
	"time"

	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

func TestNewClientFactory(t *testing.T) {
	cfg := &config.CLIConfig{
		AWS: config.AWSConfig{
			Region: "us-west-2",
		},
	}
	
	factory, err := NewClientFactory(cfg)
	if err != nil {
		t.Fatalf("Expected no error creating client factory, got %v", err)
	}
	
	if factory == nil {
		t.Fatal("Expected client factory to be created")
	}
	
	if factory.session == nil {
		t.Fatal("Expected session to be created")
	}
}

func TestGetClients(t *testing.T) {
	cfg := &config.CLIConfig{
		AWS: config.AWSConfig{
			Region: "us-west-2",
		},
	}
	
	factory, err := NewClientFactory(cfg)
	if err != nil {
		t.Fatalf("Failed to create client factory: %v", err)
	}
	
	clients := factory.GetClients()
	
	// Test that interfaces are properly created
	if clients.CloudFormation == nil {
		t.Error("Expected CloudFormation client to be created")
	}
	if clients.Lambda == nil {
		t.Error("Expected Lambda client to be created")
	}
	if clients.S3 == nil {
		t.Error("Expected S3 client to be created")
	}
	if clients.STS == nil {
		t.Error("Expected STS client to be created")
	}
	if clients.CloudWatchLogs == nil {
		t.Error("Expected CloudWatchLogs client to be created")
	}
}

func TestGetRegion(t *testing.T) {
	cfg := &config.CLIConfig{
		AWS: config.AWSConfig{
			Region: "eu-west-1",
		},
	}
	
	factory, err := NewClientFactory(cfg)
	if err != nil {
		t.Fatalf("Failed to create client factory: %v", err)
	}
	
	region := factory.GetRegion()
	if region != "eu-west-1" {
		t.Errorf("Expected region eu-west-1, got %s", region)
	}
}

func TestWaitForOperation(t *testing.T) {
	ctx := context.Background()
	
	// Test successful operation
	called := false
	checkFn := func() (bool, error) {
		called = true
		return true, nil
	}
	
	err := WaitForOperation(ctx, checkFn, 1*time.Second)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !called {
		t.Error("Expected check function to be called")
	}
}

func TestWaitForOperationTimeout(t *testing.T) {
	ctx := context.Background()
	
	// Test timeout
	checkFn := func() (bool, error) {
		return false, nil // Never complete
	}
	
	err := WaitForOperation(ctx, checkFn, 10*time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestWaitForOperationContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	checkFn := func() (bool, error) {
		return false, nil
	}
	
	err := WaitForOperation(ctx, checkFn, 1*time.Second)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}