package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

const (
	DefaultBounceRateAlertThreshold = 5.0 // 5% bounce rate threshold
)

// EmailAnalyticsService provides email event tracking, suppression handling, and delivery analytics.
type EmailAnalyticsService interface {
	RecordSent(messageID, recipient, subject, provider string) (*models.EmailDelivery, error)
	RecordDelivery(messageID, recipient string) (*models.EmailDelivery, error)
	RecordBounce(messageID, recipient string, bounceType models.BounceType, reason string) (*models.EmailDelivery, error)
	RecordOpen(messageID, recipient string) (*models.EmailDelivery, error)
	RecordClick(messageID, recipient string) (*models.EmailDelivery, error)
	IsSuppressed(recipient string) bool
	SuppressRecipient(recipient, reason string) error
	GetAnalyticsSummary(since *time.Time) (*models.EmailAnalyticsSummary, error)
	ListDeliveries(recipient string, status models.EmailDeliveryStatus, limit, offset int) ([]models.EmailDelivery, int64, error)
	CheckBounceRateAlert(thresholdPercent float64) (bool, float64, string, error)
}

type emailAnalyticsService struct {
	db                     *gorm.DB
	bounceRateThreshold    float64
}

// NewEmailAnalyticsService initializes an email analytics and tracking service.
func NewEmailAnalyticsService(db *gorm.DB) EmailAnalyticsService {
	return &emailAnalyticsService{
		db:                  db,
		bounceRateThreshold: DefaultBounceRateAlertThreshold,
	}
}

// RecordSent records an email sent event.
func (s *emailAnalyticsService) RecordSent(messageID, recipient, subject, provider string) (*models.EmailDelivery, error) {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	if recipient == "" {
		return nil, errors.New("recipient is required")
	}

	now := time.Now()
	delivery := models.EmailDelivery{
		MessageID: messageID,
		Recipient: recipient,
		Subject:   subject,
		Provider:  provider,
		Status:    models.EmailStatusSent,
		SentAt:    &now,
	}

	if err := s.db.Create(&delivery).Error; err != nil {
		return nil, fmt.Errorf("failed to record sent email: %w", err)
	}
	return &delivery, nil
}

// RecordDelivery updates delivery status when a confirmation webhook is received.
func (s *emailAnalyticsService) RecordDelivery(messageID, recipient string) (*models.EmailDelivery, error) {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	now := time.Now()

	var delivery models.EmailDelivery
	query := s.db.Model(&delivery)
	if messageID != "" {
		query = query.Where("message_id = ?", messageID)
	} else if recipient != "" {
		query = query.Where("recipient = ?", recipient).Order("id DESC")
	} else {
		return nil, errors.New("messageID or recipient is required")
	}

	if err := query.First(&delivery).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create entry if not previously recorded
			delivery = models.EmailDelivery{
				MessageID:   messageID,
				Recipient:   recipient,
				Status:      models.EmailStatusDelivered,
				DeliveredAt: &now,
			}
			if err := s.db.Create(&delivery).Error; err != nil {
				return nil, err
			}
			return &delivery, nil
		}
		return nil, err
	}

	delivery.Status = models.EmailStatusDelivered
	delivery.DeliveredAt = &now
	if err := s.db.Save(&delivery).Error; err != nil {
		return nil, err
	}
	return &delivery, nil
}

// RecordBounce records a bounce event, classifies bounce type, and adds hard bounces to suppression.
func (s *emailAnalyticsService) RecordBounce(messageID, recipient string, bounceType models.BounceType, reason string) (*models.EmailDelivery, error) {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	if recipient == "" && messageID == "" {
		return nil, errors.New("recipient or messageID is required")
	}

	if bounceType == "" {
		bounceType = models.BounceTypeSoft
	}

	now := time.Now()
	var delivery models.EmailDelivery
	query := s.db.Model(&delivery)
	if messageID != "" {
		query = query.Where("message_id = ?", messageID)
	} else {
		query = query.Where("recipient = ?", recipient).Order("id DESC")
	}

	if err := query.First(&delivery).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			delivery = models.EmailDelivery{
				MessageID:    messageID,
				Recipient:    recipient,
				Status:       models.EmailStatusBounced,
				BounceType:   bounceType,
				BounceReason: reason,
				BouncedAt:    &now,
			}
			if err := s.db.Create(&delivery).Error; err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		delivery.Status = models.EmailStatusBounced
		delivery.BounceType = bounceType
		delivery.BounceReason = reason
		delivery.BouncedAt = &now
		if err := s.db.Save(&delivery).Error; err != nil {
			return nil, err
		}
	}

	// Hard bounces automatically trigger suppression
	if bounceType == models.BounceTypeHard && recipient != "" {
		_ = s.SuppressRecipient(recipient, fmt.Sprintf("Hard bounce: %s", reason))
	}

	return &delivery, nil
}

