package shared

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// DiscoverPublicIPHTTP discovers public IP using HTTP-based service
func DiscoverPublicIPHTTP() (string, error) {
	return DiscoverPublicIPHTTPWithTimeout(3 * time.Second)
}

// DiscoverPublicIPHTTPWithTimeout discovers public IP using HTTP-based service with custom timeout
func DiscoverPublicIPHTTPWithTimeout(timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get("http://checkip.amazonaws.com")
	if err != nil {
		return "", fmt.Errorf("failed to get public IP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid IP address received: %s", ip)
	}

	return ip, nil
}

// CreateUDPSocketWithPort creates a UDP socket bound to a specific port
func CreateUDPSocketWithPort(port int) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP socket: %w", err)
	}

	return conn, nil
}

// ReuseUDPPort creates a new UDP connection on the same local address
func ReuseUDPPort(localAddr *net.UDPAddr) (*net.UDPConn, error) {
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to reuse UDP port %d: %w", localAddr.Port, err)
	}
	return conn, nil
}

// CloseUDPSocketGracefully closes a UDP socket with a small delay to ensure port release
func CloseUDPSocketGracefully(conn *net.UDPConn) {
	if conn != nil {
		conn.Close()
		time.Sleep(DefaultSocketReleaseDelay)
	}
}

// ValidateNetworkAddress validates that an address can be resolved
func ValidateNetworkAddress(network, address string) error {
	switch network {
	case "tcp":
		_, err := net.ResolveTCPAddr(network, address)
		return err
	case "udp":
		_, err := net.ResolveUDPAddr(network, address)
		return err
	default:
		return fmt.Errorf("unsupported network type: %s", network)
	}
}

// ConnectWithTimeout creates a network connection with a timeout
func ConnectWithTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, address, timeout)
}

// OptimizedCopy performs high-performance bidirectional copying between two connections
// Optimized for streaming with larger buffers and concurrent copying
func OptimizedCopy(dst, src net.Conn) {
	OptimizedCopyWithBufferSize(dst, src, OptimizedBufferSize)
}

// OptimizedCopyWithBufferSize performs optimized copying with custom buffer size
func OptimizedCopyWithBufferSize(dst, src net.Conn, bufferSize int) {
	done := make(chan struct{}, 2)
	
	// Copy from src to dst
	go func() {
		defer func() { done <- struct{}{} }()
		copyWithBuffer(dst, src, bufferSize)
	}()
	
	// Copy from dst to src
	go func() {
		defer func() { done <- struct{}{} }()
		copyWithBuffer(src, dst, bufferSize)
	}()
	
	// Wait for either direction to complete
	<-done
	
	// Close both connections to stop the other direction
	dst.Close()
	src.Close()
	
	// Wait for the other direction to complete
	<-done
}

