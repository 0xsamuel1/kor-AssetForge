package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/constants"
)

func setupBatchValidatorRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperrors.ErrorHandler(false))
	r.Use(BatchValidator())
	r.POST("/batch/test", func(c *gin.Context) {
		var body map[string]interface{}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "operations_count": len(body["operations"].([]interface{}))})
	})
	return r
}

func generateOperations(count int) []map[string]interface{} {
	ops := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		ops[i] = map[string]interface{}{
			"type":     "transfer",
			"asset_id": i + 1,
			"amount":   100,
		}
	}
	return ops
}

func TestBatchValidator_WithinLimit(t *testing.T) {
	config.ResetBatchConfig()
	r := setupBatchValidatorRouter()

	payload := map[string]interface{}{
		"operations": generateOperations(constants.MAX_BATCH_SIZE),
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/batch/test", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBatchValidator_ExceedsLimit(t *testing.T) {
	config.ResetBatchConfig()
	r := setupBatchValidatorRouter()

	payload := map[string]interface{}{
		"operations": generateOperations(constants.MAX_BATCH_SIZE + 1),
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/batch/test", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), fmt.Sprintf("maximum allowed is %d operations", constants.MAX_BATCH_SIZE))
}

func TestBatchValidator_EmptyOperations(t *testing.T) {
	config.ResetBatchConfig()
	r := setupBatchValidatorRouter()

	payload := map[string]interface{}{
		"operations": []map[string]interface{}{},
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/batch/test", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Batch must contain at least one operation")
}

func TestBatchValidator_DynamicLimitAdjustment(t *testing.T) {
	config.ResetBatchConfig()
	defer config.ResetBatchConfig()
	r := setupBatchValidatorRouter()

	// Adjust limit to 10 dynamically
	config.SetMaxBatchSize(10)
	assert.Equal(t, 10, config.GetMaxBatchSize())

	// 10 operations should pass
	payload10 := map[string]interface{}{"operations": generateOperations(10)}
	body10, _ := json.Marshal(payload10)
	w10 := httptest.NewRecorder()
	req10, _ := http.NewRequest(http.MethodPost, "/batch/test", bytes.NewBuffer(body10))
	req10.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w10, req10)
	assert.Equal(t, http.StatusOK, w10.Code)

	// 11 operations should fail
	payload11 := map[string]interface{}{"operations": generateOperations(11)}
	body11, _ := json.Marshal(payload11)
	w11 := httptest.NewRecorder()
	req11, _ := http.NewRequest(http.MethodPost, "/batch/test", bytes.NewBuffer(body11))
	req11.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w11, req11)
	assert.Equal(t, http.StatusBadRequest, w11.Code)
	assert.Contains(t, w11.Body.String(), "maximum allowed is 10 operations (received 11)")
}
