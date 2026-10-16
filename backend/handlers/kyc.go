package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/validator"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// KYCHandler handles all KYC / AML / compliance endpoints.
type KYCHandler struct {
	db           *gorm.DB
	provider     services.KYCProvider
	amlProvider  services.AMLScreeningProvider
	emailService services.EmailService
}

// NewKYCHandler constructs a KYCHandler. Pass nil for provider to use the
// mock KYC provider, and nil for amlProvider to build one from
// config.LoadComplianceConfig() (which falls back to NoopAMLProvider when
// AML_PROVIDER is unset).
func NewKYCHandler(db *gorm.DB, provider services.KYCProvider, emailService services.EmailService, amlProvider services.AMLScreeningProvider) *KYCHandler {
	if provider == nil {
		provider = services.NewMockKYCProvider()
	}
	if amlProvider == nil {
		amlProvider = services.NewAMLScreeningProvider(config.LoadComplianceConfig())
	}
	return &KYCHandler{db: db, provider: provider, amlProvider: amlProvider, emailService: emailService}
}

// ---- request / response types -----------------------------------------------

type submitKYCRequest struct {
	UserID        uint   `json:"user_id"        binding:"required,gt=0"`
	FullName      string `json:"full_name"      binding:"required,min=2,max=200,no_html"`
	DateOfBirth   string `json:"date_of_birth"  binding:"required,no_html"` // YYYY-MM-DD
	Nationality   string `json:"nationality"    binding:"required,no_html"`
	DocumentType  string `json:"document_type"  binding:"required,no_html"` // passport | driver_license | national_id
	DocumentNumber string `json:"document_number" binding:"required,no_html"` // hashed before storage
}

type uploadDocumentRequest struct {
	KYCRecordID  uint   `json:"kyc_record_id"  binding:"required,gt=0"`
	DocumentType string `json:"document_type"  binding:"required,no_html"` // front | back | selfie | proof_of_address
	FileName     string `json:"file_name"      binding:"required,no_html"`
	FileHash     string `json:"file_hash"      binding:"required,no_html"` // client-side SHA-256
	MimeType     string `json:"mime_type"      binding:"omitempty,no_html"`
	SizeBytes    int64  `json:"size_bytes"     binding:"omitempty,gte=0"`
}

type amlScreenRequest struct {
	KYCRecordID uint `json:"kyc_record_id" binding:"required,gt=0"`
}

type amlReviewResolutionRequest struct {
	Cleared     bool   `json:"cleared"`
	ReviewNotes string `json:"review_notes" binding:"omitempty,no_html"`
}

type accreditedInvestorRequest struct {
	UserID      uint    `json:"user_id"       binding:"required,gt=0"`
	NetWorthUSD float64 `json:"net_worth_usd" binding:"required,gt=0"`
}

type kycWebhookPayload struct {
	ProviderRecordID string `json:"provider_record_id" binding:"required"`
	Status           string `json:"status"             binding:"required"`
	RiskScore        int    `json:"risk_score"`
	AMLCleared       bool   `json:"aml_cleared"`
	ReviewNotes      string `json:"review_notes"`
}

// ---- endpoints --------------------------------------------------------------

