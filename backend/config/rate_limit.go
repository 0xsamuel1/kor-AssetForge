package config

// WSRateLimitConfig controls the per-connection WebSocket message bucket.
type WSRateLimitConfig struct {
	MessagesPerSecond float64
	Burst             int
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
