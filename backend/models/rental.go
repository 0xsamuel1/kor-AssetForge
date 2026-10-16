package models

import (
	"time"

	"gorm.io/gorm"
)

type RentalPeriod string

const (
	RentalDaily   RentalPeriod = "daily"
	RentalWeekly  RentalPeriod = "weekly"
	RentalMonthly RentalPeriod = "monthly"
)

type RentalStatus string

const (
	RentalActive     RentalStatus = "active"
	RentalCompleted  RentalStatus = "completed"
	RentalCancelled  RentalStatus = "cancelled"
	RentalOverdue    RentalStatus = "overdue"
	RentalDisputed   RentalStatus = "disputed"
)

type Rental struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	AssetID           uint           `gorm:"not null;index" json:"asset_id"`
	Asset             Asset          `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	LessorID          uint           `gorm:"not null;index" json:"lessor_id"`
	Lessor            User           `gorm:"foreignKey:LessorID" json:"-"`
	LesseeID          uint           `gorm:"not null;index" json:"lessee_id"`
	Lessee            User           `gorm:"foreignKey:LesseeID" json:"-"`
	Period            RentalPeriod   `gorm:"not null" json:"period"`
	RateAmount        int64          `gorm:"not null" json:"rate_amount"`
	RateCurrency      string         `gorm:"default:'USD'" json:"rate_currency"`
	SecurityDeposit   int64          `gorm:"default:0" json:"security_deposit"`
	StartDate         time.Time      `gorm:"not null" json:"start_date"`
	EndDate           time.Time      `gorm:"not null" json:"end_date"`
	Status            RentalStatus   `gorm:"default:'active'" json:"status"`
	AutoRenew         bool           `gorm:"default:false" json:"auto_renew"`
	LateFeePercent    float64        `gorm:"default:5.0" json:"late_fee_percent"`
	Terms             string         `gorm:"type:text" json:"terms,omitempty"`
	SignedByLessor    bool           `gorm:"default:false" json:"signed_by_lessor"`
	SignedByLessee    bool           `gorm:"default:false" json:"signed_by_lessee"`
	ContractHash      string         `json:"contract_hash,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

type RentalPayment struct {
	ID              uint          `gorm:"primaryKey" json:"id"`
	RentalID        uint          `gorm:"not null;index" json:"rental_id"`
	Rental          Rental        `gorm:"foreignKey:RentalID" json:"-"`
	Amount          int64         `gorm:"not null" json:"amount"`
	Currency        string        `gorm:"default:'USD'" json:"currency"`
	DueDate         time.Time     `gorm:"not null" json:"due_date"`
	PaidAt          *time.Time    `json:"paid_at,omitempty"`
	Status          string        `gorm:"default:'pending'" json:"status"`
	TransactionHash string        `json:"transaction_hash,omitempty"`
	LateFee         int64         `gorm:"default:0" json:"late_fee"`
	Notes           string        `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
}

type RentalHistory struct {
	ID        uint         `gorm:"primaryKey" json:"id"`
	RentalID  uint         `gorm:"not null;index" json:"rental_id"`
	Rental    Rental       `gorm:"foreignKey:RentalID" json:"-"`
	Event     string       `gorm:"not null" json:"event"`
	Detail    string       `gorm:"type:text" json:"detail,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}
