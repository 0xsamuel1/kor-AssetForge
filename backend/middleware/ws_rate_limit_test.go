package middleware

import "testing"

func TestWebSocketRateLimiterHonorsBurst(t *testing.T) {
	limiter := NewWebSocketRateLimiter(1, 2)
	if !limiter.Allow() || !limiter.Allow() {
		t.Fatal("initial burst should be allowed")
	}
	if limiter.Allow() {
		t.Fatal("message beyond burst should be rejected")
	}
}
