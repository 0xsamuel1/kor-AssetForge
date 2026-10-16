package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := fmt.Sprintf("file:mem_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.BatchTransaction{}, &models.BatchQueueEntry{}, &models.Asset{}, &models.Transaction{}, &models.Listing{})
	require.NoError(t, err)
	return db
}

func TestBatchRetry_CalculateBackoff(t *testing.T) {
	cfg := config.BatchConfig{
		MaxBatchSize:      50,
		MaxRetries:        3,
		InitialBackoff:    2 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}

	retryService := NewBatchRetryService(nil, nil, cfg)

	// Attempt 0 -> InitialBackoff (2s)
	assert.Equal(t, 2*time.Second, retryService.CalculateBackoff(0))

	// Attempt 1 -> 2s * 2^1 = 4s
	assert.Equal(t, 4*time.Second, retryService.CalculateBackoff(1))

	// Attempt 2 -> 2s * 2^2 = 8s
	assert.Equal(t, 8*time.Second, retryService.CalculateBackoff(2))

	// Attempt 3 -> 2s * 2^3 = 16s
	assert.Equal(t, 16*time.Second, retryService.CalculateBackoff(3))

	// Attempt 5 -> 2s * 2^5 = 64s -> capped at MaxBackoff (30s)
	assert.Equal(t, 30*time.Second, retryService.CalculateBackoff(5))
}

func TestBatchRetry_IsTransientError(t *testing.T) {
	retryService := NewBatchRetryService(nil, nil, config.LoadBatchConfig())

	// Transient errors
	assert.True(t, retryService.IsTransientError(errors.New("connection reset by peer"), ""))
	assert.True(t, retryService.IsTransientError(errors.New("context deadline exceeded (Client.Timeout)"), ""))
	assert.True(t, retryService.IsTransientError(errors.New("deadlock detected"), ""))
	assert.True(t, retryService.IsTransientError(errors.New("HTTP 429 Too Many Requests"), ""))
	assert.True(t, retryService.IsTransientError(nil, "lock wait timeout exceeded; try restarting transaction"))
	assert.True(t, retryService.IsTransientError(errors.New("batch queue is full"), ""))

	// Non-transient errors
	assert.False(t, retryService.IsTransientError(errors.New("invalid signature"), ""))
	assert.False(t, retryService.IsTransientError(errors.New("asset not found"), ""))
	assert.False(t, retryService.IsTransientError(errors.New("insufficient balance"), ""))
	assert.False(t, retryService.IsTransientError(nil, ""))
}

func TestBatchRetry_ScheduleRetry_Success(t *testing.T) {
	db := setupTestDB(t)
	processor := NewBatchProcessor(db, 1)
	defer processor.Stop()

	cfg := config.BatchConfig{
		MaxBatchSize:      50,
		MaxRetries:        3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        1 * time.Second,
		BackoffMultiplier: 2.0,
	}

	retryService := NewBatchRetryService(db, processor, cfg)

	batch := models.BatchTransaction{
		UserID:          1,
		Operations:      `[{"type":"transfer","amount":100}]`,
		Status:          models.BatchStatusFailed,
		TotalOperations: 1,
		IsRetryable:     true,
		MaxRetries:      3,
	}
	require.NoError(t, db.Create(&batch).Error)

	// First transient failure should schedule retry #1
	scheduled, err := retryService.ScheduleRetry(batch.ID, errors.New("database lock timeout"))
	require.NoError(t, err)
	assert.True(t, scheduled)

	var updated models.BatchTransaction
	db.First(&updated, batch.ID)
	assert.Equal(t, models.BatchStatusQueued, updated.Status)
	assert.Equal(t, 1, updated.RetryCount)
	assert.NotNil(t, updated.NextRetryAt)
	assert.True(t, updated.IsRetryable)

	// Verify queue entry was created/updated
	var queueEntry models.BatchQueueEntry
	err = db.Where("batch_id = ?", batch.ID).First(&queueEntry).Error
	require.NoError(t, err)
	assert.Equal(t, 1, queueEntry.AttemptCount)
}

