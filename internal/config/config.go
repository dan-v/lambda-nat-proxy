package config

import (
	"log"
	"os"
	"time"

	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)

// PerformanceMode defines different operational modes
type PerformanceMode string

const (
	ModeTest        PerformanceMode = "test"        // Fast testing, minimal resources
	ModeNormal      PerformanceMode = "normal"      // Balanced performance and cost
	ModePerformance PerformanceMode = "performance" // Maximum performance for streaming
)

// ModeConfig holds all configuration for a specific performance mode
type ModeConfig struct {
	Name           string
	LambdaTimeout  int           // Lambda timeout in seconds
	LambdaMemory   int           // Lambda memory in MB
	SessionTTL     time.Duration // Session time-to-live
	OverlapWindow  time.Duration // Overlap window for rotation
	DrainTimeout   time.Duration // Drain timeout
	BufferSize     int           // Data buffer size
	MaxStreams     int           // Maximum concurrent streams
	KeepAlive      time.Duration // Connection keep-alive
	IdleTimeout    time.Duration // Connection idle timeout
}

// RotationConfig holds session rotation configuration
type RotationConfig struct {
	OverlapWindow time.Duration
	DrainTimeout  time.Duration
	SessionTTL    time.Duration
}

// Config holds all configuration for the orchestrator
type Config struct {
	// AWS configuration
	AWSRegion    string
	S3BucketName string

	// Network configuration
	STUNServer string
	SOCKS5Port int

	// Timeout configuration
	LambdaResponseTimeout time.Duration
	NATHolePunchTimeout   time.Duration
	
	// Rotation configuration
	Rotation RotationConfig
	
	// Performance mode configuration
	Mode       PerformanceMode
	ModeConfig ModeConfig
}

// GetModeConfigs returns predefined mode configurations
func GetModeConfigs() map[PerformanceMode]ModeConfig {
	return map[PerformanceMode]ModeConfig{
		ModeTest: {
			Name:          "Test Mode",
			LambdaTimeout: 120,                     // 2 minutes - quick testing
			LambdaMemory:  128,                     // Minimal memory
			SessionTTL:    90 * time.Second,        // 1.5 min sessions
			OverlapWindow: 30 * time.Second,        // 30s overlap
			DrainTimeout:  15 * time.Second,        // Quick drain
			BufferSize:    8 * 1024,                // 8KB buffers
			MaxStreams:    100,                     // Limited streams
			KeepAlive:     10 * time.Second,        // Short keep-alive
			IdleTimeout:   2 * time.Minute,         // Short idle
		},
		ModeNormal: {
			Name:          "Normal Mode",
			LambdaTimeout: 600,                     // 10 minutes - balanced
			LambdaMemory:  256,                     // Balanced memory
			SessionTTL:    8 * time.Minute,         // 8 min sessions
			OverlapWindow: 90 * time.Second,        // 1.5 min overlap
			DrainTimeout:  45 * time.Second,        // Moderate drain
			BufferSize:    32 * 1024,               // 32KB buffers
			MaxStreams:    500,                     // Good stream count
			KeepAlive:     30 * time.Second,        // Standard keep-alive
			IdleTimeout:   5 * time.Minute,         // Standard idle
		},
		ModePerformance: {
			Name:          "Performance Mode",
			LambdaTimeout: 900,                     // 15 minutes - maximum
			LambdaMemory:  512,                     // High memory
			SessionTTL:    12 * time.Minute,        // 12 min sessions
			OverlapWindow: 2 * time.Minute,         // 2 min overlap
			DrainTimeout:  60 * time.Second,        // Full drain time
			BufferSize:    64 * 1024,               // 64KB buffers
			MaxStreams:    1000,                    // Maximum streams
			KeepAlive:     30 * time.Second,        // Optimal keep-alive
			IdleTimeout:   5 * time.Minute,         // Optimal idle
		},
	}
}

// New creates a new configuration with defaults from environment variables
func New() *Config {
	// Determine performance mode from environment
	modeStr := os.Getenv("MODE")
	if modeStr == "" {
		modeStr = "normal" // Default to normal mode
	}
	
	mode := PerformanceMode(modeStr)
	modeConfigs := GetModeConfigs()
	
	// Validate mode
	modeConfig, exists := modeConfigs[mode]
	if !exists {
		log.Printf("⚠️  Invalid mode '%s', using 'normal' mode", modeStr)
		mode = ModeNormal
		modeConfig = modeConfigs[ModeNormal]
	}
	
	log.Printf("🚀 %s: %s", modeConfig.Name, getModeDescription(mode))
	
	config := &Config{
		// Set defaults
		AWSRegion:             shared.DefaultAWSRegion,
		STUNServer:            shared.DefaultSTUNServer,
		SOCKS5Port:            shared.DefaultSOCKS5Port,
		LambdaResponseTimeout: shared.DefaultLambdaResponseTimeout,
		NATHolePunchTimeout:   shared.DefaultNATHolePunchTimeout,
		
		// Apply mode configuration
		Mode:       mode,
		ModeConfig: modeConfig,
		Rotation: RotationConfig{
			OverlapWindow: modeConfig.OverlapWindow,
			DrainTimeout:  modeConfig.DrainTimeout,
			SessionTTL:    modeConfig.SessionTTL,
		},
	}

	// Override with environment variables
	config.S3BucketName = os.Getenv("AWS_S3_BUCKET")

	if region := os.Getenv("AWS_REGION"); region != "" {
		config.AWSRegion = region
	}

	return config
}

// getModeDescription returns a description for the given mode
func getModeDescription(mode PerformanceMode) string {
	switch mode {
	case ModeTest:
		return "Fast testing with minimal resources (2min Lambda, 128MB)"
	case ModeNormal:
		return "Balanced performance and cost (10min Lambda, 256MB)"
	case ModePerformance:
		return "Maximum performance for streaming (15min Lambda, 512MB)"
	default:
		return "Unknown mode"
	}
}