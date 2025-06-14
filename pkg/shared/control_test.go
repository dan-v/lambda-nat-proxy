package shared

import (
	"bytes"
	"testing"
)

func TestPingPongRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		nonce uint64
	}{
		{"zero nonce", 0},
		{"small nonce", 42},
		{"large nonce", 0xDEADBEEFCAFEBABE},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write ping
			var buf bytes.Buffer
			if err := WritePing(&buf, tt.nonce); err != nil {
				t.Fatalf("WritePing failed: %v", err)
			}
			
			// Read it back
			opcode, nonce, err := ReadControlMessage(&buf)
			if err != nil {
				t.Fatalf("ReadControlMessage failed: %v", err)
			}
			
			if opcode != OpPing {
				t.Errorf("Expected OpPing (0x%02x), got 0x%02x", OpPing, opcode)
			}
			if nonce != tt.nonce {
				t.Errorf("Expected nonce %d, got %d", tt.nonce, nonce)
			}
		})
	}
}

func TestPongMessage(t *testing.T) {
	var buf bytes.Buffer
	testNonce := uint64(12345)
	
	// Write pong
	if err := WritePong(&buf, testNonce); err != nil {
		t.Fatalf("WritePong failed: %v", err)
	}
	
	// Read it back
	opcode, nonce, err := ReadControlMessage(&buf)
	if err != nil {
		t.Fatalf("ReadControlMessage failed: %v", err)
	}
	
	if opcode != OpPong {
		t.Errorf("Expected OpPong (0x%02x), got 0x%02x", OpPong, opcode)
	}
	if nonce != testNonce {
		t.Errorf("Expected nonce %d, got %d", testNonce, nonce)
	}
}

func TestShutdownMessage(t *testing.T) {
	var buf bytes.Buffer
	
	// Write shutdown
	if err := WriteShutdown(&buf); err != nil {
		t.Fatalf("WriteShutdown failed: %v", err)
	}
	
	// Read it back
	opcode, _, err := ReadControlMessage(&buf)
	if err != nil {
		t.Fatalf("ReadControlMessage failed: %v", err)
	}
	
	if opcode != OpShutdown {
		t.Errorf("Expected OpShutdown (0x%02x), got 0x%02x", OpShutdown, opcode)
	}
}

func TestUnknownOpcode(t *testing.T) {
	var buf bytes.Buffer
	
	// Write invalid opcode
	buf.WriteByte(0xFF)
	
	// Should fail to read
	_, _, err := ReadControlMessage(&buf)
	if err == nil {
		t.Error("Expected error for unknown opcode, got nil")
	}
}