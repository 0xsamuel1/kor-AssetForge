package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/gorm"
)

type GDPRHandler struct {
	db            *gorm.DB
	exportService *services.DataExportService
}

// NewGDPRHandler constructs a new GDPRHandler
func NewGDPRHandler(db *gorm.DB, exportService *services.DataExportService) *GDPRHandler {
	return &GDPRHandler{
		db:            db,
		exportService: exportService,
	}
}

// RequestDataExport handles POST /api/v1/legal/gdpr/export
// @Summary Request GDPR data export
// @Description Initiate asynchronous gathering and encryption of all user personal data
// @Tags gdpr
// @Accept json
// @Produce json
// @Param format body map[string]string true "Export format: 'json' or 'csv'"
// @Success 202 {object} map[string]interface{}
// @Router /legal/gdpr/export [post]
func (h *GDPRHandler) RequestDataExport(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	var req struct {
		Format string `json:"format" binding:"required"` // "json" or "csv"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job, err := h.exportService.StartExportJob(userID, req.Format)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Data export job created successfully and is being processed in the background. You will receive an email with the download link once ready.",
		"job_id":  job.ID,
		"status":  job.Status,
	})
}

// GetDataExportStatus handles GET /api/v1/legal/gdpr/export/:id
// @Summary Get GDPR data export status
// @Description Check current status of user's requested data export job
// @Tags gdpr
// @Param id path int true "Job ID"
// @Success 200 {object} map[string]interface{}
// @Router /legal/gdpr/export/{id} [get]
func (h *GDPRHandler) GetDataExportStatus(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	idStr := c.Param("id")
	jobID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job ID"})
		return
	}

	var job models.ExportJob
	if err := h.db.Where("id = ? AND user_id = ?", uint(jobID), userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Export job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query export job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"job_id":     job.ID,
		"status":     job.Status,
		"format":     job.Format,
		"expires_at": job.ExpiresAt,
		"created_at": job.CreatedAt,
	})
}

// DownloadExport handles GET /api/v1/legal/gdpr/export/download/:token
// @Summary Download GDPR data export
// @Description Fetch and decrypt GDPR user data archive using the link token
// @Tags gdpr
// @Param token path string true "Download Token"
// @Success 200 {file} binary
// @Router /legal/gdpr/export/download/{token} [get]
func (h *GDPRHandler) DownloadExport(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "download token is required"})
		return
	}

	plaintext, filename, err := h.exportService.GetDecryptedExport(token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set headers to trigger file download
	c.Header("Content-Disposition", "attachment; filename="+filename)
	if len(filename) > 4 && filename[len(filename)-4:] == ".zip" {
		c.Header("Content-Type", "application/zip")
	} else {
		c.Header("Content-Type", "application/json")
	}
	c.Data(http.StatusOK, c.Writer.Header().Get("Content-Type"), plaintext)
}
