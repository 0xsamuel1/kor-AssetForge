package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// ErrTemplateNotFound is returned when no active template matches the requested
// key/language.
var ErrTemplateNotFound = errors.New("email template not found")

// placeholderPattern matches {{variable}} tokens, allowing surrounding spaces.
var placeholderPattern = regexp.MustCompile(`{{\s*([a-zA-Z0-9_]+)\s*}}`)

// RenderedEmail is the fully substituted output ready to be sent.
type RenderedEmail struct {
	Subject  string `json:"subject"`
	BodyHTML string `json:"body_html"`
	BodyText string `json:"body_text"`
	// Variant, when non-empty, identifies the A/B variant that was selected.
	Variant string `json:"variant,omitempty"`
}

// EmailTemplateService manages CRUD, validation, versioning, A/B selection and
// rendering of database-backed email templates (#163).
type EmailTemplateService struct {
	db    *gorm.DB
	email EmailService
}

// NewEmailTemplateService creates an EmailTemplateService. The email argument may
// be nil if the caller only needs rendering/validation (not delivery).
func NewEmailTemplateService(db *gorm.DB, email EmailService) *EmailTemplateService {
	return &EmailTemplateService{db: db, email: email}
}

// ValidateTemplate verifies a template's structure: required fields, valid
// declared-variable JSON, and that every {{placeholder}} used in the subject or
// body is declared in Variables. It returns a descriptive error on failure.
func (s *EmailTemplateService) ValidateTemplate(subject, bodyHTML, bodyText, variablesJSON string) error {
	if strings.TrimSpace(subject) == "" {
		return errors.New("subject is required")
	}
	if strings.TrimSpace(bodyHTML) == "" {
		return errors.New("body_html is required")
	}

	declared, err := parseVariableList(variablesJSON)
	if err != nil {
		return err
	}
	declaredSet := make(map[string]bool, len(declared))
	for _, v := range declared {
		declaredSet[v] = true
	}

	used := extractPlaceholders(subject + " " + bodyHTML + " " + bodyText)
	var undeclared []string
	for _, name := range used {
		if !declaredSet[name] {
			undeclared = append(undeclared, name)
		}
	}
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		return fmt.Errorf("template uses undeclared variables: %s", strings.Join(undeclared, ", "))
	}
	return nil
}

// Render loads the active template for key/language (falling back to English),
// selects an A/B variant when applicable, substitutes the provided variables and
// returns the rendered email. It errors if any declared variable is missing.
func (s *EmailTemplateService) Render(templateKey, language string, vars map[string]string) (*RenderedEmail, error) {
	tmpl, err := s.activeTemplate(templateKey, language)
	if err != nil {
		return nil, err
	}

	subject, bodyHTML, bodyText, variant := tmpl.Subject, tmpl.BodyHTML, tmpl.BodyText, ""
	if v := s.selectVariant(tmpl.ID); v != nil {
		subject, bodyHTML, bodyText, variant = v.Subject, v.BodyHTML, v.BodyText, v.Name
		s.db.Model(&models.EmailTemplateVariant{}).Where("id = ?", v.ID).
			UpdateColumn("sent_count", gorm.Expr("sent_count + 1"))
	}

	declared, err := parseVariableList(tmpl.Variables)
	if err != nil {
		return nil, err
	}
	for _, name := range declared {
		if _, ok := vars[name]; !ok {
			return nil, fmt.Errorf("missing value for template variable %q", name)
		}
	}

	return &RenderedEmail{
		Subject:  substitute(subject, vars),
		BodyHTML: substitute(bodyHTML, vars),
		BodyText: substitute(bodyText, vars),
		Variant:  variant,
	}, nil
}

// SendTemplated renders the named template and queues it for delivery using the
// configured EmailService.
func (s *EmailTemplateService) SendTemplated(toEmail, toName, templateKey, language string, vars map[string]string) error {
	if s.email == nil {
		return errors.New("email delivery is not configured")
	}
	rendered, err := s.Render(templateKey, language, vars)
	if err != nil {
		return err
	}
	return s.email.SendCustomEmail(toEmail, toName, rendered.Subject, rendered.BodyHTML, rendered.BodyText)
}

// Preview renders a template draft (not necessarily stored) with sample values,
// returning the output without sending anything. Unprovided declared variables
// are substituted with a readable placeholder so the preview never fails.
func (s *EmailTemplateService) Preview(subject, bodyHTML, bodyText, variablesJSON string, vars map[string]string) (*RenderedEmail, error) {
	if err := s.ValidateTemplate(subject, bodyHTML, bodyText, variablesJSON); err != nil {
		return nil, err
	}
	declared, _ := parseVariableList(variablesJSON)
	filled := make(map[string]string, len(declared))
	for _, name := range declared {
		filled[name] = "{" + name + "}"
	}
	for k, v := range vars {
		filled[k] = v
	}
	return &RenderedEmail{
		Subject:  substitute(subject, filled),
		BodyHTML: substitute(bodyHTML, filled),
		BodyText: substitute(bodyText, filled),
	}, nil
}

// activeTemplate fetches the active template for key/language, falling back to
// the platform default language ("en") when the requested locale has none.
func (s *EmailTemplateService) activeTemplate(templateKey, language string) (*models.EmailTemplate, error) {
	if language == "" {
		language = "en"
	}
	var tmpl models.EmailTemplate
	err := s.db.Where("template_key = ? AND language = ? AND is_active = ?", templateKey, language, true).
		First(&tmpl).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && language != "en" {
		err = s.db.Where("template_key = ? AND language = ? AND is_active = ?", templateKey, "en", true).
			First(&tmpl).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTemplateNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tmpl, nil
}

// selectVariant returns a randomly chosen active A/B variant weighted by Weight,
// or nil when the template has no active variants.
func (s *EmailTemplateService) selectVariant(templateID uint) *models.EmailTemplateVariant {
	var variants []models.EmailTemplateVariant
	if err := s.db.Where("template_id = ? AND is_active = ?", templateID, true).Find(&variants).Error; err != nil || len(variants) == 0 {
		return nil
	}
	total := 0
	for _, v := range variants {
		if v.Weight > 0 {
			total += v.Weight
		}
	}
	if total == 0 {
		return nil
	}
	pick := rand.Intn(total)
	for i := range variants {
		w := variants[i].Weight
		if w <= 0 {
			continue
		}
		if pick < w {
			return &variants[i]
		}
		pick -= w
	}
	return nil
}

// parseVariableList decodes a JSON array of variable names. An empty/blank value
// yields an empty list rather than an error.
func parseVariableList(variablesJSON string) ([]string, error) {
	variablesJSON = strings.TrimSpace(variablesJSON)
	if variablesJSON == "" {
		return nil, nil
	}
	var vars []string
	if err := json.Unmarshal([]byte(variablesJSON), &vars); err != nil {
		return nil, fmt.Errorf("invalid variables list (expected JSON array of strings): %w", err)
	}
	return vars, nil
}

// extractPlaceholders returns the unique variable names referenced in s.
func extractPlaceholders(s string) []string {
	matches := placeholderPattern.FindAllStringSubmatch(s, -1)
	seen := make(map[string]bool)
	var names []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	return names
}

// substitute replaces every {{name}} token (ignoring internal spaces) with its
// value from vars, leaving unknown tokens untouched.
func substitute(s string, vars map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := placeholderPattern.FindStringSubmatch(match)[1]
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
}
