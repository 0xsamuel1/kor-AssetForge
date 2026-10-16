package models

import "time"

// ComparisonHistory records a saved side-by-side asset comparison so users can
// revisit prior analyses (#167). AssetIDs and Criteria are stored as JSON arrays.
type ComparisonHistory struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	AssetIDs    string    `gorm:"type:text;not null" json:"asset_ids"`
	Criteria    string    `gorm:"type:text" json:"criteria,omitempty"`
	AssetsCount int       `gorm:"column:assets_count" json:"assets_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName pins the table name independent of the struct field naming.
func (ComparisonHistory) TableName() string {
	return "comparison_histories"
}
