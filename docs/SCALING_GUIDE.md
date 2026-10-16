# Horizontal Scaling Guide

kor-AssetForge's API is designed to be **stateless** so that any number of
identical instances can serve traffic behind a load balancer. This guide
explains how to scale the API horizontally, size database connection pools,
and plan capacity.

- **Related:** [`ARCHITECTURE_DIAGRAM.md`](./ARCHITECTURE_DIAGRAM.md), [`LOAD_BALANCER_CONFIG.md`](./LOAD_BALANCER_CONFIG.md), [`../kubernetes/deployment.yaml`](../kubernetes/deployment.yaml), [`DATABASE_REPLICATION.md`](./DATABASE_REPLICATION.md)

---

## 1. Why the API is Stateless

The API holds no per-instance durable state. Anything that must survive a
restart or be shared across instances lives in a backing service:

| Concern | Storage | Notes |
|---------|---------|-------|
| Application data | PostgreSQL | Single source of truth |
| Cache (L2) & rate limits | Redis | Shared, atomic counters/locks |
| In-memory cache (L1) | Process local | Best-effort; falls back to L2/DB |
| Sessions / auth | JWT | Signed tokens, no server session state |
| Search | Elasticsearch | Stateless queries, index rebuilt from DB |
| Background jobs | Worker process | Uses Redis locks to avoid duplicate work |

Consequences:

- Any instance can serve any request; there is **no sticky-session requirement**.
- Instances can be added or removed at any time (rolling deploys, autoscaling).
- L1 caches diverge per instance; treat them as a latency optimization only.
- Distributed locks (`utils.CacheManager.GetOrLoadDistributed`) prevent
  duplicated cache regeneration across instances.

> ⚠️ Do not store request-scoped state in package-level variables. That would
> break statelessness and cause correctness bugs under multiple instances.

---

## 2. Scaling the API Tier

### 2.1 Vertical vs horizontal

Start vertically (more CPU/RAM per instance) until the instance is well
utilized, then scale horizontally. Horizontal scaling is limited by shared
dependencies — chiefly PostgreSQL and Redis, not the API itself.

### 2.2 Sizing a single instance

| Resource | Request | Limit | Notes |
|----------|---------|-------|-------|
| CPU | 250m | 1000m | API is I/O bound; ~1 vCPU handles hundreds of RPS |
| Memory | 512Mi | 1Gi | L1 cache is bounded (`CACHE_L1_MAX_SIZE_MB`) |
| Connections | — | — | GOMAXPROCS follows CPU limit |

### 2.3 Autoscaling

Scale on CPU utilization and request concurrency, with a floor of 2 replicas
for availability. See [`../kubernetes/deployment.yaml`](../kubernetes/deployment.yaml)
for an example HorizontalPodAutoscaler.

```yaml
minReplicas: 2
maxReplicas: 10
metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 65
```

Rules of thumb:

- Scale **out** when p95 CPU > 65% for 5 minutes or request queueing grows.
- Scale **in** slowly (300s stabilization) to avoid flapping.
- Keep `terminationGracePeriodSeconds` long enough to drain in-flight requests.

### 2.4 Graceful shutdown

On SIGTERM the API must finish in-flight requests before exiting. Ensure the
load balancer deregisters the instance first (readiness endpoint flips to
unhealthy) and allow a drain window before the process is killed. Never send
SIGKILL immediately.

---

## 3. Connection Pooling at Scale

Every API instance maintains its own database connection pool. **Total
connections = replicas × pool size**, which is the most common cause of
database overload after scaling out.

### 3.1 Pool sizing formula

```
total_connections = replicas × DB_MAX_OPEN_CONNS
```

Keep `total_connections` below the PostgreSQL `max_connections` minus headroom
for migrations, admin access, and monitoring. PostgreSQL forks a process per
connection, so fewer, longer-lived connections are better.

| Replicas | `DB_MAX_OPEN_CONNS` | Total | Fits `max_connections=200`? |
|----------|---------------------|-------|-----------------------------|
| 2 | 25 | 50 | Yes |
| 4 | 25 | 100 | Yes |
| 8 | 25 | 200 | No — use a pooler |
| 8 | 10 | 80 | Yes (with `DB_MAX_IDLE_CONNS=5`) |

### 3.2 Recommendations

- Set `DB_MAX_OPEN_CONNS` so that `replicas × max_open < max_connections × 0.8`.
- Set `DB_MAX_IDLE_CONNS` to roughly 40–50% of `DB_MAX_OPEN_CONNS`.
- Set `DB_CONN_MAX_LIFETIME` to 30m and `DB_CONN_MAX_IDLE_TIME` to 5m so
  connections cycle and load-balance across primaries/replicas.