func TestBatchRetry_ScheduleRetry_MaxRetriesExceeded(t *testing.T) {
	db := setupTestDB(t)
	retryService := NewBatchRetryService(db, nil, config.LoadBatchConfig())

	batch := models.BatchTransaction{
		UserID:          1,
		Operations:      `[]`,
		Status:          models.BatchStatusFailed,
		TotalOperations: 1,
		RetryCount:      3,
		MaxRetries:      3,
		IsRetryable:     true,
	}
	require.NoError(t, db.Create(&batch).Error)

	scheduled, err := retryService.ScheduleRetry(batch.ID, errors.New("timeout"))
	require.NoError(t, err)
	assert.False(t, scheduled)

	var updated models.BatchTransaction
	db.First(&updated, batch.ID)
	assert.Equal(t, models.BatchStatusFailed, updated.Status)
	assert.False(t, updated.IsRetryable)
	assert.Contains(t, updated.ErrorDetails, "max retries of 3 exceeded")
}

func TestBatchRetry_ScheduleRetry_NonTransientError(t *testing.T) {
	db := setupTestDB(t)
	retryService := NewBatchRetryService(db, nil, config.LoadBatchConfig())

	batch := models.BatchTransaction{
		UserID:          1,
		Operations:      `[]`,
		Status:          models.BatchStatusFailed,
		TotalOperations: 1,
		RetryCount:      0,
		MaxRetries:      3,
		IsRetryable:     true,
	}
	require.NoError(t, db.Create(&batch).Error)

	scheduled, err := retryService.ScheduleRetry(batch.ID, errors.New("asset not found"))
	require.NoError(t, err)
	assert.False(t, scheduled)

	var updated models.BatchTransaction
	db.First(&updated, batch.ID)
	assert.Equal(t, models.BatchStatusFailed, updated.Status)
	assert.False(t, updated.IsRetryable)
	assert.Contains(t, updated.ErrorDetails, "non-transient failure")
}

func TestBatchRetry_ManualRetry(t *testing.T) {
	db := setupTestDB(t)
	processor := NewBatchProcessor(db, 1)
	defer processor.Stop()

	retryService := NewBatchRetryService(db, processor, config.LoadBatchConfig())

	batch := models.BatchTransaction{
		UserID:          42,
		Operations:      `[{"type":"transfer","amount":50}]`,
		Status:          models.BatchStatusFailed,
		TotalOperations: 1,
		RetryCount:      2,
		MaxRetries:      3,
		IsRetryable:     false,
	}
	require.NoError(t, db.Create(&batch).Error)

	// Unauthorized user should fail
	_, err := retryService.ManualRetry(batch.ID, 999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")

	// Authorized user retry succeeds
	retriedBatch, err := retryService.ManualRetry(batch.ID, 42)
	require.NoError(t, err)
	assert.Equal(t, models.BatchStatusQueued, retriedBatch.Status)
	assert.Equal(t, 3, retriedBatch.RetryCount)
	assert.True(t, retriedBatch.IsRetryable)

	// Queue entry must exist
	var q models.BatchQueueEntry
	err = db.Where("batch_id = ?", batch.ID).First(&q).Error
	require.NoError(t, err)
	assert.Equal(t, 3, q.AttemptCount)
}

func TestBatchRetry_ProcessPendingRetries(t *testing.T) {
	db := setupTestDB(t)
	processor := NewBatchProcessor(db, 1)
	defer processor.Stop()

	retryService := NewBatchRetryService(db, processor, config.LoadBatchConfig())

	// Insert due batch queue entry
	past := time.Now().Add(-10 * time.Second)
	entry := models.BatchQueueEntry{
		BatchID:      101,
		ScheduledAt:  past,
		AttemptCount: 1,
		MaxAttempts:  3,
	}
	require.NoError(t, db.Create(&entry).Error)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := retryService.ProcessPendingRetries(ctx)
	require.NoError(t, err)

	// Verify entry was marked as processed
	var updated models.BatchQueueEntry
	db.First(&updated, entry.ID)
	assert.NotNil(t, updated.ProcessedAt)
}
