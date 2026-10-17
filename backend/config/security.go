package config

import "os"

// SecurityHeadersConfig controls browser security headers.
type SecurityHeadersConfig struct {
	ContentSecurityPolicy string
	HSTSMaxAge            string
}

func LoadSecurityHeadersConfig() SecurityHeadersConfig {
	csp := os.Getenv("CONTENT_SECURITY_POLICY")
	if csp == "" {
		csp = "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'"
	}
	maxAge := os.Getenv("HSTS_MAX_AGE")
	if maxAge == "" {
		maxAge = "31536000"
	}
	return SecurityHeadersConfig{ContentSecurityPolicy: csp, HSTSMaxAge: maxAge}
}
