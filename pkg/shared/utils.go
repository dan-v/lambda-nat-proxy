package shared

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateSessionID creates a unique session identifier
func GenerateSessionID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// This should never happen with crypto/rand, but handle gracefully
		// Fall back to a timestamp-based ID if crypto/rand fails
		LogError("Failed to generate cryptographic session ID, falling back to timestamp", err)
		return GenerateTimestampID()
	}
	return hex.EncodeToString(bytes)
}

// GenerateTimestampID creates a session ID based on current time as fallback
func GenerateTimestampID() string {
	// Use nanosecond timestamp as fallback ID
	timestamp := time.Now().UnixNano()
	return hex.EncodeToString([]byte(fmt.Sprintf("%d", timestamp)))
}