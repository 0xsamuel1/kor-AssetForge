package middleware

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/utils"
)

// responseRecorder captures the response body and status so we can cache it.
type responseRecorder struct {
	gin.ResponseWriter
	body   bytes.Buffer
	status int
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// HTTPCache returns a Gin middleware that caches GET responses in the
// CacheManager. Only 200-status JSON responses are cached.
//
//   - ttl: how long each cached response lives
//   - keyPrefix: prepended to the request path to form the cache key
//   - skip: optional predicate; return true to bypass caching for a request
//   - tags: optional cache tags (see docs/CACHING_STRATEGY.md) allowing this
//     entry to be invalidated together with other related entries via
//     InvalidateOnWriteTags, even when they don't share a key prefix.
func HTTPCache(cm *utils.CacheManager, ttl time.Duration, keyPrefix string, skip func(*gin.Context) bool, tags ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only cache idempotent GET requests.
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}
		if skip != nil && skip(c) {
			c.Next()
			return
		}

		cacheKey := buildCacheKey(keyPrefix, c.Request.URL.RequestURI())

		// Cache-read (cache-aside: serve from cache on hit).
		if cached, err := cm.Get(c.Request.Context(), cacheKey); err == nil {
			c.Header("X-Cache", "HIT")
			c.Data(http.StatusOK, "application/json; charset=utf-8", cached)
			c.Abort()
			return
		}

		// Cache miss: execute the handler and intercept the response.
		c.Header("X-Cache", "MISS")
		rec := &responseRecorder{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = rec

		c.Next()

		// Only cache successful JSON responses.
		if rec.status == http.StatusOK && isJSON(c) && rec.body.Len() > 0 {
			if len(tags) > 0 {
				_ = cm.SetWithTags(c.Request.Context(), cacheKey, rec.body.Bytes(), ttl, tags...)
			} else {
				_ = cm.Set(c.Request.Context(), cacheKey, rec.body.Bytes(), ttl)
			}
		}
	}
}

// InvalidateOnWrite returns a Gin middleware that deletes cache keys whose
// prefixes match the configured patterns whenever a mutating request succeeds.
// Use this on POST/PUT/DELETE routes to keep the cache consistent.
func InvalidateOnWrite(cm *utils.CacheManager, patterns ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Only invalidate when the mutation succeeded (2xx).
		if c.Writer.Status() < 200 || c.Writer.Status() >= 300 {
			return
		}
		for _, p := range patterns {
			_ = cm.Invalidate(c.Request.Context(), p)
		}
	}
}

// InvalidateOnWriteTags returns a Gin middleware that invalidates every cache
// entry associated with the given tags (see docs/CACHING_STRATEGY.md)
// whenever a mutating request succeeds. Use this alongside or instead of
// InvalidateOnWrite when related cached entries don't share a key prefix
// (e.g. an asset update that should also invalidate search results).
func InvalidateOnWriteTags(cm *utils.CacheManager, tags ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Status() < 200 || c.Writer.Status() >= 300 {
			return
		}
		for _, tag := range tags {
			_ = cm.InvalidateTag(c.Request.Context(), tag)
		}
	}
}

// CacheMetricsHandler returns a Gin handler that exposes live cache metrics.
func CacheMetricsHandler(cm *utils.CacheManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := cm.Metrics.Snapshot()
		snap["hit_rate_pct"] = int64(cm.Metrics.HitRate() * 100)
		c.JSON(http.StatusOK, gin.H{
			"cache_metrics": snap,
			"timestamp":     time.Now(),
		})
	}
}

// ---- helpers ----------------------------------------------------------------

func buildCacheKey(prefix, uri string) string {
	if prefix == "" {
		return "cache:" + uri
	}
	return prefix + ":" + uri
}

func isJSON(c *gin.Context) bool {
	ct := c.Writer.Header().Get("Content-Type")
	return strings.Contains(ct, "application/json")
}
