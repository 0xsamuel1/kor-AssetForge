package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

const (
	maxRetries        = 5
	initialBackoff    = 1 * time.Minute
	dlqRetentionDays  = 30
)

var backoffSchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
}

type retryEntry struct {
	Attempt    int       `json:"attempt"`
	Timestamp  time.Time `json:"timestamp"`
	HTTPStatus int       `json:"http_status"`
	Error      string    `json:"error,omitempty"`
}

type WebhookDeliveryService struct {
	db         *gorm.DB
	httpClient *http.Client
}

func NewWebhookDeliveryService(db *gorm.DB) *WebhookDeliveryService {
	return &WebhookDeliveryService{
		db: db,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *WebhookDeliveryService) Dispatch(eventType models.WebhookEventType, payload interface{}) {
	raw, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[webhook] failed to marshal payload: %v", err)
		return
	}

	var subs []models.WebhookSubscription
	s.db.Where("active = ?", true).Find(&subs)

	for _, sub := range subs {
		if !s.subscribedTo(sub, eventType) {
			continue
		}

		entry := models.WebhookDeliveryLog{
			SubscriptionID: sub.ID,
			EventType:      eventType,
			Payload:        string(raw),
			Status:         models.WebhookDeliveryPending,
			MaxRetries:     maxRetries,
		}
		if err := s.db.Create(&entry).Error; err != nil {
			log.Printf("[webhook] failed to create delivery log: %v", err)
			continue
		}

		go s.deliver(sub, &entry)
	}
}

func (s *WebhookDeliveryService) RetryPending() {
	var entries []models.WebhookDeliveryLog
	now := time.Now()
	s.db.Where(
		"status IN ? AND (next_retry_at IS NULL OR next_retry_at <= ?) AND attempt_count < max_retries",
		[]string{
			string(models.WebhookDeliveryPending),
			string(models.WebhookDeliveryRetrying),
			string(models.WebhookDeliveryFailed),
		},
		now,
	).Find(&entries)

	for i := range entries {
		var sub models.WebhookSubscription
		if err := s.db.First(&sub, entries[i].SubscriptionID).Error; err != nil {
			continue
		}
		go s.deliver(sub, &entries[i])
	}
}

func (s *WebhookDeliveryService) RetrySpecific(entryID uint) error {
	var entry models.WebhookDeliveryLog
	if err := s.db.First(&entry, entryID).Error; err != nil {
		return fmt.Errorf("delivery log not found: %w", err)
	}

	if entry.Status == models.WebhookDeliverySuccess {
		return fmt.Errorf("delivery already succeeded")
	}

	var sub models.WebhookSubscription
	if err := s.db.First(&sub, entry.SubscriptionID).Error; err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	entry.AttemptCount = 0
	entry.Status = models.WebhookDeliveryPending
	entry.LastError = ""
	entry.DLQReason = ""
	entry.MaxRetries = maxRetries
	s.db.Save(&entry)

	go s.deliver(sub, &entry)
	return nil
}

func (s *WebhookDeliveryService) RetryAllFailed() (int, error) {
	var entries []models.WebhookDeliveryLog
	s.db.Where(
		"status IN ?",
		[]string{
			string(models.WebhookDeliveryFailed),
			string(models.WebhookDeliveryAbandoned),
		},
	).Find(&entries)

	count := 0
	for i := range entries {
		if err := s.RetrySpecific(entries[i].ID); err == nil {
			count++
		}
	}
	return count, nil
}

func (s *WebhookDeliveryService) GetDeliveryDashboard() (map[string]interface{}, error) {
	var total, success, failed, pending, abandoned int64
	s.db.Model(&models.WebhookDeliveryLog{}).Count(&total)
	s.db.Model(&models.WebhookDeliveryLog{}).Where("status = ?", models.WebhookDeliverySuccess).Count(&success)
	s.db.Model(&models.WebhookDeliveryLog{}).Where("status IN ?", []string{string(models.WebhookDeliveryFailed), string(models.WebhookDeliveryAbandoned)}).Count(&failed)
	s.db.Model(&models.WebhookDeliveryLog{}).Where("status = ?", models.WebhookDeliveryPending).Count(&pending)
	s.db.Model(&models.WebhookDeliveryLog{}).Where("status = ?", models.WebhookDeliveryAbandoned).Count(&abandoned)

	return map[string]interface{}{
		"total_deliveries":    total,
		"successful":          success,
		"failed":              failed,
		"pending":             pending,
		"abandoned":           abandoned,
		"success_rate":        fmt.Sprintf("%.2f%%", float64(success)/float64(total+1)*100),
	}, nil
}

