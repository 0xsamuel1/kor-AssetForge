package config

import "time"

// WSRateLimitConfig controls the per-connection WebSocket message bucket.
type WSRateLimitConfig struct {
	MessagesPerSecond float64
	Burst             int
}

// EmailRateLimitConfig controls verification-email rate limiting per account (#331).
type EmailRateLimitConfig struct {
	MaxPerWindow int
	Window       time.Duration
}

// LoadEmailRateLimitConfig reads verification email limits from the environment.
//
// EMAIL_VERIFICATION_MAX_PER_HOUR  max verification emails per window, default 3
// EMAIL_VERIFICATION_WINDOW        rate-limit window, default 1 hour
func LoadEmailRateLimitConfig() EmailRateLimitConfig {
	return EmailRateLimitConfig{
		MaxPerWindow: envInt("EMAIL_VERIFICATION_MAX_PER_HOUR", 3),
		Window:       envDuration("EMAIL_VERIFICATION_WINDOW", time.Hour),
	}
}

// LoadWSRateLimitConfig reads WebSocket message limits from the environment.
//
// WS_RATE_LIMIT_RPS   sustained messages per second, default 20
// WS_RATE_LIMIT_BURST maximum messages allowed immediately, default 40
func LoadWSRateLimitConfig() WSRateLimitConfig {
	return WSRateLimitConfig{
		MessagesPerSecond: envFloat("WS_RATE_LIMIT_RPS", 20),
		Burst:             envInt("WS_RATE_LIMIT_BURST", 40),
	}
}
