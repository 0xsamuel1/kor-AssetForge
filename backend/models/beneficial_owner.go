package models

import (
	"time"

	"gorm.io/gorm"
)

// UBOVerificationStatus tracks the lifecycle of a beneficial-owner declaration.
type UBOVerificationStatus string

const (
	UBOStatusDeclared  UBOVerificationStatus = "declared"
	UBOStatusReview    UBOVerificationStatus = "review"
	UBOStatusVerified  UBOVerificationStatus = "verified"
	UBOStatusRejected  UBOVerificationStatus = "rejected"
)

// UBOOwnershipThresholdPct is the ownership percentage at or above which an
// individual must be declared as a beneficial owner, matching the FinCEN
// Customer Due Diligence rule's 25% threshold (and the FATF / EU AMLD
// recommendation of the same figure). Below this, an individual may still
// be declared as a controlling person without an ownership stake.
const UBOOwnershipThresholdPct = 25.0

// BeneficialOwner represents one individual with an ownership stake in, or
// control over, an entity (a corporate/organizational User account) on the
// platform. An entity may have several owners; percentages are validated to
// sum to <=100 per entity at the service layer, not at the DB layer, because
// partial declarations (not yet complete) are a valid in-progress state.
type BeneficialOwner struct {
	ID                 uint                  `gorm:"primaryKey"           json:"id"`
	EntityUserID       uint                  `gorm:"not null;index"       json:"entity_user_id"` // the corporate User this owner is declared for
	EntityUser         User                  `gorm:"foreignKey:EntityUserID" json:"-"`
	FullName           string                `gorm:"not null"             json:"full_name"`
	DateOfBirth        string                `                            json:"date_of_birth"`
	Nationality        string                `                            json:"nationality"`
	OwnershipPercent   float64               `gorm:"not null"             json:"ownership_percent"`
	IsControllingParty bool                  `gorm:"default:false"        json:"is_controlling_party"` // control without equity, e.g. a managing director
	OwnershipLayer      string                `gorm:"type:text"            json:"ownership_layer,omitempty"` // free-text description of the chain through intermediate entities
	DocumentNumberHash string                `gorm:"column:document_number_hash" json:"-"` // SHA-256, never exposed; reuses services.HashDocumentNumber
	Status             UBOVerificationStatus `gorm:"default:'declared'"   json:"status"`
	ReviewNotes        string                `gorm:"type:text"            json:"review_notes,omitempty"`
	VerifiedAt         *time.Time            `                            json:"verified_at"`
	CreatedAt          time.Time             `                            json:"created_at"`
	UpdatedAt          time.Time             `                            json:"updated_at"`
	DeletedAt          gorm.DeletedAt        `gorm:"index"                json:"-"`
}

// RequiresDeclaration reports whether this owner meets the disclosure
// threshold on ownership alone (a controlling party is always required to be
// declared regardless of equity stake).
func (b *BeneficialOwner) RequiresDeclaration() bool {
	return b.IsControllingParty || b.OwnershipPercent >= UBOOwnershipThresholdPct
}

// CorporateStructureDocument records supporting documentation of an entity's
// ownership chain (cap table, trust deed, articles of incorporation, etc.),
// mirroring KYCDocument's storage model.
type CorporateStructureDocument struct {
	ID           uint           `gorm:"primaryKey"    json:"id"`
	EntityUserID uint           `gorm:"not null;index" json:"entity_user_id"`
	DocumentType string         `gorm:"not null"      json:"document_type"` // cap_table | trust_deed | articles_of_incorporation | org_chart
	FileName     string         `gorm:"not null"      json:"file_name"`
	FileHash     string         `gorm:"not null"      json:"file_hash"`
	StoragePath  string         `gorm:"not null"      json:"-"`
	MimeType     string         `                     json:"mime_type"`
	SizeBytes    int64          `                     json:"size_bytes"`
	CreatedAt    time.Time      `                     json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index"         json:"-"`
}
