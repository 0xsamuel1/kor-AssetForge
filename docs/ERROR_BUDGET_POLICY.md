# Error Budget Policy (closes #320)

## SLO / budget

- **SLO:** 99.9% successful responses per 30-day window (5xx-aware).
- **Error budget:** 0.1% of `http_requests_total` may be `app_errors_total`.
- **Tracked per endpoint:** `app_errors_total{code,endpoint}` (route templates
  via `c.FullPath()`, bridged from `apperrors` by `tracking.go`) divided by
  `http_requests_total{path}` (`handlers.MetricsMiddleware`).
- **Durability:** Prometheus time-series (not the reset-on-restart in-memory
  `GetErrorMetrics()` map, which remains for tests/debug at `MetricsHandler`).

## Alerts

Defined in `monitoring/alerts/error_budget.yml` (Prometheus rule group
`kor-assetforge-error-budget`):

| Alert | Condition | Action |
|---|---|---|
| `ErrorBudgetFastBurn` (critical) | burn > 20x for 5m | page on-call, freeze deploys |
| `ErrorBudgetSlowBurn` (warning) | burn > 2x for 30m | ticket, investigate within a day |
| `EndpointErrorRateHigh` (warning) | endpoint > 1% for 10m | owner of `endpoint` investigates |

Legacy single-window `HighErrorRate > 5%` in
`monitoring/prometheus-rules.yml` is unchanged.

## Dashboard

Grafana error-rate panel query:

```promql
sum(rate(app_errors_total[5m])) / sum(rate(http_requests_total[5m]))
```

Per-endpoint:

```promql
sum by (endpoint) (rate(app_errors_total[5m]))
/ sum by (endpoint) (rate(http_requests_total[5m]))
```

Remaining budget (set by reconciler via `metrics.SetBudgetRemaining`):

```promql
error_budget_remaining_ratio{service="kor-assetforge"}
```

## Response process

1. Alert fires → find `endpoint` + `code` labels in Prometheus.
2. Pull `trace_id`/`request_id` logs in Kibana (see `docs/LOGGING_STRATEGY.md`).
3. Mitigate (rollback / feature-flag), then fix forward.
4. If budget is exhausted: feature freeze except fixes until the 30d window recovers.

## Postmortem template

```markdown
# Postmortem: <date> <endpoint> <code>
- Impact: <error % / duration / users affected>
- Detection: <which alert, time to detect>
- Root cause:
- Mitigation:
- Follow-ups: <owner + due date each>
- Budget impact: <% of 30d budget consumed>
```
