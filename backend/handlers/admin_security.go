package handlers

import (
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// AdminSecurityHandler manages IP whitelist entries for sensitive admin endpoints.
type AdminSecurityHandler struct {
	db *gorm.DB
}

// NewAdminSecurityHandler creates a new AdminSecurityHandler.
func NewAdminSecurityHandler(db *gorm.DB) *AdminSecurityHandler {
	return &AdminSecurityHandler{db: db}
}

type createIPWhitelistRequest struct {
	CIDR        string `json:"cidr" binding:"required"`
	Description string `json:"description" binding:"omitempty,max=255"`
}

// AddIPWhitelistEntry adds an IP address or CIDR block to the whitelist.
// @Summary Add IP to whitelist
// @Description Add an IP address or CIDR range to the admin endpoint whitelist
// @Tags admin,security
// @Accept json
// @Produce json
// @Param entry body createIPWhitelistRequest true "IP/CIDR entry"
// @Success 201 {object} models.IPWhitelistEntry
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 409 {object} apperrors.ErrorResponse
// @Router /admin/security/ip-whitelist [post]
func (h *AdminSecurityHandler) AddIPWhitelistEntry(c *gin.Context) {
	var req createIPWhitelistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request payload", err))
		return
	}

	// Normalize and validate the CIDR.
	_, ipNet, err := net.ParseCIDR(req.CIDR)
	if err != nil {
		// Try treating it as a bare IP and convert it to /32 or /128.
		ip := net.ParseIP(req.CIDR)
		if ip == nil {
			apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid IP address or CIDR notation"))
			return
		}
		if ip.To4() != nil {
			req.CIDR = ip.String() + "/32"
		} else {
			req.CIDR = ip.String() + "/128"
		}
		_, ipNet, _ = net.ParseCIDR(req.CIDR)
	} else {
		req.CIDR = ipNet.String()
	}
	_ = ipNet

	userID, _ := c.Get("user_id")
	entry := models.IPWhitelistEntry{
		CIDR:        req.CIDR,
		Description: req.Description,
		CreatedBy:   userID.(uint),
	}

	if err := h.db.Create(&entry).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewConflictError("IP or CIDR already exists in whitelist"))
		return
	}

	c.JSON(http.StatusCreated, entry)
}

// ListIPWhitelistEntries lists all IP whitelist entries.
// @Summary List IP whitelist entries
// @Description List all whitelisted IP addresses and CIDR ranges
// @Tags admin,security
// @Produce json
// @Success 200 {array} models.IPWhitelistEntry
// @Router /admin/security/ip-whitelist [get]
func (h *AdminSecurityHandler) ListIPWhitelistEntries(c *gin.Context) {
	var entries []models.IPWhitelistEntry
	if err := h.db.Order("created_at desc").Find(&entries).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewDatabaseError("Failed to fetch whitelist entries", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries, "count": len(entries)})
}

// DeleteIPWhitelistEntry removes a whitelist entry by ID.
// @Summary Remove IP from whitelist
// @Description Remove an IP address or CIDR range from the admin whitelist
// @Tags admin,security
// @Produce json
// @Param id path int true "Entry ID"
// @Success 200 {object} gin.H
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /admin/security/ip-whitelist/{id} [delete]
func (h *AdminSecurityHandler) DeleteIPWhitelistEntry(c *gin.Context) {
	id := c.Param("id")
	result := h.db.Delete(&models.IPWhitelistEntry{}, id)
	if result.Error != nil {
		apperrors.AbortWithError(c, apperrors.NewDatabaseError("Failed to delete whitelist entry", result.Error))
		return
	}
	if result.RowsAffected == 0 {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Whitelist entry not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Whitelist entry deleted"})
}
