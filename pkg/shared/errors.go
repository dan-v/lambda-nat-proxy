package shared

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// LogError logs an error with consistent formatting and emoji prefix
func LogError(operation string, err error) {
	msg := fmt.Sprintf("❌ %s: %v", operation, err)
	GetLogger().Error(msg, 
		slog.String("operation", operation),
		slog.String("error", err.Error()),
		slog.Time("timestamp", time.Now()),
	)
}

// LogErrorf logs a formatted error message with emoji prefix
func LogErrorf(format string, args ...interface{}) {
	msg := fmt.Sprintf("❌ "+format, args...)
	GetLogger().Error(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// LogSuccess logs a success message with emoji prefix
func LogSuccess(operation string, details ...interface{}) {
	var msg string
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}
	
	if len(details) > 0 {
		detailStr := fmt.Sprint(details...)
		msg = fmt.Sprintf("✅ %s: %v", operation, detailStr)
		attrs = append(attrs, slog.String("details", detailStr))
	} else {
		msg = fmt.Sprintf("✅ %s", operation)
	}
	
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// LogSuccessf logs a formatted success message with emoji prefix
func LogSuccessf(format string, args ...interface{}) {
	msg := fmt.Sprintf("✅ "+format, args...)
	GetLogger().Info(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// LogInfo logs an informational message with emoji prefix
func LogInfo(operation string, details ...interface{}) {
	var msg string
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}
	
	if len(details) > 0 {
		detailStr := fmt.Sprint(details...)
		msg = fmt.Sprintf("ℹ️ %s: %v", operation, detailStr)
		attrs = append(attrs, slog.String("details", detailStr))
	} else {
		msg = fmt.Sprintf("ℹ️ %s", operation)
	}
	
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// LogInfof logs a formatted informational message with emoji prefix
func LogInfof(format string, args ...interface{}) {
	msg := fmt.Sprintf("ℹ️ "+format, args...)
	GetLogger().Info(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// LogProgress logs a progress/activity message with emoji prefix
func LogProgress(operation string, details ...interface{}) {
	var msg string
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}
	
	if len(details) > 0 {
		detailStr := fmt.Sprint(details...)
		msg = fmt.Sprintf("🔄 %s: %v", operation, detailStr)
		attrs = append(attrs, slog.String("details", detailStr))
	} else {
		msg = fmt.Sprintf("🔄 %s", operation)
	}
	
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// LogProgressf logs a formatted progress message with emoji prefix
func LogProgressf(format string, args ...interface{}) {
	msg := fmt.Sprintf("🔄 "+format, args...)
	GetLogger().Info(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// LogTarget logs a target/action message with emoji prefix
func LogTarget(operation string, details ...interface{}) {
	var msg string
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}
	
	if len(details) > 0 {
		detailStr := fmt.Sprint(details...)
		msg = fmt.Sprintf("🎯 %s: %v", operation, detailStr)
		attrs = append(attrs, slog.String("details", detailStr))
	} else {
		msg = fmt.Sprintf("🎯 %s", operation)
	}
	
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// LogTargetf logs a formatted target message with emoji prefix
func LogTargetf(format string, args ...interface{}) {
	msg := fmt.Sprintf("🎯 "+format, args...)
	GetLogger().Info(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// LogNetwork logs a network-related message with emoji prefix
func LogNetwork(operation string, details ...interface{}) {
	var msg string
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}
	
	if len(details) > 0 {
		detailStr := fmt.Sprint(details...)
		msg = fmt.Sprintf("🌐 %s: %v", operation, detailStr)
		attrs = append(attrs, slog.String("details", detailStr))
	} else {
		msg = fmt.Sprintf("🌐 %s", operation)
	}
	
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// LogNetworkf logs a formatted network message with emoji prefix
func LogNetworkf(format string, args ...interface{}) {
	msg := fmt.Sprintf("🌐 "+format, args...)
	GetLogger().Info(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// LogConnection logs a connection-related message with emoji prefix
func LogConnection(operation string, details ...interface{}) {
	var msg string
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}
	
	if len(details) > 0 {
		detailStr := fmt.Sprint(details...)
		msg = fmt.Sprintf("🔗 %s: %v", operation, detailStr)
		attrs = append(attrs, slog.String("details", detailStr))
	} else {
		msg = fmt.Sprintf("🔗 %s", operation)
	}
	
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// LogConnectionf logs a formatted connection message with emoji prefix
func LogConnectionf(format string, args ...interface{}) {
	msg := fmt.Sprintf("🔗 "+format, args...)
	GetLogger().Info(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// LogStorage logs a storage-related message with emoji prefix
func LogStorage(operation string, details ...interface{}) {
	var msg string
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}
	
	if len(details) > 0 {
		detailStr := fmt.Sprint(details...)
		msg = fmt.Sprintf("📂 %s: %v", operation, detailStr)
		attrs = append(attrs, slog.String("details", detailStr))
	} else {
		msg = fmt.Sprintf("📂 %s", operation)
	}
	
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// LogStoragef logs a formatted storage message with emoji prefix
func LogStoragef(format string, args ...interface{}) {
	msg := fmt.Sprintf("📂 "+format, args...)
	GetLogger().Info(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// LogClose logs a closure/end message with emoji prefix
func LogClose(operation string, details ...interface{}) {
	var msg string
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.Time("timestamp", time.Now()),
	}
	
	if len(details) > 0 {
		detailStr := fmt.Sprint(details...)
		msg = fmt.Sprintf("🔚 %s: %v", operation, detailStr)
		attrs = append(attrs, slog.String("details", detailStr))
	} else {
		msg = fmt.Sprintf("🔚 %s", operation)
	}
	
	GetLogger().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// LogClosef logs a formatted closure message with emoji prefix
func LogClosef(format string, args ...interface{}) {
	msg := fmt.Sprintf("🔚 "+format, args...)
	GetLogger().Info(msg,
		slog.String("formatted_message", fmt.Sprintf(format, args...)),
		slog.Time("timestamp", time.Now()),
	)
}

// WrapError wraps an error with additional context
func WrapError(err error, operation string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// WrapErrorf wraps an error with formatted additional context
func WrapErrorf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}