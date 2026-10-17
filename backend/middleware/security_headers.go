package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/config"
)

// SecurityHeaders adds headers that prevent common browser-side attacks.
func SecurityHeaders(cfg config.SecurityHeadersConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", cfg.ContentSecurityPolicy)
		c.Header("Strict-Transport-Security", "max-age="+cfg.HSTSMaxAge+"; includeSubDomains")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Cross-Origin-Opener-Policy", "same-origin")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")
		c.Next()
	}
}
