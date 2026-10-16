# Caching Strategy

This document describes how caching works in the backend and how related cache
entries are kept consistent.

## Architecture

`backend/utils/cache_manager.go` implements a two-tier `CacheManager`:

- **L1**: in-process (`sync.Map`), fast but not shared across instances.
- **L2**: Redis, shared across all instances.

Reads are cache-aside (`Get`: L1 → L2 → miss). Writes are write-through
(`Set`: write L1 and L2 together).

## Key prefixes

Cache keys are built from a prefix plus the request path, e.g. `kor:asset:...`
for asset routes and `kor:oracle:...` for oracle price routes. Prefix-based
invalidation (`CacheManager.Invalidate`, wired via the `InvalidateOnWrite`
middleware) deletes every key matching a prefix pattern on a successful write.

## Stampede protection

Within a single process, `GetOrLoad` uses an in-memory `singleflight` group so
concurrent requests for the same missing key only call the loader once.

Across multiple horizontally-scaled instances, `GetOrLoadDistributed` adds a
Redis `SETNX`-based lock (`lock:<key>`) on top of this: only the instance that
acquires the lock regenerates the value; other instances (and other
goroutines) wait up to a configurable timeout (`CACHE_LOCK_WAIT_TIMEOUT`, see
`config.CacheLockWaitTimeout`) and then fall back to a stale L1 entry if one
exists, or a cache miss. Lock contention is tracked in
`CacheManager.Metrics.LockContention` and exposed via the existing
`/cache/metrics`-style `CacheMetricsHandler`.

## Tag-based invalidation

Prefix-based invalidation only works when related cache entries share a key
prefix. It does not cover cache entries that depend on the same underlying
data but live under a different prefix — e.g. an asset's own cache entry
(`kor:asset:...`) and a cached search-results page that includes that asset
(a different prefix entirely).

`SetWithTags(ctx, key, value, ttl, tags...)` additionally records `key` under
each tag's Redis set (`tag:<tag>`). `InvalidateTag(ctx, tag)` deletes every
key recorded under that tag, across both cache tiers, then clears the tag's
own index.

`middleware.HTTPCache` accepts optional tags (as a trailing variadic
argument) to tag the response it caches; `middleware.InvalidateOnWriteTags`
invalidates by tag on a successful mutating request. Today, the `"asset"` tag
is applied to every asset-derived cache entry (`/assets`, `/assets/:id`,
`/assets/:id/metadata`) and invalidated on every asset-mutating route
(tokenize, metadata update, batch execute, marketplace list/transfer). The
`/search/*` routes are not currently cached at all, so there is nothing cached
there to go stale — if search result caching is added in the future, tagging
those entries `"asset"` as well will keep them consistent with asset writes
using the same mechanism.
