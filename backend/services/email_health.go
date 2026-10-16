package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/yourusername/kor-assetforge/config"
)

// ProviderHealthStatus denotes the operational health status of an email provider.
type ProviderHealthStatus string

const (
	HealthStatusHealthy   ProviderHealthStatus = "healthy"
	HealthStatusDegraded  ProviderHealthStatus = "degraded"
	HealthStatusUnhealthy ProviderHealthStatus = "unhealthy"
)

// ProviderHealth captures operational metrics and health status for a single email provider.
type ProviderHealth struct {
	Provider             config.EmailProvider `json:"provider"`
	Status               ProviderHealthStatus `json:"status"`
	ConsecutiveFailures  int                  `json:"consecutive_failures"`
	ConsecutiveSuccesses int                  `json:"consecutive_successes"`
	TotalSent            int64                `json:"total_sent"`
	TotalFailures        int64                `json:"total_failures"`
	LastFailure          *time.Time           `json:"last_failure,omitempty"`
	LastError            string               `json:"last_error,omitempty"`
	LastSuccess          *time.Time           `json:"last_success,omitempty"`
	LastLatencyMs        int64                `json:"last_latency_ms,omitempty"`
}

// EmailHealthChecker monitors and reports provider health.
type EmailHealthChecker interface {
	RecordSuccess(provider config.EmailProvider, latency time.Duration)
	RecordFailure(provider config.EmailProvider, err error)
	IsHealthy(provider config.EmailProvider) bool
	GetStatus(provider config.EmailProvider) ProviderHealthStatus
	GetHealthReport() map[config.EmailProvider]ProviderHealth
	GetProviderHealth(provider config.EmailProvider) ProviderHealth
	Reset(provider config.EmailProvider)
	CheckHealth(ctx context.Context, provider config.EmailProvider) error
}

type emailHealthChecker struct {
	mu               sync.RWMutex
	failureThreshold int
	healthMap        map[config.EmailProvider]*ProviderHealth
	probeFunc        func(ctx context.Context, provider config.EmailProvider) error
}

// NewEmailHealthChecker initializes a health checker with the specified consecutive failure threshold.
func NewEmailHealthChecker(failureThreshold int) EmailHealthChecker {
	if failureThreshold <= 0 {
		failureThreshold = config.DefaultHealthFailureThreshold
	}

	checker := &emailHealthChecker{
		failureThreshold: failureThreshold,
		healthMap:        make(map[config.EmailProvider]*ProviderHealth),
	}

	// Initialize default providers
	for _, p := range []config.EmailProvider{config.ProviderSendGrid, config.ProviderSES, config.ProviderSMTP} {
		checker.healthMap[p] = &ProviderHealth{
			Provider: p,
			Status:   HealthStatusHealthy,
		}
	}

	return checker
}

// RecordSuccess updates provider state after a successful dispatch.
func (c *emailHealthChecker) RecordSuccess(provider config.EmailProvider, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	h, exists := c.healthMap[provider]
	if !exists {
		h = &ProviderHealth{Provider: provider, Status: HealthStatusHealthy}
		c.healthMap[provider] = h
	}

	now := time.Now()
	h.TotalSent++
	h.ConsecutiveSuccesses++
	h.ConsecutiveFailures = 0
	h.LastSuccess = &now
	h.LastLatencyMs = latency.Milliseconds()
	h.Status = HealthStatusHealthy
}

// RecordFailure updates provider state after an error or send failure.
func (c *emailHealthChecker) RecordFailure(provider config.EmailProvider, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	h, exists := c.healthMap[provider]
	if !exists {
		h = &ProviderHealth{Provider: provider, Status: HealthStatusHealthy}
		c.healthMap[provider] = h
	}

	now := time.Now()
	h.TotalFailures++
	h.ConsecutiveFailures++
	h.ConsecutiveSuccesses = 0
	h.LastFailure = &now
	if err != nil {
		h.LastError = err.Error()
	}

	if h.ConsecutiveFailures >= c.failureThreshold {
		h.Status = HealthStatusUnhealthy
	} else {
		h.Status = HealthStatusDegraded
	}
}

