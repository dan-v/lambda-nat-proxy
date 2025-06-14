package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/quic-go/quic-go"
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)


var s3Client *s3.S3

func init() {
	// Initialize structured logging for Lambda
	shared.InitLogger(&shared.LogConfig{
		Level:       shared.LevelInfo,
		Format:      "json", // JSON format for Lambda logs
		AddSource:   true,
		ServiceName: "lambda-nat-proxy",
	})
	// S3 client will be initialized lazily in getS3Client()
}

// getS3Client returns the S3 client, initializing it if necessary
func getS3Client() (*s3.S3, error) {
	if s3Client == nil {
		var err error
		s3Client, err = shared.CreateS3Client(shared.DefaultAWSRegion)
		if err != nil {
			shared.LogError("Failed to create S3 client", err)
			return nil, fmt.Errorf("failed to initialize S3 client: %w", err)
		}
	}
	return s3Client, nil
}

func LambdaHandler(ctx context.Context, s3Event events.S3Event) error {
	shared.LogTargetf("Lambda triggered with %d S3 events", len(s3Event.Records))
	
	// Create a channel to signal when we're done
	done := make(chan error, 1)
	
	for _, record := range s3Event.Records {
		shared.LogStoragef("Processing S3 event: %s", record.S3.Object.Key)
		handleHolePunchRequest(ctx, record, done)
	}
	
	// Wait for completion or context cancellation
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func handleHolePunchRequest(ctx context.Context, record events.S3EventRecord, done chan<- error) {
	// 1. Get S3 client
	client, err := getS3Client()
	if err != nil {
		shared.LogError("Failed to get S3 client", err)
		done <- fmt.Errorf("S3 client initialization failed: %w", err)
		return
	}
	
	// 2. Read coordination data from S3
	coord, err := shared.GetCoordinationData(client, record.S3.Bucket.Name, record.S3.Object.Key)
	if err != nil {
		shared.LogError("Failed to read coordination data from S3", err)
		done <- fmt.Errorf("failed to read coordination data: %w", err)
		return
	}
	
	shared.LogSuccessf("Target orchestrator: %s:%d", coord.LaptopPublicIP, coord.LaptopPublicPort)
	
	// 3. Discover our public IP
	lambdaPublicIP, err := shared.DiscoverPublicIPHTTP()
	if err != nil {
		shared.LogError("Failed to discover public IP", err)
		done <- fmt.Errorf("failed to discover public IP: %w", err)
		return
	}
	shared.LogSuccessf("Lambda public IP: %s", lambdaPublicIP)
	
	// 4. Create UDP socket (will be used for hole punching)
	udpConn, lambdaPort, err := shared.CreateUDPSocket()
	if err != nil {
		shared.LogError("Failed to create UDP socket", err)
		done <- fmt.Errorf("failed to create UDP socket: %w", err)
		return
	}
	shared.LogSuccessf("UDP socket created on port %d", lambdaPort)
	
	// 5. Write Lambda's response to S3
	response := shared.LambdaResponse{
		SessionID:        coord.SessionID,
		LambdaPublicIP:   lambdaPublicIP,
		LambdaPublicPort: lambdaPort,
		Status:           "ready",
		Timestamp:        time.Now().Unix(),
	}
	
	if err := shared.PutLambdaResponse(client, record.S3.Bucket.Name, coord.SessionID, response); err != nil {
		shared.LogError("Failed to write response to S3", err)
		done <- fmt.Errorf("failed to write response to S3: %w", err)
		return
	}
	shared.LogSuccess("Lambda response written to S3")
	
	// 6. Perform NAT hole punching
	orchestratorAddr := &net.UDPAddr{
		IP:   net.ParseIP(coord.LaptopPublicIP),
		Port: coord.LaptopPublicPort,
	}
	
	if !performNATPunch(udpConn, coord.SessionID, orchestratorAddr) {
		shared.LogError("NAT hole punching failed", nil)
		udpConn.Close()
		done <- fmt.Errorf("NAT hole punching failed")
		return
	}
	shared.LogSuccess("NAT hole punched successfully!")
	
	// 7. Connect to orchestrator's QUIC server
	shared.LogNetwork("Connecting to orchestrator QUIC server...")
	startQUICClient(ctx, coord.LaptopPublicIP, coord.LaptopPublicPort, lambdaPort, udpConn, done)
}

func startQUICClient(ctx context.Context, orchestratorIP string, orchestratorPort int, localPort int, udpConn *net.UDPConn, done chan<- error) {
	// Connect to orchestrator's QUIC server using the same local port
	remoteAddr := fmt.Sprintf("%s:%d", orchestratorIP, orchestratorPort)
	
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h3"},
	}
	
	// Get local address for port reuse
	localAddr := udpConn.LocalAddr().(*net.UDPAddr)
	
	// Close UDP socket gracefully
	shared.CloseUDPSocketGracefully(udpConn)
	
	// Parse remote address
	remoteUDPAddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		shared.LogError("Failed to resolve orchestrator address", err)
		done <- err
		return
	}
	
	shared.LogConnectionf("Connecting to orchestrator QUIC server at %s from local port %d", remoteAddr, localAddr.Port)
	
	// Create UDP connection on same local port
	udpDialConn, err := shared.ReuseUDPPort(localAddr)
	if err != nil {
		shared.LogError("Failed to create UDP connection", err)
		done <- err
		return
	}
	
	// Create high-performance QUIC configuration (same as server)
	quicConfig := &quic.Config{
		// Flow control optimization for streaming
		InitialStreamReceiveWindow:     shared.QUICInitialStreamReceiveWindow,
		MaxStreamReceiveWindow:         shared.QUICMaxStreamReceiveWindow,
		InitialConnectionReceiveWindow: shared.QUICInitialConnectionReceiveWindow,
		MaxConnectionReceiveWindow:     shared.QUICMaxConnectionReceiveWindow,
		
		// Stream limits for concurrent connections
		MaxIncomingStreams:    shared.QUICMaxIncomingStreams,
		MaxIncomingUniStreams: shared.QUICMaxIncomingUniStreams,
		
		// Timeout optimization
		MaxIdleTimeout:       shared.QUICIdleTimeout,
		HandshakeIdleTimeout: shared.QUICHandshakeTimeout,
		KeepAlivePeriod:      shared.QUICKeepAlive,
		
		// Enable connection migration for better reliability
		DisablePathMTUDiscovery: false,
		EnableDatagrams:         false, // Focus on stream performance
	}

	// Connect to orchestrator's QUIC server with optimized config
	quicConn, err := quic.Dial(ctx, udpDialConn, remoteUDPAddr, tlsConfig, quicConfig)
	if err != nil {
		shared.LogError("Failed to connect to orchestrator", err)
		done <- err
		return
	}
	defer quicConn.CloseWithError(0, "done")
	
	shared.LogSuccess("Connected to orchestrator QUIC server!")
	
	// Handle QUIC connection streams
	handleQUICConnection(ctx, quicConn, done)
}


