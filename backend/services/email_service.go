package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/kor-assetforge/config"
)

type EmailProvider = config.EmailProvider

const (
	ProviderSendGrid = config.ProviderSendGrid
	ProviderSES      = config.ProviderSES
	ProviderSMTP     = config.ProviderSMTP
)

const (
	defaultFromName           = "AssetForge"
	defaultFromAddress        = "no-reply@assetforge.io"
	defaultVerificationURL    = "https://app.assetforge.io/verify-email"
	emailQueueBuffer          = 100
	emailContentBoundary      = "--assetforge-boundary"
	emailContentTypeHTML      = "text/html; charset=UTF-8"
	emailContentTypePlainText = "text/plain; charset=UTF-8"
)

type EmailService interface {
	SendVerificationEmail(toEmail, toName, verificationToken string) error
	SendKYCStatusUpdate(toEmail, toName, status, reviewNotes string) error
	SendTransactionConfirmation(toEmail, toName, txHash string, amount int64, assetID uint, fromAddress, toAddress string) error
	SendApprovalPendingEmail(toEmail, toName string, requestID uint, expiresAt time.Time) error
	SendScheduledReportEmail(recipients []string, reportName, format, fileName, body string) error
	SendGDPRDataExportLink(toEmail, toName, downloadLink string) error
	// SendCustomEmail queues an email with fully pre-rendered subject/body, used
	// by the database-backed dynamic template engine (#163).
	SendCustomEmail(toEmail, toName, subject, html, plainText string) error
}

// FailoverEmailService extends EmailService with provider health inspection and failover controls.
type FailoverEmailService interface {
	EmailService
	GetHealthChecker() EmailHealthChecker
	SendWithFailover(msg *EmailMessage) error
	SetSender(provider config.EmailProvider, fn func(msg *EmailMessage) error)
	GetConfig() config.EmailConfig
}

type emailService struct {
	cfg           config.EmailConfig
	healthChecker EmailHealthChecker
	queue         chan *EmailMessage
	customSenders map[config.EmailProvider]func(msg *EmailMessage) error
	mu            sync.RWMutex
}

type EmailMessage struct {
	To        string
	ToName    string
	Subject   string
	PlainText string
	HTML      string
}

// NewEmailServiceFromEnv creates an EmailService instance from configuration loaded via env.
func NewEmailServiceFromEnv() FailoverEmailService {
	cfg := config.LoadEmailConfig()
	healthChecker := NewEmailHealthChecker(cfg.HealthFailureThreshold)
	return NewFailoverEmailService(cfg, healthChecker)
}

// NewFailoverEmailService constructs an EmailService with explicit config and health checker.
func NewFailoverEmailService(cfg config.EmailConfig, health EmailHealthChecker) FailoverEmailService {
	if health == nil {
		health = NewEmailHealthChecker(cfg.HealthFailureThreshold)
	}

	svc := &emailService{
		cfg:           cfg,
		healthChecker: health,
		queue:         make(chan *EmailMessage, emailQueueBuffer),
		customSenders: make(map[config.EmailProvider]func(msg *EmailMessage) error),
	}

	go svc.worker()
	return svc
}

func (s *emailService) GetHealthChecker() EmailHealthChecker {
	return s.healthChecker
}

func (s *emailService) GetConfig() config.EmailConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// SetSender registers a custom send function for a provider (useful for testing and mocks).
func (s *emailService) SetSender(provider config.EmailProvider, fn func(msg *EmailMessage) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.customSenders[provider] = fn
}

func (s *emailService) queueEmail(msg *EmailMessage) error {
	select {
	case s.queue <- msg:
		return nil
	default:
		return errors.New("email queue is full")
	}
}

func (s *emailService) worker() {
	for msg := range s.queue {
		if err := s.SendWithFailover(msg); err != nil {
			log.Printf("email_service: failed to send email to %s across all providers: %v", msg.To, err)
		}
	}
}

