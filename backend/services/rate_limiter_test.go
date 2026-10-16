package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEmailRateLimiterAllowsUpToLimit(t *testing.T) {
	rl := NewEmailRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		assert.True(t, rl.Allow("user@example.com"), "attempt %d should be allowed", i+1)
	}
	assert.False(t, rl.Allow("user@example.com"), "4th attempt should be rejected")
}

func TestEmailRateLimiterPerAccount(t *testing.T) {
	rl := NewEmailRateLimiter(1, time.Minute)
	assert.True(t, rl.Allow("alice@example.com"))
	assert.False(t, rl.Allow("alice@example.com"))
	assert.True(t, rl.Allow("bob@example.com"), "different account should not share limit")
}

func TestEmailRateLimiterResetsAfterWindow(t *testing.T) {
	rl := NewEmailRateLimiter(1, 50*time.Millisecond)
	assert.True(t, rl.Allow("user@example.com"))
	assert.False(t, rl.Allow("user@example.com"))
	time.Sleep(100 * time.Millisecond)
	assert.True(t, rl.Allow("user@example.com"), "should be allowed after window expires")
}
