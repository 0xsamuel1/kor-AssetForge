package services

import (
	"errors"
	"fmt"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// TemplateVersioningService provides version history, diff, and rollback
// operations for email templates (#373).
//
// The versioning data model uses models.EmailTemplateVersion records that are
// created as immutable snapshots each time a template is saved. This service
// wraps the query and rollback logic so that handlers remain thin.
type TemplateVersioningService struct {
	db *gorm.DB
}

// NewTemplateVersioningService creates a TemplateVersioningService.
func NewTemplateVersioningService(db *gorm.DB) *TemplateVersioningService {
	return &TemplateVersioningService{db: db}
}

// ListVersions returns all version snapshots for the given template, ordered
// newest-first.
func (s *TemplateVersioningService) ListVersions(templateID uint) ([]models.EmailTemplateVersion, error) {
	var versions []models.EmailTemplateVersion
	if err := s.db.
		Where("template_id = ?", templateID).
		Order("version DESC").
		Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}
	return versions, nil
}

// GetVersion returns a single version snapshot by its sequential version number
// within the given template.
func (s *TemplateVersioningService) GetVersion(templateID uint, version int) (*models.EmailTemplateVersion, error) {
	var v models.EmailTemplateVersion
	err := s.db.
		Where("template_id = ? AND version = ?", templateID, version).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("version %d not found for template %d", version, templateID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch version: %w", err)
	}
	return &v, nil
}

// TemplateDiff holds a pairwise comparison of two version snapshots.
type TemplateDiff struct {
	TemplateID   uint   `json:"template_id"`
	FromVersion  int    `json:"from_version"`
	ToVersion    int    `json:"to_version"`
	SubjectChanged bool `json:"subject_changed"`
	BodyHTMLChanged bool `json:"body_html_changed"`
	BodyTextChanged bool `json:"body_text_changed"`
	VariablesChanged bool `json:"variables_changed"`
	From         *models.EmailTemplateVersion `json:"from"`
	To           *models.EmailTemplateVersion `json:"to"`
}

// DiffVersions returns a structural diff between two version snapshots.
func (s *TemplateVersioningService) DiffVersions(templateID uint, fromVersion, toVersion int) (*TemplateDiff, error) {
	from, err := s.GetVersion(templateID, fromVersion)
	if err != nil {
		return nil, err
	}
	to, err := s.GetVersion(templateID, toVersion)
	if err != nil {
		return nil, err
	}

	return &TemplateDiff{
		TemplateID:       templateID,
		FromVersion:      fromVersion,
		ToVersion:        toVersion,
		SubjectChanged:   from.Subject != to.Subject,
		BodyHTMLChanged:  from.BodyHTML != to.BodyHTML,
		BodyTextChanged:  from.BodyText != to.BodyText,
		VariablesChanged: from.Variables != to.Variables,
		From:             from,
		To:               to,
	}, nil
}

// RollbackResult carries the restored template and the new version snapshot
// created to record the rollback.
type RollbackResult struct {
	Template   *models.EmailTemplate        `json:"template"`
	NewVersion *models.EmailTemplateVersion `json:"new_version"`
}

// RollbackToVersion restores a template's content to a historical version.
//
// The rollback is non-destructive: a new version snapshot is appended so there
// is a full audit trail of the revert. The active template's Version counter is
// incremented (not reset to the target version number) to preserve monotonicity.
//
// changedBy is the ID of the user performing the rollback; changeNote is an
// optional human-readable explanation stored in the snapshot.
func (s *TemplateVersioningService) RollbackToVersion(
	templateID uint,
	targetVersion int,
	changedBy uint,
	changeNote string,
) (*RollbackResult, error) {
	// Load the live template.
	var tmpl models.EmailTemplate
	if err := s.db.First(&tmpl, templateID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("template %d not found", templateID)
		}
		return nil, fmt.Errorf("failed to load template: %w", err)
	}

	// Load the target snapshot.
	snap, err := s.GetVersion(templateID, targetVersion)
	if err != nil {
		return nil, err
	}

	if changeNote == "" {
		changeNote = fmt.Sprintf("rolled back to version %d", targetVersion)
	}

	var result RollbackResult
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Apply historical content to the live template.
		tmpl.Subject = snap.Subject
		tmpl.BodyHTML = snap.BodyHTML
		tmpl.BodyText = snap.BodyText
		tmpl.Variables = snap.Variables
		tmpl.Version++

		if err := tx.Save(&tmpl).Error; err != nil {
			return fmt.Errorf("failed to update template: %w", err)
		}

		// Record the rollback as a new version snapshot for audit purposes.
		newSnap := models.EmailTemplateVersion{
			TemplateID: tmpl.ID,
			Version:    tmpl.Version,
			Subject:    tmpl.Subject,
			BodyHTML:   tmpl.BodyHTML,
			BodyText:   tmpl.BodyText,
			Variables:  tmpl.Variables,
			ChangedBy:  changedBy,
			ChangeNote: changeNote,
		}
		if err := tx.Create(&newSnap).Error; err != nil {
			return fmt.Errorf("failed to record rollback snapshot: %w", err)
		}

		result.Template = &tmpl
		result.NewVersion = &newSnap
		return nil
	})

	if txErr != nil {
		return nil, txErr
	}

	return &result, nil
}
