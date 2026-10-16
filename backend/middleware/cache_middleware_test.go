package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/kor-assetforge/services/cache"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestResilientCacheMiddleware_HitAndMiss(t *testing.T) {
	// Cache without Redis (in-memory L1 only)
	twoTier := cache.NewTwoTierCache(100, 1024*1024, nil, 3, time.Second, true)

	r := gin.New()
	callCount := 0
	r.GET("/api/v1/assets", ResilientCacheMiddleware(twoTier, 1*time.Minute, "test", nil), func(c *gin.Context) {
		callCount++
		c.JSON(http.StatusOK, gin.H{"data": "asset-list", "call": callCount})
	})

	// First request: Cache MISS
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	r.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "MISS", w1.Header().Get("X-Cache"))
	assert.Equal(t, "true", w1.Header().Get("X-Cache-Degraded")) // because Redis is nil
	assert.Equal(t, 1, callCount)

	// Second request: Cache HIT (served from L1)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "HIT", w2.Header().Get("X-Cache"))
	assert.Equal(t, 1, callCount) // handler not invoked again
}

func TestResilientCacheMiddleware_GracefulDegradationOnRedisFailure(t *testing.T) {
	// Create cache with circuit breaker configured
	twoTier := cache.NewTwoTierCache(10, 1024, nil, 2, 50*time.Millisecond, true)

	r := gin.New()
	handlerCalled := 0
	r.GET("/items", ResilientCacheMiddleware(twoTier, 1*time.Minute, "item", nil), func(c *gin.Context) {
		handlerCalled++
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Even when Redis is down/nil, it degrades gracefully to DB handler
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/items", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, handlerCalled)
}

func TestResilientCacheHealthHandler(t *testing.T) {
	twoTier := cache.NewTwoTierCache(100, 1024*1024, nil, 3, time.Second, true)

	r := gin.New()
	r.GET("/health/cache", ResilientCacheHealthHandler(twoTier))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health/cache", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "DEGRADED")
	assert.Contains(t, w.Body.String(), "l1_memory")
}

func TestResilientInvalidateOnWrite(t *testing.T) {
	twoTier := cache.NewTwoTierCache(100, 1024*1024, nil, 3, time.Second, true)
	ctx := context.Background()
	_ = twoTier.Set(ctx, "items:1", []byte(`{"id": 1}`), time.Minute)

	r := gin.New()
	r.POST("/items", ResilientInvalidateOnWrite(twoTier, "items"), func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": 1})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/items", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	_, found, _ := twoTier.Get(ctx, "items:1")
	assert.False(t, found)
}
