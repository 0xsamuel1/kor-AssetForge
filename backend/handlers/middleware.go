package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/i18n"
	"github.com/yourusername/kor-assetforge/middleware"
)

// errorCodeTranslationKeys maps apperrors error codes to i18n keys so error
// responses can be localized for the requester's resolved language (#185).
var errorCodeTranslationKeys = map[apperrors.ErrorCode]string{
	apperrors.CodeValidationFailed:     "errors.validation_failed",
	apperrors.CodeNotFound:             "errors.not_found",
	apperrors.CodeUnauthorized:         "errors.unauthorized",
	apperrors.CodeForbidden:            "errors.forbidden",
	apperrors.CodeConflict:             "errors.conflict",
	apperrors.CodeTooManyRequests:      "errors.too_many_requests",
	apperrors.CodeInternalServerError:  "errors.internal_server_error",
}

// localizedErrorMessage returns the translated message for err's AppError code
// in the request's resolved language, falling back to err's own message text
// for error codes that don't have a translation entry.
func localizedErrorMessage(c *gin.Context, err error) string {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		if key, ok := errorCodeTranslationKeys[appErr.Code]; ok {
			lang := middleware.LanguageFromContext(c)
			return i18n.T(lang, key, nil)
		}
		return appErr.Message
	}
	return err.Error()
}

var Logger *zap.Logger

func init() {
	var err error
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	Logger, err = config.Build()
	if err != nil {
		panic(err)
	}
}

// RequestLogger middleware adds a request ID and logs detailed request/response info
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := uuid.New().String()
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		// Process request
		c.Next()

		// Log request details
		duration := time.Since(start)
		traceID, _ := c.Get("trace_id")
		Logger.Info("HTTP Request",
			zap.String("request_id", requestID),
			zap.Any("trace_id", traceID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.String("ip", c.ClientIP()),
			zap.Duration("duration", duration),
			zap.String("user_agent", c.Request.UserAgent()),
		)
	}
}

// GlobalErrorHandler middleware recovers from panics and standardizes error responses
func GlobalErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic with stack trace
				requestID, _ := c.Get("request_id")
				traceID, _ := c.Get("trace_id")
				Logger.Error("Panic recovered",
					zap.Any("error", err),
					zap.Any("request_id", requestID),
					zap.Any("trace_id", traceID),
					zap.String("stack", string(debug.Stack())),
				)

				// Standardized error response
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":      "Internal Server Error",
					"message":    fmt.Sprintf("%v", err),
					"request_id": requestID,
					"trace_id":   traceID,
					"code":       500,
				})
				c.Abort()
			}
		}()

		c.Next()

		// Check if there are errors in the context
		if len(c.Errors) > 0 {
			requestID, _ := c.Get("request_id")
			traceID, _ := c.Get("trace_id")
			err := c.Errors.Last()

			// Standardized JSON error response, localized per the resolved request language (#185)
			c.JSON(c.Writer.Status(), gin.H{
				"error":      "Processing Error",
				"message":    localizedErrorMessage(c, err.Err),
				"request_id": requestID,
				"trace_id":   traceID,
				"code":       c.Writer.Status(),
			})
		}
	}
}
