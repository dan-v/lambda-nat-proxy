package s3

import (
	"testing"

	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
)

func TestNew(t *testing.T) {
	// Test that New creates a coordinator successfully
	// Using nil client since we're just testing construction
	coord := New(nil, "test-bucket")
	
	if coord == nil {
		t.Error("Expected coordinator to be created")
	}
	
	// Test that it implements the interface
	var _ Coordinator = coord
}

func TestCoordinatorInterface(t *testing.T) {
	// Test that DefaultCoordinator implements Coordinator interface
	var s3Client awsclients.S3API
	coord := New(s3Client, "test-bucket")
	
	// This will compile only if DefaultCoordinator implements Coordinator
	var _ Coordinator = coord
}