// SubmitKYC handles POST /api/v1/kyc/submit
// @Summary Submit KYC verification
// @Description Initiates KYC verification for a user by calling the provider and creating a KYCRecord with status=pending.
// @Tags kyc
// @Accept json
// @Produce json
// @Param submit body submitKYCRequest true "KYC submission details"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 409 {object} apperrors.ErrorResponse
// @Failure 502 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /kyc/submit [post]
func (h *KYCHandler) SubmitKYC(c *gin.Context) {
	var req submitKYCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validator.SanitizeStruct(&req)

	// Check for an existing pending/approved record to prevent duplicates.
	var existing models.KYCRecord
	if err := h.db.Where("user_id = ?", req.UserID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":  "KYC record already exists for this user",
			"status": existing.Status,
		})
		return
	}

	docHash := services.HashDocumentNumber(req.DocumentNumber)

	result, err := h.provider.SubmitVerification(
		req.UserID, req.FullName, req.DateOfBirth,
		req.Nationality, req.DocumentType, docHash,
	)
	if err != nil {
		Logger.Error("KYC provider submission failed", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "KYC provider unavailable"})
		return
	}

	record := models.KYCRecord{
		UserID:             req.UserID,
		ProviderRecordID:   result.ProviderRecordID,
		Status:             models.KYCStatus(result.Status),
		FullName:           req.FullName,
		DateOfBirth:        req.DateOfBirth,
		Nationality:        req.Nationality,
		DocumentType:       req.DocumentType,
		DocumentNumberHash: docHash,
	}

	if err := h.db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save KYC record"})
		return
	}

	h.writeAuditLog(req.UserID, "kyc_submitted", "KYCRecord", record.ID,
		map[string]interface{}{"provider_record_id": result.ProviderRecordID}, c.ClientIP())

	// AML screening runs automatically on submission rather than waiting for
	// a separate manual call (#377). A flagged result moves the record to
	// "review" instead of letting identity verification proceed to approval
	// on its own; a provider error also routes to manual review rather than
	// silently skipping the screen.
	amlScreening, amlErr := h.runAMLScreening(&record, c.ClientIP())
	if amlErr != nil {
		Logger.Warn("automatic AML screening failed at submission; routed to manual review", zap.Error(amlErr))
	}

	resp := gin.H{
		"message":            "KYC verification submitted successfully",
		"kyc_record_id":      record.ID,
		"provider_record_id": record.ProviderRecordID,
		"status":             record.Status,
	}
	if amlScreening != nil {
		resp["aml_screening_status"] = amlScreening.Status
		resp["aml_risk_level"] = amlScreening.RiskLevel
	}
	c.JSON(http.StatusCreated, resp)
}

// GetKYCStatus handles GET /api/v1/kyc/status?user_id=<n>
// @Summary Get KYC status
// @Description Polls the provider for the latest status and updates the local record.
// @Tags kyc
// @Accept json
// @Produce json
// @Param user_id query uint true "User ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /kyc/status [get]
func (h *KYCHandler) GetKYCStatus(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id query param required"})
		return
	}

	var userID uint
	if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil || userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	var record models.KYCRecord
	if err := h.db.Where("user_id = ?", userID).First(&record).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "KYC record not found"})
		return
	}

	// Poll provider for an update.
	result, err := h.provider.GetVerificationStatus(record.ProviderRecordID)
	if err != nil {
		// Return cached DB state on provider error.
		Logger.Warn("KYC provider status poll failed", zap.Error(err))
		c.JSON(http.StatusOK, record)
		return
	}

	// Persist any status change.
	if string(record.Status) != result.Status {
		updates := map[string]interface{}{
			"status":       result.Status,
			"risk_score":   result.RiskScore,
			"aml_cleared":  result.AMLCleared,
			"review_notes": result.ReviewNotes,
		}
		h.db.Model(&record).Updates(updates)

		// Sync accredited-investor flag on the User record.
		if result.Status == "approved" {
			h.db.Model(&models.User{}).Where("id = ?", userID).
				Updates(map[string]interface{}{"kyc_verified": true})
		}

		h.writeAuditLog(userID, "status_changed", "KYCRecord", record.ID,
			map[string]interface{}{"from": record.Status, "to": result.Status}, c.ClientIP())

		if h.emailService != nil {
			if err := h.notifyKYCStatusChange(userID, result.Status, result.ReviewNotes); err != nil {
				log.Printf("failed to queue KYC status update email: %v", err)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"kyc_record_id": record.ID,
		"status":        result.Status,
		"risk_score":    result.RiskScore,
		"aml_cleared":   result.AMLCleared,
		"review_notes":  result.ReviewNotes,
	})
}