func (s *WebhookDeliveryService) deliver(sub models.WebhookSubscription, entry *models.WebhookDeliveryLog) {
	if entry.MaxRetries == 0 {
		entry.MaxRetries = maxRetries
	}

	entry.AttemptCount++
	entry.Status = models.WebhookDeliveryRetrying

	retryHist := entry.RetryHistory
	var history []retryEntry
	if retryHist != "" {
		json.Unmarshal([]byte(retryHist), &history)
	}

	sig := s.sign([]byte(entry.Payload), sub.Secret)
	req, err := http.NewRequest(http.MethodPost, sub.URL, bytes.NewBufferString(entry.Payload))
	if err != nil {
		s.markFailed(entry, history, 0, fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AssetForge-Signature", sig)
	req.Header.Set("X-AssetForge-Event", string(entry.EventType))
	req.Header.Set("X-AssetForge-Delivery", fmt.Sprintf("%d", entry.ID))
	req.Header.Set("X-AssetForge-Attempt", fmt.Sprintf("%d", entry.AttemptCount))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		history = append(history, retryEntry{
			Attempt:   entry.AttemptCount,
			Timestamp: time.Now(),
			Error:     err.Error(),
		})
		s.scheduleRetry(entry, history, 0, fmt.Sprintf("http error: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	entry.HTTPStatus = resp.StatusCode
	entry.ResponseBody = string(body)

	history = append(history, retryEntry{
		Attempt:    entry.AttemptCount,
		Timestamp:  time.Now(),
		HTTPStatus: resp.StatusCode,
	})
	entry.LastError = string(body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		now := time.Now()
		entry.Status = models.WebhookDeliverySuccess
		entry.DeliveredAt = &now
		entry.RetryHistory = marshalHistory(history)
		entry.LastError = ""
		s.db.Save(entry)
		return
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		history = append(history, retryEntry{
			Attempt:    entry.AttemptCount,
			Timestamp:  time.Now(),
			HTTPStatus: resp.StatusCode,
			Error:      fmt.Sprintf("client error (%d) – not retrying", resp.StatusCode),
		})
		s.moveToDLQ(entry, history, fmt.Sprintf("Client error %d – delivery abandoned", resp.StatusCode))
		return
	}

	if entry.AttemptCount >= entry.MaxRetries {
		history = append(history, retryEntry{
			Attempt:    entry.AttemptCount,
			Timestamp:  time.Now(),
			HTTPStatus: resp.StatusCode,
			Error:      "max retries exceeded",
		})
		s.moveToDLQ(entry, history, "Max retry attempts reached")
		return
	}

	s.scheduleRetry(entry, history, resp.StatusCode, string(body))
}

func (s *WebhookDeliveryService) scheduleRetry(entry *models.WebhookDeliveryLog, history []retryEntry, httpStatus int, responseBody string) {
	backoff := s.calculateBackoff(entry.AttemptCount)
	next := time.Now().Add(backoff)
	entry.Status = models.WebhookDeliveryFailed
	entry.HTTPStatus = httpStatus
	entry.ResponseBody = responseBody
	entry.NextRetryAt = &next
	entry.RetryHistory = marshalHistory(history)
	entry.LastError = responseBody
	s.db.Save(entry)

	log.Printf("[webhook] delivery %d scheduled retry %d/%d at %s (backoff: %s)",
		entry.ID, entry.AttemptCount, entry.MaxRetries, next.Format(time.RFC3339), backoff)
}

func (s *WebhookDeliveryService) calculateBackoff(attempt int) time.Duration {
	idx := attempt - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(backoffSchedule) {
		return backoffSchedule[len(backoffSchedule)-1]
	}
	return backoffSchedule[idx]
}

func (s *WebhookDeliveryService) moveToDLQ(entry *models.WebhookDeliveryLog, history []retryEntry, reason string) {
	entry.Status = models.WebhookDeliveryAbandoned
	entry.DLQReason = reason
	entry.RetryHistory = marshalHistory(history)
	entry.LastError = reason
	s.db.Save(entry)

	log.Printf("[webhook] delivery %d moved to DLQ after %d attempts: %s",
		entry.ID, entry.AttemptCount, reason)
}

func (s *WebhookDeliveryService) markFailed(entry *models.WebhookDeliveryLog, history []retryEntry, httpStatus int, reason string) {
	entry.Status = models.WebhookDeliveryAbandoned
	entry.HTTPStatus = httpStatus
	entry.ResponseBody = reason
	entry.DLQReason = reason
	entry.LastError = reason
	entry.RetryHistory = marshalHistory(history)
	s.db.Save(entry)
}

func (s *WebhookDeliveryService) ReplayFromDLQ() (int, error) {
	var entries []models.WebhookDeliveryLog
	s.db.Where("status = ? AND dlq_reason != ''", models.WebhookDeliveryAbandoned).Find(&entries)

	count := 0
	for i := range entries {
		if err := s.RetrySpecific(entries[i].ID); err == nil {
			count++
		}
	}
	return count, nil
}

func (s *WebhookDeliveryService) PurgeOldDLQ() (int64, error) {
	cutoff := time.Now().Add(-dlqRetentionDays * 24 * time.Hour)
	result := s.db.Where("status = ? AND updated_at < ?", models.WebhookDeliveryAbandoned, cutoff).
		Delete(&models.WebhookDeliveryLog{})
	return result.RowsAffected, result.Error
}

func (s *WebhookDeliveryService) subscribedTo(sub models.WebhookSubscription, event models.WebhookEventType) bool {
	for _, e := range strings.Split(sub.Events, ",") {
		if strings.TrimSpace(e) == string(event) || strings.TrimSpace(e) == "*" {
			return true
		}
	}
	return false
}

func (s *WebhookDeliveryService) sign(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func marshalHistory(history []retryEntry) string {
	b, _ := json.Marshal(history)
	return string(b)
}
