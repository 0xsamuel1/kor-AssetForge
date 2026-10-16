package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	threshold := 3
	timeout := 100 * time.Millisecond
	cb := NewCircuitBreaker(threshold, timeout)

	// Initial state is CLOSED
	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.AllowExecution())

	// Failures below threshold do not open circuit
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.AllowExecution())

	// Reaching threshold trips to OPEN
	cb.RecordFailure()
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.AllowExecution())

	// Still open before timeout
	time.Sleep(20 * time.Millisecond)
	assert.False(t, cb.AllowExecution())

	// Wait for open timeout to elapse
	time.Sleep(100 * time.Millisecond)

	// Now transitions to HALF_OPEN to probe downstream
	assert.True(t, cb.AllowExecution())
	assert.Equal(t, StateHalfOpen, cb.State())

	// Success closes circuit
	cb.RecordSuccess()
	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.AllowExecution())
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, StateOpen, cb.State())

	// Wait for cool-off
	time.Sleep(60 * time.Millisecond)
	assert.True(t, cb.AllowExecution())
	assert.Equal(t, StateHalfOpen, cb.State())

	// Failure in HALF_OPEN immediately re-opens circuit
	cb.RecordFailure()
	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(2, 10*time.Second)
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, StateOpen, cb.State())

	cb.Reset()
	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.AllowExecution())
}
