# Elasticsearch search failures

## Symptoms

- `SearchUsingDatabaseFallback` or `ElasticsearchSearchUnavailable` is firing.
- `search_elasticsearch_available` is `0` and `search_fallback_total` is increasing.
- Search remains available, but database load or search latency increases.

## Triage

1. Check Elasticsearch cluster health, reachability from the API, credentials, TLS certificates, and disk watermarks.
2. Compare `search_backend_duration_seconds` for `backend="database"` and `backend="elasticsearch"`.
3. Inspect API logs for `Elasticsearch became unavailable` and the recorded failure reason.
4. Protect PostgreSQL capacity if fallback traffic is sustained; reduce upstream search traffic before changing database limits.

## Recovery

Restore Elasticsearch rather than restarting the API. While degraded, the search service probes Elasticsearch every 15 seconds. A successful probe automatically restores Elasticsearch traffic and increments `search_elasticsearch_recoveries_total`.

Confirm that `search_elasticsearch_available` returns to `1`, fallback rate reaches zero, and result/latency panels normalize. If index contents are stale, run the normal reindex procedure only after cluster health is green.
