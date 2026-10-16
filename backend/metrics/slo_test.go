package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRecordSLORequest_LatencyAndErrors(t *testing.T) {
	// Normal fast request (50ms, 200 OK)
	RecordSLORequest("/api/v1/assets", "GET", http.StatusOK, 50*time.Millisecond)

	// Slow request (600ms > 500ms threshold)
	RecordSLORequest("/api/v1/assets", "GET", http.StatusOK, 600*time.Millisecond)

	// Server error (500)
	RecordSLORequest("/api/v1/assets", "POST", http.StatusInternalServerError, 100*time.Millisecond)

	SetCircuitBreakerMetric("CLOSED")
	SetCircuitBreakerMetric("OPEN")
	SetCircuitBreakerMetric("HALF_OPEN")
}

func TestSLOMiddleware(t *testing.T) {
	r := gin.New()
	r.Use(SLOMiddleware())
	r.GET("/api/v1/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "pong"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
