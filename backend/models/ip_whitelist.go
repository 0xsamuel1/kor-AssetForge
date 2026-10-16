package models

import (
	"time"

	"gorm.io/gorm"
)

// IPWhitelistEntry represents a single allowed IP address or CIDR range.
type IPWhitelistEntry struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CIDR        string         `gorm:"not null;uniqueIndex" json:"cidr"`
	Description string         `gorm:"type:varchar(255)" json:"description,omitempty"`
	CreatedBy   uint           `gorm:"not null;index" json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