// UploadDocument handles POST /api/v1/kyc/documents
// @Summary Upload KYC document
// @Description Records document metadata. Actual binary upload goes to object storage.
// @Tags kyc
// @Accept json
// @Produce json
// @Param document body uploadDocumentRequest true "Document upload details"
// @Success 201 {object} models.KYCDocument
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 404 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /kyc/documents [post]
func (h *KYCHandler) UploadDocument(c *gin.Context) {
	var req uploadDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validator.SanitizeStruct(&req)

	var kyc models.KYCRecord
	if err := h.db.First(&kyc, req.KYCRecordID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "KYC record not found"})
		return
	}

	// StoragePath mocked — in production this would be an S3/GCS/encrypted-blob URL.
	storagePath := fmt.Sprintf("encrypted/kyc/%d/%s/%s",
		req.KYCRecordID, req.DocumentType, req.FileName)

	doc := models.KYCDocument{
		KYCRecordID:  req.KYCRecordID,
		DocumentType: req.DocumentType,
		FileName:     req.FileName,
		FileHash:     req.FileHash,
		StoragePath:  storagePath,
		MimeType:     req.MimeType,
		SizeBytes:    req.SizeBytes,
		Status:       "pending",
	}

	if err := h.db.Create(&doc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save document record"})
		return
	}

	h.writeAuditLog(kyc.UserID, "document_uploaded", "KYCDocument", doc.ID,
		map[string]interface{}{"type": req.DocumentType, "hash": req.FileHash}, c.ClientIP())

	c.JSON(http.StatusCreated, doc)
}

// ScreenAML handles POST /api/v1/kyc/aml/screen
// @Summary Screen for AML
// @Description Triggers AML screening via the provider and persists the result.
// @Tags kyc
// @Accept json
// @Produce json
// @Param aml_screen body amlScreenRequest true "AML screening request"
// @Success 201 {object} models.AMLScreening
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 404 {object} apperrors.ErrorResponse
// @Failure 502 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /kyc/aml/screen [post]
func (h *KYCHandler) ScreenAML(c *gin.Context) {
	var req amlScreenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var kyc models.KYCRecord
	if err := h.db.First(&kyc, req.KYCRecordID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "KYC record not found"})
		return
	}

	screening, err := h.runAMLScreening(&kyc, c.ClientIP())
	if err != nil {
		Logger.Error("AML screening failed", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "AML provider unavailable"})
		return
	}

	c.JSON(http.StatusCreated, screening)
}

// ResolveAMLReview handles POST /api/v1/kyc/aml/review/:id
// @Summary Resolve a flagged AML screening
// @Description Manual-review decision on a flagged AMLScreening: clears it or confirms the match and rejects the KYC record.
// @Tags kyc
// @Accept json
// @Produce json
// @Param id path uint true "AMLScreening ID"
// @Param resolution body amlReviewResolutionRequest true "Review resolution"
// @Success 200 {object} models.AMLScreening
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /kyc/aml/review/{id} [post]
func (h *KYCHandler) ResolveAMLReview(c *gin.Context) {
	var screeningID uint
	if _, err := fmt.Sscanf(c.Param("id"), "%d", &screeningID); err != nil || screeningID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req amlReviewResolutionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validator.SanitizeStruct(&req)

	var screening models.AMLScreening
	if err := h.db.First(&screening, screeningID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "AML screening not found"})
		return
	}

	var kyc models.KYCRecord
	if err := h.db.First(&kyc, screening.KYCRecordID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "KYC record not found"})
		return
	}

	newStatus := "flagged"
	if req.Cleared {
		newStatus = "clear"
	}
	if err := h.db.Model(&screening).Update("status", newStatus).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update AML screening"})
		return
	}

	if req.Cleared {
		h.db.Model(&kyc).Updates(map[string]interface{}{"aml_cleared": true})
	} else {
		h.db.Model(&kyc).Updates(map[string]interface{}{"aml_cleared": false, "status": models.KYCStatusRejected, "review_notes": req.ReviewNotes})
	}

	h.writeAuditLog(kyc.UserID, "aml_review_resolved", "AMLScreening", screening.ID,
		map[string]interface{}{"cleared": req.Cleared, "notes": req.ReviewNotes}, c.ClientIP())

	h.db.First(&screening, screeningID)
	c.JSON(http.StatusOK, screening)
}

