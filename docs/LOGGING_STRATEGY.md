# Logging Strategy (closes #318)

## Architecture

```
app (zap JSON → stdout) → Docker json-file → Fluentd → Elasticsearch → Kibana
```

- **Format:** JSON to `stdout` only (never files inside containers).
  Required keys: `timestamp` (ISO8601), `level`, `msg`, `service`,
  plus `request_id` / `trace_id` / `span_id` per request (see
  `backend/logger/structured.go` + `middleware.TracingMiddleware`).
- **Collection:** `backend` container uses the `fluentd` log driver in
  production compose; local dev keeps `json-file` with rotation.
  Fluentd config: `infrastructure/logging/fluentd.conf`.
- **Storage/search:** Elasticsearch (`elasticsearch` service) with daily
  `kor-assetforge-*` indices, viewed in Kibana (`kibana` service).
- **Integration with monitoring:** error-rate alerts
  (`monitoring/prometheus-rules.yml`, `monitoring/alerts/error_budget.yml`)
  fire from metrics; logs give the `trace_id`/`request_id` to drill in.

## Usage

```go
import "github.com/yourusername/kor-assetforge/logger"
import "github.com/yourusername/kor-assetforge/middleware"

var base = logger.MustNewLogger("kor-assetforge-api")

// inside a handler/job with ctx:
l := logger.WithRequest(base,
    c.GetString("request_id"),
    middleware.TraceIDFromContext(ctx),
    middleware.SpanIDFromContext(ctx),
)
l.Info("asset tokenized", zap.String("asset_id", id))
```

Level via `LOG_LEVEL=debug|info|warn|error` (default `info`).

## Retention

| Tier | Policy |
|---|---|
| Docker json-file (all envs) | `max-size: 10m`, `max-file: 5` (≈50 MB/container) |
| Elasticsearch hot indices | 30 days, then delete (dev) / snapshot to S3 (prod) |
| Audit/compliance rows (`audit_logs` table) | 90 days, gzip-compressed after 30d (existing `AuditService`) |
| Backup job logs | kept with the backup artifact in S3 |

## Search (Kibana / Elasticsearch examples)

All production logs:

```
service: "kor-assetforge-api"
```

One request across services/jobs:

```
trace_id: "4bf92f3577b34da6a3ce929d0e0e4736"
```

Errors in the last hour with trace correlation:

```
level: "error" AND timestamp >= now-1h
```

High-latency requests (from `RequestLogger` duration field):

```
msg: "HTTP Request" AND duration >= 1.0
```

Audit + app join: filter app logs by `request_id`, then query
`audit_logs` for the same `path`/`ip` window.

## Migration notes

- `handlers.Logger` and `apperrors.Logger` keep working; prefer
  `logger.MustNewLogger` for new packages.
- Replace bare `log.Printf` call sites incrementally with the request-scoped
  logger above so every line carries `request_id`/`trace_id`.
