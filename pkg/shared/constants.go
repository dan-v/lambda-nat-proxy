package shared

import "time"

// Network constants
const (
	DefaultAWSRegion         = "us-west-2"
	DefaultSOCKS5Port        = 1080
	DefaultSTUNServer        = "stun.l.google.com:19302"
	DefaultSocketReleaseDelay = 100 * time.Millisecond
)

// Timeout constants
const (
	DefaultLambdaResponseTimeout = 10 * time.Second
	DefaultNATHolePunchTimeout   = 6 * time.Second
	DefaultConnectionTimeout     = 10 * time.Second
	DefaultPollingInterval       = 500 * time.Millisecond
	HolePunchInterval           = 100 * time.Millisecond
	ResponsePollInterval        = 500 * time.Millisecond
	UDPReadTimeout             = 200 * time.Millisecond
)

// NAT traversal constants
const (
	HolePunchPacketCount      = 50
	UDPBufferSize            = 1500
	MaxTargetAddressLength   = 1024
)

// Buffer size constants (mode-aware defaults)
const (
	OptimizedBufferSize = 32 * 1024  // 32KB default, overridden by mode
)

// S3 key patterns
const (
	CoordinationKeyPattern = "coordination/%s.json"
	ResponseKeyPattern     = "punch-response/%s.json"
)

// SOCKS5 protocol constants
const (
	SOCKS5Version    = 0x05
	SOCKS5Connect    = 0x01
	SOCKS5NoAuth     = 0x00
	SOCKS5Success    = 0x00
	SOCKS5Failed     = 0x01
	SOCKS5IPv4       = 0x01
	SOCKS5DomainName = 0x03
)

// TLS certificate constants
const (
	TLSKeyBits         = 2048
	CertValidityPeriod = 365 * 24 * time.Hour
)

// QUIC performance constants (mode-aware)
const (
	// Base QUIC settings (scaled by mode)
	QUICBaseStreamReceiveWindow     = 16 * 1024 * 1024  // 16MB per stream (base)
	QUICBaseConnectionReceiveWindow = 64 * 1024 * 1024  // 64MB per connection (base)
	
	// Default QUIC settings
	QUICHandshakeTimeout = 10 * time.Second
	QUICMaxIncomingUniStreams = 100
)

// Legacy QUIC constants (will be replaced by mode-based config)
const (
	QUICInitialStreamReceiveWindow     = 16 * 1024 * 1024  // 16MB per stream
	QUICMaxStreamReceiveWindow         = 32 * 1024 * 1024  // 32MB max per stream
	QUICInitialConnectionReceiveWindow = 64 * 1024 * 1024  // 64MB initial connection
	QUICMaxConnectionReceiveWindow     = 128 * 1024 * 1024 // 128MB max connection
	QUICMaxIncomingStreams            = 1000              // Max concurrent streams
	QUICIdleTimeout                   = 5 * time.Minute   // Connection idle timeout
	QUICKeepAlive                     = 30 * time.Second  // Keep-alive period
)

// GetQUICConfig returns QUIC configuration values based on buffer size and max streams
func GetQUICConfig(bufferSize, maxStreams int) (streamWindow, connWindow, maxIncomingStreams, maxIncomingUniStreams int64) {
	// Scale flow control windows based on buffer size
	scale := float64(bufferSize) / float64(8*1024) // 8KB is base
	if scale < 1 {
		scale = 1
	}
	
	streamWindow = int64(float64(QUICBaseStreamReceiveWindow) * scale)
	connWindow = int64(float64(QUICBaseConnectionReceiveWindow) * scale)
	maxIncomingStreams = int64(maxStreams)
	maxIncomingUniStreams = int64(QUICMaxIncomingUniStreams)
	
	return
}

// SOCKS5 response templates
var (
	SOCKS5AuthResponse    = []byte{SOCKS5Version, SOCKS5NoAuth}
	SOCKS5SuccessResponse = []byte{SOCKS5Version, SOCKS5Success, 0x00, SOCKS5IPv4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	SOCKS5FailureResponse = []byte{SOCKS5Version, SOCKS5Failed, 0x00, SOCKS5IPv4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
)