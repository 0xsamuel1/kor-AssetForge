package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/gorm"
)

// PaymentHandler exposes fiat on-ramp endpoints.
type PaymentHandler struct {
	gw *services.PaymentGatewayService
}

// NewPaymentHandler creates a PaymentHandler.
func NewPaymentHandler(db *gorm.DB) *PaymentHandler {
	return &PaymentHandler{gw: services.NewPaymentGatewayService(db)}
}

type createPaymentRequest struct {
	AssetID    uint   `json:"asset_id" binding:"required,gt=0"`
	AmountFiat int64  `json:"amount_fiat" binding:"required,gt=0"` // in cents
	Currency   string `json:"currency" binding:"omitempty,len=3"`
	Gateway    string `json:"gateway" binding:"required,oneof=stripe paypal"`
}

// CreatePayment initiates a fiat purchase via the chosen gateway.
// @Summary Initiate fiat payment
// @Description Start a fiat-to-token purchase through Stripe or PayPal
// @Tags payments
// @Accept json
// @Produce json
// @Param body body createPaymentRequest true "Payment details"
// @Success 201 {object} gin.H
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /payments [post]
func (h *PaymentHandler) CreatePayment(c *gin.Context) {
	var req createPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid payment request", err))
		return
	}

	if req.Currency == "" {
		req.Currency = "USD"
	}
	req.Currency = strings.ToUpper(req.Currency)

	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Authentication required"))
		return
	}

	session, payment, err := h.gw.CreateCheckoutSession(
		userID.(uint),
		req.AssetID,
		req.AmountFiat,
		req.Currency,
		models.PaymentGateway(req.Gateway),
	)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to create payment session: "+err.Error()))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"payment_id":         payment.ID,
		"gateway_payment_id": session.GatewayPaymentID,
		"checkout_url":       session.CheckoutURL,
		"expires_at":         session.ExpiresAt,
		"status":             payment.Status,
	})
}

// GetPayment returns details of a specific payment.
// @Summary Get payment
// @Description Get details of a payment by its internal ID
// @Tags payments
// @Produce json
// @Param id path int true "Payment ID"
// @Success 200 {object} models.Payment
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /payments/{id} [get]
func (h *PaymentHandler) GetPayment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid payment ID"))
		return
	}

	payment, err := h.gw.GetPayment(uint(id))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Payment not found"))
		return
	}

	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	if payment.UserID != userID.(uint) && role != string(models.RoleAdmin) {
		apperrors.AbortWithError(c, apperrors.NewForbiddenError("Access denied"))
		return
	}

	c.JSON(http.StatusOK, payment)
}

// ListPayments returns the authenticated user's payment history.
// @Summary List payments
// @Description Get paginated payment history for the current user
// @Tags payments
// @Produce json
// @Param page query int false "Page number"
// @Param limit query int false "Page size"
// @Success 200 {object} gin.H
// @Router /payments [get]
func (h *PaymentHandler) ListPayments(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Authentication required"))
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	payments, total, err := h.gw.ListUserPayments(userID.(uint), page, limit)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewDatabaseError("Failed to fetch payments", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  payments,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// webhookBody is the shape expected in gateway webhook POST bodies.
type webhookBody struct {
	GatewayPaymentID string `json:"gateway_payment_id"`
	Status           string `json:"status"`
	FailureReason    string `json:"failure_reason"`
}

// HandleWebhook processes inbound payment gateway webhook events.
// @Summary Payment webhook
// @Description Receive and process payment status updates from gateways
// @Tags payments
// @Accept json
// @Produce json
// @Param gateway path string true "Gateway name (stripe|paypal)"
// @Success 200 {object} gin.H
// @Router /payments/webhooks/{gateway} [post]
func (h *PaymentHandler) HandleWebhook(c *gin.Context) {
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Failed to read webhook body"))
		return
	}

	var body webhookBody
	if jsonErr := json.Unmarshal(rawBody, &body); jsonErr != nil {
		// Gateway may encode differently — fall back to query params used in test flows.
		body.GatewayPaymentID = c.Query("gateway_payment_id")
		body.Status = c.Query("status")
		body.FailureReason = c.Query("failure_reason")
	}

	if body.GatewayPaymentID == "" || body.Status == "" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	status := models.PaymentStatus(body.Status)
	if err := h.gw.HandleWebhook(body.GatewayPaymentID, rawBody, status, body.FailureReason); err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to process webhook"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

type reconcileRequest struct {
	Gateway string `json:"gateway" binding:"required,oneof=stripe paypal"`
	Start   string `json:"start" binding:"required"`
	End     string `json:"end" binding:"required"`
}

// ReconcilePayments triggers a reconciliation run for a gateway and time window.
// @Summary Reconcile payments
// @Description Admin: reconcile fiat payment records against gateway data
// @Tags payments,admin
// @Accept json
// @Produce json
// @Param body body reconcileRequest true "Reconciliation window"
// @Success 200 {object} models.PaymentReconciliation
// @Router /admin/payments/reconcile [post]
func (h *PaymentHandler) ReconcilePayments(c *gin.Context) {
	var req reconcileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid reconciliation request", err))
		return
	}

	const layout = "2006-01-02"
	start, err := time.Parse(layout, req.Start)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid start date format (use YYYY-MM-DD)"))
		return
	}
	end, err := time.Parse(layout, req.End)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid end date format (use YYYY-MM-DD)"))
		return
	}

	rec, err := h.gw.Reconcile(models.PaymentGateway(req.Gateway), start, end.Add(24*time.Hour-time.Second))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Reconciliation failed"))
		return
	}

	c.JSON(http.StatusOK, rec)
}
