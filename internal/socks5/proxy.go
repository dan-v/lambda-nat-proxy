package socks5

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/dan-v/lambda-nat-punch-proxy/internal/dashboard"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/manager"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/metrics"
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
	"github.com/quic-go/quic-go"
)

// Proxy handles SOCKS5 protocol and data forwarding
type Proxy interface {
	Start(port int, quicConn quic.Connection) error
	StartWithConfig(port int, quicConn quic.Connection, bufferSize int) error
	StartWithConnManager(port int, cm *manager.ConnManager) error
	StartWithContext(ctx context.Context, port int, quicConn quic.Connection) error
	StartWithConfigAndContext(ctx context.Context, port int, quicConn quic.Connection, bufferSize int) error
	StartWithConnManagerAndContext(ctx context.Context, port int, cm *manager.ConnManager) error
}

// DefaultProxy implements Proxy
type DefaultProxy struct{}

// New creates a new SOCKS5 proxy
func New() Proxy {
	return &DefaultProxy{}
}

// Start starts the SOCKS5 proxy server
func (p *DefaultProxy) Start(port int, quicConn quic.Connection) error {
	return p.StartWithContext(context.Background(), port, quicConn)
}

// generateConnectionID creates a unique ID for tracking connections
func generateConnectionID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// handleSOCKS5Connection handles a single SOCKS5 connection
func (p *DefaultProxy) handleSOCKS5Connection(clientConn net.Conn, quicConn quic.Connection) {
	// Generate unique connection ID for tracking
	connID := generateConnectionID()
	
	defer func() {
		clientConn.Close()
		metrics.DecrementActiveSOCKS5Connections()
		// Clean up connection tracking
		dashboard.GlobalConnectionTracker.RemoveConnection(connID)
	}()

	// Record new connection
	metrics.RecordSOCKS5Connection()
	metrics.IncrementActiveSOCKS5Connections()
	connStart := time.Now()

	log.Printf("📞 New SOCKS5 connection from %s", clientConn.RemoteAddr())

	// Handle SOCKS5 handshake (use optimized buffer size)
	buf := make([]byte, shared.OptimizedBufferSize)
	_, err := clientConn.Read(buf)
	if err != nil {
		log.Printf("Failed to read SOCKS5 handshake: %v", err)
		metrics.RecordSOCKS5FailedConnection()
		return
	}

	// Respond to SOCKS5 handshake (no auth)
	if buf[0] == shared.SOCKS5Version {
		clientConn.Write(shared.SOCKS5AuthResponse)
	} else {
		log.Printf("Not a SOCKS5 connection")
		return
	}

	// Read SOCKS5 request
	_, err = clientConn.Read(buf)
	if err != nil {
		log.Printf("Failed to read SOCKS5 request: %v", err)
		return
	}

	if buf[0] != shared.SOCKS5Version || buf[1] != shared.SOCKS5Connect {
		log.Printf("Only SOCKS5 CONNECT supported")
		return
	}

	// Parse target address
	var targetAddr string
	var targetPort uint16

	switch buf[3] { // Address type
	case shared.SOCKS5IPv4:
		targetAddr = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		targetPort = binary.BigEndian.Uint16(buf[8:10])
	case shared.SOCKS5DomainName:
		domainLen := buf[4]
		targetAddr = string(buf[5 : 5+domainLen])
		targetPort = binary.BigEndian.Uint16(buf[5+domainLen : 7+domainLen])
	default:
		log.Printf("Unsupported address type: %d", buf[3])
		return
	}

	target := fmt.Sprintf("%s:%d", targetAddr, targetPort)
	log.Printf("🎯 SOCKS5 request to %s", target)
	
	// Add connection to tracker now that we know the destination
	dashboard.GlobalConnectionTracker.AddConnection(connID, clientConn.RemoteAddr().String(), target)

	// Open QUIC stream for this connection
	stream, err := quicConn.OpenStreamSync(context.Background())
	if err != nil {
		log.Printf("Failed to open QUIC stream: %v", err)
		metrics.RecordSOCKS5FailedConnection()
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}
	defer stream.Close()

	// Send target address to lambda over QUIC
	targetBytes := []byte(target)
	targetLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(targetLenBytes, uint32(len(targetBytes)))

	if _, err := stream.Write(targetLenBytes); err != nil {
		log.Printf("Failed to write target length: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if _, err := stream.Write(targetBytes); err != nil {
		log.Printf("Failed to write target address: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Read response from lambda
	responseBuf := make([]byte, 1)
	if _, err := stream.Read(responseBuf); err != nil {
		log.Printf("Failed to read lambda response: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if responseBuf[0] != 0x00 { // Success
		log.Printf("Lambda failed to connect to target")
		metrics.RecordSOCKS5FailedConnection()
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Send SOCKS5 success response
	clientConn.Write(shared.SOCKS5SuccessResponse)

	log.Printf("✅ SOCKS5 tunnel established to %s", target)

	// Create a combined metrics recording function
	recordBytes := func(bytes int64) {
		metrics.RecordSOCKS5BytesTransferred(bytes)
		dashboard.GlobalConnectionTracker.UpdateConnection(connID, bytes, 0, 0) // Update dashboard tracker
	}
	
	// Start optimized bidirectional data forwarding with metrics
	shared.OptimizedCopyWithMetrics(clientConn, &streamConn{stream}, recordBytes)
	
	// Record connection latency
	connectionTime := time.Since(connStart)
	metrics.RecordSOCKS5Latency(connectionTime)
	
	log.Printf("🔚 SOCKS5 connection to %s closed", target)
}

// StartWithConfig starts the SOCKS5 proxy server with configuration
func (p *DefaultProxy) StartWithConfig(port int, quicConn quic.Connection, bufferSize int) error {
	return p.StartWithConfigAndContext(context.Background(), port, quicConn, bufferSize)
}

// StartWithConnManager starts the SOCKS5 proxy server with a connection manager
func (p *DefaultProxy) StartWithConnManager(port int, cm *manager.ConnManager) error {
	return p.StartWithConnManagerAndContext(context.Background(), port, cm)
}

// handleSOCKS5ConnectionWithSession handles a single SOCKS5 connection using a specific session
func (p *DefaultProxy) handleSOCKS5ConnectionWithSession(clientConn net.Conn, session *manager.Session) {
	defer clientConn.Close()

	log.Printf("📞 New SOCKS5 connection from %s using session %s", clientConn.RemoteAddr(), session.ID)

	// Handle SOCKS5 handshake (use optimized buffer size)
	buf := make([]byte, shared.OptimizedBufferSize)
	_, err := clientConn.Read(buf)
	if err != nil {
		log.Printf("Failed to read SOCKS5 handshake: %v", err)
		return
	}

	// Respond to SOCKS5 handshake (no auth)
	if buf[0] == shared.SOCKS5Version {
		clientConn.Write(shared.SOCKS5AuthResponse)
	} else {
		log.Printf("Not a SOCKS5 connection")
		return
	}

	// Read SOCKS5 request
	_, err = clientConn.Read(buf)
	if err != nil {
		log.Printf("Failed to read SOCKS5 request: %v", err)
		return
	}

	if buf[0] != shared.SOCKS5Version || buf[1] != shared.SOCKS5Connect {
		log.Printf("Only SOCKS5 CONNECT supported")
		return
	}

	// Parse target address
	var targetAddr string
	var targetPort uint16

	switch buf[3] { // Address type
	case shared.SOCKS5IPv4:
		targetAddr = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		targetPort = binary.BigEndian.Uint16(buf[8:10])
	case shared.SOCKS5DomainName:
		domainLen := buf[4]
		targetAddr = string(buf[5 : 5+domainLen])
		targetPort = binary.BigEndian.Uint16(buf[5+domainLen : 7+domainLen])
	default:
		log.Printf("Unsupported address type: %d", buf[3])
		return
	}

	target := fmt.Sprintf("%s:%d", targetAddr, targetPort)
	log.Printf("🎯 SOCKS5 request to %s via session %s", target, session.ID)

	// Open QUIC stream for this connection on the primary session
	stream, err := session.QuicConn.OpenStreamSync(context.Background())
	if err != nil {
		log.Printf("Failed to open QUIC stream on session %s: %v", session.ID, err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}
	defer stream.Close()

	// Send target address to lambda over QUIC
	targetBytes := []byte(target)
	targetLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(targetLenBytes, uint32(len(targetBytes)))

	if _, err := stream.Write(targetLenBytes); err != nil {
		log.Printf("Failed to write target length: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if _, err := stream.Write(targetBytes); err != nil {
		log.Printf("Failed to write target address: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Read response from lambda
	responseBuf := make([]byte, 1)
	if _, err := stream.Read(responseBuf); err != nil {
		log.Printf("Failed to read lambda response: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if responseBuf[0] != 0x00 { // Success
		log.Printf("Lambda failed to connect to target")
		metrics.RecordSOCKS5FailedConnection()
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Send SOCKS5 success response
	clientConn.Write(shared.SOCKS5SuccessResponse)

	log.Printf("✅ SOCKS5 tunnel established to %s via session %s", target, session.ID)

	// Start optimized bidirectional data forwarding
	shared.OptimizedCopy(clientConn, &streamConn{stream})
	log.Printf("🔚 SOCKS5 connection to %s closed (session %s)", target, session.ID)
}

// handleSOCKS5ConnectionWithConfig handles a single SOCKS5 connection with custom buffer size
func (p *DefaultProxy) handleSOCKS5ConnectionWithConfig(clientConn net.Conn, quicConn quic.Connection, bufferSize int) {
	defer clientConn.Close()

	log.Printf("📞 New SOCKS5 connection from %s (mode-optimized)", clientConn.RemoteAddr())

	// Handle SOCKS5 handshake (use mode-specific buffer size)
	buf := make([]byte, bufferSize)
	_, err := clientConn.Read(buf)
	if err != nil {
		log.Printf("Failed to read SOCKS5 handshake: %v", err)
		return
	}

	// Respond to SOCKS5 handshake (no auth)
	if buf[0] == shared.SOCKS5Version {
		clientConn.Write(shared.SOCKS5AuthResponse)
	} else {
		log.Printf("Not a SOCKS5 connection")
		return
	}

	// Read SOCKS5 request
	_, err = clientConn.Read(buf)
	if err != nil {
		log.Printf("Failed to read SOCKS5 request: %v", err)
		return
	}

	if buf[0] != shared.SOCKS5Version || buf[1] != shared.SOCKS5Connect {
		log.Printf("Only SOCKS5 CONNECT supported")
		return
	}

	// Parse target address
	var targetAddr string
	var targetPort uint16

	switch buf[3] { // Address type
	case shared.SOCKS5IPv4:
		targetAddr = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		targetPort = binary.BigEndian.Uint16(buf[8:10])
	case shared.SOCKS5DomainName:
		domainLen := buf[4]
		targetAddr = string(buf[5 : 5+domainLen])
		targetPort = binary.BigEndian.Uint16(buf[5+domainLen : 7+domainLen])
	default:
		log.Printf("Unsupported address type: %d", buf[3])
		return
	}

	target := fmt.Sprintf("%s:%d", targetAddr, targetPort)
	log.Printf("🎯 SOCKS5 request to %s (mode-optimized)", target)

	// Open QUIC stream for this connection
	stream, err := quicConn.OpenStreamSync(context.Background())
	if err != nil {
		log.Printf("Failed to open QUIC stream: %v", err)
		metrics.RecordSOCKS5FailedConnection()
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}
	defer stream.Close()

	// Send target address to lambda over QUIC
	targetBytes := []byte(target)
	targetLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(targetLenBytes, uint32(len(targetBytes)))

	if _, err := stream.Write(targetLenBytes); err != nil {
		log.Printf("Failed to write target length: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if _, err := stream.Write(targetBytes); err != nil {
		log.Printf("Failed to write target address: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Read response from lambda
	responseBuf := make([]byte, 1)
	if _, err := stream.Read(responseBuf); err != nil {
		log.Printf("Failed to read lambda response: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if responseBuf[0] != 0x00 { // Success
		log.Printf("Lambda failed to connect to target")
		metrics.RecordSOCKS5FailedConnection()
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Send SOCKS5 success response
	clientConn.Write(shared.SOCKS5SuccessResponse)

	log.Printf("✅ SOCKS5 tunnel established to %s (mode-optimized)", target)

	// Start optimized bidirectional data forwarding with custom buffer size
	shared.OptimizedCopyWithBufferSize(clientConn, &streamConn{stream}, bufferSize)
	log.Printf("🔚 SOCKS5 connection to %s closed (mode-optimized)", target)
}

// streamConn adapts a QUIC stream to net.Conn interface for optimized copying
type streamConn struct {
	quic.Stream
}

func (sc *streamConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (sc *streamConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (sc *streamConn) SetDeadline(t time.Time) error {
	sc.SetReadDeadline(t)
	sc.SetWriteDeadline(t)
	return nil
}

// handleSOCKS5ConnectionWithContext handles a single SOCKS5 connection with context support
func (p *DefaultProxy) handleSOCKS5ConnectionWithContext(ctx context.Context, clientConn net.Conn, quicConn quic.Connection) {
	defer clientConn.Close()

	shared.LogConnectionf("New SOCKS5 connection from %s", clientConn.RemoteAddr())

	// Create a context for this connection
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Monitor for context cancellation
	go func() {
		<-connCtx.Done()
		clientConn.Close()
	}()

	// Handle SOCKS5 handshake (use optimized buffer size)
	buf := make([]byte, shared.OptimizedBufferSize)
	_, err := clientConn.Read(buf)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read SOCKS5 handshake: %v", err)
		return
	}

	// Respond to SOCKS5 handshake (no auth)
	if buf[0] == shared.SOCKS5Version {
		clientConn.Write(shared.SOCKS5AuthResponse)
	} else {
		shared.LogNetwork("Not a SOCKS5 connection")
		return
	}

	// Read SOCKS5 request
	_, err = clientConn.Read(buf)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read SOCKS5 request: %v", err)
		return
	}

	if buf[0] != shared.SOCKS5Version || buf[1] != shared.SOCKS5Connect {
		shared.LogNetwork("Only SOCKS5 CONNECT supported")
		return
	}

	// Parse target address
	var targetAddr string
	var targetPort uint16

	switch buf[3] { // Address type
	case shared.SOCKS5IPv4:
		targetAddr = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		targetPort = binary.BigEndian.Uint16(buf[8:10])
	case shared.SOCKS5DomainName:
		domainLen := buf[4]
		targetAddr = string(buf[5 : 5+domainLen])
		targetPort = binary.BigEndian.Uint16(buf[5+domainLen : 7+domainLen])
	default:
		shared.LogErrorf("Unsupported address type: %d", buf[3])
		return
	}

	target := fmt.Sprintf("%s:%d", targetAddr, targetPort)
	shared.LogTargetf("SOCKS5 request to %s", target)

	// Open QUIC stream for this connection with context
	stream, err := quicConn.OpenStreamSync(connCtx)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to open QUIC stream: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}
	defer stream.Close()

	// Send target address to lambda over QUIC
	targetBytes := []byte(target)
	targetLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(targetLenBytes, uint32(len(targetBytes)))

	if _, err := stream.Write(targetLenBytes); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to write target length: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if _, err := stream.Write(targetBytes); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to write target address: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Read response from lambda
	responseBuf := make([]byte, 1)
	if _, err := stream.Read(responseBuf); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read lambda response: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if responseBuf[0] != 0x00 { // Success
		shared.LogNetwork("Lambda failed to connect to target")
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Send SOCKS5 success response
	clientConn.Write(shared.SOCKS5SuccessResponse)

	shared.LogSuccessf("SOCKS5 tunnel established to %s", target)

	// Start optimized bidirectional data forwarding with context awareness
	shared.OptimizedCopyWithContext(connCtx, clientConn, &streamConn{stream})
	shared.LogClosef("SOCKS5 connection to %s closed", target)
}

// handleSOCKS5ConnectionWithConfigAndContext handles a single SOCKS5 connection with custom buffer size and context
func (p *DefaultProxy) handleSOCKS5ConnectionWithConfigAndContext(ctx context.Context, clientConn net.Conn, quicConn quic.Connection, bufferSize int) {
	defer clientConn.Close()

	shared.LogConnectionf("New SOCKS5 connection from %s (optimized)", clientConn.RemoteAddr())

	// Create a context for this connection
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Monitor for context cancellation
	go func() {
		<-connCtx.Done()
		clientConn.Close()
	}()

	// Handle SOCKS5 handshake (use mode-specific buffer size)
	buf := make([]byte, bufferSize)
	_, err := clientConn.Read(buf)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read SOCKS5 handshake: %v", err)
		return
	}

	// Respond to SOCKS5 handshake (no auth)
	if buf[0] == shared.SOCKS5Version {
		clientConn.Write(shared.SOCKS5AuthResponse)
	} else {
		shared.LogNetwork("Not a SOCKS5 connection")
		return
	}

	// Read SOCKS5 request
	_, err = clientConn.Read(buf)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read SOCKS5 request: %v", err)
		return
	}

	if buf[0] != shared.SOCKS5Version || buf[1] != shared.SOCKS5Connect {
		shared.LogNetwork("Only SOCKS5 CONNECT supported")
		return
	}

	// Parse target address
	var targetAddr string
	var targetPort uint16

	switch buf[3] { // Address type
	case shared.SOCKS5IPv4:
		targetAddr = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		targetPort = binary.BigEndian.Uint16(buf[8:10])
	case shared.SOCKS5DomainName:
		domainLen := buf[4]
		targetAddr = string(buf[5 : 5+domainLen])
		targetPort = binary.BigEndian.Uint16(buf[5+domainLen : 7+domainLen])
	default:
		shared.LogErrorf("Unsupported address type: %d", buf[3])
		return
	}

	target := fmt.Sprintf("%s:%d", targetAddr, targetPort)
	shared.LogTargetf("SOCKS5 request to %s (optimized)", target)

	// Open QUIC stream for this connection with context
	stream, err := quicConn.OpenStreamSync(connCtx)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to open QUIC stream: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}
	defer stream.Close()

	// Send target address to lambda over QUIC
	targetBytes := []byte(target)
	targetLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(targetLenBytes, uint32(len(targetBytes)))

	if _, err := stream.Write(targetLenBytes); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to write target length: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if _, err := stream.Write(targetBytes); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to write target address: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Read response from lambda
	responseBuf := make([]byte, 1)
	if _, err := stream.Read(responseBuf); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read lambda response: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if responseBuf[0] != 0x00 { // Success
		shared.LogNetwork("Lambda failed to connect to target")
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Send SOCKS5 success response
	clientConn.Write(shared.SOCKS5SuccessResponse)

	shared.LogSuccessf("SOCKS5 tunnel established to %s (optimized)", target)

	// Start optimized bidirectional data forwarding with context awareness and custom buffer size
	shared.OptimizedCopyWithContextAndBufferSize(connCtx, clientConn, &streamConn{stream}, bufferSize)
	shared.LogClosef("SOCKS5 connection to %s closed (optimized)", target)
}

// handleSOCKS5ConnectionWithSessionAndContext handles a single SOCKS5 connection using a specific session with context
func (p *DefaultProxy) handleSOCKS5ConnectionWithSessionAndContext(ctx context.Context, clientConn net.Conn, session *manager.Session) {
	// Generate unique connection ID for tracking
	connID := generateConnectionID()
	
	defer func() {
		clientConn.Close()
		metrics.DecrementActiveSOCKS5Connections()
		// Clean up connection tracking
		dashboard.GlobalConnectionTracker.RemoveConnection(connID)
	}()

	// Record new connection
	metrics.RecordSOCKS5Connection()
	metrics.IncrementActiveSOCKS5Connections()
	connStart := time.Now()

	shared.LogConnectionf("New SOCKS5 connection from %s using session %s", clientConn.RemoteAddr(), session.ID)

	// Create a context for this connection
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Monitor for context cancellation
	go func() {
		<-connCtx.Done()
		clientConn.Close()
	}()

	// Handle SOCKS5 handshake (use optimized buffer size)
	buf := make([]byte, shared.OptimizedBufferSize)
	_, err := clientConn.Read(buf)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read SOCKS5 handshake: %v", err)
		return
	}

	// Respond to SOCKS5 handshake (no auth)
	if buf[0] == shared.SOCKS5Version {
		clientConn.Write(shared.SOCKS5AuthResponse)
	} else {
		shared.LogNetwork("Not a SOCKS5 connection")
		return
	}

	// Read SOCKS5 request
	_, err = clientConn.Read(buf)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read SOCKS5 request: %v", err)
		return
	}

	if buf[0] != shared.SOCKS5Version || buf[1] != shared.SOCKS5Connect {
		shared.LogNetwork("Only SOCKS5 CONNECT supported")
		return
	}

	// Parse target address
	var targetAddr string
	var targetPort uint16

	switch buf[3] { // Address type
	case shared.SOCKS5IPv4:
		targetAddr = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		targetPort = binary.BigEndian.Uint16(buf[8:10])
	case shared.SOCKS5DomainName:
		domainLen := buf[4]
		targetAddr = string(buf[5 : 5+domainLen])
		targetPort = binary.BigEndian.Uint16(buf[5+domainLen : 7+domainLen])
	default:
		shared.LogErrorf("Unsupported address type: %d", buf[3])
		return
	}

	target := fmt.Sprintf("%s:%d", targetAddr, targetPort)
	shared.LogTargetf("SOCKS5 request to %s via session %s", target, session.ID)
	
	// Add connection to tracker now that we know the destination
	dashboard.GlobalConnectionTracker.AddConnection(connID, clientConn.RemoteAddr().String(), target)

	// Open QUIC stream for this connection on the primary session with context
	stream, err := session.QuicConn.OpenStreamSync(connCtx)
	if err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to open QUIC stream on session %s: %v", session.ID, err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}
	defer stream.Close()

	// Send target address to lambda over QUIC
	targetBytes := []byte(target)
	targetLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(targetLenBytes, uint32(len(targetBytes)))

	if _, err := stream.Write(targetLenBytes); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to write target length: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if _, err := stream.Write(targetBytes); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to write target address: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Read response from lambda
	responseBuf := make([]byte, 1)
	if _, err := stream.Read(responseBuf); err != nil {
		if connCtx.Err() != nil {
			return // Context cancelled
		}
		shared.LogErrorf("Failed to read lambda response: %v", err)
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	if responseBuf[0] != 0x00 { // Success
		shared.LogNetwork("Lambda failed to connect to target")
		clientConn.Write(shared.SOCKS5FailureResponse)
		return
	}

	// Send SOCKS5 success response
	clientConn.Write(shared.SOCKS5SuccessResponse)

	shared.LogSuccessf("SOCKS5 tunnel established to %s via session %s", target, session.ID)

	// Create a combined metrics recording function
	recordBytes := func(bytes int64) {
		metrics.RecordSOCKS5BytesTransferred(bytes)
		dashboard.GlobalConnectionTracker.UpdateConnection(connID, bytes, 0, 0) // Update dashboard tracker
	}
	
	// Start optimized bidirectional data forwarding with context awareness and metrics
	shared.OptimizedCopyWithContextAndMetrics(connCtx, clientConn, &streamConn{stream}, recordBytes)
	
	// Record connection latency
	connectionTime := time.Since(connStart)
	metrics.RecordSOCKS5Latency(connectionTime)
	
	shared.LogClosef("SOCKS5 connection to %s closed (session %s)", target, session.ID)
}

// StartWithContext starts the SOCKS5 proxy server with context support for graceful shutdown
func (p *DefaultProxy) StartWithContext(ctx context.Context, port int, quicConn quic.Connection) error {
	socksAddr := fmt.Sprintf(":%d", port)
	socksListener, err := net.Listen("tcp", socksAddr)
	if err != nil {
		return fmt.Errorf("failed to start SOCKS5 server: %w", err)
	}
	defer socksListener.Close()

	// Set up graceful shutdown
	go func() {
		<-ctx.Done()
		shared.LogNetwork("Shutting down SOCKS5 proxy server")
		socksListener.Close()
	}()

	shared.LogSuccessf("SOCKS5 proxy server started on %s", socksAddr)
	shared.LogInfof("Configure your browser to use SOCKS5 proxy: localhost%s", socksAddr)

	// Accept SOCKS5 connections
	for {
		conn, err := socksListener.Accept()
		if err != nil {
			// Check if this is due to context cancellation (expected)
			if ctx.Err() != nil {
				shared.LogNetwork("SOCKS5 proxy server shutdown completed")
				return nil
			}
			// Check if listener was closed
			if ne, ok := err.(net.Error); ok && !ne.Temporary() {
				shared.LogNetwork("SOCKS5 listener closed")
				break
			}
			shared.LogErrorf("Failed to accept connection: %v", err)
			continue
		}

		go p.handleSOCKS5ConnectionWithContext(ctx, conn, quicConn)
	}

	return nil
}

// StartWithConfigAndContext starts the SOCKS5 proxy server with configuration and context support
func (p *DefaultProxy) StartWithConfigAndContext(ctx context.Context, port int, quicConn quic.Connection, bufferSize int) error {
	socksAddr := fmt.Sprintf(":%d", port)
	socksListener, err := net.Listen("tcp", socksAddr)
	if err != nil {
		return fmt.Errorf("failed to start SOCKS5 server: %w", err)
	}
	defer socksListener.Close()

	// Set up graceful shutdown
	go func() {
		<-ctx.Done()
		shared.LogNetwork("Shutting down SOCKS5 proxy server")
		socksListener.Close()
	}()

	shared.LogSuccessf("SOCKS5 proxy server started on %s (optimized)", socksAddr)
	shared.LogInfof("Configure your browser to use SOCKS5 proxy: localhost%s", socksAddr)

	// Accept SOCKS5 connections
	for {
		conn, err := socksListener.Accept()
		if err != nil {
			// Check if this is due to context cancellation (expected)
			if ctx.Err() != nil {
				shared.LogNetwork("SOCKS5 proxy server shutdown completed")
				return nil
			}
			// Check if listener was closed
			if ne, ok := err.(net.Error); ok && !ne.Temporary() {
				shared.LogNetwork("SOCKS5 listener closed")
				break
			}
			shared.LogErrorf("Failed to accept connection: %v", err)
			continue
		}

		go p.handleSOCKS5ConnectionWithConfigAndContext(ctx, conn, quicConn, bufferSize)
	}

	return nil
}

// StartWithConnManagerAndContext starts the SOCKS5 proxy server with a connection manager and context support
func (p *DefaultProxy) StartWithConnManagerAndContext(ctx context.Context, port int, cm *manager.ConnManager) error {
	socksAddr := fmt.Sprintf(":%d", port)
	socksListener, err := net.Listen("tcp", socksAddr)
	if err != nil {
		return fmt.Errorf("failed to start SOCKS5 server: %w", err)
	}
	defer socksListener.Close()

	// Set up graceful shutdown
	go func() {
		<-ctx.Done()
		shared.LogNetwork("Shutting down SOCKS5 proxy server")
		socksListener.Close()
	}()

	shared.LogSuccessf("SOCKS5 proxy server started on %s", socksAddr)
	shared.LogInfof("Configure your browser to use SOCKS5 proxy: localhost%s", socksAddr)

	// Accept SOCKS5 connections
	for {
		conn, err := socksListener.Accept()
		if err != nil {
			// Check if this is due to context cancellation (expected)
			if ctx.Err() != nil {
				shared.LogNetwork("SOCKS5 proxy server shutdown completed")
				return nil
			}
			// Check if listener was closed
			if ne, ok := err.(net.Error); ok && !ne.Temporary() {
				shared.LogNetwork("SOCKS5 listener closed")
				break
			}
			shared.LogErrorf("Failed to accept connection: %v", err)
			continue
		}
		// Get current primary session from ConnManager
		session := cm.Primary()
		if session == nil || session.IsDraining() || !session.IsHealthy() {
			shared.LogNetworkf("No suitable session available for connection from %s", conn.RemoteAddr())
			conn.Close()
			continue
		}

		go p.handleSOCKS5ConnectionWithSessionAndContext(ctx, conn, session)
	}

	return nil
}