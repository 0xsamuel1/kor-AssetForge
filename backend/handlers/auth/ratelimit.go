package auth

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/yourusername/kor-assetforge/apperrors"
)

// AuthRateLimiter manages per-IP rate limiters for auth endpoints
type AuthRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	burst    int
}

// RegistrationRateLimiter is deliberately separate from the general auth
// limiter so registration cannot inherit login's burst budget.
type RegistrationRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	limit    rate.Limit
	burst    int
}

func NewRegistrationRateLimiter(maxPerWindow int, window time.Duration) *RegistrationRateLimiter {
	if maxPerWindow < 1 {
		maxPerWindow = 5
	}
	if window <= 0 {
		window = time.Hour
	}
	return &RegistrationRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		limit:    rate.Limit(float64(maxPerWindow) / window.Seconds()),
		burst:    maxPerWindow,
	}
}

func (rl *RegistrationRateLimiter) RegisterRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		rl.mu.Lock()
		limiter, ok := rl.limiters[c.ClientIP()]
		if !ok {
			limiter = rate.NewLimiter(rl.limit, rl.burst)
			rl.limiters[c.ClientIP()] = limiter
		}
		allowed := limiter.Allow()
		rl.mu.Unlock()
		if !allowed {
			apperrors.AbortWithError(c, apperrors.NewTooManyRequestsError("Registration limit exceeded. Please try again later."))
			return
		}
		c.Next()
	}
}

// NewAuthRateLimiter creates a new auth rate limiter
func NewAuthRateLimiter(r rate.Limit, burst int) *AuthRateLimiter {
	return &AuthRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		burst:    burst,
	}
}

func (rl *AuthRateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	l, ok := rl.limiters[key]
	if !ok {
		l = rate.NewLimiter(rl.r, rl.burst)
		rl.limiters[key] = l
	}
	return l
}

func (rl *AuthRateLimiter) limit(prefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.getLimiter(prefix + ":" + c.ClientIP()).Allow() {
			apperrors.AbortWithError(c, apperrors.NewTooManyRequestsError("Too many requests. Please try again later."))
			return
		}
		c.Next()
	}
}

// GeneralAuthRateLimit applies to all auth endpoints
func (rl *AuthRateLimiter) GeneralAuthRateLimit() gin.HandlerFunc {
	return rl.limit("auth")
}

// LoginRateLimit limits login attempts per IP
func (rl *AuthRateLimiter) LoginRateLimit() gin.HandlerFunc {
	return rl.limit("login")
}

// RegisterRateLimit limits registration attempts per IP
func (rl *AuthRateLimiter) RegisterRateLimit() gin.HandlerFunc {
	return rl.limit("register")
}

// PasswordResetRateLimit limits password reset attempts per IP
func (rl *AuthRateLimiter) PasswordResetRateLimit() gin.HandlerFunc {
	return rl.limit("password_reset")
}

// EmailVerificationRateLimit limits email verification attempts per IP
func (rl *AuthRateLimiter) EmailVerificationRateLimit() gin.HandlerFunc {
	return rl.limit("email_verify")
}
