package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckRedisHealth_NilClient(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	health := CheckRedisHealth(context.Background(), nil, cb)

	assert.Equal(t, StatusDegraded, health.Status)
	assert.Equal(t, "NOT_CONFIGURED", health.RedisState)
	assert.Equal(t, string(StateClosed), health.CircuitState)
}

func TestCheckRedisHealth_CircuitOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Minute)
	cb.RecordFailure()

	health := CheckRedisHealth(context.Background(), nil, cb)
	assert.Equal(t, StatusDegraded, health.Status)
	assert.Equal(t, string(StateOpen), health.CircuitState)
}

func TestTwoTierCache_L1FallbackWhenRedisNil(t *testing.T) {
	twoTier := NewTwoTierCache(10, 1024*1024, nil, 3, time.Second, true)
	ctx := context.Background()

	// Should write to L1 seamlessly
	err := twoTier.Set(ctx, "k1", []byte("val1"), time.Minute)
	assert.NoError(t, err)

	val, found, err := twoTier.Get(ctx, "k1")
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "val1", string(val))

	// Delete from L1
	err = twoTier.Delete(ctx, "k1")
	assert.NoError(t, err)

	_, found, err = twoTier.Get(ctx, "k1")
	assert.NoError(t, err)
	assert.False(t, found)
}
