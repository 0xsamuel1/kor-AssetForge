package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/utils"
	"gorm.io/gorm"
)

// EmailTemplateHandler exposes administrator endpoints for managing customizable,
// multi-language, versioned email templates with preview and A/B testing (#163).
type EmailTemplateHandler struct {
	DB         *gorm.DB
	Service    *services.EmailTemplateService
	Versioning *services.TemplateVersioningService
}

// NewEmailTemplateHandler creates an EmailTemplateHandler.
func NewEmailTemplateHandler(db *gorm.DB) *EmailTemplateHandler {
	return &EmailTemplateHandler{
		DB:         db,
		Service:    services.NewEmailTemplateService(db, nil),
		Versioning: services.NewTemplateVersioningService(db),
	}
}

type emailTemplateRequest struct {
	TemplateKey string   `json:"template_key" binding:"required,min=1,max=100"`
	Language    string   `json:"language"`
	Name        string   `json:"name" binding:"required,min=1,max=150"`
	Description string   `json:"description"`
	Subject     string   `json:"subject" binding:"required"`
	BodyHTML    string   `json:"body_html" binding:"required"`
	BodyText    string   `json:"body_text"`
	Variables   []string `json:"variables"`
	IsActive    bool     `json:"is_active"`
}

func marshalVariables(vars []string) string {
	if len(vars) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(vars)
	return string(b)
}

// ListTemplates returns templates, optionally filtered by template_key/language.
// GET /api/v1/admin/email-templates
func (h *EmailTemplateHandler) ListTemplates(c *gin.Context) {
	q := h.DB.Model(&models.EmailTemplate{})
	if key := c.Query("template_key"); key != "" {
		q = q.Where("template_key = ?", key)
	}
	if lang := c.Query("language"); lang != "" {
		q = q.Where("language = ?", lang)
	}
	var templates []models.EmailTemplate
	if err := q.Order("template_key ASC, language ASC, version DESC").Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch templates"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": templates})
}

