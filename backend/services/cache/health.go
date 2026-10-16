package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// HealthStatus represents the health of the cache subsystem.
type HealthStatus string

const (
	StatusHealthy  HealthStatus = "HEALTHY"
	StatusDegraded HealthStatus = "DEGRADED"
	StatusDown     HealthStatus = "DOWN"
)

// CacheHealth reports status and diagnostics for the cache subsystem.
type CacheHealth struct {
	Status       HealthStatus `json:"status"`
	RedisState   string       `json:"redis_state"`
	CircuitState string       `json:"circuit_state"`
	PingDuration string       `json:"ping_duration,omitempty"`
	ErrorMessage string       `json:"error_message,omitempty"`
	Timestamp    time.Time    `json:"timestamp"`
}

// CheckRedisHealth performs an active health probe against Redis and checks circuit state.
func CheckRedisHealth(ctx context.Context, client *redis.Client, cb *CircuitBreaker) CacheHealth {
	now := time.Now()
	res := CacheHealth{
		Timestamp: now,
	}

	if cb != nil {
		res.CircuitState = string(cb.State())
	} else {
		res.CircuitState = string(StateClosed)
	}

	if client == nil {
		res.Status = StatusDegraded
		res.RedisState = "NOT_CONFIGURED"
		res.ErrorMessage = "Redis client is nil; running in in-memory degraded mode"
		return res
	}

	if cb != nil && cb.State() == StateOpen {
		res.Status = StatusDegraded
		res.RedisState = "CIRCUIT_OPEN"
		res.ErrorMessage = "Redis circuit breaker is open; traffic degrading to database"
		return res
	}

	start := time.Now()
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := client.Ping(pingCtx).Result()
	duration := time.Since(start)
	res.PingDuration = duration.String()

	if err != nil {
		if cb != nil {
			cb.RecordFailure()
		}
		res.Status = StatusDegraded
		res.RedisState = "UNREACHABLE"
		res.ErrorMessage = err.Error()
		return res
	}

	if cb != nil {
		cb.RecordSuccess()
	}
	res.Status = StatusHealthy
	res.RedisState = "CONNECTED"
	return res
}