// IsHealthy returns true if the provider is not in unhealthy status.
func (c *emailHealthChecker) IsHealthy(provider config.EmailProvider) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	h, exists := c.healthMap[provider]
	if !exists {
		return true
	}
	return h.Status != HealthStatusUnhealthy
}

// GetStatus returns the current ProviderHealthStatus for the specified provider.
func (c *emailHealthChecker) GetStatus(provider config.EmailProvider) ProviderHealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if h, exists := c.healthMap[provider]; exists {
		return h.Status
	}
	return HealthStatusHealthy
}

// GetHealthReport returns a snapshot of health statuses for all monitored providers.
func (c *emailHealthChecker) GetHealthReport() map[config.EmailProvider]ProviderHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()

	report := make(map[config.EmailProvider]ProviderHealth, len(c.healthMap))
	for k, v := range c.healthMap {
		report[k] = *v
	}
	return report
}

// GetProviderHealth returns metrics for a specific provider.
func (c *emailHealthChecker) GetProviderHealth(provider config.EmailProvider) ProviderHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if h, exists := c.healthMap[provider]; exists {
		return *h
	}
	return ProviderHealth{Provider: provider, Status: HealthStatusHealthy}
}

// Reset resets statistics and marks provider healthy.
func (c *emailHealthChecker) Reset(provider config.EmailProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.healthMap[provider] = &ProviderHealth{
		Provider: provider,
		Status:   HealthStatusHealthy,
	}
}

// SetProbeFunc allows overriding the health check probe (useful for tests).
func (c *emailHealthChecker) SetProbeFunc(fn func(ctx context.Context, provider config.EmailProvider) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.probeFunc = fn
}

// CheckHealth probes provider connectivity and updates metrics.
func (c *emailHealthChecker) CheckHealth(ctx context.Context, provider config.EmailProvider) error {
	c.mu.RLock()
	customProbe := c.probeFunc
	c.mu.RUnlock()

	if customProbe != nil {
		start := time.Now()
		err := customProbe(ctx, provider)
		if err != nil {
			c.RecordFailure(provider, err)
			return err
		}
		c.RecordSuccess(provider, time.Since(start))
		return nil
	}

	start := time.Now()
	var err error
	switch provider {
	case config.ProviderSendGrid:
		err = checkSendGridHealth(ctx)
	case config.ProviderSES:
		err = checkSESHealth(ctx)
	case config.ProviderSMTP:
		err = checkSMTPHealth(ctx)
	default:
		err = fmt.Errorf("unknown provider %s", provider)
	}

	if err != nil {
		c.RecordFailure(provider, err)
		return err
	}
	c.RecordSuccess(provider, time.Since(start))
	return nil
}

func checkSendGridHealth(ctx context.Context) error {
	cfg := config.GetEmailConfig()
	if cfg.SendGridAPIKey == "" {
		return errors.New("SendGrid API key not configured")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.sendgrid.com/v3/scopes", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.SendGridAPIKey)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != 401 && resp.StatusCode != 403 {
		return fmt.Errorf("SendGrid API returned status %d", resp.StatusCode)
	}
	return nil
}

func checkSESHealth(ctx context.Context) error {
	cfg := config.GetEmailConfig()
	if cfg.SESSMTPUsername == "" && cfg.SESRegion == "" {
		return errors.New("SES credentials not configured")
	}
	// Verify DNS resolution of SES SMTP endpoint
	host := fmt.Sprintf("email-smtp.%s.amazonaws.com", cfg.SESRegion)
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", host+":587")
	if err != nil {
		return fmt.Errorf("failed connecting to SES SMTP endpoint: %w", err)
	}
	conn.Close()
	return nil
}

func checkSMTPHealth(ctx context.Context) error {
	cfg := config.GetEmailConfig()
	if cfg.SMTPHost == "" || cfg.SMTPPort == "" {
		return errors.New("SMTP host or port not configured")
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(cfg.SMTPHost, cfg.SMTPPort))
	if err != nil {
		return fmt.Errorf("failed connecting to SMTP host: %w", err)
	}
	conn.Close()
	return nil
}