// SendWithFailover attempts delivery across configured providers in priority order with exponential backoff.
func (s *emailService) SendWithFailover(msg *EmailMessage) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	priority := cfg.ProviderPriority
	if len(priority) == 0 {
		priority = []config.EmailProvider{cfg.PrimaryProvider}
	}

	// Filter healthy candidates first
	var candidates []config.EmailProvider
	for _, p := range priority {
		if s.healthChecker == nil || s.healthChecker.IsHealthy(p) {
			candidates = append(candidates, p)
		}
	}

	// If all providers are deemed unhealthy, fallback to trying all in priority order
	if len(candidates) == 0 {
		candidates = priority
	}

	var lastErr error
	for i, provider := range candidates {
		maxRetries := cfg.MaxRetries
		if maxRetries < 1 {
			maxRetries = 1
		}
		backoff := cfg.InitialBackoff

		var providerErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			start := time.Now()
			providerErr = s.dispatch(provider, msg)
			latency := time.Since(start)

			if providerErr == nil {
				if s.healthChecker != nil {
					s.healthChecker.RecordSuccess(provider, latency)
				}
				if provider != cfg.PrimaryProvider {
					log.Printf("email_service: email to %s successfully delivered via fallback provider %s", msg.To, provider)
				}
				return nil
			}

			if s.healthChecker != nil {
				s.healthChecker.RecordFailure(provider, providerErr)
			}
			log.Printf("email_service: provider %s attempt %d/%d failed for %s: %v", provider, attempt, maxRetries, msg.To, providerErr)

			if attempt < maxRetries && backoff > 0 {
				time.Sleep(backoff)
				backoff = time.Duration(float64(backoff) * cfg.BackoffMultiplier)
				if backoff > cfg.MaxBackoff {
					backoff = cfg.MaxBackoff
				}
			}
		}

		lastErr = providerErr
		if i+1 < len(candidates) {
			log.Printf("email_service: provider %s failed all attempts; initiating failover to %s", provider, candidates[i+1])
		}
	}

	return fmt.Errorf("all email providers failed; last error: %w", lastErr)
}

func (s *emailService) dispatch(provider config.EmailProvider, msg *EmailMessage) error {
	s.mu.RLock()
	sender, custom := s.customSenders[provider]
	s.mu.RUnlock()

	if custom && sender != nil {
		return sender(msg)
	}

	switch provider {
	case config.ProviderSendGrid:
		return s.sendWithSendGrid(msg)
	case config.ProviderSES:
		return s.sendWithSES(msg)
	case config.ProviderSMTP:
		return s.sendWithSMTP(msg)
	default:
		return s.sendWithSMTP(msg)
	}
}

func (s *emailService) sendWithSMTP(msg *EmailMessage) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg.SMTPHost == "" || cfg.SMTPPort == "" {
		log.Printf("email_service: SMTP_HOST/SMTP_PORT not configured; writing email to log:\nTo: %s\nSubject: %s\nBody: %s", msg.To, msg.Subject, msg.PlainText)
		return nil
	}

	var auth smtp.Auth
	if cfg.SMTPUsername != "" {
		auth = smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	}

	body := fmt.Sprintf("MIME-Version: 1.0\r\nContent-Type: %s; boundary=%s\r\nSubject: %s\r\nTo: %s\r\nFrom: %s <%s>\r\n\r\n", "multipart/alternative", emailContentBoundary, msg.Subject, msg.To, cfg.FromName, cfg.FromAddress)
	body += fmt.Sprintf("--%s\r\nContent-Type: %s\r\n\r\n%s\r\n", emailContentBoundary, emailContentTypePlainText, msg.PlainText)
	body += fmt.Sprintf("--%s\r\nContent-Type: %s\r\n\r\n%s\r\n", emailContentBoundary, emailContentTypeHTML, msg.HTML)
	body += fmt.Sprintf("--%s--", emailContentBoundary)

	return smtp.SendMail(cfg.SMTPHost+":"+cfg.SMTPPort, auth, cfg.FromAddress, []string{msg.To}, []byte(body))
}

