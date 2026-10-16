package models

import (
	"time"

	"gorm.io/gorm"
)

type FractionalizationConfig struct {
	ID                     uint           `gorm:"primaryKey" json:"id"`
	AssetType              string         `gorm:"uniqueIndex;not null" json:"asset_type"`
	MinFractionSize        float64        `gorm:"default:0.01" json:"min_fraction_size"`
	MaxFractionSize        float64        `gorm:"default:100.0" json:"max_fraction_size"`
	MinInvestmentAmount    float64        `gorm:"default:10.0" json:"min_investment_amount"`
	MaxFractionalOwners    int            `gorm:"default:1000" json:"max_fractional_owners"`
	RequireAccreditation   bool           `gorm:"default:false" json:"require_accreditation"`
	MinHoldingPeriodDays   int            `gorm:"default:0" json:"min_holding_period_days"`
	MaxHoldingPerOwner     float64        `gorm:"default:25.0" json:"max_holding_per_owner_percent"`
	Enabled                bool           `gorm:"default:true" json:"enabled"`
	CreatedBy              uint           `json:"created_by"`
	UpdatedBy              uint           `json:"updated_by"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	DeletedAt              gorm.DeletedAt `gorm:"index" json:"-"`
}

type AssetFractionLimit struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	AssetID           uint           `gorm:"uniqueIndex;not null" json:"asset_id"`
	Asset             Asset          `gorm:"foreignKey:AssetID" json:"-"`
	MinFractionSize   float64        `json:"min_fraction_size"`
	MaxFractionSize   float64        `json:"max_fraction_size"`
	MinInvestment     float64        `json:"min_investment"`
	MaxOwners         int            `json:"max_owners"`
	MaxPerOwner       float64        `json:"max_per_owner_percent"`
	OverrideGlobal    bool           `gorm:"default:false" json:"override_global"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