- Use **read replicas** for read-heavy endpoints to offload the primary
  (see [`DATABASE_REPLICATION.md`](./DATABASE_REPLICATION.md)).
- For large fleets, deploy **PgBouncer** in transaction pooling mode in front
  of PostgreSQL and lower per-instance pool sizes.

### 3.3 PgBouncer pattern

```
API replicas ──► PgBouncer (transaction pooling) ──► PostgreSQL primary/read replicas
```

With PgBouncer, per-instance pools can be small (e.g. 5–10) while the pooler
maintains a capped set of server connections. Avoid server-side prepared
statements spanning transactions when transaction pooling is enabled.

---

## 4. Caching Under Scale

- **L1 (in-process)** scales linearly but is cold after each deploy and per
  instance. Keep it small and short-TTL.
- **L2 (Redis)** is shared and should absorb most read traffic for hot keys.
  Watch Redis latency/connection count as you scale out.
- **Cache stampede**: distributed locks coalesce regeneration across instances.
- **Rate limiting** uses Redis shared counters so limits are enforced globally,
  not per instance. Verify `RATE_LIMIT_*` sizing as replica count grows.

---

## 5. Load Balancing

Requests must be distributed across instances with health-based routing.
Details, including NGINX and cloud LB configuration, are in
[`LOAD_BALANCER_CONFIG.md`](./LOAD_BALANCER_CONFIG.md).

Key points:

- Use `least_conn` or round-robin; **do not** enable sticky sessions.
- Health check `/health` (liveness) and `/ready` (readiness) separately.
- Terminate TLS at the load balancer; forward `X-Forwarded-*` headers.
- Enable HTTP/2 and keep-alive between LB and API.
- Configure sensible timeouts (`proxy_read_timeout` > p99 latency).

---

## 6. Capacity Planning

### 6.1 Workload assumptions

| Metric | Value |
|--------|-------|
| Average request cost | ~15 ms CPU-light, mostly I/O wait |
| Target p95 latency | < 200 ms (see [`SLA_SLO_DEFINITIONS.md`](./SLA_SLO_DEFINITIONS.md)) |
| Read:write ratio | ~9:1 (reads can go to replicas) |
| Redis op per request | 1–3 |

### 6.2 Capacity estimate

Use this small model to estimate replicas:

```
replicas = ceil( (peak_RPS × avg_latency_seconds) / target_utilization )
```

Example: peak 1,000 RPS, 15 ms per request, target 65% utilization:

```
replicas = ceil( (1000 × 0.015) / 0.65 ) = ceil(23) → ~24 CPU-cores worth
```

At 1 vCPU per instance's share, that is roughly **8–12 instances** (each with a
1 vCPU limit, 65% target). Validate with a load test before committing.

| Traffic tier | Peak RPS | API replicas | DB pool/replica | Notes |
|--------------|----------|--------------|-----------------|-------|
| Small | 100 | 2 | 25 | Single primary |
| Medium | 500 | 4 | 20 | Primary + 1 replica |
| Large | 2,000 | 10 | 10 + PgBouncer | Primary + 2–3 replicas |
| XL | 10,000 | 20+ | PgBouncer + sharding/caching | Read replicas, CDN, queue offload |

### 6.3 What breaks first

1. **Database connections** — mitigated by pooler/replica sizing (§3).
2. **Database CPU / queries** — add read replicas, index, or cache.
3. **Redis throughput** — scale Redis (cluster/sentinel) or reduce ops.
4. **API CPU** — add replicas.
5. **Load balancer bandwidth** — scale the LB tier / add nodes.

### 6.4 Load testing

Load test before each major scaling change. Use representative read/write
mixes and measure p50/p95/p99 plus DB pool wait metrics
(`db_pool_wait_count`, `db_pool_wait_duration_seconds`).

---

## 7. Rollout Checklist

- [ ] Stateless: no package-level mutable request state.
- [ ] Externalized config via environment variables (no baked-in hosts).
- [ ] `DATABASE_URL` and `REDIS_URL` point at shared services.
- [ ] Connection pools sized for the target replica count (§3).
- [ ] `/health` and `/ready` endpoints implemented and wired to probes.
- [ ] Graceful shutdown drains in-flight requests on SIGTERM.
- [ ] Load balancer uses health checks and no sticky sessions.
- [ ] Autoscaler min/max and stabilization windows configured.
- [ ] Metrics/alerts in place for p99 latency, error rate, pool saturation.
- [ ] Load test executed at expected peak with passing SLOs.
