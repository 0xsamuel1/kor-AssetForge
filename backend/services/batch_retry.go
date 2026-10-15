package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// BatchRetryService manages automated and manual retries for batch transactions.
type BatchRetryService struct {
	db        *gorm.DB
	processor *BatchProcessor
	config    config.BatchConfig
}

// NewBatchRetryService creates a new BatchRetryService instance.
func NewBatchRetryService(db *gorm.DB, processor *BatchProcessor, cfg config.BatchConfig) *BatchRetryService {
	return &BatchRetryService{
		db:        db,
		processor: processor,
		config:    cfg,
	}
}

// CalculateBackoff computes the exponential backoff duration based on the retry attempt.
func (s *BatchRetryService) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return s.config.InitialBackoff
	}

	multiplier := s.config.BackoffMultiplier
	if multiplier <= 1.0 {
		multiplier = 2.0
	}

	factor := math.Pow(multiplier, float64(attempt))
	backoff := time.Duration(float64(s.config.InitialBackoff) * factor)

	if backoff > s.config.MaxBackoff {
		backoff = s.config.MaxBackoff
	}
	return backoff
}

// IsTransientError identifies whether a given error represents a transient failure eligible for automatic retry.
func (s *BatchRetryService) IsTransientError(err error, errDetails string) bool {
	if err == nil && errDetails == "" {
		return false
	}

	var combined string
	if err != nil {
		combined = strings.ToLower(err.Error() + " " + errDetails)
	} else {
		combined = strings.ToLower(errDetails)
	}

	transientKeywords := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"network error",
		"broken pipe",
		"deadlock",
		"lock wait timeout",
		"lock timeout",
		"too many requests",
		"rate limit",
		"429",
		"503",
		"service unavailable",
		"bad gateway",
		"502",
		"queue is full",
		"temporary failure",
		"try again",
		"resource temporarily unavailable",
	}

	for _, kw := range transientKeywords {
		if strings.Contains(combined, kw) {
			return true
		}
	}
	return false
}

// ScheduleRetry checks if a failed batch is eligible for retry, updates its schedule with exponential backoff, and queues it.
func (s *BatchRetryService) ScheduleRetry(batchID uint, causeErr error) (bool, error) {
	var batch models.BatchTransaction
	if err := s.db.First(&batch, batchID).Error; err != nil {
		return false, fmt.Errorf("batch %d not found: %w", batchID, err)
	}

	maxRetries := s.config.MaxRetries
	if batch.MaxRetries > 0 {
		maxRetries = batch.MaxRetries
	}

	errStr := ""
	if causeErr != nil {
		errStr = causeErr.Error()
	}

	// Check retryability
	if !batch.IsRetryable || batch.RetryCount >= maxRetries {
		s.db.Model(&batch).Updates(map[string]interface{}{
			"status":        models.BatchStatusFailed,
			"is_retryable":  false,
			"error_details": fmt.Sprintf("%s (max retries of %d exceeded)", errStr, maxRetries),
		})
		return false, nil
	}

	// Verify error is transient
	if !s.IsTransientError(causeErr, batch.ErrorDetails) {
		s.db.Model(&batch).Updates(map[string]interface{}{
			"status":        models.BatchStatusFailed,
			"is_retryable":  false,
			"error_details": fmt.Sprintf("%s (non-transient failure)", errStr),
		})
		return false, nil
	}

	backoff := s.CalculateBackoff(batch.RetryCount)
	now := time.Now()
	nextRetry := now.Add(backoff)
	newRetryCount := batch.RetryCount + 1

	if err := s.db.Model(&batch).Updates(map[string]interface{}{
		"status":        models.BatchStatusQueued,
		"retry_count":   newRetryCount,
		"last_retry_at": &now,
		"next_retry_at": &nextRetry,
		"error_details": errStr,
	}).Error; err != nil {
		return false, fmt.Errorf("failed to update batch retry status: %w", err)
	}

	// Upsert queue entry with new scheduled time
	var queueEntry models.BatchQueueEntry
	if err := s.db.Where("batch_id = ?", batchID).First(&queueEntry).Error; err != nil {
		queueEntry = models.BatchQueueEntry{
			BatchID:      batchID,
			Priority:     0,
			AttemptCount: newRetryCount,
			MaxAttempts:  maxRetries,
			ScheduledAt:  nextRetry,
		}
		s.db.Create(&queueEntry)
	} else {
		s.db.Model(&queueEntry).Updates(map[string]interface{}{
			"attempt_count": newRetryCount,
			"scheduled_at":  nextRetry,
			"processed_at":  nil,
		})
	}

	// If backoff is immediate (or <= 0), enqueue immediately
	if backoff <= 0 && s.processor != nil {
		_ = s.processor.Enqueue(batchID)
	}

	return true, nil
}

