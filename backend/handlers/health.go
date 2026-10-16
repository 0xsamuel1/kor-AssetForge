package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/utils"
	"gorm.io/gorm"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	db            *gorm.DB
	redisClient   *redis.Client
	stellarClient *utils.StellarClient
	checker       *services.HealthChecker
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(db *gorm.DB, redisClient *redis.Client, stellarClient *utils.StellarClient) *HealthHandler {
	return &HealthHandler{
		db:            db,
		redisClient:   redisClient,
		stellarClient: stellarClient,
		checker:       services.NewHealthChecker(db, redisClient, stellarClient),
	}
}

// LivenessCheck handles liveness probes
// @Summary Liveness check
// @Description Basic check to see if the service is up
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
// @Router /health/live [get]
func (h *HealthHandler) LivenessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "up",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ReadinessCheck handles readiness probes
// @Summary Readiness check
// @Description Comprehensive check to see if all dependencies are ready
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /health/ready [get]
func (h *HealthHandler) ReadinessCheck(c *gin.Context) {
	report := h.checker.Check(c.Request.Context())
	status := http.StatusOK
	if report.Status == services.OverallHealthDown {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, report)
}
