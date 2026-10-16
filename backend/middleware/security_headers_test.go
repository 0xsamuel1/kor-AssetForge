package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/config"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders(config.SecurityHeadersConfig{ContentSecurityPolicy: "default-src 'self'", HSTSMaxAge: "31536000"}))
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	for key, want := range map[string]string{
		"Content-Security-Policy":   "default-src 'self'",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
	} {
		if got := w.Header().Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
