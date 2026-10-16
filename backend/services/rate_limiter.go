package services

import (
	"sync"
	"time"
)

// EmailRateLimiter limits verification emails per account (email), not per IP (#331).
type EmailRateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rateWindow
	limit   int
	ttl     time.Duration
}

type rateWindow struct {
	count int
	reset time.Time
}

// NewEmailRateLimiter creates an EmailRateLimiter allowing `limit` emails per `ttl` window per account.
func NewEmailRateLimiter(limit int, ttl time.Duration) *EmailRateLimiter {
	return &EmailRateLimiter{
		windows: make(map[string]*rateWindow),
		limit:   limit,
		ttl:     ttl,
	}
}

// Allow reports whether another verification email may be sent to the account.
func (rl *EmailRateLimiter) Allow(email string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	w, ok := rl.windows[email]
	if !ok || now.After(w.reset) {
		rl.windows[email] = &rateWindow{count: 1, reset: now.Add(rl.ttl)}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}
