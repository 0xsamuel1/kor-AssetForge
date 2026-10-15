package services

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/kor-assetforge/config"
)

func TestEmailFailover_PrimarySuccess(t *testing.T) {
	cfg := config.EmailConfig{
		PrimaryProvider:  config.ProviderSendGrid,
		ProviderPriority: []config.EmailProvider{config.ProviderSendGrid, config.ProviderSES},
		MaxRetries:       2,
		InitialBackoff:   1 * time.Millisecond,
		MaxBackoff:       5 * time.Millisecond,
	}

	health := NewEmailHealthChecker(3)
	svc := NewFailoverEmailService(cfg, health)

	var sendGridCalls int32
	var sesCalls int32

	svc.SetSender(config.ProviderSendGrid, func(msg *EmailMessage) error {
		atomic.AddInt32(&sendGridCalls, 1)
		return nil
	})
	svc.SetSender(config.ProviderSES, func(msg *EmailMessage) error {
		atomic.AddInt32(&sesCalls, 1)
		return nil
	})

	err := svc.SendWithFailover(&EmailMessage{
		To:      "alice@example.com",
		Subject: "Test",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&sendGridCalls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&sesCalls))
	assert.True(t, health.IsHealthy(config.ProviderSendGrid))
	assert.Equal(t, int64(1), health.GetProviderHealth(config.ProviderSendGrid).TotalSent)
}

func TestEmailFailover_AutomaticFailoverToBackup(t *testing.T) {
	cfg := config.EmailConfig{
		PrimaryProvider:  config.ProviderSendGrid,
		ProviderPriority: []config.EmailProvider{config.ProviderSendGrid, config.ProviderSES},
		MaxRetries:       2,
		InitialBackoff:   1 * time.Millisecond,
		MaxBackoff:       2 * time.Millisecond,
	}

	health := NewEmailHealthChecker(3)
	svc := NewFailoverEmailService(cfg, health)

	var sendGridCalls int32
	var sesCalls int32

	// SendGrid fails repeatedly
	svc.SetSender(config.ProviderSendGrid, func(msg *EmailMessage) error {
		atomic.AddInt32(&sendGridCalls, 1)
		return errors.New("sendgrid rate limit exceeded")
	})
	// SES succeeds
	svc.SetSender(config.ProviderSES, func(msg *EmailMessage) error {
		atomic.AddInt32(&sesCalls, 1)
		return nil
	})

	err := svc.SendWithFailover(&EmailMessage{
		To:      "bob@example.com",
		Subject: "Failover Test",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&sendGridCalls)) // retried 2 times
	assert.Equal(t, int32(1), atomic.LoadInt32(&sesCalls))      // then succeeded on backup
	assert.Equal(t, int64(2), health.GetProviderHealth(config.ProviderSendGrid).TotalFailures)
	assert.Equal(t, int64(1), health.GetProviderHealth(config.ProviderSES).TotalSent)
}

