package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadEmailConfig_Defaults(t *testing.T) {
	os.Unsetenv("EMAIL_PROVIDER")
	os.Unsetenv("EMAIL_PROVIDER_PRIORITY")
	os.Unsetenv("EMAIL_MAX_RETRIES")
	os.Unsetenv("EMAIL_INITIAL_BACKOFF")
	os.Unsetenv("EMAIL_MAX_BACKOFF")
	os.Unsetenv("EMAIL_HEALTH_FAILURE_THRESHOLD")
	os.Unsetenv("SENDGRID_API_KEY")
	os.Unsetenv("SES_SMTP_USERNAME")
	os.Unsetenv("SES_SMTP_PASSWORD")

	cfg := LoadEmailConfig()
	assert.Equal(t, ProviderSMTP, cfg.PrimaryProvider)
	assert.Contains(t, cfg.ProviderPriority, ProviderSendGrid)
	assert.Contains(t, cfg.ProviderPriority, ProviderSES)
	assert.Contains(t, cfg.ProviderPriority, ProviderSMTP)
	assert.Equal(t, DefaultEmailMaxRetries, cfg.MaxRetries)
	assert.Equal(t, DefaultInitialBackoff, cfg.InitialBackoff)
	assert.Equal(t, DefaultHealthFailureThreshold, cfg.HealthFailureThreshold)
}

func TestLoadEmailConfig_CustomPriority(t *testing.T) {
	os.Setenv("EMAIL_PROVIDER", "ses")
	os.Setenv("EMAIL_PROVIDER_PRIORITY", "ses,sendgrid,smtp")
	os.Setenv("EMAIL_MAX_RETRIES", "5")
	os.Setenv("EMAIL_INITIAL_BACKOFF", "200ms")
	os.Setenv("EMAIL_MAX_BACKOFF", "5s")
	os.Setenv("EMAIL_HEALTH_FAILURE_THRESHOLD", "4")
	defer func() {
		os.Unsetenv("EMAIL_PROVIDER")
		os.Unsetenv("EMAIL_PROVIDER_PRIORITY")
		os.Unsetenv("EMAIL_MAX_RETRIES")
		os.Unsetenv("EMAIL_INITIAL_BACKOFF")
		os.Unsetenv("EMAIL_MAX_BACKOFF")
		os.Unsetenv("EMAIL_HEALTH_FAILURE_THRESHOLD")
	}()

	cfg := LoadEmailConfig()
	assert.Equal(t, ProviderSES, cfg.PrimaryProvider)
	assert.Equal(t, []EmailProvider{ProviderSES, ProviderSendGrid, ProviderSMTP}, cfg.ProviderPriority)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 200*time.Millisecond, cfg.InitialBackoff)
	assert.Equal(t, 5*time.Second, cfg.MaxBackoff)
	assert.Equal(t, 4, cfg.HealthFailureThreshold)
}
