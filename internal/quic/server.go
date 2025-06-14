package quic

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
	"github.com/quic-go/quic-go"
)

// ServerAPI defines the interface for QUIC server operations
type ServerAPI interface {
	StartAndAccept(ctx context.Context, udpConn *net.UDPConn, cfg *config.Config) (quic.Connection, error)
}

// Server manages QUIC server functionality
type Server struct{}

// New creates a new QUIC server
func New() *Server {
	return &Server{}
}

// StartAndAccept starts QUIC server and waits for Lambda connection
func (s *Server) StartAndAccept(ctx context.Context, udpConn *net.UDPConn, cfg *config.Config) (quic.Connection, error) {
	// Get the local address from our UDP socket (same port used for hole punching)
	localAddr := udpConn.LocalAddr().(*net.UDPAddr)

	// Close UDP socket to free the port for QUIC server
	udpConn.Close()

	// Small delay to ensure port is released
	time.Sleep(shared.DefaultSocketReleaseDelay)

	// Generate TLS config for server
	tlsConfig, err := shared.GenerateTLSConfig(shared.TLSConfigOptions{
		Organization: "Orchestrator QUIC Server",
		DNSNames:     []string{"orchestrator.local"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TLS config: %w", err)
	}

	log.Printf("🔗 Starting QUIC server on %s (same port as hole punch)", localAddr.String())

	// Get mode-based QUIC configuration
	streamWindow, connWindow, maxIncomingStreams, maxIncomingUniStreams := shared.GetQUICConfig(
		cfg.ModeConfig.BufferSize, 
		cfg.ModeConfig.MaxStreams,
	)
	
	log.Printf("🔧 QUIC config for %s mode: stream=%dMB, conn=%dMB, streams=%d", 
		cfg.Mode, streamWindow/(1024*1024), connWindow/(1024*1024), maxIncomingStreams)

	// Create mode-optimized QUIC configuration
	quicConfig := &quic.Config{
		// Flow control optimization based on mode
		InitialStreamReceiveWindow:     uint64(streamWindow / 2),
		MaxStreamReceiveWindow:         uint64(streamWindow),
		InitialConnectionReceiveWindow: uint64(connWindow / 2),
		MaxConnectionReceiveWindow:     uint64(connWindow),
		
		// Stream limits from mode configuration
		MaxIncomingStreams:    int64(maxIncomingStreams),
		MaxIncomingUniStreams: maxIncomingUniStreams,
		
		// Timeout optimization based on mode
		MaxIdleTimeout:       cfg.ModeConfig.IdleTimeout,
		HandshakeIdleTimeout: shared.QUICHandshakeTimeout,
		KeepAlivePeriod:      cfg.ModeConfig.KeepAlive,
		
		// Enable connection migration for better reliability
		DisablePathMTUDiscovery: false,
		EnableDatagrams:         false, // Focus on stream performance
	}

	// Create QUIC listener on the same port with optimized config
	listener, err := quic.ListenAddr(localAddr.String(), tlsConfig, quicConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create QUIC listener: %w", err)
	}
	
	// Set up graceful shutdown of listener on context cancellation
	go func() {
		<-ctx.Done()
		shared.LogNetwork("Shutting down QUIC listener")
		listener.Close()
	}()

	shared.LogNetwork("QUIC server ready to accept Lambda connection")

	// Wait for Lambda to connect
	quicConn, err := listener.Accept(ctx)
	if err != nil {
		// Check if this is due to context cancellation (expected)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to accept Lambda connection: %w", err)
	}

	log.Printf("✅ Lambda connected from %s!", quicConn.RemoteAddr())

	return quicConn, nil
}