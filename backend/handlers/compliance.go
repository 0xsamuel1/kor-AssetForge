package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/validator"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ComplianceHandler handles beneficial-ownership (UBO) declaration,
// verification and reporting endpoints. Kept separate from KYCHandler
// because UBO is entity-level (declared *about* an organization's owners),
// not identity verification of the account holder themselves.
type ComplianceHandler struct {
	db  *gorm.DB
	ubo *services.UBOVerificationService
}

// NewComplianceHandler constructs a ComplianceHandler.
func NewComplianceHandler(db *gorm.DB) *ComplianceHandler {
	return &ComplianceHandler{db: db, ubo: services.NewUBOVerificationService()}
}

type declareBeneficialOwnerRequest struct {
	EntityUserID       uint    `json:"entity_user_id"        binding:"required,gt=0"`
	FullName           string  `json:"full_name"             binding:"required,min=2,max=200,no_html"`
	DateOfBirth        string  `json:"date_of_birth"         binding:"omitempty,no_html"`
	Nationality        string  `json:"nationality"           binding:"omitempty,no_html"`
	OwnershipPercent   float64 `json:"ownership_percent"     binding:"gte=0,lte=100"`
	IsControllingParty bool    `json:"is_controlling_party"`
	OwnershipLayer     string  `json:"ownership_layer"       binding:"omitempty,no_html"`
	DocumentNumber     string  `json:"document_number"       binding:"omitempty,no_html"` // hashed before storage, like KYC
}

type resolveUBOReviewRequest struct {
	Status      models.UBOVerificationStatus `json:"status"       binding:"required,oneof=verified rejected"`
	ReviewNotes string                        `json:"review_notes" binding:"omitempty,no_html"`
}

// DeclareBeneficialOwner handles POST /api/v1/compliance/ubo
// @Summary Declare a beneficial owner
// @Description Records an individual's ownership stake or control over a corporate/organizational account for UBO disclosure.
// @Tags compliance
// @Accept json
// @Produce json
// @Param owner body declareBeneficialOwnerRequest true "Beneficial owner declaration"
// @Success 201 {object} models.BeneficialOwner
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /compliance/ubo [post]
func (h *ComplianceHandler) DeclareBeneficialOwner(c *gin.Context) {
	var req declareBeneficialOwnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validator.SanitizeStruct(&req)

	owner := models.BeneficialOwner{
		EntityUserID:       req.EntityUserID,
		FullName:           req.FullName,
		DateOfBirth:        req.DateOfBirth,
		Nationality:        req.Nationality,
		OwnershipPercent:   req.OwnershipPercent,
		IsControllingParty: req.IsControllingParty,
		OwnershipLayer:     req.OwnershipLayer,
		Status:             models.UBOStatusDeclared,
	}
	if req.DocumentNumber != "" {
		owner.DocumentNumberHash = services.HashDocumentNumber(req.DocumentNumber)
	}

	if err := h.ubo.ValidateDeclaration(&owner); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var existing []models.BeneficialOwner
	if err := h.db.Where("entity_user_id = ?", req.EntityUserID).Find(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load existing declarations"})
		return
	}
	if err := h.ubo.ValidateTotalOwnership(append(existing, owner)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Create(&owner).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save beneficial owner declaration"})
		return
	}

	h.writeAuditLog(req.EntityUserID, "ubo_declared", "BeneficialOwner", owner.ID,
		map[string]interface{}{"ownership_percent": owner.OwnershipPercent}, c.ClientIP())

	c.JSON(http.StatusCreated, owner)
}

// ListBeneficialOwners handles GET /api/v1/compliance/ubo?entity_user_id=<n>
// @Summary List beneficial owners for an entity
// @Description Returns all UBO declarations for a corporate/organizational account.
// @Tags compliance
// @Accept json
// @Produce json
// @Param entity_user_id query uint true "Entity user ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /compliance/ubo [get]
func (h *ComplianceHandler) ListBeneficialOwners(c *gin.Context) {
	var entityUserID uint
	if _, err := fmt.Sscanf(c.Query("entity_user_id"), "%d", &entityUserID); err != nil || entityUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid entity_user_id query param required"})
		return
	}

	var owners []models.BeneficialOwner
	if err := h.db.Where("entity_user_id = ?", entityUserID).Find(&owners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch beneficial owners"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entity_user_id":        entityUserID,
		"owners":                owners,
		"requires_ubo_program":  h.ubo.RequiresUBOProgram(owners),
		"pending_review_count":  len(h.ubo.PendingReview(owners)),
	})
}

