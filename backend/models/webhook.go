package models

import (
	"time"

	"gorm.io/gorm"
)

// WebhookEventType enumerates supported outgoing webhook events
type WebhookEventType string

const (
	WebhookEventAssetMinted    WebhookEventType = "asset.minted"
	WebhookEventAssetListed    WebhookEventType = "asset.listed"
	WebhookEventAssetTransfer  WebhookEventType = "asset.transferred"
	WebhookEventP2POrderFilled WebhookEventType = "p2p.order.filled"
	WebhookEventStakeCreated   WebhookEventType = "staking.staked"
	WebhookEventRewardClaimed  WebhookEventType = "staking.reward_claimed"
	WebhookEventDisputeFiled   WebhookEventType = "dispute.filed"
	WebhookEventDisputeResolved WebhookEventType = "dispute.resolved"
)

// WebhookDeliveryStatus tracks the delivery state of a webhook attempt
type WebhookDeliveryStatus string

const (
	WebhookDeliveryPending   WebhookDeliveryStatus = "pending"
	WebhookDeliverySuccess   WebhookDeliveryStatus = "success"
	WebhookDeliveryFailed    WebhookDeliveryStatus = "failed"
	WebhookDeliveryRetrying  WebhookDeliveryStatus = "retrying"
	WebhookDeliveryAbandoned WebhookDeliveryStatus = "abandoned"
)

// WebhookSubscription represents a registered outgoing webhook endpoint
type WebhookSubscription struct {
	ID          uint             `gorm:"primaryKey" json:"id"`
	UserID      uint             `gorm:"not null;index" json:"user_id"`
	User        User             `gorm:"foreignKey:UserID" json:"-"`
	URL         string           `gorm:"not null" json:"url"`
	Secret      string           `gorm:"not null" json:"-"`
	Events      string           `gorm:"type:text;not null" json:"events"`
	Description string           `gorm:"type:text" json:"description"`
	Active      bool             `gorm:"default:true" json:"active"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	DeletedAt   gorm.DeletedAt   `gorm:"index" json:"-"`
}

// WebhookDeliveryLog records each outgoing webhook attempt
type WebhookDeliveryLog struct {
	ID              uint                  `gorm:"primaryKey" json:"id"`
	SubscriptionID  uint                  `gorm:"not null;index" json:"subscription_id"`
	EventType       WebhookEventType      `gorm:"not null" json:"event_type"`
	Payload         string                `gorm:"type:text;not null" json:"payload"`
	Status          WebhookDeliveryStatus `gorm:"default:'pending'" json:"status"`
	HTTPStatus      int                   `json:"http_status,omitempty"`
	ResponseBody    string                `gorm:"type:text" json:"response_body,omitempty"`
	AttemptCount    int                   `gorm:"default:0" json:"attempt_count"`
	MaxRetries      int                   `gorm:"default:5" json:"max_retries"`
	NextRetryAt     *time.Time            `json:"next_retry_at,omitempty"`
	DeliveredAt     *time.Time            `json:"delivered_at,omitempty"`
	LastError       string                `gorm:"type:text" json:"last_error,omitempty"`
	RetryHistory    string                `gorm:"type:jsonb" json:"retry_history,omitempty"`
	DLQReason       string                `gorm:"type:text" json:"dlq_reason,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}