// copyWithBuffer performs optimized copying with a custom buffer size
func copyWithBuffer(dst io.Writer, src io.Reader, bufferSize int) (written int64, err error) {
	buf := make([]byte, bufferSize)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// copyWithBufferAndMetrics performs optimized copying with metrics tracking
func copyWithBufferAndMetrics(dst io.Writer, src io.Reader, bufferSize int, recordBytes func(int64)) (written int64, err error) {
	buf := make([]byte, bufferSize)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)
			// Record bytes transferred for metrics
			if recordBytes != nil && nw > 0 {
				recordBytes(int64(nw))
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// OptimizedCopyWithContext performs high-performance bidirectional copying between two connections with context support
func OptimizedCopyWithContext(ctx context.Context, dst, src net.Conn) {
	OptimizedCopyWithContextAndBufferSize(ctx, dst, src, OptimizedBufferSize)
}

// OptimizedCopyWithContextAndBufferSize performs optimized copying with custom buffer size and context support
func OptimizedCopyWithContextAndBufferSize(ctx context.Context, dst, src net.Conn, bufferSize int) {
	done := make(chan struct{}, 2)
	
	// Create a context for this copy operation
	copyCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	// Copy from src to dst
	go func() {
		defer func() { done <- struct{}{} }()
		copyWithBufferAndContext(copyCtx, dst, src, bufferSize)
	}()
	
	// Copy from dst to src
	go func() {
		defer func() { done <- struct{}{} }()
		copyWithBufferAndContext(copyCtx, src, dst, bufferSize)
	}()
	
	// Monitor for context cancellation
	go func() {
		<-copyCtx.Done()
		// Close both connections to interrupt any ongoing operations
		dst.Close()
		src.Close()
	}()
	
	// Wait for either direction to complete
	<-done
	
	// Cancel context to signal the other direction to stop
	cancel()
	
	// Close both connections to stop the other direction
	dst.Close()
	src.Close()
	
	// Wait for the other direction to complete
	<-done
}

// copyWithBufferAndContext performs optimized copying with a custom buffer size and context awareness
func copyWithBufferAndContext(ctx context.Context, dst io.Writer, src io.Reader, bufferSize int) (written int64, err error) {
	buf := make([]byte, bufferSize)
	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}
		
		// Set a read deadline if possible to avoid blocking indefinitely
		if conn, ok := src.(net.Conn); ok {
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		}
		
		nr, er := src.Read(buf)
		if nr > 0 {
			// Check for context cancellation before writing
			select {
			case <-ctx.Done():
				return written, ctx.Err()
			default:
			}
			
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			// Check if this is a timeout error due to read deadline
			if netErr, ok := er.(net.Error); ok && netErr.Timeout() {
				continue // Continue if it's just a timeout, not a real error
			}
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// OptimizedCopyWithMetrics performs high-performance bidirectional copying with metrics tracking
func OptimizedCopyWithMetrics(dst, src net.Conn, recordBytes func(int64)) {
	OptimizedCopyWithBufferSizeAndMetrics(dst, src, OptimizedBufferSize, recordBytes)
}

// OptimizedCopyWithBufferSizeAndMetrics performs optimized copying with custom buffer size and metrics
func OptimizedCopyWithBufferSizeAndMetrics(dst, src net.Conn, bufferSize int, recordBytes func(int64)) {
	done := make(chan struct{}, 2)
	
	// Copy from src to dst
	go func() {
		defer func() { done <- struct{}{} }()
		copyWithBufferAndMetrics(dst, src, bufferSize, recordBytes)
	}()
	
	// Copy from dst to src
	go func() {
		defer func() { done <- struct{}{} }()
		copyWithBufferAndMetrics(src, dst, bufferSize, recordBytes)
	}()
	
	// Wait for either direction to complete
	<-done
	
	// Close both connections to stop the other direction
	dst.Close()
	src.Close()
	
	// Wait for the other direction to complete
	<-done
}

// OptimizedCopyWithContextAndMetrics performs high-performance bidirectional copying with context and metrics
func OptimizedCopyWithContextAndMetrics(ctx context.Context, dst, src net.Conn, recordBytes func(int64)) {
	OptimizedCopyWithContextBufferSizeAndMetrics(ctx, dst, src, OptimizedBufferSize, recordBytes)
}

// OptimizedCopyWithContextBufferSizeAndMetrics performs optimized copying with context, buffer size, and metrics
func OptimizedCopyWithContextBufferSizeAndMetrics(ctx context.Context, dst, src net.Conn, bufferSize int, recordBytes func(int64)) {
	done := make(chan struct{}, 2)
	
	// Create a context for this copy operation
	copyCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	// Copy from src to dst
	go func() {
		defer func() { done <- struct{}{} }()
		copyWithBufferContextAndMetrics(copyCtx, dst, src, bufferSize, recordBytes)
	}()
	
	// Copy from dst to src
	go func() {
		defer func() { done <- struct{}{} }()
		copyWithBufferContextAndMetrics(copyCtx, src, dst, bufferSize, recordBytes)
	}()
	
	// Monitor for context cancellation
	go func() {
		<-copyCtx.Done()
		// Close both connections to interrupt any ongoing operations
		dst.Close()
		src.Close()
	}()
	
	// Wait for either direction to complete
	<-done
	
	// Cancel context to signal the other direction to stop
	cancel()
	
	// Close both connections to stop the other direction
	dst.Close()
	src.Close()
	
	// Wait for the other direction to complete
	<-done
}

// copyWithBufferContextAndMetrics performs optimized copying with context, custom buffer size, and metrics tracking
func copyWithBufferContextAndMetrics(ctx context.Context, dst io.Writer, src io.Reader, bufferSize int, recordBytes func(int64)) (written int64, err error) {
	buf := make([]byte, bufferSize)
	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}
		
		// Set a read deadline if possible to avoid blocking indefinitely
		if conn, ok := src.(net.Conn); ok {
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		}
		
		nr, er := src.Read(buf)
		if nr > 0 {
			// Check for context cancellation before writing
			select {
			case <-ctx.Done():
				return written, ctx.Err()
			default:
			}
			
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)
			// Record bytes transferred for metrics
			if recordBytes != nil && nw > 0 {
				recordBytes(int64(nw))
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			// Check if this is a timeout error due to read deadline
			if netErr, ok := er.(net.Error); ok && netErr.Timeout() {
				continue // Continue if it's just a timeout, not a real error
			}
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}