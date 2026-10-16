package cache

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrCacheUnavailable = errors.New("cache unavailable")
)

// CacheService defines the contract for two-tier resilient caching.
type CacheService interface {
	Get(ctx context.Context, key string) (val []byte, found bool, err error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	InvalidatePrefix(ctx context.Context, prefix string) error
	Health(ctx context.Context) CacheHealth
	L1Stats() *MemoryStats
}

// TwoTierCache provides a resilient L1 in-memory + L2 Redis cache with circuit breaker.
type TwoTierCache struct {
	l1             *L1Cache
	redis          *redis.Client
	circuitBreaker *CircuitBreaker
	fallbackMode   bool
	mu             sync.RWMutex
}

// NewTwoTierCache constructs a new TwoTierCache.
func NewTwoTierCache(
	maxL1Entries int,
	maxL1Bytes int64,
	redisClient *redis.Client,
	cbThreshold int,
	cbTimeout time.Duration,
	fallbackEnabled bool,
) *TwoTierCache {
	return &TwoTierCache{
		l1:             NewL1Cache(maxL1Entries, maxL1Bytes),
		redis:          redisClient,
		circuitBreaker: NewCircuitBreaker(cbThreshold, cbTimeout),
		fallbackMode:   fallbackEnabled,
	}
}

// Get implements cache-aside read: L1 -> L2 (with circuit breaker) -> miss.
// When Redis fails or circuit is open, degrades gracefully without returning errors.
func (c *TwoTierCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	// Tier 1: In-memory L1 cache
	if val, ok := c.l1.Get(key); ok {
		return val, true, nil
	}

	// Tier 2: Redis (if configured and circuit breaker allows)
	if c.redis != nil && c.circuitBreaker.AllowExecution() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()

		val, err := c.redis.Get(timeoutCtx, key).Bytes()
		if err == nil {
			c.circuitBreaker.RecordSuccess()
			// Backfill L1 with a defensive TTL (e.g., 1 minute)
			c.l1.Set(key, val, 1*time.Minute)
			return val, true, nil
		}

		if errors.Is(err, redis.Nil) {
			// Normal cache miss
			return nil, false, nil
		}

		// Downstream failure: trip breaker and log warning
		c.circuitBreaker.RecordFailure()
		log.Printf("TwoTierCache: Redis Get failed for key %s (circuit=%s): %v",
			key, c.circuitBreaker.State(), err)

		// If fallback is enabled, gracefully degrade to database
		if c.fallbackMode {
			return nil, false, nil
		}
		return nil, false, err
	}

	return nil, false, nil
}

// Set implements write-through to both L1 and L2.
func (c *TwoTierCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	// Always write to L1
	c.l1.Set(key, val, ttl)

	// Write to Redis if allowed
	if c.redis != nil && c.circuitBreaker.AllowExecution() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		if err := c.redis.Set(timeoutCtx, key, val, ttl).Err(); err != nil {
			c.circuitBreaker.RecordFailure()
			log.Printf("TwoTierCache: Redis Set failed for key %s (circuit=%s): %v",
				key, c.circuitBreaker.State(), err)
			if !c.fallbackMode {
				return err
			}
		} else {
			c.circuitBreaker.RecordSuccess()
		}
	}

	return nil
}

// Delete invalidates keys in both L1 and L2.
func (c *TwoTierCache) Delete(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		c.l1.Delete(k)
	}

	if c.redis != nil && c.circuitBreaker.AllowExecution() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		if err := c.redis.Del(timeoutCtx, keys...).Err(); err != nil {
			c.circuitBreaker.RecordFailure()
			log.Printf("TwoTierCache: Redis Del failed: %v", err)
			if !c.fallbackMode {
				return err
			}
		} else {
			c.circuitBreaker.RecordSuccess()
		}
	}
	return nil
}

// InvalidatePrefix clears matching keys from L1 and Redis.
func (c *TwoTierCache) InvalidatePrefix(ctx context.Context, prefix string) error {
	c.l1.Clear() // Safe fast invalidation for L1

	if c.redis != nil && c.circuitBreaker.AllowExecution() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		var cursor uint64
		for {
			var keys []string
			var err error
			keys, cursor, err = c.redis.Scan(timeoutCtx, cursor, prefix+"*", 100).Result()
			if err != nil {
				c.circuitBreaker.RecordFailure()
				return err
			}
			if len(keys) > 0 {
				_ = c.redis.Del(timeoutCtx, keys...).Err()
			}
			if cursor == 0 {
				break
			}
		}
		c.circuitBreaker.RecordSuccess()
	}
	return nil
}

// Health returns current status of the cache subsystem.
func (c *TwoTierCache) Health(ctx context.Context) CacheHealth {
	return CheckRedisHealth(ctx, c.redis, c.circuitBreaker)
}

// L1Stats returns memory statistics from the L1 cache tier.
func (c *TwoTierCache) L1Stats() *MemoryStats {
	return c.l1.Stats()
}