func (s *emailService) sendWithSendGrid(msg *EmailMessage) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg.SendGridAPIKey == "" {
		return errors.New("SendGrid API key not configured")
	}
	url := "https://api.sendgrid.com/v3/mail/send"
	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{
				"to": []map[string]string{
					{"email": msg.To, "name": msg.ToName},
				},
			},
		},
		"from":    map[string]string{"email": cfg.FromAddress, "name": cfg.FromName},
		"subject": msg.Subject,
		"content": []map[string]string{
			{"type": "text/plain", "value": msg.PlainText},
			{"type": "text/html", "value": msg.HTML},
		},
	}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Authorization", "Bearer "+cfg.SendGridAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("SendGrid API returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *emailService) sendWithSES(msg *EmailMessage) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg.SESSMTPUsername == "" || cfg.SESSMTPPassword == "" {
		return errors.New("AWS SES SMTP credentials not configured")
	}

	sesHost := fmt.Sprintf("email-smtp.%s.amazonaws.com", cfg.SESRegion)
	sesPort := "587"
	auth := smtp.PlainAuth("", cfg.SESSMTPUsername, cfg.SESSMTPPassword, sesHost)

	body := fmt.Sprintf("MIME-Version: 1.0\r\nContent-Type: %s; boundary=%s\r\nSubject: %s\r\nTo: %s\r\nFrom: %s <%s>\r\n\r\n", "multipart/alternative", emailContentBoundary, msg.Subject, msg.To, cfg.FromName, cfg.FromAddress)
	body += fmt.Sprintf("--%s\r\nContent-Type: %s\r\n\r\n%s\r\n", emailContentBoundary, emailContentTypePlainText, msg.PlainText)
	body += fmt.Sprintf("--%s\r\nContent-Type: %s\r\n\r\n%s\r\n", emailContentBoundary, emailContentTypeHTML, msg.HTML)
	body += fmt.Sprintf("--%s--", emailContentBoundary)

	return smtp.SendMail(sesHost+":"+sesPort, auth, cfg.FromAddress, []string{msg.To}, []byte(body))
}

// SendCustomEmail queues an arbitrary, already-rendered message.
func (s *emailService) SendCustomEmail(toEmail, toName, subject, html, plainText string) error {
	if strings.TrimSpace(toEmail) == "" {
		return errors.New("recipient email is required")
	}
	if strings.TrimSpace(subject) == "" {
		return errors.New("subject is required")
	}
	return s.queueEmail(&EmailMessage{To: toEmail, ToName: toName, Subject: subject, PlainText: plainText, HTML: html})
}

func (s *emailService) SendApprovalPendingEmail(toEmail, toName string, requestID uint, expiresAt time.Time) error {
	subject := "Asset transfer approval required"
	plain := fmt.Sprintf("Hi %s,\n\nApproval request #%d is waiting for your decision. It expires at %s.\n", toName, requestID, expiresAt.Format(time.RFC3339))
	html := fmt.Sprintf("<p>Hi %s,</p><p>Approval request <strong>#%d</strong> is waiting for your decision.</p><p>It expires at %s.</p>", toName, requestID, expiresAt.Format(time.RFC3339))
	return s.queueEmail(&EmailMessage{To: toEmail, ToName: toName, Subject: subject, PlainText: plain, HTML: html})
}

func (s *emailService) SendVerificationEmail(toEmail, toName, verificationToken string) error {
	s.mu.RLock()
	verificationURL := s.cfg.VerificationURL
	s.mu.RUnlock()
	link := fmt.Sprintf("%s?token=%s", verificationURL, verificationToken)
	subject := "Verify your email address"
	plain := fmt.Sprintf("Hi %s,\n\nPlease verify your email by clicking: %s\n", toName, link)
	html := fmt.Sprintf("<p>Hi %s,</p><p>Please verify your email by clicking <a href=\"%s\">here</a>.</p>", toName, link)
	return s.queueEmail(&EmailMessage{To: toEmail, ToName: toName, Subject: subject, PlainText: plain, HTML: html})
}

