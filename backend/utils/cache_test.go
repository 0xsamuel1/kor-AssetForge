package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestInitRedis(t *testing.T) {
	// Start a mini redis server for testing
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to initialize miniredis: %v", err)
	}
	defer mr.Close()

	// Connect our client
	client, err := InitRedis(mr.Addr())
	if err != nil {
		t.Fatalf("Failed to InitRedis: %v", err)
	}
	defer client.Close()

	// Test Set and Get
	ctx := context.Background()
	testKey := "test:key"
	testVal := "hello world"

	err = client.Set(ctx, testKey, testVal, 5*time.Minute).Err()
	if err != nil {
		t.Errorf("Failed to set key: %v", err)
	}

	val, err := client.Get(ctx, testKey).Result()
	if err != nil {
		t.Errorf("Failed to get key: %v", err)
	}

	if val != testVal {
		t.Errorf("Expected %s, got %s", testVal, val)
	}

	// Test Del
	err = client.Del(ctx, testKey).Err()
	if err != nil {
		t.Errorf("Failed to del key: %v", err)
	}

	// Should not exist
	_, err = client.Get(ctx, testKey).Result()
	if err != redis.Nil {
		t.Errorf("Expected redis.Nil, got %v", err)
	}
}

// TestGetOrLoadDistributed_SingleLoadUnderContention verifies that, when two
// CacheManager instances (simulating two process instances sharing Redis)
// race to populate the same key, the loader runs exactly once and the loser
// observes lock contention rather than also hitting the loader.
func TestGetOrLoadDistributed_SingleLoadUnderContention(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to initialize miniredis: %v", err)
	}
	defer mr.Close()

	newClient := func() *redis.Client {
		c, err := InitRedis(mr.Addr())
		if err != nil {
			t.Fatalf("Failed to InitRedis: %v", err)
		}
		return c
	}

	cmA := NewCacheManager(newClient())
	cmB := NewCacheManager(newClient())

	var loadCount atomic.Int64
	loader := func() (interface{}, error) {
		loadCount.Add(1)
		time.Sleep(100 * time.Millisecond) // simulate slow regeneration
		return map[string]string{"hello": "world"}, nil
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = cmA.GetOrLoadDistributed(ctx, "shared:key", time.Minute, time.Second, loader)
	}()
	go func() {
		defer wg.Done()
		_, _ = cmB.GetOrLoadDistributed(ctx, "shared:key", time.Minute, time.Second, loader)
	}()
	wg.Wait()

	if got := loadCount.Load(); got != 1 {
		t.Errorf("Expected loader to run exactly once across instances, ran %d times", got)
	}
	total := cmA.Metrics.LockContention.Load() + cmB.Metrics.LockContention.Load()
	if total == 0 {
		t.Errorf("Expected at least one instance to record lock contention, got 0")
	}
}

// TestSetWithTagsAndInvalidateTag verifies that keys sharing a tag, even
// across different prefixes, are all invalidated together by InvalidateTag.
func TestSetWithTagsAndInvalidateTag(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to initialize miniredis: %v", err)
	}
	defer mr.Close()

	client, err := InitRedis(mr.Addr())
	if err != nil {
		t.Fatalf("Failed to InitRedis: %v", err)
	}
	cm := NewCacheManager(client)
	ctx := context.Background()

	if err := cm.SetWithTags(ctx, "kor:asset:1", []byte(`{"id":1}`), time.Minute, "asset"); err != nil {
		t.Fatalf("SetWithTags asset failed: %v", err)
	}
	if err := cm.SetWithTags(ctx, "kor:search:results:q=foo", []byte(`[]`), time.Minute, "asset"); err != nil {
		t.Fatalf("SetWithTags search failed: %v", err)
	}

	if err := cm.InvalidateTag(ctx, "asset"); err != nil {
		t.Fatalf("InvalidateTag failed: %v", err)
	}

	if _, err := cm.Get(ctx, "kor:asset:1"); err != ErrCacheMiss {
		t.Errorf("Expected kor:asset:1 to be invalidated, got err=%v", err)
	}
	if _, err := cm.Get(ctx, "kor:search:results:q=foo"); err != ErrCacheMiss {
		t.Errorf("Expected kor:search:results:q=foo to be invalidated, got err=%v", err)
	}
}
