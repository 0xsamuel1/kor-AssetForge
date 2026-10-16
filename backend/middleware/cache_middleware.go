package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/services/cache"
)

// ResilientCacheMiddleware provides HTTP response caching with graceful degradation.
// When Redis is unreachable or circuit breaker trips, it falls back seamlessly to
// in-memory L1 or serves directly from the underlying database handler.
func ResilientCacheMiddleware(
	cacheService cache.CacheService,
	ttl time.Duration,
	keyPrefix string,
	skip func(*gin.Context) bool,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only cache idempotent GET requests
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}

		if skip != nil && skip(c) {
			c.Next()
			return
		}

		cacheKey := buildCacheKey(keyPrefix, c.Request.URL.RequestURI())

		// Check cache health for diagnostics header
		health := cacheService.Health(c.Request.Context())
		if health.Status == cache.StatusDegraded {
			c.Header("X-Cache-Degraded", "true")
			c.Header("X-Cache-Circuit", health.CircuitState)
		}

		// Attempt cache-aside read
		val, found, err := cacheService.Get(c.Request.Context(), cacheKey)
		if err == nil && found && len(val) > 0 {
			c.Header("X-Cache", "HIT")
			c.Data(http.StatusOK, "application/json; charset=utf-8", val)
			c.Abort()
			return
		}

		// Cache miss or graceful degradation: proceed to database handler
		c.Header("X-Cache", "MISS")
		rec := &responseRecorder{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = rec

		c.Next()

		// Write-through to cache on successful JSON responses
		if rec.status == http.StatusOK && isJSON(c) && rec.body.Len() > 0 {
			_ = cacheService.Set(c.Request.Context(), cacheKey, rec.body.Bytes(), ttl)
		}
	}
}

// ResilientInvalidateOnWrite invalidates cache keys by prefix when mutating requests succeed.
func ResilientInvalidateOnWrite(cacheService cache.CacheService, prefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Only invalidate on successful mutations (2xx)
		if c.Writer.Status() < 200 || c.Writer.Status() >= 300 {
			return
		}

		for _, prefix := range prefixes {
			_ = cacheService.InvalidatePrefix(c.Request.Context(), prefix)
		}
	}
}

// ResilientCacheHealthHandler exposes cache status, Redis connectivity, and L1 memory metrics.
func ResilientCacheHealthHandler(cacheService cache.CacheService) gin.HandlerFunc {
	return func(c *gin.Context) {
		health := cacheService.Health(c.Request.Context())
		l1Stats := cacheService.L1Stats()

		statusCode := http.StatusOK
		if health.Status == cache.StatusDown {
			statusCode = http.StatusServiceUnavailable
		}

		c.JSON(statusCode, gin.H{
			"cache_health": health,
			"l1_memory":    l1Stats.Snapshot(),
			"timestamp":    time.Now(),
		})
	}
}
