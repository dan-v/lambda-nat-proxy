package metrics

import (
	"testing"
	"time"
)

func TestGetLastRTT(t *testing.T) {
	testRTT := 123 * time.Millisecond
	RecordRTT(testRTT)
	
	if got := GetLastRTT(); got != testRTT {
		t.Errorf("Expected last RTT %v, got %v", testRTT, got)
	}
}

func TestMetricsRecording(t *testing.T) {
	// Test basic metric recording without HTTP server overhead
	RecordPingSent()
	RecordRTT(50 * time.Millisecond)
	SetSessionHealthy(true)
	
	// Verify RTT was recorded
	if got := GetLastRTT(); got != 50*time.Millisecond {
		t.Errorf("Expected RTT 50ms, got %v", got)
	}
}