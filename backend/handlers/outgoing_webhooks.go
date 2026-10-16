package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/gorm"
)

type OutgoingWebhookHandler struct {
	db            *gorm.DB
	deliverySvc   *services.WebhookDeliveryService
}

func NewOutgoingWebhookHandler(db *gorm.DB) *OutgoingWebhookHandler {
	return &OutgoingWebhookHandler{
		db:          db,
		deliverySvc: services.NewWebhookDeliveryService(db),
	}
}

type createSubscriptionRequest struct {
	URL         string   `json:"url" binding:"required,url"`
	Events      []string `json:"events" binding:"required,min=1"`
	Description string   `json:"description" binding:"omitempty,max=255"`
}

type updateSubscriptionRequest struct {
	URL         string   `json:"url" binding:"omitempty,url"`
	Events      []string `json:"events" binding:"omitempty,min=1"`
	Description string   `json:"description" binding:"omitempty,max=255"`
	Active      *bool    `json:"active"`
}

func (h *OutgoingWebhookHandler) CreateSubscription(c *gin.Context) {
	var req createSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, err.Error(), http.StatusBadRequest))
		return
	}

	userID := c.GetUint("user_id")

	secret, err := generateSecret()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.New(apperrors.CodeInternalServerError, "failed to generate secret", http.StatusInternalServerError))
		return
	}

	sub := models.WebhookSubscription{
		UserID:      userID,
		URL:         req.URL,
		Secret:      secret,
		Events:      strings.Join(req.Events, ","),
		Description: req.Description,
		Active:      true,
	}

	if err := h.db.Create(&sub).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeInternalServerError, "failed to create subscription", http.StatusInternalServerError))
		return
	}

	resp := gin.H{
		"id":          sub.ID,
		"url":         sub.URL,
		"events":      req.Events,
		"description": sub.Description,
		"active":      sub.Active,
		"secret":      secret,
		"created_at":  sub.CreatedAt,
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *OutgoingWebhookHandler) ListSubscriptions(c *gin.Context) {
	userID := c.GetUint("user_id")
	var subs []models.WebhookSubscription
	h.db.Where("user_id = ?", userID).Find(&subs)
	c.JSON(http.StatusOK, subs)
}

func (h *OutgoingWebhookHandler) UpdateSubscription(c *gin.Context) {
	userID := c.GetUint("user_id")
	var sub models.WebhookSubscription
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), userID).First(&sub).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.New(apperrors.CodeNotFound, "subscription not found", http.StatusNotFound))
		return
	}

	var req updateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, err.Error(), http.StatusBadRequest))
		return
	}

	if req.URL != "" {
		sub.URL = req.URL
	}
	if len(req.Events) > 0 {
		sub.Events = strings.Join(req.Events, ",")
	}
	if req.Description != "" {
		sub.Description = req.Description
	}
	if req.Active != nil {
		sub.Active = *req.Active
	}

	h.db.Save(&sub)
	c.JSON(http.StatusOK, sub)
}

func (h *OutgoingWebhookHandler) DeleteSubscription(c *gin.Context) {
	userID := c.GetUint("user_id")
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), userID).
		Delete(&models.WebhookSubscription{}).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.New(apperrors.CodeNotFound, "subscription not found", http.StatusNotFound))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *OutgoingWebhookHandler) GetDeliveryLogs(c *gin.Context) {
	userID := c.GetUint("user_id")
	var sub models.WebhookSubscription
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), userID).First(&sub).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.New(apperrors.CodeNotFound, "subscription not found", http.StatusNotFound))
		return
	}

	var logs []models.WebhookDeliveryLog
	h.db.Where("subscription_id = ?", sub.ID).Order("created_at DESC").Limit(100).Find(&logs)
	c.JSON(http.StatusOK, logs)
}

func (h *OutgoingWebhookHandler) RetryDelivery(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid delivery log ID"))
		return
	}

	if err := h.deliverySvc.RetrySpecific(uint(id)); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, err.Error(), http.StatusBadRequest))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Delivery retry scheduled"})
}

func (h *OutgoingWebhookHandler) RetryAllFailedDeliveries(c *gin.Context) {
	count, err := h.deliverySvc.RetryAllFailed()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to retry deliveries"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "retried": count})
}

func (h *OutgoingWebhookHandler) GetDeliveryDashboard(c *gin.Context) {
	dashboard, err := h.deliverySvc.GetDeliveryDashboard()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to get delivery dashboard"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": dashboard})
}

func (h *OutgoingWebhookHandler) GetDeliveryLog(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid delivery log ID"))
		return
	}

	var log models.WebhookDeliveryLog
	if err := h.db.First(&log, id).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Delivery log not found"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": log})
}

func (h *OutgoingWebhookHandler) ReplayDLQ(c *gin.Context) {
	count, err := h.deliverySvc.ReplayFromDLQ()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to replay DLQ"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "replayed": count})
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