// GetTemplate returns a single template by ID.
// GET /api/v1/admin/email-templates/:id
func (h *EmailTemplateHandler) GetTemplate(c *gin.Context) {
	var tmpl models.EmailTemplate
	if err := h.DB.First(&tmpl, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": tmpl})
}

// CreateTemplate creates a new template (version 1). Activating it deactivates
// any other active template sharing the same key/language.
// POST /api/v1/admin/email-templates
func (h *EmailTemplateHandler) CreateTemplate(c *gin.Context) {
	var req emailTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}
	variables := marshalVariables(req.Variables)
	if err := h.Service.ValidateTemplate(req.Subject, req.BodyHTML, req.BodyText, variables); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tmpl := models.EmailTemplate{
		TemplateKey: req.TemplateKey,
		Language:    req.Language,
		Name:        req.Name,
		Description: req.Description,
		Subject:     req.Subject,
		BodyHTML:    req.BodyHTML,
		BodyText:    req.BodyText,
		Variables:   variables,
		Version:     1,
		IsActive:    req.IsActive,
		CreatedBy:   currentUserID(c),
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if tmpl.IsActive {
			if err := tx.Model(&models.EmailTemplate{}).
				Where("template_key = ? AND language = ?", tmpl.TemplateKey, tmpl.Language).
				Update("is_active", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&tmpl).Error; err != nil {
			return err
		}
		return tx.Create(&models.EmailTemplateVersion{
			TemplateID: tmpl.ID,
			Version:    tmpl.Version,
			Subject:    tmpl.Subject,
			BodyHTML:   tmpl.BodyHTML,
			BodyText:   tmpl.BodyText,
			Variables:  tmpl.Variables,
			ChangedBy:  tmpl.CreatedBy,
			ChangeNote: "initial version",
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create template"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": tmpl})
}

// UpdateTemplate edits a template in place, bumping its version and recording a
// version snapshot for rollback/history.
// PUT /api/v1/admin/email-templates/:id
func (h *EmailTemplateHandler) UpdateTemplate(c *gin.Context) {
	var tmpl models.EmailTemplate
	if err := h.DB.First(&tmpl, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}

	var req struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Subject     *string   `json:"subject"`
		BodyHTML    *string   `json:"body_html"`
		BodyText    *string   `json:"body_text"`
		Variables   *[]string `json:"variables"`
		ChangeNote  string    `json:"change_note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		tmpl.Name = *req.Name
	}
	if req.Description != nil {
		tmpl.Description = *req.Description
	}
	if req.Subject != nil {
		tmpl.Subject = *req.Subject
	}
	if req.BodyHTML != nil {
		tmpl.BodyHTML = *req.BodyHTML
	}
	if req.BodyText != nil {
		tmpl.BodyText = *req.BodyText
	}
	if req.Variables != nil {
		tmpl.Variables = marshalVariables(*req.Variables)
	}

	if err := h.Service.ValidateTemplate(tmpl.Subject, tmpl.BodyHTML, tmpl.BodyText, tmpl.Variables); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tmpl.Version++
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&tmpl).Error; err != nil {
			return err
		}
		return tx.Create(&models.EmailTemplateVersion{
			TemplateID: tmpl.ID,
			Version:    tmpl.Version,
			Subject:    tmpl.Subject,
			BodyHTML:   tmpl.BodyHTML,
			BodyText:   tmpl.BodyText,
			Variables:  tmpl.Variables,
			ChangedBy:  currentUserID(c),
			ChangeNote: req.ChangeNote,
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update template"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": tmpl})
}

// ActivateTemplate marks a template active and deactivates other templates with
// the same key/language, so exactly one localized template is live at a time.
// POST /api/v1/admin/email-templates/:id/activate
func (h *EmailTemplateHandler) ActivateTemplate(c *gin.Context) {
	var tmpl models.EmailTemplate
	if err := h.DB.First(&tmpl, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.EmailTemplate{}).
			Where("template_key = ? AND language = ? AND id <> ?", tmpl.TemplateKey, tmpl.Language, tmpl.ID).
			Update("is_active", false).Error; err != nil {
			return err
		}
		return tx.Model(&tmpl).Update("is_active", true).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate template"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": tmpl})
}

// DeleteTemplate soft-deletes a template.
// DELETE /api/v1/admin/email-templates/:id
func (h *EmailTemplateHandler) DeleteTemplate(c *gin.Context) {
	if err := h.DB.Delete(&models.EmailTemplate{}, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete template"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListVersions returns the version history for a template.
// GET /api/v1/admin/email-templates/:id/versions
func (h *EmailTemplateHandler) ListVersions(c *gin.Context) {
	var versions []models.EmailTemplateVersion
	if err := h.DB.Where("template_id = ?", c.Param("id")).
		Order("version DESC").Find(&versions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch versions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": versions})
}

// Preview renders the supplied draft content (or an existing template's stored
// content) with sample variables, without sending anything. WYSIWYG editors call
// this to show a live rendering.
// POST /api/v1/admin/email-templates/preview
func (h *EmailTemplateHandler) Preview(c *gin.Context) {
	var req struct {
		Subject   string            `json:"subject" binding:"required"`
		BodyHTML  string            `json:"body_html" binding:"required"`
		BodyText  string            `json:"body_text"`
		Variables []string          `json:"variables"`
		SampleData map[string]string `json:"sample_data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rendered, err := h.Service.Preview(req.Subject, req.BodyHTML, req.BodyText, marshalVariables(req.Variables), req.SampleData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rendered})
}

// CreateVariant adds an A/B testing variant to a template.
// POST /api/v1/admin/email-templates/:id/variants
func (h *EmailTemplateHandler) CreateVariant(c *gin.Context) {
	templateID, err := utils.ParseUintParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}
	var tmpl models.EmailTemplate
	if err := h.DB.First(&tmpl, templateID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}

	var req struct {
		Name     string `json:"name" binding:"required"`
		Subject  string `json:"subject" binding:"required"`
		BodyHTML string `json:"body_html" binding:"required"`
		BodyText string `json:"body_text"`
		Weight   int    `json:"weight"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Variants must use the same declared variables as the base template.
	if err := h.Service.ValidateTemplate(req.Subject, req.BodyHTML, req.BodyText, tmpl.Variables); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}

	variant := models.EmailTemplateVariant{
		TemplateID: templateID,
		Name:       req.Name,
		Subject:    req.Subject,
		BodyHTML:   req.BodyHTML,
		BodyText:   req.BodyText,
		Weight:     req.Weight,
		IsActive:   true,
	}
	if err := h.DB.Create(&variant).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create variant"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": variant})
}

// ListVariants returns the A/B variants (with send counts) for a template.
// GET /api/v1/admin/email-templates/:id/variants
func (h *EmailTemplateHandler) ListVariants(c *gin.Context) {
	var variants []models.EmailTemplateVariant
	if err := h.DB.Where("template_id = ?", c.Param("id")).Find(&variants).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch variants"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": variants})
}

// RenderTemplate renders a stored, active template for the given key/language and
// variables — useful for verifying a template resolves correctly end-to-end.
// POST /api/v1/admin/email-templates/render
func (h *EmailTemplateHandler) RenderTemplate(c *gin.Context) {
	var req struct {
		TemplateKey string            `json:"template_key" binding:"required"`
		Language    string            `json:"language"`
		Variables   map[string]string `json:"variables"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rendered, err := h.Service.Render(req.TemplateKey, req.Language, req.Variables)
	if err != nil {
		if errors.Is(err, services.ErrTemplateNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no active template for this key/language"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rendered})
}

// RollbackToVersion restores a template to a previous version snapshot.
// A new version record is appended so the rollback itself is audited.
// POST /api/v1/admin/email-templates/:id/versions/:version/rollback
func (h *EmailTemplateHandler) RollbackToVersion(c *gin.Context) {
	templateID, err := utils.ParseUintParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}
	targetVersion, err := strconv.Atoi(c.Param("version"))
	if err != nil || targetVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version number"})
		return
	}

	var req struct {
		ChangeNote string `json:"change_note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template payload", "details": err.Error()})
		return
	}

	result, err := h.Versioning.RollbackToVersion(templateID, targetVersion, currentUserID(c), req.ChangeNote)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// DiffVersions returns a structural diff between two version snapshots.
// GET /api/v1/admin/email-templates/:id/versions/diff?from=1&to=2
func (h *EmailTemplateHandler) DiffVersions(c *gin.Context) {
	templateID, err := utils.ParseUintParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}
	fromVersion, err := strconv.Atoi(c.Query("from"))
	if err != nil || fromVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query param 'from' must be a positive integer"})
		return
	}
	toVersion, err := strconv.Atoi(c.Query("to"))
	if err != nil || toVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query param 'to' must be a positive integer"})
		return
	}

	diff, err := h.Versioning.DiffVersions(templateID, fromVersion, toVersion)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": diff})
}

