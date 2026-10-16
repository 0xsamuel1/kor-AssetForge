package config

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EmailProvider represents supported email service provider identifiers.
type EmailProvider string

const (
	ProviderSendGrid EmailProvider = "sendgrid"
	ProviderSES      EmailProvider = "ses"
	ProviderSMTP     EmailProvider = "smtp"
)

// EmailConfig holds configuration for email providers, priority order, retries, and health checks.
type EmailConfig struct {
	PrimaryProvider        EmailProvider   `json:"primary_provider"`
	ProviderPriority       []EmailProvider `json:"provider_priority"`
	MaxRetries             int             `json:"max_retries"`
	InitialBackoff         time.Duration   `json:"initial_backoff"`
	MaxBackoff             time.Duration   `json:"max_backoff"`
	BackoffMultiplier      float64         `json:"backoff_multiplier"`
	HealthCheckInterval    time.Duration   `json:"health_check_interval"`
	HealthFailureThreshold int             `json:"health_failure_threshold"`
	FromAddress            string          `json:"from_address"`
	FromName               string          `json:"from_name"`
	VerificationURL        string          `json:"verification_url"`
	SendGridAPIKey         string          `json:"-"`
	SESRegion              string          `json:"ses_region"`
	SESSMTPUsername        string          `json:"-"`
	SESSMTPPassword        string          `json:"-"`
	SMTPHost               string          `json:"smtp_host"`
	SMTPPort               string          `json:"smtp_port"`
	SMTPUsername           string          `json:"-"`
	SMTPPassword           string          `json:"-"`
}

const (
	DefaultEmailFromAddress        = "no-reply@assetforge.io"
	DefaultEmailFromName           = "AssetForge"
	DefaultVerificationURLBase     = "https://app.assetforge.io/verify-email"
	DefaultEmailMaxRetries         = 3
	DefaultInitialBackoff          = 500 * time.Millisecond
	DefaultMaxBackoff              = 10 * time.Second
	DefaultBackoffMultiplier       = 2.0
	DefaultHealthCheckInterval     = 60 * time.Second
	DefaultHealthFailureThreshold  = 3
)

var (
	emailConfigMu   sync.RWMutex
	currEmailConfig *EmailConfig
)

// LoadEmailConfig loads email configuration from environment variables with sensible defaults.
func LoadEmailConfig() EmailConfig {
	emailConfigMu.Lock()
	defer emailConfigMu.Unlock()

	primary := EmailProvider(strings.ToLower(strings.TrimSpace(os.Getenv("EMAIL_PROVIDER"))))
	if primary == "" {
		if os.Getenv("SENDGRID_API_KEY") != "" {
			primary = ProviderSendGrid
		} else if os.Getenv("SES_SMTP_USERNAME") != "" && os.Getenv("SES_SMTP_PASSWORD") != "" {
			primary = ProviderSES
		} else {
			primary = ProviderSMTP
		}
	}

	priorityStr := os.Getenv("EMAIL_PROVIDER_PRIORITY")
	var priority []EmailProvider
	if strings.TrimSpace(priorityStr) != "" {
		for _, part := range strings.Split(priorityStr, ",") {
			p := EmailProvider(strings.ToLower(strings.TrimSpace(part)))
			if p != "" && !containsProvider(priority, p) {
				priority = append(priority, p)
			}
		}
	}

	// Fallback priority: primary first, followed by others
	allProviders := []EmailProvider{ProviderSendGrid, ProviderSES, ProviderSMTP}
	if len(priority) == 0 {
		priority = append(priority, primary)
		for _, p := range allProviders {
			if !containsProvider(priority, p) {
				priority = append(priority, p)
			}
		}
	} else {
		// Ensure primary is present if not specified
		if !containsProvider(priority, primary) {
			priority = append([]EmailProvider{primary}, priority...)
		}
		// Ensure all standard providers are present for complete failover
		for _, p := range allProviders {
			if !containsProvider(priority, p) {
				priority = append(priority, p)
			}
		}
	}

	maxRetries := DefaultEmailMaxRetries
	if val, err := strconv.Atoi(os.Getenv("EMAIL_MAX_RETRIES")); err == nil && val >= 0 {
		maxRetries = val
	}

	initialBackoff := DefaultInitialBackoff
	if d, err := time.ParseDuration(os.Getenv("EMAIL_INITIAL_BACKOFF")); err == nil && d > 0 {
		initialBackoff = d
	}

	maxBackoff := DefaultMaxBackoff
	if d, err := time.ParseDuration(os.Getenv("EMAIL_MAX_BACKOFF")); err == nil && d > 0 {
		maxBackoff = d
	}

	healthFailureThreshold := DefaultHealthFailureThreshold
	if val, err := strconv.Atoi(os.Getenv("EMAIL_HEALTH_FAILURE_THRESHOLD")); err == nil && val > 0 {
		healthFailureThreshold = val
	}

	fromAddress := os.Getenv("EMAIL_FROM_ADDRESS")
	if fromAddress == "" {
		fromAddress = DefaultEmailFromAddress
	}

	fromName := os.Getenv("EMAIL_FROM_NAME")
	if fromName == "" {
		fromName = DefaultEmailFromName
	}

	verificationURL := os.Getenv("EMAIL_VERIFICATION_URL_BASE")
	if verificationURL == "" {
		verificationURL = DefaultVerificationURLBase
	}

	sesRegion := os.Getenv("SES_REGION")
	if sesRegion == "" {
		sesRegion = "us-east-1"
	}

	cfg := EmailConfig{
		PrimaryProvider:        primary,
		ProviderPriority:       priority,
		MaxRetries:             maxRetries,
		InitialBackoff:         initialBackoff,
		MaxBackoff:             maxBackoff,
		BackoffMultiplier:      DefaultBackoffMultiplier,
		HealthCheckInterval:    DefaultHealthCheckInterval,
		HealthFailureThreshold: healthFailureThreshold,
		FromAddress:            fromAddress,
		FromName:               fromName,
		VerificationURL:        verificationURL,
		SendGridAPIKey:         os.Getenv("SENDGRID_API_KEY"),
		SESRegion:              sesRegion,
		SESSMTPUsername:        os.Getenv("SES_SMTP_USERNAME"),
		SESSMTPPassword:        os.Getenv("SES_SMTP_PASSWORD"),
		SMTPHost:               os.Getenv("SMTP_HOST"),
		SMTPPort:               os.Getenv("SMTP_PORT"),
		SMTPUsername:           os.Getenv("SMTP_USERNAME"),
		SMTPPassword:           os.Getenv("SMTP_PASSWORD"),
	}

	currEmailConfig = &cfg
	return cfg
}

// GetEmailConfig returns the currently cached or freshly loaded EmailConfig.
func GetEmailConfig() EmailConfig {
	emailConfigMu.RLock()
	if currEmailConfig != nil {
		cfg := *currEmailConfig
		emailConfigMu.RUnlock()
		return cfg
	}
	emailConfigMu.RUnlock()
	return LoadEmailConfig()
}

// SetEmailConfig sets the runtime EmailConfig (useful for tests and dynamic updates).
func SetEmailConfig(cfg EmailConfig) {
	emailConfigMu.Lock()
	defer emailConfigMu.Unlock()
	currEmailConfig = &cfg
}

func containsProvider(list []EmailProvider, p EmailProvider) bool {
	for _, item := range list {
		if item == p {
			return true
		}
	}
	return false
}