func TestEmailFailover_RetryWithExponentialBackoff(t *testing.T) {
	cfg := config.EmailConfig{
		PrimaryProvider:   config.ProviderSendGrid,
		ProviderPriority:  []config.EmailProvider{config.ProviderSendGrid},
		MaxRetries:        3,
		InitialBackoff:    5 * time.Millisecond,
		MaxBackoff:        50 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	health := NewEmailHealthChecker(3)
	svc := NewFailoverEmailService(cfg, health)

	var attempts int32
	svc.SetSender(config.ProviderSendGrid, func(msg *EmailMessage) error {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			return errors.New("temporary 503 service unavailable")
		}
		return nil
	})

	start := time.Now()
	err := svc.SendWithFailover(&EmailMessage{
		To:      "carol@example.com",
		Subject: "Retry Test",
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
	// Should have slept backoff 5ms + 10ms = 15ms minimum
	assert.True(t, elapsed >= 10*time.Millisecond)
	assert.True(t, health.IsHealthy(config.ProviderSendGrid))
}

func TestEmailFailover_SkipsUnhealthyProvider(t *testing.T) {
	cfg := config.EmailConfig{
		PrimaryProvider:  config.ProviderSendGrid,
		ProviderPriority: []config.EmailProvider{config.ProviderSendGrid, config.ProviderSES},
		MaxRetries:       1,
		InitialBackoff:   1 * time.Millisecond,
	}

	health := NewEmailHealthChecker(2)
	// Mark SendGrid unhealthy with 2 failures
	health.RecordFailure(config.ProviderSendGrid, errors.New("fail 1"))
	health.RecordFailure(config.ProviderSendGrid, errors.New("fail 2"))
	require.Equal(t, HealthStatusUnhealthy, health.GetStatus(config.ProviderSendGrid))

	svc := NewFailoverEmailService(cfg, health)

	var sendGridCalls int32
	var sesCalls int32

	svc.SetSender(config.ProviderSendGrid, func(msg *EmailMessage) error {
		atomic.AddInt32(&sendGridCalls, 1)
		return nil
	})
	svc.SetSender(config.ProviderSES, func(msg *EmailMessage) error {
		atomic.AddInt32(&sesCalls, 1)
		return nil
	})

	err := svc.SendWithFailover(&EmailMessage{
		To:      "david@example.com",
		Subject: "Skip Unhealthy",
	})

	require.NoError(t, err)
	// SendGrid was skipped because it was marked unhealthy!
	assert.Equal(t, int32(0), atomic.LoadInt32(&sendGridCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&sesCalls))
}

func TestEmailFailover_AllProvidersFail(t *testing.T) {
	cfg := config.EmailConfig{
		PrimaryProvider:  config.ProviderSendGrid,
		ProviderPriority: []config.EmailProvider{config.ProviderSendGrid, config.ProviderSES},
		MaxRetries:       1,
		InitialBackoff:   1 * time.Millisecond,
	}

	health := NewEmailHealthChecker(3)
	svc := NewFailoverEmailService(cfg, health)

	svc.SetSender(config.ProviderSendGrid, func(msg *EmailMessage) error {
		return errors.New("sendgrid error")
	})
	svc.SetSender(config.ProviderSES, func(msg *EmailMessage) error {
		return errors.New("ses error")
	})

	err := svc.SendWithFailover(&EmailMessage{
		To:      "erin@example.com",
		Subject: "Total Outage",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "all email providers failed")
}

func TestEmailHealthChecker_StatusTransitions(t *testing.T) {
	health := NewEmailHealthChecker(3)
	provider := config.ProviderSendGrid

	assert.Equal(t, HealthStatusHealthy, health.GetStatus(provider))
	assert.True(t, health.IsHealthy(provider))

	// 1 failure -> Degraded
	health.RecordFailure(provider, errors.New("err 1"))
	assert.Equal(t, HealthStatusDegraded, health.GetStatus(provider))
	assert.True(t, health.IsHealthy(provider))

	// 2 failures -> Degraded
	health.RecordFailure(provider, errors.New("err 2"))
	assert.Equal(t, HealthStatusDegraded, health.GetStatus(provider))
	assert.True(t, health.IsHealthy(provider))

	// 3 failures >= threshold -> Unhealthy
	health.RecordFailure(provider, errors.New("err 3"))
	assert.Equal(t, HealthStatusUnhealthy, health.GetStatus(provider))
	assert.False(t, health.IsHealthy(provider))

	// Success restores to Healthy
	health.RecordSuccess(provider, 20*time.Millisecond)
	assert.Equal(t, HealthStatusHealthy, health.GetStatus(provider))
	assert.True(t, health.IsHealthy(provider))

	report := health.GetHealthReport()
	assert.Contains(t, report, provider)
	assert.Equal(t, int64(1), report[provider].TotalSent)
	assert.Equal(t, int64(3), report[provider].TotalFailures)
}