func handleQUICConnection(ctx context.Context, conn quic.Connection, done chan<- error) {
	defer conn.CloseWithError(0, "done")
	
	// Accept the first stream as control stream
	controlStream, err := conn.AcceptStream(ctx)
	if err != nil {
		shared.LogError("Failed to accept control stream", err)
		done <- err
		return
	}
	
	// Handle control stream in background
	controlDone := make(chan error, 1)
	go handleControlStream(controlStream, controlDone)
	
	// Create a context that cancels when we need to exit
	exitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	// Monitor for connection loss or control stream error
	go func() {
		select {
		case <-conn.Context().Done():
			shared.LogNetwork("QUIC connection lost, exiting immediately")
			cancel()
			done <- fmt.Errorf("QUIC connection lost")
		case err := <-controlDone:
			shared.LogNetwork("Control stream closed, exiting")
			cancel()
			done <- err
		case <-ctx.Done():
			shared.LogNetwork("Lambda context cancelled, exiting")
			cancel()
			done <- ctx.Err()
		}
	}()
	
	// Accept subsequent streams for SOCKS5
	for {
		stream, err := conn.AcceptStream(exitCtx)
		if err != nil {
			// Check if this is due to context cancellation (expected)
			if exitCtx.Err() != nil {
				return // Clean exit, done channel already signaled
			}
			shared.LogError("Failed to accept stream", err)
			// Signal completion on connection error
			done <- err
			return
		}
		
		go handleSOCKS5Stream(stream)
	}
}

func handleControlStream(stream quic.Stream, done chan<- error) {
	defer stream.Close()
	shared.LogNetwork("Control stream established")
	
	for {
		opcode, nonce, err := shared.ReadControlMessage(stream)
		if err != nil {
			// EOF is expected when client disconnects - treat as normal shutdown
			if err == io.EOF || errors.Is(err, io.EOF) {
				shared.LogNetwork("Control stream EOF - client disconnected normally")
				done <- nil
				return
			}
			shared.LogError("Failed to read control message", err)
			// Signal completion on control stream error
			done <- err
			return
		}
		
		switch opcode {
		case shared.OpPing:
			// Respond with pong
			if err := shared.WritePong(stream, nonce); err != nil {
				shared.LogError("Failed to send pong", err)
				return
			}
			
		case shared.OpShutdown:
			shared.LogNetwork("Received shutdown signal, exiting immediately")
			done <- nil
			return
			
		default:
			shared.LogErrorf("Unknown control opcode: %02x", opcode)
		}
	}
}

func handleSOCKS5Stream(stream quic.Stream) {
	defer stream.Close()
	
	// Read target address using shared utility
	target, err := shared.ReadSOCKS5TargetAddress(stream)
	if err != nil {
		shared.LogError("Failed to read target address", err)
		shared.WriteSOCKS5Response(stream, shared.SOCKS5ResponseError)
		return
	}
	
	shared.LogTargetf("Connecting to target: %s", target)
	
	// Connect to target
	targetConn, err := shared.ConnectToTarget(target, shared.DefaultConnectionTimeout)
	if err != nil {
		shared.LogErrorf("Failed to connect to target %s: %v", target, err)
		shared.WriteSOCKS5Response(stream, shared.SOCKS5ResponseError)
		return
	}
	defer targetConn.Close()
	
	// Send success response
	if err := shared.WriteSOCKS5Response(stream, shared.SOCKS5ResponseSuccess); err != nil {
		shared.LogError("Failed to send success response", err)
		return
	}
	
	shared.LogSuccessf("Connected to %s, starting data forwarding", target)
	
	// Start bidirectional forwarding using shared utility
	shared.ForwardData(stream, targetConn)
	shared.LogClosef("Connection to %s closed", target)
}


func performNATPunch(udpConn *net.UDPConn, sessionID string, orchestratorAddr *net.UDPAddr) bool {
	err := shared.PerformNATHolePunch(udpConn, sessionID, orchestratorAddr, shared.DefaultNATHolePunchTimeout, false)
	return err == nil
}

func main() {
	lambda.Start(LambdaHandler)
}