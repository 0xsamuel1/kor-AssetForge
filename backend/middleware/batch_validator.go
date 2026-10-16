package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/config"
)

// BatchPayload represents the minimal structure required to validate batch sizes.
type BatchPayload struct {
	Operations []json.RawMessage `json:"operations"`
}

// BatchValidator ensures incoming batch requests do not exceed the configured maximum batch size.
func BatchValidator() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only validate POST / PUT / PATCH requests with body
		if c.Request.Method != http.MethodPost && c.Request.Method != http.MethodPut && c.Request.Method != http.MethodPatch {
			c.Next()
			return
		}

		if c.Request.Body == nil {
			c.Next()
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.NewBadRequestError("Failed to read request body"))
			return
		}

		// Restore request body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		if len(bodyBytes) == 0 {
			c.Next()
			return
		}

		var payload BatchPayload
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			// If JSON is malformed, let the handler's ShouldBindJSON return detailed validation errors
			c.Next()
			return
		}

		// Enforce size limits if operations are present
		maxBatchSize := config.GetMaxBatchSize()
		count := len(payload.Operations)

		if count == 0 {
			apperrors.AbortWithError(c, apperrors.NewBadRequestError("Batch must contain at least one operation"))
			return
		}

		if count > maxBatchSize {
			apperrors.AbortWithError(c, apperrors.NewBadRequestError(
				fmt.Sprintf("Batch size limit exceeded: maximum allowed is %d operations (received %d)", maxBatchSize, count),
			))
			return
		}

		c.Next()
	}
}
