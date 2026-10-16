package middleware

// W3C Trace Context propagation for kor-AssetForge.
//
// Minimal, stdlib-only implementation (no new external dependencies):
//   - Extracts incoming `traceparent` (+ `tracestate`) per W3C spec.
//   - Generates a new trace ID when absent/invalid.
//   - Derives a child span ID per request and puts both into
//     context.Context so backend/services/* can propagate them
//     via c.Request.Context() / ctx parameters.
//   - Echoes `traceparent` + `X-Trace-ID` on the response and joins
//     trace state with the existing `X-Request-ID` / `request_id`
//     logging convention (handlers.RequestLogger).
//
// Background jobs (backend/jobs, services backup/audit/cache warmers)
// should start from StartJobSpan which continues the incoming trace
// when present or starts a fresh one otherwise.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type traceContextKey struct{}

// W3C header names.
const (
	HeaderTraceParent = "traceparent"
	HeaderTraceState  = "tracestate"
	HeaderTraceID     = "X-Trace-ID"
	HeaderRequestID   = "X-Request-ID"
)

// TraceInfo carries the W3C identifiers for one request/job span.
type TraceInfo struct {
	TraceID  string
	SpanID   string
	Sampled  bool
	State    string
	IsNew    bool
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

// TracingMiddleware extracts/creates W3C trace context for each request.
// It must run before RequestLogger so request_id and trace_id are joined.
func TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var info TraceInfo
		if tp := c.GetHeader(HeaderTraceParent); tp != "" {
			if traceID, parentSpan, sampled, ok := ParseTraceParent(tp); ok {
				info = TraceInfo{
					TraceID: traceID,
					SpanID:  newHex(8), // this request is a child of the incoming span
					Sampled: sampled,
					State:   c.GetHeader(HeaderTraceState),
				}
				_ = parentSpan
			}
		}
		if info.TraceID == "" {
			info = TraceInfo{TraceID: newHex(16), SpanID: newHex(8), Sampled: true, IsNew: true}
		}

		ctx := ContextWithTrace(c.Request.Context(), info)
		c.Request = c.Request.WithContext(ctx)

		// Join with existing request_id convention when present.
		if rid, ok := c.Get("request_id"); ok {
			if s, ok := rid.(string); ok && s != "" {
				c.Set("trace_id", info.TraceID)
				_ = s
			}
		} else {
			c.Set("trace_id", info.TraceID)
		}

		c.Header(HeaderTraceParent, FormatTraceParent(info.TraceID, info.SpanID, info.Sampled))
		if info.State != "" {
			c.Header(HeaderTraceState, info.State)
		}
		c.Header(HeaderTraceID, info.TraceID)

		c.Next()
	}
}