// ResolveUBOReview handles POST /api/v1/compliance/ubo/:id/resolve
// @Summary Resolve a beneficial-owner review
// @Description Compliance-team decision on a declared owner: verified or rejected.
// @Tags compliance
// @Accept json
// @Produce json
// @Param id path uint true "BeneficialOwner ID"
// @Param resolution body resolveUBOReviewRequest true "Review resolution"
// @Success 200 {object} models.BeneficialOwner
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /compliance/ubo/{id}/resolve [post]
func (h *ComplianceHandler) ResolveUBOReview(c *gin.Context) {
	var ownerID uint
	if _, err := fmt.Sscanf(c.Param("id"), "%d", &ownerID); err != nil || ownerID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req resolveUBOReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validator.SanitizeStruct(&req)

	var owner models.BeneficialOwner
	if err := h.db.First(&owner, ownerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "beneficial owner declaration not found"})
		return
	}

	updates := map[string]interface{}{
		"status":       req.Status,
		"review_notes": req.ReviewNotes,
	}
	if req.Status == models.UBOStatusVerified {
		updates["verified_at"] = gorm.Expr("NOW()")
	}
	if err := h.db.Model(&owner).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update review status"})
		return
	}

	h.writeAuditLog(owner.EntityUserID, "ubo_review_resolved", "BeneficialOwner", owner.ID,
		map[string]interface{}{"status": req.Status}, c.ClientIP())

	h.db.First(&owner, ownerID)
	c.JSON(http.StatusOK, owner)
}

// UploadCorporateStructureDocument handles POST /api/v1/compliance/ubo/documents
// @Summary Upload corporate structure documentation
// @Description Records metadata for a supporting document (cap table, trust deed, articles of incorporation). Binary upload goes to object storage, matching KYCDocument.
// @Tags compliance
// @Accept json
// @Produce json
// @Success 201 {object} models.CorporateStructureDocument
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /compliance/ubo/documents [post]
func (h *ComplianceHandler) UploadCorporateStructureDocument(c *gin.Context) {
	var req struct {
		EntityUserID uint   `json:"entity_user_id" binding:"required,gt=0"`
		DocumentType string `json:"document_type"  binding:"required,no_html"`
		FileName     string `json:"file_name"      binding:"required,no_html"`
		FileHash     string `json:"file_hash"      binding:"required,no_html"`
		MimeType     string `json:"mime_type"      binding:"omitempty,no_html"`
		SizeBytes    int64  `json:"size_bytes"     binding:"omitempty,gte=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validator.SanitizeStruct(&req)

	storagePath := fmt.Sprintf("encrypted/ubo/%d/%s/%s", req.EntityUserID, req.DocumentType, req.FileName)
	doc := models.CorporateStructureDocument{
		EntityUserID: req.EntityUserID,
		DocumentType: req.DocumentType,
		FileName:     req.FileName,
		FileHash:     req.FileHash,
		StoragePath:  storagePath,
		MimeType:     req.MimeType,
		SizeBytes:    req.SizeBytes,
	}
	if err := h.db.Create(&doc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save document record"})
		return
	}

	h.writeAuditLog(req.EntityUserID, "ubo_document_uploaded", "CorporateStructureDocument", doc.ID,
		map[string]interface{}{"type": req.DocumentType}, c.ClientIP())

	c.JSON(http.StatusCreated, doc)
}

func (h *ComplianceHandler) writeAuditLog(userID uint, action, entityType string, entityID uint, details map[string]interface{}, ip string) {
	raw, _ := json.Marshal(details)
	entry := models.ComplianceAuditLog{
		UserID:     userID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Details:    string(raw),
		IPAddress:  ip,
	}
	if err := h.db.Create(&entry).Error; err != nil {
		Logger.Warn("failed to write compliance audit log", zap.Error(err))
	}
}
