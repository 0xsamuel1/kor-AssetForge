package models

import (
	"time"

	"gorm.io/gorm"
)

// EmailDeliveryStatus enumerates lifecycle delivery states for dispatched emails.
type EmailDeliveryStatus string

const (
	EmailStatusQueued     EmailDeliveryStatus = "queued"
	EmailStatusSent       EmailDeliveryStatus = "sent"
	EmailStatusDelivered  EmailDeliveryStatus = "delivered"
	EmailStatusBounced    EmailDeliveryStatus = "bounced"
	EmailStatusOpened     EmailDeliveryStatus = "opened"
	EmailStatusClicked    EmailDeliveryStatus = "clicked"
	EmailStatusFailed     EmailDeliveryStatus = "failed"
	EmailStatusSpamReport EmailDeliveryStatus = "spam_report"
)

// BounceType classifies email bounce severities.
type BounceType string

const (
	BounceTypeHard      BounceType = "hard"
	BounceTypeSoft      BounceType = "soft"
	BounceTypeTransient BounceType = "transient"
)

// EmailDelivery tracks delivery events, timestamps, provider info, and bounce details for an email.
type EmailDelivery struct {
	ID           uint                `gorm:"primaryKey" json:"id"`
	MessageID    string              `gorm:"index" json:"message_id"`
	Recipient    string              `gorm:"index;not null" json:"recipient"`
	Subject      string              `json:"subject"`
	Provider     string              `json:"provider"`
	Status       EmailDeliveryStatus `gorm:"index;not null;default:'sent'" json:"status"`
	BounceType   BounceType          `json:"bounce_type,omitempty"`
	BounceReason string              `gorm:"type:text" json:"bounce_reason,omitempty"`
	AttemptCount int                 `gorm:"default:1" json:"attempt_count"`
	SentAt       *time.Time          `json:"sent_at,omitempty"`
	DeliveredAt  *time.Time          `json:"delivered_at,omitempty"`
	BouncedAt    *time.Time          `json:"bounced_at,omitempty"`
	OpenedAt     *time.Time          `json:"opened_at,omitempty"`
	ClickedAt    *time.Time          `json:"clicked_at,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	DeletedAt    gorm.DeletedAt      `gorm:"index" json:"-"`
}

// EmailSuppression tracks recipients whose emails hard-bounced or were marked spam to suppress future sends.
type EmailSuppression struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Recipient string    `gorm:"uniqueIndex;not null" json:"recipient"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailAnalyticsSummary aggregates delivery performance metrics and alert indicators.
type EmailAnalyticsSummary struct {
	TotalSent           int64   `json:"total_sent"`
	TotalDelivered      int64   `json:"total_delivered"`
	TotalBounced        int64   `json:"total_bounced"`
	TotalOpened         int64   `json:"total_opened"`
	TotalClicked        int64   `json:"total_clicked"`
	DeliveryRate        float64 `json:"delivery_rate"`         // percentage (0.0 to 100.0)
	BounceRate          float64 `json:"bounce_rate"`           // percentage (0.0 to 100.0)
	OpenRate            float64 `json:"open_rate"`             // percentage (0.0 to 100.0)
	HighBounceRateAlert bool    `json:"high_bounce_rate_alert"` // true if bounce rate exceeds threshold
	AlertMessage        string  `json:"alert_message,omitempty"`
}
