package logger

// Central structured-logging factory (issue #318).
//
// Today zap loggers are built ad-hoc in handlers/middleware.go and
// apperrors/middleware.go with divergent configs and no trace/request
// correlation. New code should use this package; existing Logger vars keep
// working untouched.
//
// Conventions:
//   - JSON to stdout (Docker json-file driver + Fluentd tail it).
//   - Required fields: timestamp (ISO8601), level, msg, service.
//   - Per-request fields: request_id, trace_id, span_id (see
//     docs/LOGGING_STRATEGY.md and middleware/tracing.go).
//   - Level from LOG_LEVEL (debug/info/warn/error), default info.
//   - Production sampling enabled; development mode via APP_ENV=development.

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger builds a JSON zap logger writing to stdout.
func NewLogger(serviceName string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.InitialFields = map[string]interface{}{"service": serviceName}
	cfg.Level = zap.NewAtomicLevelAt(levelFromEnv())
	// Keep production sampling (drops repetitive info logs under load).
	return cfg.Build()
}

// MustNewLogger is NewLogger that panics on error (for main/init).
func MustNewLogger(serviceName string) *zap.Logger {
	l, err := NewLogger(serviceName)
	if err != nil {
		panic(err)
	}
	return l
}

func levelFromEnv() zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// WithRequestID returns a logger enriched with request_id.
func WithRequestID(l *zap.Logger, requestID string) *zap.Logger {
	if l == nil || requestID == "" {
		return l
	}
	return l.With(zap.String("request_id", requestID))
}

// WithTraceID returns a logger enriched with trace/span correlation.
func WithTraceID(l *zap.Logger, traceID, spanID string) *zap.Logger {
	if l == nil || traceID == "" {
		return l
	}
	fields := []zap.Field{zap.String("trace_id", traceID)}
	if spanID != "" {
		fields = append(fields, zap.String("span_id", spanID))
	}
	return l.With(fields...)
}

// WithRequest returns a logger enriched from gin-style string values.
func WithRequest(l *zap.Logger, requestID, traceID, spanID string) *zap.Logger {
	l = WithRequestID(l, requestID)
	return WithTraceID(l, traceID, spanID)
}
