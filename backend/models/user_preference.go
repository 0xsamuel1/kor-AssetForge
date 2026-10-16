package models

import (
	"time"

	"gorm.io/gorm"
)

type InteractionType string

const (
	InteractionView    InteractionType = "view"
	InteractionPurchase InteractionType = "purchase"
	InteractionList    InteractionType = "list"
	InteractionSearch  InteractionType = "search"
)

type UserInteraction struct {
	ID              uint            `gorm:"primaryKey" json:"id"`
	UserID          uint            `gorm:"not null;index:idx_interactions_user" json:"user_id"`
	User            User            `gorm:"foreignKey:UserID" json:"-"`
	AssetID         uint            `gorm:"not null;index:idx_interactions_user" json:"asset_id"`
	Asset           Asset           `gorm:"foreignKey:AssetID" json:"-"`
	InteractionType InteractionType `gorm:"not null;index" json:"interaction_type"`
	Weight          float64         `gorm:"default:1.0" json:"weight"`
	Metadata        string          `gorm:"type:text" json:"metadata,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	DeletedAt       gorm.DeletedAt  `gorm:"index" json:"-"`
}

type UserPreference struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	UserID            uint           `gorm:"uniqueIndex;not null" json:"user_id"`
	User              User           `gorm:"foreignKey:UserID" json:"-"`
	PreferredCategories string       `gorm:"type:text" json:"preferred_categories"`
	PreferredTags     string         `gorm:"type:text" json:"preferred_tags"`
	PriceRangeMin     float64        `json:"price_range_min"`
	PriceRangeMax     float64        `json:"price_range_max"`
	RiskTolerance     string         `gorm:"default:'moderate'" json:"risk_tolerance"`
	UpdatedAt         time.Time      `json:"updated_at"`
	CreatedAt         time.Time      `json:"created_at"`
}

type AssetRecommendation struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"not null;index:idx_recommendations_user" json:"user_id"`
	AssetID        uint      `gorm:"not null;index:idx_recommendations_user" json:"asset_id"`
	Asset          Asset     `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	Score          float64   `gorm:"not null" json:"score"`
	Reason         string    `gorm:"type:text" json:"reason"`
	IsViewed       bool      `gorm:"default:false" json:"is_viewed"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}