// runAMLScreening calls the configured AML provider for kyc, persists the
// result, and updates kyc.AMLCleared / kyc.Status. Shared by the automatic
// screen on submission and the manual POST /kyc/aml/screen endpoint. A
// provider error or a flagged result both route the record to
// KYCStatusReview rather than letting it proceed toward approval.
func (h *KYCHandler) runAMLScreening(kyc *models.KYCRecord, clientIP string) (*models.AMLScreening, error) {
	result, err := h.amlProvider.Screen(kyc.FullName, kyc.Nationality, kyc.DateOfBirth)
	if err != nil {
		h.db.Model(kyc).Update("status", models.KYCStatusReview)
		h.writeAuditLog(kyc.UserID, "aml_screening_error", "KYCRecord", kyc.ID,
			map[string]interface{}{"error": err.Error()}, clientIP)
		return nil, err
	}

	matchesJSON, _ := json.Marshal(result.Matches)
	now := result.ScreenedAt
	screening := models.AMLScreening{
		KYCRecordID: kyc.ID,
		ScreeningID: result.ScreeningID,
		Status:      result.Status,
		RiskLevel:   models.AMLRiskLevel(result.RiskLevel),
		Matches:     string(matchesJSON),
		ScreenedAt:  &now,
	}
	if err := h.db.Create(&screening).Error; err != nil {
		return nil, fmt.Errorf("failed to save AML screening: %w", err)
	}

	switch result.Status {
	case "clear":
		h.db.Model(kyc).Update("aml_cleared", true)
	case "flagged":
		// Flagged never auto-clears or auto-rejects -- it waits for a human
		// decision via ResolveAMLReview.
		h.db.Model(kyc).Updates(map[string]interface{}{"aml_cleared": false, "status": models.KYCStatusReview})
	}

	h.writeAuditLog(kyc.UserID, "aml_screened", "AMLScreening", screening.ID,
		map[string]interface{}{"status": result.Status, "risk_level": result.RiskLevel}, clientIP)

	return &screening, nil
}

// VerifyAccreditedInvestor handles POST /api/v1/kyc/accredited
// @Summary Verify accredited investor
// @Description Verifies if a user is an accredited investor based on net worth.
// @Tags kyc
// @Accept json
// @Produce json
// @Param accredited_investor body accreditedInvestorRequest true "Accredited investor verification request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 502 {object} apperrors.ErrorResponse
// @Router /kyc/accredited [post]
func (h *KYCHandler) VerifyAccreditedInvestor(c *gin.Context) {
	var req accreditedInvestorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isAccredited, err := h.provider.VerifyAccreditedInvestor(req.UserID, req.NetWorthUSD)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider error"})
		return
	}

	h.db.Model(&models.User{}).Where("id = ?", req.UserID).
		Update("accredited_investor", isAccredited)

	h.writeAuditLog(req.UserID, "accredited_investor_verified", "User", req.UserID,
		map[string]interface{}{"result": isAccredited}, c.ClientIP())

	c.JSON(http.StatusOK, gin.H{
		"user_id":             req.UserID,
		"accredited_investor": isAccredited,
	})
}

// GetAuditLog handles GET /api/v1/kyc/audit?user_id=<n>
// @Summary Get KYC audit log
// @Description Retrieves a list of compliance audit logs for a specific user.
// @Tags kyc
// @Accept json
// @Produce json
// @Param user_id query uint true "User ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /kyc/audit [get]
func (h *KYCHandler) GetAuditLog(c *gin.Context) {
	var userID uint
	if _, err := fmt.Sscanf(c.Query("user_id"), "%d", &userID); err != nil || userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid user_id query param required"})
		return
	}

	var logs []models.ComplianceAuditLog
	if err := h.db.Where("user_id = ?", userID).
		Order("created_at desc").Limit(100).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch audit log"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user_id": userID, "audit_log": logs})
}

