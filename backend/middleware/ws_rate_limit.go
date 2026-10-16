package middleware

import "golang.org/x/time/rate"

// WebSocketRateLimiter applies a token bucket to one WebSocket connection.
type WebSocketRateLimiter struct {
	limiter *rate.Limiter
}

func NewWebSocketRateLimiter(rps float64, burst int) *WebSocketRateLimiter {
	if rps <= 0 {
		rps = 20
	}
	if burst < 1 {
		burst = 40
	}
	return &WebSocketRateLimiter{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
}

// Allow reports whether the next inbound message fits the connection budget.
func (l *WebSocketRateLimiter) Allow() bool {
	return l == nil || l.limiter.Allow()
}
