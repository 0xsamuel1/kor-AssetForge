package models

import (
	"time"

	"gorm.io/gorm"
)

// PaymentStatus represents the lifecycle state of a fiat payment.
type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusProcessing PaymentStatus = "processing"
	PaymentStatusCompleted  PaymentStatus = "completed"
	PaymentStatusFailed     PaymentStatus = "failed"
	PaymentStatusRefunded   PaymentStatus = "refunded"
)

// PaymentGateway identifies which payment provider processed the payment.
type PaymentGateway string

const (
	GatewayStripe PaymentGateway = "stripe"
	GatewayPayPal PaymentGateway = "paypal"
)

// Payment records a fiat-to-token purchase initiated through a payment gateway.
type Payment struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	UserID          uint           `gorm:"not null;index" json:"user_id"`
	User            User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	AssetID         uint           `gorm:"not null;index" json:"asset_id"`
	Asset           Asset          `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	Gateway         PaymentGateway `gorm:"not null" json:"gateway"`
	GatewayPaymentID string        `gorm:"uniqueIndex" json:"gateway_payment_id"`
	AmountFiat      int64          `gorm:"not null" json:"amount_fiat"`   // in cents
	Currency        string         `gorm:"not null;default:'USD'" json:"currency"`
	TokenAmount     int64          `gorm:"not null" json:"token_amount"` // tokens to credit
	Status          PaymentStatus  `gorm:"not null;default:'pending'" json:"status"`
	FailureReason   string         `gorm:"type:text" json:"failure_reason,omitempty"`
	WebhookPayload  string         `gorm:"type:text" json:"-"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// PaymentReconciliation tracks gateway reconciliation runs.
type PaymentReconciliation struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Gateway         PaymentGateway `gorm:"not null" json:"gateway"`
	PeriodStart     time.Time `gorm:"not null" json:"period_start"`
	PeriodEnd       time.Time `gorm:"not null" json:"period_end"`
	TotalPayments   int       `gorm:"default:0" json:"total_payments"`
	TotalAmount     int64     `gorm:"default:0" json:"total_amount"`
	Discrepancies   int       `gorm:"default:0" json:"discrepancies"`
	CreatedAt       time.Time `json:"created_at"`
}
