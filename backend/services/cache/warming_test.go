package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockCacheService struct {
	data atomic.Pointer[map[string][]byte]
}

func newMockCache() *mockCacheService {
	m := &mockCacheService{}
	initial := make(map[string][]byte)
	m.data.Store(&initial)
	return m
}

func (m *mockCacheService) Get(ctx context.Context, key string) ([]byte, bool, error) {
	d := *m.data.Load()
	val, ok := d[key]
	return val, ok, nil
}

func (m *mockCacheService) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	for {
		oldMap := m.data.Load()
		newMap := make(map[string][]byte)
		for k, v := range *oldMap {
			newMap[k] = v
		}
		newMap[key] = val
		if m.data.CompareAndSwap(oldMap, &newMap) {
			break
		}
	}
	return nil
}

func (m *mockCacheService) Delete(ctx context.Context, keys ...string) error {
	return nil
}

func (m *mockCacheService) InvalidatePrefix(ctx context.Context, prefix string) error {
	return nil
}

func (m *mockCacheService) Health(ctx context.Context) CacheHealth {
	return CacheHealth{Status: StatusHealthy}
}

func (m *mockCacheService) L1Stats() *MemoryStats {
	return &MemoryStats{}
}

func TestCacheWarmer_SyncAndAsync(t *testing.T) {
	mock := newMockCache()
	warmer := NewCacheWarmer(mock, 2)

	targets := []WarmTarget{
		{
			Key: "asset:popular:1",
			TTL: 10 * time.Minute,
			Loader: func(ctx context.Context) (interface{}, error) {
				return map[string]string{"name": "Alpha Asset"}, nil
			},
		},
		{
			Key: "asset:popular:2",
			TTL: 10 * time.Minute,
			Loader: func(ctx context.Context) (interface{}, error) {
				return map[string]string{"name": "Beta Asset"}, nil
			},
		},
		{
			Key: "asset:failed",
			TTL: 10 * time.Minute,
			Loader: func(ctx context.Context) (interface{}, error) {
				return nil, errors.New("db error")
			},
		},
	}

	stats := warmer.WarmSync(context.Background(), targets)
	assert.Equal(t, 3, stats.TotalTargets)
	assert.Equal(t, 2, stats.WarmedKeys)
	assert.Equal(t, 1, stats.FailedKeys)

	// Verify entries were stored
	val, found, _ := mock.Get(context.Background(), "asset:popular:1")
	assert.True(t, found)
	assert.Contains(t, string(val), "Alpha Asset")

	// Running again should skip already warmed keys
	repeatStats := warmer.WarmSync(context.Background(), targets)
	assert.Equal(t, 2, repeatStats.SkippedKeys)
}
