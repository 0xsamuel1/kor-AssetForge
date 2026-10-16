package handlers

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
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/constants"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBatchTestDB(t *testing.T) *gorm.DB {
	dsn := fmt.Sprintf("file:batch_h_mem_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.BatchTransaction{}, &models.BatchQueueEntry{}, &models.User{})
	require.NoError(t, err)
	return db
}

func setupBatchRouter(h *BatchHandler, authUserID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperrors.ErrorHandler(false))

	// Mock auth middleware injecting user_id
	if authUserID != 0 {
		r.Use(func(c *gin.Context) {
			c.Set("user_id", authUserID)
			c.Next()
		})
	}

	r.POST("/batch/submit", h.SubmitBatch)
	r.POST("/batch/retry/:id", h.RetryBatch)
	r.POST("/batch/:id/retry", h.RetryBatch)
	return r
}

func TestBatchHandler_SubmitBatch_LimitEnforcement(t *testing.T) {
	db := setupBatchTestDB(t)
	processor := services.NewBatchProcessor(db, 1)
	defer processor.Stop()

	h := NewBatchHandler(db, processor)
	r := setupBatchRouter(h, 1)

	// Test exceeding limit
	ops := make([]models.BatchOperation, constants.MAX_BATCH_SIZE+1)
	for i := range ops {
		ops[i] = models.BatchOperation{Type: "transfer", Amount: 10}
	}
	body, _ := json.Marshal(gin.H{"operations": ops})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/batch/submit", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), fmt.Sprintf("maximum allowed is %d operations", constants.MAX_BATCH_SIZE))
}

func TestBatchHandler_RetryBatch_Endpoint(t *testing.T) {
	db := setupBatchTestDB(t)
	processor := services.NewBatchProcessor(db, 1)
	defer processor.Stop()

	h := NewBatchHandler(db, processor)
	r := setupBatchRouter(h, 42)

	// Create failed batch
	batch := models.BatchTransaction{
		UserID:          42,
		Operations:      `[{"type":"transfer","amount":100}]`,
		Status:          models.BatchStatusFailed,
		TotalOperations: 1,
		RetryCount:      1,
		MaxRetries:      3,
		IsRetryable:     true,
	}
	require.NoError(t, db.Create(&batch).Error)

	// Trigger manual retry via endpoint
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/batch/retry/%d", batch.ID), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.BatchRetryResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, batch.ID, resp.BatchID)
	assert.Equal(t, models.BatchStatusQueued, resp.Status)
	assert.Equal(t, 2, resp.RetryCount)
	assert.Contains(t, resp.Message, "queued for retry")
}
