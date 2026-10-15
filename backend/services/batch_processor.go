package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

type BatchProcessor struct {
	db           *gorm.DB
	queue        chan uint // batch IDs to process
	workers      int
	retryService *BatchRetryService
	mu           sync.Mutex
	done         chan struct{}
}

func NewBatchProcessor(db *gorm.DB, workers int) *BatchProcessor {
	if workers <= 0 {
		workers = 3
	}
	return &BatchProcessor{
		db:      db,
		queue:   make(chan uint, 200),
		workers: workers,
		done:    make(chan struct{}),
	}
}

// SetRetryService attaches a retry service to the processor
func (p *BatchProcessor) SetRetryService(rs *BatchRetryService) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retryService = rs
}

// Start launches worker goroutines
func (p *BatchProcessor) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		go p.worker(ctx)
	}
}

// Stop signals workers to stop
func (p *BatchProcessor) Stop() {
	close(p.done)
}

// Enqueue adds a batch ID to the processing queue
// Returns error if queue is full (backpressure)
func (p *BatchProcessor) Enqueue(batchID uint) error {
	select {
	case p.queue <- batchID:
		return nil
	default:
		return fmt.Errorf("batch queue is full")
	}
}

// worker processes batches from the queue
func (p *BatchProcessor) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case batchID := <-p.queue:
			p.processBatch(batchID)
		}
	}
}

// processBatch executes all operations in a batch with partial-success support
func (p *BatchProcessor) processBatch(batchID uint) {
	var batch models.BatchTransaction
	if err := p.db.First(&batch, batchID).Error; err != nil {
		log.Printf("batch_processor: batch %d not found: %v", batchID, err)
		return
	}

	if batch.Status != models.BatchStatusQueued {
		return
	}

	// Mark as processing
	p.db.Model(&batch).Update("status", models.BatchStatusProcessing)

	var ops []models.BatchOperation
	if err := json.Unmarshal([]byte(batch.Operations), &ops); err != nil {
		p.db.Model(&batch).Updates(map[string]interface{}{
			"status":        models.BatchStatusFailed,
			"error_details": "failed to parse operations",
		})
		return
	}

	results := make([]models.BatchOperationResult, 0, len(ops))
	completedCount := 0
	failedCount := 0

	tx := p.db.Begin()

	for i, op := range ops {
		result := p.executeOperation(tx, &batch, i, op)
		results = append(results, result)
		if result.Success {
			completedCount++
		} else {
			failedCount++
		}
	}

	var finalStatus string
	if completedCount == 0 {
		tx.Rollback()
		finalStatus = models.BatchStatusFailed
	} else {
		tx.Commit()
		if failedCount > 0 {
			finalStatus = models.BatchStatusCompletedWithErrors
		} else {
			finalStatus = models.BatchStatusCompleted
		}
	}

	resultsJSON, _ := json.Marshal(results)
	errDetails := ""
	for _, r := range results {
		if !r.Success {
			errDetails = r.Error
			break
		}
	}

	p.db.Model(&batch).Updates(map[string]interface{}{
		"status":          finalStatus,
		"completed_count": completedCount,
		"failed_count":    failedCount,
		"error_details":   errDetails,
		"tx_hash":         fmt.Sprintf("batch_tx_%d_%d", batchID, time.Now().Unix()),
		"operations":      string(resultsJSON),
	})

	// Check if failed batch can be automatically retried
	retried := false
	if finalStatus == models.BatchStatusFailed && p.retryService != nil && errDetails != "" {
		if ok, err := p.retryService.ScheduleRetry(batchID, errors.New(errDetails)); err == nil && ok {
			retried = true
		}
	}

	// Remove from queue entry if not rescheduled for retry
	if !retried {
		p.db.Where("batch_id = ?", batchID).Delete(&models.BatchQueueEntry{})
	}
}

// executeOperation processes a single batch operation
func (p *BatchProcessor) executeOperation(tx *gorm.DB, batch *models.BatchTransaction, idx int, op models.BatchOperation) models.BatchOperationResult {
	result := models.BatchOperationResult{
		Index: idx,
		Type:  op.Type,
	}

	switch op.Type {
	case "transfer":
		var asset models.Asset
		if err := tx.First(&asset, op.AssetID).Error; err != nil {
			result.Error = fmt.Sprintf("asset %d not found", op.AssetID)
			return result
		}
		txn := models.Transaction{
			AssetID:     op.AssetID,
			FromAddress: op.FromAddress,
			ToAddress:   op.ToAddress,
			Amount:      op.Amount,
			TxHash:      fmt.Sprintf("batch_%d_op_%d_%d", batch.ID, idx, time.Now().UnixNano()),
			Status:      "confirmed",
		}
		if err := tx.Create(&txn).Error; err != nil {
			result.Error = err.Error()
			return result
		}
		result.TxHash = txn.TxHash
		result.Success = true

	case "list":
		listingID := fmt.Sprintf("batch_listing_%d_%d", batch.ID, idx)
		listing := models.Listing{
			AssetID:      op.AssetID,
			SellerAddr:   op.FromAddress,
			Amount:       op.Amount,
			PricePerUnit: 0,
			Active:       true,
			ListingID:    listingID,
		}
		if op.ExtraParams != nil {
			if price, ok := op.ExtraParams["price_per_unit"].(float64); ok {
				listing.PricePerUnit = int64(price)
			}
		}
		if err := tx.Create(&listing).Error; err != nil {
			result.Error = err.Error()
			return result
		}
		result.Success = true

	case "cancel_listing":
		var listingID string
		if op.ExtraParams != nil {
			if id, ok := op.ExtraParams["listing_id"].(string); ok {
				listingID = id
			}
		}
		if listingID == "" {
			result.Error = "listing_id required"
			return result
		}
		if err := tx.Model(&models.Listing{}).Where("listing_id = ?", listingID).Update("active", false).Error; err != nil {
			result.Error = err.Error()
			return result
		}
		result.Success = true

	default:
		result.Error = fmt.Sprintf("unsupported operation type: %s", op.Type)
	}

	return result
}
