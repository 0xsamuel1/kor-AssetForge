package monitoring

// OTLP exporter setup for kor-AssetForge.
//
// NOTE: The previous version of this file imported
// go.opentelemetry.io/* packages that are not vendored in go.mod,
// which broke `go build ./...`. This revision keeps the same
// InitializeTracing/ShutdownTracing API but is stdlib-only: it records
// the configured endpoint and relies on middleware.TracingMiddleware
// for W3C Trace Context propagation (see backend/middleware/tracing.go
// and docs/OBSERVABILITY.md). Swap the body of InitializeTracing with a
// real OTLP exporter once the otel dependencies are vendored — callers
// do not need to change.

import (
	"context"
	"log"
)

// TracerProvider is a minimal stub holding tracing configuration.
// It intentionally mirrors the lifecycle of an OTel TracerProvider
// (initialize once at startup, shut down on exit) without new deps.
type TracerProvider struct {
	ServiceName  string
	CollectorURL string
}

// InitializeTracing records the tracing configuration. Safe to call once
// from main(); returns a provider to pass to ShutdownTracing on exit.
func InitializeTracing(ctx context.Context, serviceName, collectorURL string) (*TracerProvider, error) {
	_ = ctx
	tp := &TracerProvider{ServiceName: serviceName, CollectorURL: collectorURL}
	if collectorURL == "" {
		log.Printf("[tracing] collector URL empty: running with in-process W3C propagation only (service=%s)", serviceName)
	} else {
		log.Printf("[tracing] initialized for service=%s collector=%s (W3C traceparent propagation enabled)", serviceName, collectorURL)
	}
	return tp, nil
}

// ShutdownTracing flushes any pending spans. Currently a no-op with logging
// so main() can defer it regardless of exporter implementation.
func ShutdownTracing(ctx context.Context, tp *TracerProvider) {
	_ = ctx
	if tp == nil {
		return
	}
	log.Printf("[tracing] shutdown (service=%s)", tp.ServiceName)
}