// ManualRetry allows users or operators to manually trigger a retry for a failed or partially completed batch.
func (s *BatchRetryService) ManualRetry(batchID uint, userID uint) (*models.BatchTransaction, error) {
	var batch models.BatchTransaction
	if err := s.db.First(&batch, batchID).Error; err != nil {
		return nil, fmt.Errorf("batch not found")
	}

	// Authorization check
	if userID != 0 && batch.UserID != userID {
		return nil, errors.New("access denied")
	}

	// Only failed or completed_with_errors batches can be manually retried
	if batch.Status != models.BatchStatusFailed &&
		batch.Status != models.BatchStatusCompletedWithErrors &&
		batch.Status != models.BatchStatusRolledBack {
		return nil, fmt.Errorf("cannot retry batch with status '%s'; only failed or rolled back batches can be retried", batch.Status)
	}

	now := time.Now()
	newRetryCount := batch.RetryCount + 1

	updates := map[string]interface{}{
		"status":        models.BatchStatusQueued,
		"retry_count":   newRetryCount,
		"last_retry_at": &now,
		"next_retry_at": &now,
		"error_details": "",
		"is_retryable":  true,
	}

	if err := s.db.Model(&batch).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to reset batch status: %w", err)
	}

	// Upsert queue entry
	var queueEntry models.BatchQueueEntry
	if err := s.db.Where("batch_id = ?", batchID).First(&queueEntry).Error; err != nil {
		queueEntry = models.BatchQueueEntry{
			BatchID:      batchID,
			Priority:     1, // Higher priority for manual retry
			AttemptCount: newRetryCount,
			MaxAttempts:  batch.MaxRetries,
			ScheduledAt:  now,
		}
		s.db.Create(&queueEntry)
	} else {
		s.db.Model(&queueEntry).Updates(map[string]interface{}{
			"attempt_count": newRetryCount,
			"scheduled_at":  now,
			"processed_at":  nil,
		})
	}

	if s.processor != nil {
		if err := s.processor.Enqueue(batchID); err != nil {
			log.Printf("batch_retry: failed to enqueue batch %d immediately: %v", batchID, err)
		}
	}

	batch.Status = models.BatchStatusQueued
	batch.RetryCount = newRetryCount
	batch.LastRetryAt = &now
	batch.NextRetryAt = &now
	return &batch, nil
}

// ProcessPendingRetries checks for queued batches whose ScheduledAt has passed and pushes them to the processor.
func (s *BatchRetryService) ProcessPendingRetries(ctx context.Context) error {
	var entries []models.BatchQueueEntry
	now := time.Now()

	err := s.db.Where("scheduled_at <= ? AND processed_at IS NULL", now).
		Order("priority desc, scheduled_at asc").
		Limit(50).
		Find(&entries).Error
	if err != nil {
		return err
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if s.processor != nil {
			if err := s.processor.Enqueue(entry.BatchID); err == nil {
				s.db.Model(&entry).Update("processed_at", &now)
			}
		}
	}
	return nil
}
