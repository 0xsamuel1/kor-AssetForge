# Observability — Distributed Tracing

Trace context propagation for `kor-AssetForge` (closes #317).

## Architecture

- **Propagation standard:** W3C Trace Context (`traceparent`, `tracestate`).
- **HTTP in:** `middleware.TracingMiddleware()` (`backend/middleware/tracing.go`)
  extracts `traceparent` or starts a new trace, stores `TraceInfo`
  in `context.Context`, and echoes `traceparent` + `X-Trace-ID`.
- **Export stub:** `monitoring.InitializeTracing(ctx, service, collectorURL)`
  records config and logs; swap its body for a real OTLP exporter once
  `go.opentelemetry.io/*` deps are vendored — callers stay unchanged.
- **Services:** all `services/*` methods already accept `ctx`; forward
  `c.Request.Context()` (or the job ctx) and use
  `middleware.StartChildSpan(ctx)` for each downstream call
  (DB via `db.WithContext(ctx)`, Redis via `rdb.*(ctx, ...)`, outgoing HTTP
  via `middleware.InjectTraceHeaders(ctx, req.Header)`).
- **Jobs:** wrap every background entry point with `jobs.RunTraced`
  (`backend/jobs/traced.go`); scheduled loops use `jobs.ContextForSchedule`.

## Wiring

`main.go` (order matters — tracing first so `request_id` joins `trace_id`):

```go
router.Use(
    middleware.TracingMiddleware(),
    handlers.RequestLogger(),
    ...
)
```

Startup / shutdown:

```go
tp, _ := monitoring.InitializeTracing(ctx, "kor-assetforge", os.Getenv("OTEL_COLLECTOR_URL"))
defer monitoring.ShutdownTracing(context.Background(), tp)
```

## Examples

Incoming request with trace:

```bash
curl -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" \
  localhost:8080/api/v1/assets
# response headers include:
# traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-<new-span>-01
# X-Trace-ID: 4bf92f3577b34da6a3ce929d0e0e4736
```

Propagate inside a service call:

```go
func (s *AnalyticsService) GetDashboard(ctx context.Context, userID uint) (*ActivitySummary, error) {
    childCtx, _ := middleware.StartChildSpan(ctx) // new span, same trace
    s.db.WithContext(childCtx).Model(&models.UserBalance{}).Count(&n)
    if s.redis != nil {
        s.redis.Get(childCtx, key).Bytes()
    }
    return summary, nil
}
```

Outgoing HTTP from a service:

```go
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
middleware.InjectTraceHeaders(ctx, req.Header)
```

Traced background job:

```go
// scheduled (new trace):
_ = jobs.RunTraced(context.Background(), "backup-nightly", func(ctx context.Context) error {
    _, err := backupSvc.Run(ctx)
    return err
})

// request-triggered (continues trace):
_ = jobs.RunTraced(c.Request.Context(), "webhook-delivery", func(ctx context.Context) error {
    return deliverySvc.Deliver(ctx, event)
})
```

Logging correlation: every zap log inside a request/job should include
`trace_id` (see `logger.WithTraceID` / `docs/LOGGING_STRATEGY.md`):

```go
log := logger.WithTraceID(baseLogger, middleware.TraceIDFromContext(ctx))
```

## Trace visualization end-to-end

1. Client sends `traceparent` (or none → server starts one).
2. `TracingMiddleware` creates the server span, returns it in the response.
3. Services fan out with `StartChildSpan`; each DB/Redis/HTTP hop keeps the
   same `trace_id` with a new `span_id`.
4. Jobs continue the trace via `jobs.RunTraced`.
5. Point the OTLP collector at the service (`OTEL_COLLECTOR_URL`) and filter
   by `trace_id` (== `X-Trace-ID` response header) in Jaeger/Tempo to see the
   full request → service → job waterfall.
