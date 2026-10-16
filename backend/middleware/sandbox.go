package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/config"
)

const (
	// SandboxHeader is injected into every response when running in sandbox mode
	// so API clients can confirm they are talking to the sandbox.
	SandboxHeader = "X-Sandbox-Mode"

	// SandboxWarningHeader carries a human-readable reminder that no real
	// transactions occur in sandbox mode.
	SandboxWarningHeader = "X-Sandbox-Warning"
)

// SandboxMode is a Gin middleware that:
//  1. Injects sandbox identification headers into every response.
//  2. Returns 503 on endpoints that must not be reached in sandbox mode
//     (e.g. production payment webhooks from real gateways).
//  3. Attaches a "sandbox" flag to the request context so handlers can
//     suppress real external calls (emails, blockchain writes, etc.).
//
// The middleware is a no-op when APP_ENV != "sandbox".
func SandboxMode() gin.HandlerFunc {
	if !config.IsSandbox() {
		// Not in sandbox — pass through without any overhead.
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		// Stamp every response so clients know they are in sandbox.
		c.Header(SandboxHeader, "true")
		c.Header(SandboxWarningHeader, "This is a sandbox environment. No real transactions will occur.")

		// Expose the sandbox flag for downstream handlers.
		c.Set("sandbox_mode", true)

		c.Next()
	}
}

// RequireNotSandbox aborts the request with 503 when the API is running in
// sandbox mode. Use this on endpoints that must never execute against live
// external systems (e.g. real payment gateway webhooks).
func RequireNotSandbox() gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.IsSandbox() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":   "endpoint_unavailable_in_sandbox",
				"message": "This endpoint is not available in the sandbox environment.",
			})
			return
		}
		c.Next()
	}
}

// IsSandboxRequest is a convenience helper that reads the sandbox flag set by
// SandboxMode from the gin context. Returns false if the middleware was not applied.
func IsSandboxRequest(c *gin.Context) bool {
	val, exists := c.Get("sandbox_mode")
	if !exists {
		return false
	}
	sandbox, _ := val.(bool)
	return sandbox
}
