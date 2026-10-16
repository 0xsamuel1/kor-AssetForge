package cache

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the current state of the circuit breaker.
type CircuitState string

const (
	StateClosed   CircuitState = "CLOSED"
	StateOpen     CircuitState = "OPEN"
	StateHalfOpen CircuitState = "HALF_OPEN"
)

var ErrCircuitOpen = errors.New("circuit breaker is open; downstream service unavailable")

// CircuitBreaker protects downstream services (like Redis) against cascading failures.
type CircuitBreaker struct {
	mu                sync.RWMutex
	state             CircuitState
	failureThreshold  int
	consecutiveErrors int
	openTimeout       time.Duration
	lastStateChange   time.Time
}

// NewCircuitBreaker creates a new CircuitBreaker.
func NewCircuitBreaker(failureThreshold int, openTimeout time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if openTimeout <= 0 {
		openTimeout = 30 * time.Second
	}

	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		openTimeout:      openTimeout,
		lastStateChange:  time.Now(),
	}
}

// AllowExecution determines if a call to the downstream dependency is permitted.
func (cb *CircuitBreaker) AllowExecution() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if now.Sub(cb.lastStateChange) >= cb.openTimeout {
			cb.state = StateHalfOpen
			cb.lastStateChange = now
			return true // Allow one probe execution
		}
		return false
	case StateHalfOpen:
		// In half-open, only allow probe executions (strict serialization)
		return true
	default:
		return true
	}
}

// RecordSuccess records a successful downstream invocation, resetting failure counts.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveErrors = 0
	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.lastStateChange = time.Now()
	}
}

// RecordFailure records a failed downstream invocation, tripping the circuit if threshold exceeded.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveErrors++
	if cb.state == StateHalfOpen || (cb.state == StateClosed && cb.consecutiveErrors >= cb.failureThreshold) {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the breaker to Closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.consecutiveErrors = 0
	cb.lastStateChange = time.Now()
}
