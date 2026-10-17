package config

import (
	"os"
	"strings"
	"time"
)

// ComplianceConfig holds configuration for the automated AML screening
// provider. When Provider is empty (the default, e.g. in local dev and
// tests), the AML service falls back to a local no-op provider -- see
// services.NewAMLScreeningProvider.
type ComplianceConfig struct {
	// AuditRetentionYears is the minimum regulatory retention period.
	AuditRetentionYears int
	// Provider selects the AML screening vendor. Supported: "complyadvantage".
	// Empty means "no real provider configured" -- never silently treat
	// that as "cleared".
	Provider string
	// APIKey authenticates against the provider's API.
	APIKey string
	// BaseURL overrides the provider's default API host (useful for a
	// sandbox/staging environment). Falls back to the provider's
	// production default when empty.
	BaseURL string
	// SanctionsLists are the watchlists to screen against. Defaults to
	// OFAC, UN and EU consolidated lists -- the three named in #377.
	SanctionsLists []string
	// RequestTimeout bounds a single screening call.
	RequestTimeout time.Duration
}

// LoadComplianceConfig reads AML/compliance configuration from environment
// variables.
func LoadComplianceConfig() ComplianceConfig {
	lists := []string{"OFAC", "UN", "EU"}
	if v := os.Getenv("AML_SANCTIONS_LISTS"); v != "" {
		lists = splitAndTrim(v)
	}

	timeout := 10 * time.Second
	if v := os.Getenv("AML_REQUEST_TIMEOUT_SECONDS"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil && d > 0 {
			timeout = d
		}
	}

	return ComplianceConfig{
		AuditRetentionYears: 7,
		Provider:       strings.ToLower(os.Getenv("AML_PROVIDER")),
		APIKey:         os.Getenv("AML_PROVIDER_API_KEY"),
		BaseURL:        os.Getenv("AML_PROVIDER_BASE_URL"),
		SanctionsLists: lists,
		RequestTimeout: timeout,
	}
}

func splitAndTrim(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}
