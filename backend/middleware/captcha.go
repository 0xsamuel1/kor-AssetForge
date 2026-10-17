package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type captchaAttempt struct {
	count   int
	resetAt time.Time
}

// CaptchaAfter requires a non-empty provider token once an IP crosses the
// configured threshold. The provider can validate the token before the
// request reaches this middleware; this keeps the API provider-agnostic.
func CaptchaAfter(threshold int, window time.Duration) gin.HandlerFunc {
	if threshold < 1 {
		threshold = 3
	}
	if window <= 0 {
		window = time.Hour
	}
	var mu sync.Mutex
	attempts := make(map[string]captchaAttempt)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		mu.Lock()
		attempt := attempts[ip]
		now := time.Now()
		if attempt.resetAt.Before(now) {
			attempt = captchaAttempt{resetAt: now.Add(window)}
		}
		requiresCaptcha := attempt.count >= threshold
		attempt.count++
		attempts[ip] = attempt
		mu.Unlock()

		if requiresCaptcha && strings.TrimSpace(c.GetHeader("X-Captcha-Token")) == "" {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "captcha required",
			})
			return
		}
		c.Next()
	}
}
