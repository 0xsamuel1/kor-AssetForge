package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestL1Cache_LRUEvictionByEntryCount(t *testing.T) {
	// Cache max 3 entries
	c := NewL1Cache(3, 10*1024)

	c.Set("k1", []byte("val1"), 0)
	c.Set("k2", []byte("val2"), 0)
	c.Set("k3", []byte("val3"), 0)
	assert.Equal(t, 3, c.Len())

	// Access k1 so k2 becomes the least recently used
	val, ok := c.Get("k1")
	assert.True(t, ok)
	assert.Equal(t, "val1", string(val))

	// Insert k4 -> should evict k2
	c.Set("k4", []byte("val4"), 0)
	assert.Equal(t, 3, c.Len())

	_, ok = c.Get("k2")
	assert.False(t, ok, "k2 should have been evicted")

	_, ok = c.Get("k1")
	assert.True(t, ok, "k1 should still exist")
	_, ok = c.Get("k3")
	assert.True(t, ok, "k3 should still exist")
	_, ok = c.Get("k4")
	assert.True(t, ok, "k4 should still exist")

	stats := c.Stats()
	assert.Equal(t, int64(1), stats.TotalEvictions.Load())
}

func TestL1Cache_LRUEvictionByByteLimit(t *testing.T) {
	// Max byte limit: each entry with EstimateEntrySize("k", val) is len("k") + len(val) + 64 bytes
	// A 100-byte value takes ~166 bytes. Limit to 400 bytes so max ~2 items.
	c := NewL1Cache(100, 400)

	c.Set("key1", make([]byte, 100), 0)
	c.Set("key2", make([]byte, 100), 0)
	assert.Equal(t, 2, c.Len())

	// Adding a third 100-byte item exceeds 400 bytes, forcing eviction of key1
	c.Set("key3", make([]byte, 100), 0)
	assert.Equal(t, 2, c.Len())

	_, ok := c.Get("key1")
	assert.False(t, ok, "key1 should be evicted due to size bounds")

	_, ok = c.Get("key2")
	assert.True(t, ok)
	_, ok = c.Get("key3")
	assert.True(t, ok)
}

func TestL1Cache_OversizedItemIgnored(t *testing.T) {
	c := NewL1Cache(10, 200)

	// An entry that alone exceeds max size is not cached
	c.Set("big", make([]byte, 300), 0)
	assert.Equal(t, 0, c.Len())
	_, ok := c.Get("big")
	assert.False(t, ok)
}

func TestL1Cache_Expiration(t *testing.T) {
	c := NewL1Cache(10, 1024)

	c.Set("expiring", []byte("temp"), 30*time.Millisecond)
	val, ok := c.Get("expiring")
	assert.True(t, ok)
	assert.Equal(t, "temp", string(val))

	time.Sleep(50 * time.Millisecond)

	_, ok = c.Get("expiring")
	assert.False(t, ok, "item should have expired")
}

func TestL1Cache_ConcurrentAccess(t *testing.T) {
	c := NewL1Cache(100, 1024*1024)
	var wg sync.WaitGroup
	workers := 10
	iterations := 200

	for i := 0; i < workers; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				k := fmt.Sprintf("k-%d-%d", workerID, j%20)
				c.Set(k, []byte("data"), 1*time.Minute)
				c.Get(k)
				if j%5 == 0 {
					c.Delete(k)
				}
			}
		}()
	}

	wg.Wait()
	assert.True(t, c.Len() <= 100)
}