// ComplianceReport handles GET /api/v1/compliance/report
// @Summary Get compliance report
// @Description Returns aggregate compliance statistics for regulatory reporting.
// @Tags kyc
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /compliance/report [get]
func (h *KYCHandler) ComplianceReport(c *gin.Context) {
	type report struct {
		TotalKYCRecords   int64 `json:"total_kyc_records"`
		Approved          int64 `json:"approved"`
		Pending           int64 `json:"pending"`
		Rejected          int64 `json:"rejected"`
		AMLCleared        int64 `json:"aml_cleared"`
		AccreditedInvestors int64 `json:"accredited_investors"`
		GeneratedAt       time.Time `json:"generated_at"`
	}

	var r report
	r.GeneratedAt = time.Now()

	h.db.Model(&models.KYCRecord{}).Count(&r.TotalKYCRecords)
	h.db.Model(&models.KYCRecord{}).Where("status = ?", "approved").Count(&r.Approved)
	h.db.Model(&models.KYCRecord{}).Where("status = ?", "pending").Count(&r.Pending)
	h.db.Model(&models.KYCRecord{}).Where("status = ?", "rejected").Count(&r.Rejected)
	h.db.Model(&models.KYCRecord{}).Where("aml_cleared = ?", true).Count(&r.AMLCleared)
	h.db.Model(&models.User{}).Where("accredited_investor = ?", true).Count(&r.AccreditedInvestors)

	c.JSON(http.StatusOK, r)
}

// HandleKYCWebhook handles POST /webhooks/kyc
// @Summary Handle KYC webhook
// @Description Receives status update callbacks from the KYC provider.
// @Tags kyc
// @Accept json
// @Produce json
// @Param webhook body kycWebhookPayload true "KYC webhook payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /webhooks/kyc [post]
func (h *KYCHandler) HandleKYCWebhook(c *gin.Context) {
	var payload kycWebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook payload"})
		return
	}

	var record models.KYCRecord
	if err := h.db.Where("provider_record_id = ?", payload.ProviderRecordID).
		First(&record).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "KYC record not found"})
		return
	}

	updates := map[string]interface{}{
		"status":       payload.Status,
		"risk_score":   payload.RiskScore,
		"aml_cleared":  payload.AMLCleared,
		"review_notes": payload.ReviewNotes,
	}
	h.db.Model(&record).Updates(updates)

	if payload.Status == "approved" {
		h.db.Model(&models.User{}).Where("id = ?", record.UserID).
			Update("kyc_verified", true)
	}

	h.writeAuditLog(record.UserID, "webhook_status_update", "KYCRecord", record.ID,
		map[string]interface{}{"new_status": payload.Status}, c.ClientIP())

	if h.emailService != nil {
		if err := h.notifyKYCStatusChange(record.UserID, payload.Status, payload.ReviewNotes); err != nil {
			log.Printf("failed to queue KYC webhook status email: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

// ---- internal helpers -------------------------------------------------------

func (h *KYCHandler) notifyKYCStatusChange(userID uint, status, reviewNotes string) error {
	var user models.User
	if err := h.db.Select("email", "username").First(&user, userID).Error; err != nil {
		return err
	}
	return h.emailService.SendKYCStatusUpdate(user.Email, user.Username, status, reviewNotes)
}

func (h *KYCHandler) writeAuditLog(userID uint, action, entityType string, entityID uint, details map[string]interface{}, ip string) {
	raw, _ := json.Marshal(details)
	log := models.ComplianceAuditLog{
		UserID:     userID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Details:    string(raw),
		IPAddress:  ip,
	}
	if err := h.db.Create(&log).Error; err != nil {
		Logger.Warn("failed to write compliance audit log", zap.Error(err))
	}
}
