package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const traceIDContextKey = "trace_id"

// W3C header names for manual propagation helpers below.
const (
	HeaderTraceParent = "traceparent"
	HeaderTraceState  = "tracestate"
	HeaderTraceID     = "X-Trace-ID"
	HeaderRequestID   = "X-Request-ID"
)

func TracingMiddleware(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()
	if propagator == nil {
		propagator = propagation.TraceContext{}
	}

	return func(c *gin.Context) {
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		spanName := fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())
		if c.FullPath() == "" {
			spanName = fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL.Path)
		}

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", c.Request.Method),
				attribute.String("http.route", c.FullPath()),
				attribute.String("http.target", c.Request.URL.Path),
				attribute.String("http.user_agent", c.Request.UserAgent()),
				attribute.String("net.peer.ip", c.ClientIP()),
			),
		)
		defer span.End()

		traceID := span.SpanContext().TraceID().String()
		if span.SpanContext().IsValid() {
			c.Set(traceIDContextKey, traceID)
			c.Set("trace_id", traceID)
			c.Header("X-Trace-ID", traceID)
		}

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		span.SetAttributes(attribute.Int("http.status_code", c.Writer.Status()))
		for _, err := range c.Errors {
			span.RecordError(err.Err)
		}
	}
}

// --- W3C context helpers (issues #317) ---
// Lightweight TraceInfo propagation for services/background jobs so callers
// can forward trace IDs without importing OTel directly. The OTel span
// above remains the source of truth for export; these helpers carry the IDs
// in context.Context (see docs/OBSERVABILITY.md).

type traceContextKey struct{}

// TraceInfo carries the W3C identifiers for one request/job span.
type TraceInfo struct {
	TraceID string
	SpanID  string
	Sampled bool
	State   string
	IsNew   bool
}

// ParseTraceParent parses a W3C `traceparent` header:
// version-traceid-spanid-flags (e.g. 00-4bf92f...-00f067...-01).
func ParseTraceParent(header string) (traceID, spanID string, sampled bool, ok bool) {
	parts := strings.Split(strings.TrimSpace(header), "-")
	if len(parts) < 4 {
		return "", "", false, false
	}
	if len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return "", "", false, false
	}
	if !isHex(parts[1]) || !isHex(parts[2]) || !isHex(parts[3]) {
		return "", "", false, false
	}
	if parts[1] == strings.Repeat("0", 32) || parts[2] == strings.Repeat("0", 16) {
		return "", "", false, false
	}
	sampled = parts[3][1] == '1'
	return strings.ToLower(parts[1]), strings.ToLower(parts[2]), sampled, true
}

// FormatTraceParent renders a W3C `traceparent` header value.
func FormatTraceParent(traceID, spanID string, sampled bool) string {
	flags := "00"
	if sampled {
		flags = "01"
	}
	return "00-" + traceID + "-" + spanID + "-" + flags
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func newHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// Fallback: never fail tracing because of RNG; callers still get unique-ish IDs.
		for i := range b {
			b[i] = byte(i*31 + 17)
		}
	}
	return hex.EncodeToString(b)
}

// ContextWithTrace stores TraceInfo in ctx.
func ContextWithTrace(ctx context.Context, info TraceInfo) context.Context {
	return context.WithValue(ctx, traceContextKey{}, info)
}

// TraceFromContext returns the TraceInfo stored in ctx, if any.
func TraceFromContext(ctx context.Context) (TraceInfo, bool) {
	if ctx == nil {
		return TraceInfo{}, false
	}
	info, ok := ctx.Value(traceContextKey{}).(TraceInfo)
	return info, ok
}

// TraceIDFromContext returns the trace ID or "" when absent.
func TraceIDFromContext(ctx context.Context) string {
	if info, ok := TraceFromContext(ctx); ok {
		return info.TraceID
	}
	return ""
}

// SpanIDFromContext returns the span ID or "" when absent.
func SpanIDFromContext(ctx context.Context) string {
	if info, ok := TraceFromContext(ctx); ok {
		return info.SpanID
	}
	return ""
}

// StartChildSpan derives a child span (new span ID, same trace ID) for a
// downstream service call. The returned context must be passed to the
// service/DB/HTTP call so the trace continues.
func StartChildSpan(ctx context.Context) (context.Context, TraceInfo) {
	if info, ok := TraceFromContext(ctx); ok {
		child := info
		child.SpanID = newHex(8)
		child.IsNew = false
		return ContextWithTrace(ctx, child), child
	}
	info := TraceInfo{TraceID: newHex(16), SpanID: newHex(8), Sampled: true, IsNew: true}
	return ContextWithTrace(ctx, info), info
}

// StartJobSpan starts (or continues) a trace for a background job.
// Use: ctx, end := StartJobSpan(parentCtx, "backup-nightly"); defer end().
func StartJobSpan(ctx context.Context, _ string) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := TraceFromContext(ctx); ok {
		childCtx, _ := StartChildSpan(ctx)
		return childCtx, func() {}
	}
	info := TraceInfo{TraceID: newHex(16), SpanID: newHex(8), Sampled: true, IsNew: true}
	return ContextWithTrace(ctx, info), func() {}
}

// InjectTraceHeaders writes W3C headers into an outgoing request.
func InjectTraceHeaders(ctx context.Context, header http.Header) {
	info, ok := TraceFromContext(ctx)
	if !ok {
		return
	}
	header.Set(HeaderTraceParent, FormatTraceParent(info.TraceID, info.SpanID, info.Sampled))
	if info.State != "" {
		header.Set(HeaderTraceState, info.State)
	}
	header.Set(HeaderTraceID, info.TraceID)
}