func (s *emailService) SendKYCStatusUpdate(toEmail, toName, status, reviewNotes string) error {
	subject := fmt.Sprintf("KYC status update: %s", strings.Title(status))
	plain := fmt.Sprintf("Hi %s,\n\nYour KYC status has been updated to %s.\n\nReview notes: %s\n\nIf you have questions, contact support.\n", toName, strings.Title(status), reviewNotes)
	html := fmt.Sprintf("<p>Hi %s,</p><p>Your KYC status has been updated to <strong>%s</strong>.</p><p><strong>Review notes:</strong> %s</p><p>If you have questions, reply to this email or contact support.</p>", toName, strings.Title(status), reviewNotes)
	return s.queueEmail(&EmailMessage{To: toEmail, ToName: toName, Subject: subject, PlainText: plain, HTML: html})
}

func (s *emailService) SendTransactionConfirmation(toEmail, toName, txHash string, amount int64, assetID uint, fromAddress, toAddress string) error {
	subject := "Transaction confirmation from AssetForge"
	plain := fmt.Sprintf("Hi %s,\n\nYour transaction has been recorded successfully.\n\nTransaction hash: %s\nAsset ID: %d\nAmount: %d\nFrom: %s\nTo: %s\n\nThank you for using AssetForge.\n", toName, txHash, assetID, amount, fromAddress, toAddress)
	html := fmt.Sprintf("<p>Hi %s,</p><p>Your transaction has been recorded successfully.</p><ul><li><strong>Transaction hash:</strong> %s</li><li><strong>Asset ID:</strong> %d</li><li><strong>Amount:</strong> %d</li><li><strong>From:</strong> %s</li><li><strong>To:</strong> %s</li></ul><p>Thank you for using AssetForge.</p>", toName, txHash, assetID, amount, fromAddress, toAddress)
	return s.queueEmail(&EmailMessage{To: toEmail, ToName: toName, Subject: subject, PlainText: plain, HTML: html})
}

func (s *emailService) SendScheduledReportEmail(recipients []string, reportName, format, fileName, body string) error {
	if len(recipients) == 0 {
		return errors.New("at least one recipient is required")
	}
	subject := fmt.Sprintf("Scheduled report ready: %s", reportName)
	plain := fmt.Sprintf("Your scheduled %s report is ready.\n\nFile: %s\nFormat: %s\n\n%s\n", reportName, fileName, strings.ToUpper(format), body)
	html := fmt.Sprintf("<p>Your scheduled <strong>%s</strong> report is ready.</p><p><strong>File:</strong> %s<br><strong>Format:</strong> %s</p><pre style=\"white-space:pre-wrap\">%s</pre>", reportName, fileName, strings.ToUpper(format), body)
	for _, recipient := range recipients {
		if err := s.queueEmail(&EmailMessage{To: recipient, Subject: subject, PlainText: plain, HTML: html}); err != nil {
			return err
		}
	}
	return nil
}

func (s *emailService) SendGDPRDataExportLink(toEmail, toName, downloadLink string) error {
	subject := "Your GDPR Data Export is Ready"
	plain := fmt.Sprintf("Hi %s,\n\nYour GDPR data export request is ready for download. Please click the link below to retrieve your file:\n\n%s\n\nNote: This link will expire in 7 days.\n\nThank you,\nAssetForge Team\n", toName, downloadLink)
	html := fmt.Sprintf("<p>Hi %s,</p><p>Your GDPR data export request is ready for download. Please click the link below to retrieve your file:</p><p><a href=\"%s\" style=\"display:inline-block;padding:12px 20px;background:#1a73e8;color:#fff;text-decoration:none;border-radius:4px;\">Download Data Export</a></p><p><em>Note: This link will expire in 7 days.</em></p><p>Thank you,<br/>AssetForge Team</p>", toName, downloadLink)
	return s.queueEmail(&EmailMessage{To: toEmail, ToName: toName, Subject: subject, PlainText: plain, HTML: html})
}
