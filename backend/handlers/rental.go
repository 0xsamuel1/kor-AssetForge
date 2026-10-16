package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/validator"
	"gorm.io/gorm"
)

type RentalHandler struct {
	db            *gorm.DB
	rentalService *services.RentalService
}

func NewRentalHandler(db *gorm.DB) *RentalHandler {
	return &RentalHandler{
		db:            db,
		rentalService: services.NewRentalService(db),
	}
}

type createRentalRequest struct {
	AssetID         uint              `json:"asset_id" binding:"required"`
	LesseeID        uint              `json:"lessee_id" binding:"required"`
	Period          models.RentalPeriod `json:"period" binding:"required"`
	RateAmount      int64             `json:"rate_amount" binding:"required"`
	RateCurrency    string            `json:"rate_currency"`
	SecurityDeposit int64             `json:"security_deposit"`
	StartDate       time.Time         `json:"start_date" binding:"required"`
	EndDate         time.Time         `json:"end_date" binding:"required"`
	AutoRenew       bool              `json:"auto_renew"`
	LateFeePercent  float64           `json:"late_fee_percent"`
	Terms           string            `json:"terms"`
}

func (h *RentalHandler) CreateRental(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req createRentalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Invalid request", http.StatusBadRequest))
		return
	}

	validator.SanitizeStruct(&req)

	if req.RateCurrency == "" {
		req.RateCurrency = "USD"
	}
	if req.LateFeePercent == 0 {
		req.LateFeePercent = 5.0
	}

	rental, err := h.rentalService.CreateRental(
		userID, req.LesseeID, req.AssetID,
		req.Period, req.RateAmount, req.SecurityDeposit,
		req.StartDate, req.EndDate, req.AutoRenew,
		req.LateFeePercent, req.Terms,
	)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, err.Error(), http.StatusBadRequest))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": rental})
}

func (h *RentalHandler) ListRentals(c *gin.Context) {
	userID := c.GetUint("user_id")
	role := c.GetString("role")

	var rentals []models.Rental
	query := h.db.Preload("Asset")

	if role == "admin" {
		query.Find(&rentals)
	} else {
		query.Where("lessor_id = ? OR lessee_id = ?", userID, userID).Find(&rentals)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": rentals})
}

func (h *RentalHandler) GetRental(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid rental ID"))
		return
	}

	var rental models.Rental
	if err := h.db.Preload("Asset").First(&rental, id).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Rental not found"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": rental})
}

func (h *RentalHandler) CancelRental(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid rental ID"))
		return
	}

	if err := h.rentalService.CancelRental(uint(id), userID); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, err.Error(), http.StatusBadRequest))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Rental cancelled"})
}

func (h *RentalHandler) ProcessPayment(c *gin.Context) {
	var req struct {
		RentalID uint   `json:"rental_id" binding:"required"`
		Amount   int64  `json:"amount" binding:"required"`
		TxHash   string `json:"transaction_hash" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Invalid request", http.StatusBadRequest))
		return
	}

	payment, err := h.rentalService.ProcessPayment(req.RentalID, req.Amount, req.TxHash)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, err.Error(), http.StatusBadRequest))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": payment})
}

func (h *RentalHandler) GetPaymentSchedule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid rental ID"))
		return
	}

	payments, err := h.rentalService.GetPaymentSchedule(uint(id))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Rental not found"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": payments})
}

func (h *RentalHandler) GetRentalHistory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid rental ID"))
		return
	}

	history, err := h.rentalService.GetRentalHistory(uint(id))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Rental not found"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": history})
}

func (h *RentalHandler) SignAgreement(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid rental ID"))
		return
	}

	if err := h.rentalService.SignAgreement(uint(id), userID); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeForbidden, err.Error(), http.StatusForbidden))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Agreement signed"})
}

func (h *RentalHandler) DisputeRental(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid rental ID"))
		return
	}

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Reason is required", http.StatusBadRequest))
		return
	}

	if err := h.rentalService.DisputeRental(uint(id), userID, req.Reason); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, err.Error(), http.StatusBadRequest))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Rental disputed"})
}
