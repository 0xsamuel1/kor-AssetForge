package models

import "time"

// MetadataVersion stores a historical snapshot of an asset's metadata at each change.
type MetadataVersion struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	AssetID      uint      `gorm:"not null;index" json:"asset_id"`
	Asset        Asset     `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	Version      int       `gorm:"not null" json:"version"`
	MetadataURI  string    `json:"metadata_uri"`
	MetadataHash string    `json:"metadata_hash"`
	Metadata     string    `gorm:"type:text" json:"metadata"`
	ChangedBy    uint      `gorm:"not null" json:"changed_by"`
	ChangeNote   string    `gorm:"type:varchar(500)" json:"change_note,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
