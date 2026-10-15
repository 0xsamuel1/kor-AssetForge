package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEmailTestDB(t *testing.T) *gorm.DB {
	dsn := fmt.Sprintf("file:memdb_email_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.EmailDelivery{}, &models.EmailSuppression{})
	require.NoError(t, err)
	return db
}

func TestEmailAnalytics_RecordSentAndDelivered(t *testing.T) {
	db := setupEmailTestDB(t)
	analytics := NewEmailAnalyticsService(db)

	delivery, err := analytics.RecordSent("msg-123", "alice@example.com", "Welcome", "sendgrid")
	require.NoError(t, err)
	assert.Equal(t, models.EmailStatusSent, delivery.Status)
	assert.Equal(t, "alice@example.com", delivery.Recipient)
	assert.NotNil(t, delivery.SentAt)

	// Update to delivered via messageID
	updated, err := analytics.RecordDelivery("msg-123", "alice@example.com")
	require.NoError(t, err)
	assert.Equal(t, models.EmailStatusDelivered, updated.Status)
	assert.NotNil(t, updated.DeliveredAt)
}

func TestEmailAnalytics_RecordBounceAndSuppression(t *testing.T) {
	db := setupEmailTestDB(t)
	analytics := NewEmailAnalyticsService(db)

	// Soft bounce does not suppress
	_, err := analytics.RecordBounce("msg-soft", "soft@example.com", models.BounceTypeSoft, "Mailbox full")
	require.NoError(t, err)
	assert.False(t, analytics.IsSuppressed("soft@example.com"))

	// Hard bounce triggers suppression
	_, err = analytics.RecordBounce("msg-hard", "hard@example.com", models.BounceTypeHard, "550 User unknown")
	require.NoError(t, err)
	assert.True(t, analytics.IsSuppressed("hard@example.com"))
}

func TestEmailAnalytics_RecordOpenAndClick(t *testing.T) {
	db := setupEmailTestDB(t)
	analytics := NewEmailAnalyticsService(db)

	_, err := analytics.RecordSent("msg-track", "charlie@example.com", "Newsletter", "ses")
	require.NoError(t, err)

	// Record open
	opened, err := analytics.RecordOpen("msg-track", "charlie@example.com")
	require.NoError(t, err)
	assert.Equal(t, models.EmailStatusOpened, opened.Status)
	assert.NotNil(t, opened.OpenedAt)

	// Record click
	clicked, err := analytics.RecordClick("msg-track", "charlie@example.com")
	require.NoError(t, err)
	assert.Equal(t, models.EmailStatusClicked, clicked.Status)
	assert.NotNil(t, clicked.ClickedAt)
}

func TestEmailAnalytics_SummaryAndHighBounceAlert(t *testing.T) {
	db := setupEmailTestDB(t)
	analytics := NewEmailAnalyticsService(db)

	// Create 10 sent emails, 8 delivered, 2 bounced (20% bounce rate > 5% threshold)
	for i := 0; i < 8; i++ {
		msgID := fmt.Sprintf("msg-ok-%d", i)
		rec := fmt.Sprintf("user%d@example.com", i)
		_, _ = analytics.RecordSent(msgID, rec, "Test", "sendgrid")
		_, _ = analytics.RecordDelivery(msgID, rec)
		if i < 4 {
			_, _ = analytics.RecordOpen(msgID, rec)
		}
	}
	for i := 0; i < 2; i++ {
		msgID := fmt.Sprintf("msg-bounce-%d", i)
		rec := fmt.Sprintf("bounce%d@example.com", i)
		_, _ = analytics.RecordSent(msgID, rec, "Test", "sendgrid")
		_, _ = analytics.RecordBounce(msgID, rec, models.BounceTypeHard, "Bad address")
	}

	summary, err := analytics.GetAnalyticsSummary(nil)
	require.NoError(t, err)

	assert.Equal(t, int64(10), summary.TotalSent)
	assert.Equal(t, int64(8), summary.TotalDelivered)
	assert.Equal(t, int64(2), summary.TotalBounced)
	assert.Equal(t, int64(4), summary.TotalOpened)
	assert.Equal(t, 80.0, summary.DeliveryRate)
	assert.Equal(t, 20.0, summary.BounceRate)
	assert.Equal(t, 40.0, summary.OpenRate)
	assert.True(t, summary.HighBounceRateAlert)
	assert.Contains(t, summary.AlertMessage, "High bounce rate detected")

	alert, rate, msg, err := analytics.CheckBounceRateAlert(5.0)
	require.NoError(t, err)
	assert.True(t, alert)
	assert.Equal(t, 20.0, rate)
	assert.Contains(t, msg, "High bounce rate alert")
}

func TestEmailAnalytics_ListDeliveries(t *testing.T) {
	db := setupEmailTestDB(t)
	analytics := NewEmailAnalyticsService(db)

	for i := 0; i < 5; i++ {
		_, _ = analytics.RecordSent(fmt.Sprintf("msg-list-%d", i), fmt.Sprintf("user%d@example.com", i), "Hi", "smtp")
	}

	items, total, err := analytics.ListDeliveries("", "", 3, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, items, 3)

	filtered, fTotal, err := analytics.ListDeliveries("user1@example.com", "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), fTotal)
	assert.Len(t, filtered, 1)
}