// RecordOpen tracks open events on delivered emails.
func (s *emailAnalyticsService) RecordOpen(messageID, recipient string) (*models.EmailDelivery, error) {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	now := time.Now()

	var delivery models.EmailDelivery
	query := s.db.Model(&delivery)
	if messageID != "" {
		query = query.Where("message_id = ?", messageID)
	} else if recipient != "" {
		query = query.Where("recipient = ?", recipient).Order("id DESC")
	} else {
		return nil, errors.New("messageID or recipient is required")
	}

	if err := query.First(&delivery).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			delivery = models.EmailDelivery{
				MessageID: messageID,
				Recipient: recipient,
				Status:    models.EmailStatusOpened,
				OpenedAt:  &now,
			}
			if err := s.db.Create(&delivery).Error; err != nil {
				return nil, err
			}
			return &delivery, nil
		}
		return nil, err
	}

	delivery.OpenedAt = &now
	if delivery.Status != models.EmailStatusClicked {
		delivery.Status = models.EmailStatusOpened
	}
	if err := s.db.Save(&delivery).Error; err != nil {
		return nil, err
	}
	return &delivery, nil
}

// RecordClick tracks link click events on delivered emails.
func (s *emailAnalyticsService) RecordClick(messageID, recipient string) (*models.EmailDelivery, error) {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	now := time.Now()

	var delivery models.EmailDelivery
	query := s.db.Model(&delivery)
	if messageID != "" {
		query = query.Where("message_id = ?", messageID)
	} else if recipient != "" {
		query = query.Where("recipient = ?", recipient).Order("id DESC")
	} else {
		return nil, errors.New("messageID or recipient is required")
	}

	if err := query.First(&delivery).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			delivery = models.EmailDelivery{
				MessageID: messageID,
				Recipient: recipient,
				Status:    models.EmailStatusClicked,
				ClickedAt: &now,
			}
			if err := s.db.Create(&delivery).Error; err != nil {
				return nil, err
			}
			return &delivery, nil
		}
		return nil, err
	}

	delivery.ClickedAt = &now
	delivery.Status = models.EmailStatusClicked
	if err := s.db.Save(&delivery).Error; err != nil {
		return nil, err
	}
	return &delivery, nil
}

// IsSuppressed checks whether a recipient address is suppressed.
func (s *emailAnalyticsService) IsSuppressed(recipient string) bool {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	if recipient == "" {
		return false
	}
	var count int64
	s.db.Model(&models.EmailSuppression{}).Where("recipient = ?", recipient).Count(&count)
	return count > 0
}

// SuppressRecipient adds a recipient address to suppression list.
func (s *emailAnalyticsService) SuppressRecipient(recipient, reason string) error {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	if recipient == "" {
		return errors.New("recipient is required")
	}

	var supp models.EmailSuppression
	err := s.db.Where("recipient = ?", recipient).First(&supp).Error
	if err == nil {
		supp.Reason = reason
		return s.db.Save(&supp).Error
	}

	supp = models.EmailSuppression{
		Recipient: recipient,
		Reason:    reason,
		CreatedAt: time.Now(),
	}
	return s.db.Create(&supp).Error
}

