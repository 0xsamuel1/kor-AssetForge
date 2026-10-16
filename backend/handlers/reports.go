package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
)

type ReportHandler struct {
	scheduler *services.ReportSchedulerService
}

func NewReportHandler(scheduler *services.ReportSchedulerService) *ReportHandler {
	return &ReportHandler{scheduler: scheduler}
}

type reportScheduleRequest struct {
	Name            string                       `json:"name" binding:"required"`
	ReportType      string                       `json:"report_type" binding:"required"`
	Frequency       models.ReportFrequency       `json:"frequency" binding:"required,oneof=daily weekly monthly"`
	CronExpression  string                       `json:"cron_expression"`
	Timezone        string                       `json:"timezone"`
	Format          models.ReportFormat          `json:"format" binding:"required,oneof=pdf csv excel"`
	DeliveryMethod  models.ReportDeliveryMethod  `json:"delivery_method" binding:"required,oneof=email webhook"`
	EmailRecipients string                       `json:"email_recipients"`
	WebhookURL      string                       `json:"webhook_url"`
	Filters         string                       `json:"filters"`
	RetentionDays   int                          `json:"retention_days"`
	Status          models.ScheduledReportStatus `json:"status"`
}

func (h *ReportHandler) ListSchedules(c *gin.Context) {
	schedules, err := h.scheduler.ListSchedules(currentUserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch report schedules"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": schedules})
}

func (h *ReportHandler) CreateSchedule(c *gin.Context) {
	var req reportScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	schedule := models.ScheduledReport{
		UserID:          currentUserID(c),
		Name:            req.Name,
		ReportType:      req.ReportType,
		Frequency:       req.Frequency,
		CronExpression:  req.CronExpression,
		Timezone:        req.Timezone,
		Format:          req.Format,
		DeliveryMethod:  req.DeliveryMethod,
		EmailRecipients: req.EmailRecipients,
		WebhookURL:      req.WebhookURL,
		Filters:         req.Filters,
		RetentionDays:   req.RetentionDays,
		Status:          req.Status,
	}
	if err := h.scheduler.CreateSchedule(&schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": schedule})
}

func (h *ReportHandler) GetSchedule(c *gin.Context) {
	scheduleID, err := parseReportUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule id"})
		return
	}
	schedule, err := h.scheduler.GetSchedule(currentUserID(c), scheduleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report schedule not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": schedule})
}

func (h *ReportHandler) UpdateSchedule(c *gin.Context) {
	scheduleID, err := parseReportUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule id"})
		return
	}
	schedule, err := h.scheduler.GetSchedule(currentUserID(c), scheduleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report schedule not found"})
		return
	}

	var req reportScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	schedule.Name = req.Name
	schedule.ReportType = req.ReportType
	schedule.Frequency = req.Frequency
	schedule.CronExpression = req.CronExpression
	schedule.Timezone = req.Timezone
	schedule.Format = req.Format
	schedule.DeliveryMethod = req.DeliveryMethod
	schedule.EmailRecipients = req.EmailRecipients
	schedule.WebhookURL = req.WebhookURL
	schedule.Filters = req.Filters
	schedule.RetentionDays = req.RetentionDays
	schedule.Status = req.Status

	if err := h.scheduler.UpdateSchedule(schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": schedule})
}

func (h *ReportHandler) DeleteSchedule(c *gin.Context) {
	scheduleID, err := parseReportUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule id"})
		return
	}
	if err := h.scheduler.DeleteSchedule(currentUserID(c), scheduleID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report schedule not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ReportHandler) RunSchedule(c *gin.Context) {
	scheduleID, err := parseReportUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule id"})
		return
	}
	if _, err := h.scheduler.GetSchedule(currentUserID(c), scheduleID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report schedule not found"})
		return
	}
	history, err := h.scheduler.RunScheduleNow(scheduleID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "data": history})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": history})
}

func (h *ReportHandler) ListHistory(c *gin.Context) {
	var scheduleID uint
	if raw := c.Query("schedule_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule_id"})
			return
		}
		scheduleID = uint(parsed)
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	history, err := h.scheduler.ListHistory(currentUserID(c), scheduleID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch report history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": history})
}

func parseReportUintParam(c *gin.Context, name string) (uint, error) {
	parsed, err := strconv.ParseUint(c.Param(name), 10, 64)
	return uint(parsed), err
}
