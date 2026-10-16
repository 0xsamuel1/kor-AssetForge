# AssetForge Caching Strategy & Architecture

## 1. Overview & Architecture

AssetForge implements a multi-tier, resilient caching architecture designed to achieve sub-millisecond response times for read-heavy workloads while maintaining strict memory bounds and high fault tolerance.

```
                           +---------------------------+
                           |     Incoming Request      |
                           +-------------+-------------+
                                         |
                                         v
                           +-------------+-------------+
                           |  L1 Cache (In-Memory LRU) |
                           +-------------+-------------+
                                  |             |
                            (Hit) |             | (Miss)
                                  v             v
                           [Serve Data]  +------+--------------------+
                                         |  Circuit Breaker Check    |
                                         +--------------+-------------+
                                                        |
                                           +------------+------------+
                                  (Closed) |                         | (Open / Degraded)
                                           v                         v
                                +----------+----------+     +--------+--------+
                                |      L2 Redis       |     |  PostgreSQL DB  |
                                +----------+----------+     +--------+--------+
                                     |           |                   |
                               (Hit) |           | (Miss)            |
                                     v           v                   |
                               [Backfill L1]   [Fetch DB] <----------+
                                     |           |
                                     +-----+-----+
                                           |
                                           v
                                      [Serve Data]
```

---

## 2. Tier 1: In-Memory L1 Cache

### 2.1 Sizing and LRU Eviction Policy
To prevent memory leaks and out-of-memory (OOM) conditions:
- **Maximum Entries**: Defaults to `10,000` keys (configurable via `CACHE_L1_MAX_ENTRIES`).
- **Maximum Byte Size**: Defaults to `64 MB` (configurable via `CACHE_L1_MAX_SIZE_MB`).
- **Eviction Algorithm**: True Least Recently Used (LRU) implemented via doubly linked list (`container/list`) and map lookup in O(1) time.
- **Entry Sizing**: Every entry computes its memory footprint using string length, byte array length, and pointer/struct overhead.
- **Eviction Triggers**:
  1. Entry count exceeds `CACHE_L1_MAX_ENTRIES`.
  2. Total memory footprint exceeds `CACHE_L1_MAX_SIZE_MB`.
  3. Per-key TTL expires upon read.
  4. Entries larger than the cache's total maximum capacity are rejected immediately to protect the heap.

### 2.2 Memory Monitoring & Metrics
L1 metrics are exposed atomically:
- `total_entries`: Current number of cached items.
- `used_size_bytes`: Current byte size occupied by cached values.
- `max_size_bytes`: Configured maximum byte limit.
- `usage_pct`: Memory utilization percentage (`used_size_bytes / max_size_bytes * 100`).
- `total_evictions`: Cumulative eviction counter.
- `total_hits` and `total_misses`: Request counters with point-in-time `hit_rate`.

---

## 3. Tier 2: Redis Distributed Cache & Circuit Breaker

### 3.1 Circuit Breaker Implementation
When Redis becomes unresponsive or network partitions occur, the application must not cascade or experience request pile-up. A thread-safe circuit breaker monitors Redis invocations with 3 states:

1. **CLOSED**: Normal operation. All Redis reads and writes execute with a strict 500ms timeout.
2. **OPEN**: Tripped when consecutive Redis errors reach `CACHE_REDIS_CB_THRESHOLD` (default: `5`).
   - All Redis traffic is immediately bypassed.
   - Traffic degrades gracefully to PostgreSQL without returning 500 HTTP errors.
   - Diagnostic headers (`X-Cache-Degraded: true`, `X-Cache-Circuit: OPEN`) are attached to responses.
3. **HALF_OPEN**: After `CACHE_REDIS_CB_TIMEOUT` (default: `30s`), a probe execution is permitted.
   - If the probe succeeds, the circuit resets to `CLOSED`.
   - If the probe fails, the circuit returns to `OPEN` for another timeout duration.

### 3.2 Configuration Environment Variables
```env
CACHE_L1_MAX_ENTRIES=10000
CACHE_L1_MAX_SIZE_MB=64
CACHE_L1_DEFAULT_TTL=5m
CACHE_REDIS_FALLBACK_ENABLED=true
CACHE_REDIS_CB_THRESHOLD=5
CACHE_REDIS_CB_TIMEOUT=30s
CACHE_REDIS_HEALTH_INTERVAL=15s
```

---

## 4. Cold-Start Cache Warming Strategy

### 4.1 Cold-Start Problem
When a service pod launches or after a cache flush, all requests experience cache misses, leading to database query spikes (thundering herd).

### 4.2 Non-Blocking Startup Warming
On startup, `backend/main.go` triggers cache warming asynchronously via `CacheWarmer.WarmAsync()` in a dedicated goroutine. This ensures application readiness and HTTP server listening are never blocked by warming operations.

### 4.3 Warming Target Datasets
| Dataset | Key Pattern | TTL | Priority | Description |
|---|---|---|---|---|
| **Recent Assets** | `kor:asset:list:page1` | 10m | 1 (High) | Latest 20 published assets for marketplace landing |
| **Active Listings** | `kor:listings:active:page1` | 5m | 2 | Top active marketplace listings |
| **Asset Categories** | `kor:categories:all` | 30m | 2 | Full taxonomy list of active categories |
| **Asset Tags** | `kor:tags:all` | 30m | 3 | Full taxonomy list of searchable tags |
| **Fee Configurations** | `kor:fee:active` | 1h | 1 (High) | Active platform dynamic fee schedule |

### 4.4 Standalone Warmer CLI & Script
For scheduled warming via Kubernetes CronJobs or post-deployment pipelines:

```bash
# Run manual warming script
./scripts/warm_cache.sh --concurrency 4 --target all

# Or run Go CLI directly
cd backend
go run ./cmd/warm-cache -concurrency=8 -target=assets -timeout=1m
```

---

## 5. Invalidation Policy

- **Write-Through / Invalidate-on-Write**: Whenever an asset, listing, category, or fee configuration is created, updated, or deleted, mutations trigger prefix invalidations across both L1 and Redis via `ResilientInvalidateOnWrite`.
- **Scan Invalidation**: Redis keys are removed using non-blocking cursor-based `SCAN` to prevent blocking the Redis single-threaded event loop.
- **Time-to-Live (TTL)**: All entries have defensive TTLs ensuring eventual consistency even in partitioned network conditions.