// GetAnalyticsSummary computes aggregate metrics, rates, and alerts.
func (s *emailAnalyticsService) GetAnalyticsSummary(since *time.Time) (*models.EmailAnalyticsSummary, error) {
	var totalSent, totalDelivered, totalBounced, totalOpened, totalClicked int64

	baseQuery := s.db.Model(&models.EmailDelivery{})
	if since != nil {
		baseQuery = baseQuery.Where("created_at >= ?", *since)
	}

	if err := baseQuery.Count(&totalSent).Error; err != nil {
		return nil, err
	}

	deliveredQuery := s.db.Model(&models.EmailDelivery{}).
		Where("status IN ?", []models.EmailDeliveryStatus{models.EmailStatusDelivered, models.EmailStatusOpened, models.EmailStatusClicked})
	if since != nil {
		deliveredQuery = deliveredQuery.Where("created_at >= ?", *since)
	}
	if err := deliveredQuery.Count(&totalDelivered).Error; err != nil {
		return nil, err
	}

	bouncedQuery := s.db.Model(&models.EmailDelivery{}).Where("status = ?", models.EmailStatusBounced)
	if since != nil {
		bouncedQuery = bouncedQuery.Where("created_at >= ?", *since)
	}
	if err := bouncedQuery.Count(&totalBounced).Error; err != nil {
		return nil, err
	}

	openedQuery := s.db.Model(&models.EmailDelivery{}).Where("opened_at IS NOT NULL")
	if since != nil {
		openedQuery = openedQuery.Where("created_at >= ?", *since)
	}
	if err := openedQuery.Count(&totalOpened).Error; err != nil {
		return nil, err
	}

	clickedQuery := s.db.Model(&models.EmailDelivery{}).Where("clicked_at IS NOT NULL")
	if since != nil {
		clickedQuery = clickedQuery.Where("created_at >= ?", *since)
	}
	if err := clickedQuery.Count(&totalClicked).Error; err != nil {
		return nil, err
	}

	var deliveryRate, bounceRate, openRate float64
	if totalSent > 0 {
		deliveryRate = (float64(totalDelivered) / float64(totalSent)) * 100.0
		bounceRate = (float64(totalBounced) / float64(totalSent)) * 100.0
		openRate = (float64(totalOpened) / float64(totalSent)) * 100.0
	}

	highBounce := bounceRate >= s.bounceRateThreshold && totalSent >= 10
	var alertMsg string
	if highBounce {
		alertMsg = fmt.Sprintf("High bounce rate detected: %.2f%% exceeds threshold (%.2f%%)", bounceRate, s.bounceRateThreshold)
	}

	return &models.EmailAnalyticsSummary{
		TotalSent:           totalSent,
		TotalDelivered:      totalDelivered,
		TotalBounced:        totalBounced,
		TotalOpened:         totalOpened,
		TotalClicked:        totalClicked,
		DeliveryRate:        deliveryRate,
		BounceRate:          bounceRate,
		OpenRate:            openRate,
		HighBounceRateAlert: highBounce,
		AlertMessage:        alertMsg,
	}, nil
}

// ListDeliveries retrieves paginated delivery records.
func (s *emailAnalyticsService) ListDeliveries(recipient string, status models.EmailDeliveryStatus, limit, offset int) ([]models.EmailDelivery, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := s.db.Model(&models.EmailDelivery{})
	if recipient != "" {
		query = query.Where("recipient = ?", strings.ToLower(strings.TrimSpace(recipient)))
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []models.EmailDelivery
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// CheckBounceRateAlert evaluates if the current bounce rate exceeds threshold.
func (s *emailAnalyticsService) CheckBounceRateAlert(thresholdPercent float64) (bool, float64, string, error) {
	if thresholdPercent <= 0 {
		thresholdPercent = s.bounceRateThreshold
	}

	summary, err := s.GetAnalyticsSummary(nil)
	if err != nil {
		return false, 0, "", err
	}

	alert := summary.BounceRate >= thresholdPercent && summary.TotalSent >= 10
	var msg string
	if alert {
		msg = fmt.Sprintf("High bounce rate alert: current bounce rate %.2f%% exceeds threshold %.2f%%", summary.BounceRate, thresholdPercent)
	}
	return alert, summary.BounceRate, msg, nil
}
