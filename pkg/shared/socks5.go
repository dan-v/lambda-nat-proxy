package shared

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// SOCKS5Response represents the response codes
type SOCKS5Response byte

const (
	SOCKS5ResponseSuccess SOCKS5Response = 0x00
	SOCKS5ResponseError   SOCKS5Response = 0x01
)

// SOCKS5TargetRequest represents a parsed target request
type SOCKS5TargetRequest struct {
	Address string
	Port    uint16
}

// ReadSOCKS5TargetAddress reads the target address from a SOCKS5-style stream
// Format: [4 bytes length][target address string]
func ReadSOCKS5TargetAddress(stream io.Reader) (string, error) {
	// Read target address length (4 bytes, big endian)
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(stream, lengthBuf); err != nil {
		return "", fmt.Errorf("failed to read target length: %w", err)
	}

	targetLen := binary.BigEndian.Uint32(lengthBuf)
	if targetLen > MaxTargetAddressLength {
		return "", fmt.Errorf("target address too long: %d bytes (max %d)", targetLen, MaxTargetAddressLength)
	}

	// Read target address
	targetBuf := make([]byte, targetLen)
	if _, err := io.ReadFull(stream, targetBuf); err != nil {
		return "", fmt.Errorf("failed to read target address: %w", err)
	}

	target := string(targetBuf)
	
	// Basic validation
	if target == "" {
		return "", fmt.Errorf("empty target address")
	}

	return target, nil
}

// WriteSOCKS5Response writes a SOCKS5 response code to the stream
func WriteSOCKS5Response(stream io.Writer, response SOCKS5Response) error {
	if _, err := stream.Write([]byte{byte(response)}); err != nil {
		return fmt.Errorf("failed to write SOCKS5 response: %w", err)
	}
	return nil
}

// WriteSOCKS5TargetAddress writes a target address in SOCKS5 format
// Format: [4 bytes length][target address string]
func WriteSOCKS5TargetAddress(stream io.Writer, target string) error {
	targetBytes := []byte(target)
	targetLen := uint32(len(targetBytes))

	// Write length
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, targetLen)
	
	if _, err := stream.Write(lengthBuf); err != nil {
		return fmt.Errorf("failed to write target length: %w", err)
	}

	// Write target address
	if _, err := stream.Write(targetBytes); err != nil {
		return fmt.Errorf("failed to write target address: %w", err)
	}

	return nil
}

// ConnectToTarget establishes a TCP connection to the target address with timeout
func ConnectToTarget(target string, timeout time.Duration) (net.Conn, error) {
	if timeout == 0 {
		timeout = DefaultConnectionTimeout
	}

	conn, err := net.DialTimeout("tcp", target, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to target %s: %w", target, err)
	}

	return conn, nil
}

// ForwardData handles bidirectional data forwarding between two connections
func ForwardData(conn1, conn2 io.ReadWriteCloser) {
	// Start forwarding in both directions
	done := make(chan struct{}, 2)

	// conn1 -> conn2
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(conn2, conn1)
		conn2.Close()
	}()

	// conn2 -> conn1
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(conn1, conn2)
		conn1.Close()
	}()

	// Wait for one direction to complete
	<-done
}

// ValidateTargetAddress performs basic validation on a target address
func ValidateTargetAddress(target string) error {
	if target == "" {
		return fmt.Errorf("empty target address")
	}

	if len(target) > MaxTargetAddressLength {
		return fmt.Errorf("target address too long: %d chars", len(target))
	}

	// Try to parse as host:port
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("invalid target format (expected host:port): %w", err)
	}

	if host == "" {
		return fmt.Errorf("empty host in target address")
	}

	if port == "" {
		return fmt.Errorf("empty port in target address")
	}

	return nil
}