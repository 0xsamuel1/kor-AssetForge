package models

import (
	"time"

	"gorm.io/gorm"
)

// EmailTemplate is an administrator-managed, versioned email template. Templates
// are keyed by a stable TemplateKey (e.g. "verification", "kyc_status") and a
// language code so the platform can render localized, customizable content with
// {{variable}} substitution instead of hard-coded strings (#163).
type EmailTemplate struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	TemplateKey string `gorm:"not null;index:idx_email_templates_key_lang" json:"template_key"`
	Language    string `gorm:"not null;default:'en';index:idx_email_templates_key_lang" json:"language"`
	Name        string `gorm:"not null" json:"name"`
	Description string `gorm:"type:text" json:"description,omitempty"`
	Subject     string `gorm:"not null" json:"subject"`
	BodyHTML    string `gorm:"type:text;not null" json:"body_html"`
	BodyText    string `gorm:"type:text" json:"body_text,omitempty"`
	// Variables is a JSON array of the variable names this template expects,
	// e.g. ["name","verification_url"]. Used for validation and the editor UI.
	Variables string `gorm:"type:text" json:"variables,omitempty"`
	Version   int    `gorm:"not null;default:1" json:"version"`
	IsActive  bool   `gorm:"default:false;index" json:"is_active"`
	CreatedBy uint   `json:"created_by,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// EmailTemplateVersion captures an immutable snapshot of a template each time it
// is edited, providing version-control history and rollback support.
type EmailTemplateVersion struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TemplateID uint      `gorm:"not null;index" json:"template_id"`
	Version    int       `gorm:"not null" json:"version"`
	Subject    string    `gorm:"not null" json:"subject"`
	BodyHTML   string    `gorm:"type:text;not null" json:"body_html"`
	BodyText   string    `gorm:"type:text" json:"body_text,omitempty"`
	Variables  string    `gorm:"type:text" json:"variables,omitempty"`
	ChangedBy  uint      `json:"changed_by,omitempty"`
	ChangeNote string    `gorm:"type:text" json:"change_note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// EmailTemplateVariant defines an A/B-testing variant of a template. When one or
// more active variants exist for a template they are selected probabilistically
// according to their Weight; otherwise the base template content is used.
type EmailTemplateVariant struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	TemplateID uint   `gorm:"not null;index" json:"template_id"`
	Name       string `gorm:"not null" json:"name"`
	Subject    string `gorm:"not null" json:"subject"`
	BodyHTML   string `gorm:"type:text;not null" json:"body_html"`
	BodyText   string `gorm:"type:text" json:"body_text,omitempty"`
	// Weight controls the relative selection probability (default 1).
	Weight    int  `gorm:"not null;default:1" json:"weight"`
	IsActive  bool `gorm:"default:true;index" json:"is_active"`
	SentCount int64 `gorm:"default:0" json:"sent_count"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
