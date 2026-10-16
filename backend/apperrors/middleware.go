package apperrors

import (
	"fmt"
	"net/http"
	rsdebug "runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.Logger

func init() {
	var err error
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	Logger, err = config.Build()
	if err != nil {
		panic(err)
	}
}

func AbortWithError(c *gin.Context, err *AppError) {
	c.Error(err)
	c.AbortWithStatus(err.Status)
}

func ErrorHandler(debug bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				appErr := Wrap(fmt.Errorf("%v", rec), CodeInternalServerError, "Unexpected server error", http.StatusInternalServerError)
				TrackGinError(c, appErr.Code)
				requestID, _ := c.Get("request_id")

				Logger.Error("Panic recovered",
					zap.Any("recover_value", rec),
					zap.String("request_id", fmt.Sprint(requestID)),
					zap.String("stack", string(rsdebug.Stack())),
				)

				c.AbortWithStatusJSON(appErr.Status, FormatErrorResponse(appErr, fmt.Sprint(requestID), debug))
			}
		}()

		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		lastErr := c.Errors.Last().Err
		appErr := FromError(lastErr)
		if appErr.Status == 0 {
			appErr.Status = http.StatusInternalServerError
		}

		TrackGinError(c, appErr.Code)
		requestID, _ := c.Get("request_id")

		Logger.Error("Request error",
			zap.String("request_id", fmt.Sprint(requestID)),
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
			zap.Int("status", appErr.Status),
			zap.String("error_code", string(appErr.Code)),
			zap.Error(lastErr),
		)

		c.JSON(appErr.Status, FormatErrorResponse(appErr, fmt.Sprint(requestID), debug))
	}
}
