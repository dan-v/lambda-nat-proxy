package shared

import (
	"context"
	"log/slog"
	"os"
	"time"
)

var (
	// Global structured logger
	logger *slog.Logger
	
	// Log levels
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// LogConfig holds configuration for the logger
type LogConfig struct {
	Level       slog.Level
	Format      string // "json" or "text"
	AddSource   bool
	ServiceName string
}

// DefaultLogConfig returns a default logger configuration
func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		Level:       slog.LevelInfo,
		Format:      "text",
		AddSource:   false,
		ServiceName: "lambda-nat-proxy",
	}
}

// InitLogger initializes the structured logger
func InitLogger(config *LogConfig) {
	if config == nil {
		config = DefaultLogConfig()
	}
	
	var handler slog.Handler
	
	opts := &slog.HandlerOptions{
		Level:     config.Level,
		AddSource: config.AddSource,
	}
	
	if config.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	
	logger = slog.New(handler).With(
		"service", config.ServiceName,
		"version", "1.0.0",
	)
	
	// Set as default logger
	slog.SetDefault(logger)
}

// GetLogger returns the global structured logger
func GetLogger() *slog.Logger {
	if logger == nil {
		InitLogger(nil) // Initialize with defaults
	}
	return logger
}

// Structured logging functions with context support

// LogWithContext logs a message with context and structured fields
func LogWithContext(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	GetLogger().LogAttrs(ctx, level, msg, attrs...)
}

// LogDebug logs a debug message with structured fields
func LogDebug(msg string, attrs ...slog.Attr) {
	GetLogger().LogAttrs(context.Background(), slog.LevelDebug, msg, attrs...)
}

// StructuredInfo logs an info message with structured fields
func StructuredInfo(msg string, attrs ...slog.Attr) {
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// StructuredWarn logs a warning message with structured fields
func StructuredWarn(msg string, attrs ...slog.Attr) {
	GetLogger().LogAttrs(context.Background(), slog.LevelWarn, msg, attrs...)
}

// StructuredError logs an error message with structured fields
func StructuredError(msg string, attrs ...slog.Attr) {
	GetLogger().LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
}

// Convenience functions for common operations

// LogErrorWithDetails logs an error with operation context
func LogErrorWithDetails(operation string, err error, attrs ...slog.Attr) {
	allAttrs := append([]slog.Attr{
		slog.String("operation", operation),
		slog.String("error", err.Error()),
		slog.Time("timestamp", time.Now()),
	}, attrs...)
	StructuredError("Operation failed", allAttrs...)
}

// LogSuccessWithDetails logs a successful operation
func LogSuccessWithDetails(operation string, attrs ...slog.Attr) {
	allAttrs := append([]slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}, attrs...)
	StructuredInfo("Operation completed successfully", allAttrs...)
}

// LogConnectionEvent logs connection-related events
func LogConnectionEvent(event string, remote string, attrs ...slog.Attr) {
	allAttrs := append([]slog.Attr{
		slog.String("event", event),
		slog.String("remote_addr", remote),
		slog.Time("timestamp", time.Now()),
	}, attrs...)
	StructuredInfo("Connection event", allAttrs...)
}

// LogMetrics logs performance metrics
func LogMetrics(component string, metrics map[string]interface{}) {
	attrs := []slog.Attr{
		slog.String("component", component),
		slog.Time("timestamp", time.Now()),
	}
	
	for key, value := range metrics {
		switch v := value.(type) {
		case string:
			attrs = append(attrs, slog.String(key, v))
		case int:
			attrs = append(attrs, slog.Int(key, v))
		case int64:
			attrs = append(attrs, slog.Int64(key, v))
		case float64:
			attrs = append(attrs, slog.Float64(key, v))
		case bool:
			attrs = append(attrs, slog.Bool(key, v))
		case time.Duration:
			attrs = append(attrs, slog.Duration(key, v))
		default:
			attrs = append(attrs, slog.Any(key, v))
		}
	}
	
	StructuredInfo("Performance metrics", attrs...)
}

// SetLogLevel dynamically sets the log level
func SetLogLevel(level slog.Level) {
	// Note: slog doesn't support dynamic level changes easily
	// This would require reinitializing the handler
	config := DefaultLogConfig()
	config.Level = level
	InitLogger(config)
}