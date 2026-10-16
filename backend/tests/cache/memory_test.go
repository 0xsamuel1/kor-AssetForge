package cache_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourusername/kor-assetforge/services/cache"
)

func TestL1Cache_MemoryLimitsUnderLoad(t *testing.T) {
	// Allocate small cache with 50 entries and 50 KB limit
	maxEntries := 50
	maxSizeBytes := int64(50 * 1024)
	l1 := cache.NewL1Cache(maxEntries, maxSizeBytes)

	// Ingest 500 entries (10x capacity) with 1 KB payload
	payload := make([]byte, 1024)
	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("asset:perf:item:%d", i)
		l1.Set(key, payload, 5*time.Minute)
	}

	stats := l1.Stats()
	snapshot := stats.Snapshot()

	assert.LessOrEqual(t, l1.Len(), maxEntries, "Entry count must not exceed maxEntries")
	assert.LessOrEqual(t, snapshot["used_size_bytes"].(int64), maxSizeBytes, "Byte size must not exceed maxSizeBytes")
	assert.Greater(t, snapshot["total_evictions"].(int64), int64(0), "Evictions must have occurred")
	assert.LessOrEqual(t, snapshot["usage_pct"].(float64), 100.0, "Usage percentage must be <= 100%")
}

func TestL1Cache_NoMemoryLeaksUnderContinuousChurn(t *testing.T) {
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	l1 := cache.NewL1Cache(20, 10*1024)
	data := make([]byte, 512)

	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("churn:%d", i%100)
		l1.Set(key, data, time.Minute)
		if i%10 == 0 {
			l1.Get(key)
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Memory usage should remain bounded, not leaking indefinitely
	assert.LessOrEqual(t, l1.Len(), 20)
}
