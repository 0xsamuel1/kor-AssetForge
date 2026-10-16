package webhooks

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
)

// EmailWebhookHandler handles incoming provider webhooks and provides analytics dashboard endpoints.
type EmailWebhookHandler struct {
	analytics     services.EmailAnalyticsService
	healthChecker services.EmailHealthChecker
}

// NewEmailWebhookHandler initializes an EmailWebhookHandler.
func NewEmailWebhookHandler(analytics services.EmailAnalyticsService, health services.EmailHealthChecker) *EmailWebhookHandler {
	return &EmailWebhookHandler{
		analytics:     analytics,
		healthChecker: health,
	}
}

// SendGridEvent represents a single event in SendGrid's webhook payload array.
type SendGridEvent struct {
	Email       string `json:"email"`
	Timestamp   int64  `json:"timestamp"`
	Event       string `json:"event"` // processed, dropped, delivered, bounce, open, click, spamreport
	SGMessageID string `json:"sg_message_id"`
	Reason      string `json:"reason"`
	Type        string `json:"type"` // bounce type, e.g. "blocked", "bounce"
	Status      string `json:"status"`
}

// SESNotification represents an AWS SES notification payload (often wrapped by SNS).
type SESNotification struct {
	Type        string `json:"Type"`
	Message     string `json:"Message"`
	EventType   string `json:"eventType"`
	Mail        *SESMail `json:"mail"`
	Bounce      *SESBounce `json:"bounce"`
	Delivery    *SESDelivery `json:"delivery"`
	Open        *SESOpen `json:"open"`
	Click       *SESClick `json:"click"`
}

type SESMail struct {
	MessageID   string   `json:"messageId"`
	Destination []string `json:"destination"`
}

type SESBounce struct {
	BounceType        string              `json:"bounceType"` // Permanent, Transient
	BounceSubType     string              `json:"bounceSubType"`
	BouncedRecipients []SESBouncedAddress `json:"bouncedRecipients"`
}

type SESBouncedAddress struct {
	EmailAddress   string `json:"emailAddress"`
	DiagnosticCode string `json:"diagnosticCode"`
}

type SESDelivery struct {
	Timestamp  string   `json:"timestamp"`
	Recipients []string `json:"recipients"`
}

type SESOpen struct {
	Timestamp string `json:"timestamp"`
}

type SESClick struct {
	Timestamp string `json:"timestamp"`
}

// GenericEventRequest defines the format for direct/generic email webhook ingest.
type GenericEventRequest struct {
	Event      string `json:"event" binding:"required"` // delivered, bounce, open, click
	Recipient  string `json:"recipient" binding:"required"`
	MessageID  string `json:"message_id"`
	BounceType string `json:"bounce_type"`
	Reason     string `json:"reason"`
}

// HandleSendGrid processes batch webhook events from SendGrid.
func (h *EmailWebhookHandler) HandleSendGrid(c *gin.Context) {
	var events []SendGridEvent
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	if err := json.Unmarshal(bodyBytes, &events); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json payload"})
		return
	}

	for _, event := range events {
		msgID := cleanSendGridMessageID(event.SGMessageID)
		switch strings.ToLower(event.Event) {
		case "delivered":
			_, _ = h.analytics.RecordDelivery(msgID, event.Email)
		case "bounce", "dropped":
			bType := models.BounceTypeSoft
			if strings.EqualFold(event.Type, "bounce") || strings.Contains(event.Reason, "550") {
				bType = models.BounceTypeHard
			}
			_, _ = h.analytics.RecordBounce(msgID, event.Email, bType, event.Reason)
		case "open":
			_, _ = h.analytics.RecordOpen(msgID, event.Email)
		case "click":
			_, _ = h.analytics.RecordClick(msgID, event.Email)
		case "spamreport":
			_, _ = h.analytics.RecordBounce(msgID, event.Email, models.BounceTypeHard, "Spam complaint reported")
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "processed", "count": len(events)})
}

