package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWebhookTestRouter(t *testing.T) (*gin.Engine, services.EmailAnalyticsService, services.EmailHealthChecker) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:memdb_wh_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.EmailDelivery{}, &models.EmailSuppression{})
	require.NoError(t, err)

	analytics := services.NewEmailAnalyticsService(db)
	health := services.NewEmailHealthChecker(3)
	handler := NewEmailWebhookHandler(analytics, health)

	r := gin.New()
	r.POST("/webhooks/email/sendgrid", handler.HandleSendGrid)
	r.POST("/webhooks/email/ses", handler.HandleSES)
	r.POST("/webhooks/email/generic", handler.HandleGeneric)
	r.GET("/email/dashboard", handler.GetDashboard)

	return r, analytics, health
}

func TestEmailWebhook_SendGrid(t *testing.T) {
	r, analytics, _ := setupWebhookTestRouter(t)

	// Pre-record sent email
	_, err := analytics.RecordSent("sg-123", "alice@example.com", "Hello", "sendgrid")
	require.NoError(t, err)

	payload := []SendGridEvent{
		{
			Email:       "alice@example.com",
			Event:       "delivered",
			SGMessageID: "sg-123.filterd",
			Timestamp:   time.Now().Unix(),
		},
		{
			Email:       "bad@example.com",
			Event:       "bounce",
			SGMessageID: "sg-456",
			Reason:      "550 Invalid mailbox",
			Type:        "bounce",
			Timestamp:   time.Now().Unix(),
		},
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/webhooks/email/sendgrid", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.Equal(t, "processed", res["status"])
	assert.Equal(t, float64(2), res["count"])

	// Assert bad@example.com was suppressed due to hard bounce
	assert.True(t, analytics.IsSuppressed("bad@example.com"))
}

func TestEmailWebhook_SES(t *testing.T) {
	r, analytics, _ := setupWebhookTestRouter(t)

	payload := SESNotification{
		EventType: "Delivery",
		Mail: &SESMail{
			MessageID:   "ses-msg-1",
			Destination: []string{"bob@example.com"},
		},
		Delivery: &SESDelivery{
			Recipients: []string{"bob@example.com"},
		},
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/webhooks/email/ses", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify delivery recorded
	deliveries, total, err := analytics.ListDeliveries("bob@example.com", models.EmailStatusDelivered, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "bob@example.com", deliveries[0].Recipient)
}

func TestEmailWebhook_Generic(t *testing.T) {
	r, analytics, _ := setupWebhookTestRouter(t)

	reqPayload := GenericEventRequest{
		Event:      "bounce",
		Recipient:  "charlie@example.com",
		MessageID:  "gen-123",
		BounceType: "hard",
		Reason:     "550 Unknown user",
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/webhooks/email/generic", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, analytics.IsSuppressed("charlie@example.com"))
}

func TestEmailWebhook_Dashboard(t *testing.T) {
	r, analytics, health := setupWebhookTestRouter(t)

	_, _ = analytics.RecordSent("d-1", "user1@example.com", "Welcome", "sendgrid")
	_, _ = analytics.RecordDelivery("d-1", "user1@example.com")
	health.RecordSuccess(config.ProviderSendGrid, 25*time.Millisecond)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/email/dashboard", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp["summary"])
	assert.NotNil(t, resp["recent_deliveries"])
	assert.NotNil(t, resp["provider_health"])
}