// HandleSES processes notifications from AWS SES (directly or via SNS wrapper).
func (h *EmailWebhookHandler) HandleSES(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	var notif SESNotification
	if err := json.Unmarshal(bodyBytes, &notif); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json payload"})
		return
	}

	// If SNS wrapped message string, unwrap it
	if notif.Type == "Notification" && notif.Message != "" {
		var inner SESNotification
		if err := json.Unmarshal([]byte(notif.Message), &inner); err == nil {
			notif = inner
		}
	}

	eventType := notif.EventType
	msgID := ""
	if notif.Mail != nil {
		msgID = notif.Mail.MessageID
	}

	switch strings.ToLower(eventType) {
	case "delivery":
		if notif.Delivery != nil {
			for _, rec := range notif.Delivery.Recipients {
				_, _ = h.analytics.RecordDelivery(msgID, rec)
			}
		} else if notif.Mail != nil {
			for _, rec := range notif.Mail.Destination {
				_, _ = h.analytics.RecordDelivery(msgID, rec)
			}
		}
	case "bounce":
		bType := models.BounceTypeSoft
		if notif.Bounce != nil && strings.EqualFold(notif.Bounce.BounceType, "Permanent") {
			bType = models.BounceTypeHard
		}
		if notif.Bounce != nil && len(notif.Bounce.BouncedRecipients) > 0 {
			for _, bRec := range notif.Bounce.BouncedRecipients {
				_, _ = h.analytics.RecordBounce(msgID, bRec.EmailAddress, bType, bRec.DiagnosticCode)
			}
		}
	case "open":
		if notif.Mail != nil {
			for _, rec := range notif.Mail.Destination {
				_, _ = h.analytics.RecordOpen(msgID, rec)
			}
		}
	case "click":
		if notif.Mail != nil {
			for _, rec := range notif.Mail.Destination {
				_, _ = h.analytics.RecordClick(msgID, rec)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

// HandleGeneric processes generic webhook payload events.
func (h *EmailWebhookHandler) HandleGeneric(c *gin.Context) {
	var req GenericEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch strings.ToLower(req.Event) {
	case "delivered":
		_, err := h.analytics.RecordDelivery(req.MessageID, req.Recipient)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	case "bounce":
		bType := models.BounceType(strings.ToLower(req.BounceType))
		if bType != models.BounceTypeHard && bType != models.BounceTypeSoft && bType != models.BounceTypeTransient {
			bType = models.BounceTypeSoft
		}
		_, err := h.analytics.RecordBounce(req.MessageID, req.Recipient, bType, req.Reason)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	case "open":
		_, err := h.analytics.RecordOpen(req.MessageID, req.Recipient)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	case "click":
		_, err := h.analytics.RecordClick(req.MessageID, req.Recipient)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported event type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "recorded", "event": req.Event, "recipient": req.Recipient})
}

// GetDashboard returns the delivery dashboard summary and recent tracking entries.
func (h *EmailWebhookHandler) GetDashboard(c *gin.Context) {
	var since *time.Time
	if daysStr := c.Query("days"); daysStr != "" {
		if days, err := strconv.Atoi(daysStr); err == nil && days > 0 {
			t := time.Now().AddDate(0, 0, -days)
			since = &t
		}
	}

	summary, err := h.analytics.GetAnalyticsSummary(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to calculate delivery summary"})
		return
	}

	deliveries, total, _ := h.analytics.ListDeliveries("", "", 10, 0)

	var providerHealth interface{}
	if h.healthChecker != nil {
		providerHealth = h.healthChecker.GetHealthReport()
	}

	c.JSON(http.StatusOK, gin.H{
		"summary":           summary,
		"recent_deliveries": deliveries,
		"total_deliveries":  total,
		"provider_health":   providerHealth,
	})
}

// GetAnalytics returns detailed delivery statistics.
func (h *EmailWebhookHandler) GetAnalytics(c *gin.Context) {
	h.GetDashboard(c)
}

func cleanSendGridMessageID(raw string) string {
	if idx := strings.Index(raw, "."); idx != -1 {
		return raw[:idx]
	}
	return raw
}
