package models

import (
	"time"

	"gorm.io/gorm"
)

// ReferralStatus tracks a referral through its reward lifecycle.
type ReferralStatus string

const (
	// ReferralPending means the referee signed up but has not yet met the
	// qualifying condition (e.g. completing KYC or a first transaction).
	ReferralPending ReferralStatus = "pending"
	// ReferralQualified means the qualifying condition was met; rewards are due.
	ReferralQualified ReferralStatus = "qualified"
	// ReferralRewarded means rewards have been distributed.
	ReferralRewarded ReferralStatus = "rewarded"
	// ReferralRejected means the referral was flagged as fraudulent/invalid.
	ReferralRejected ReferralStatus = "rejected"
)

// ReferralCode is a user's shareable referral code. A user has at most one
// active code (#170).
type ReferralCode struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"not null;uniqueIndex" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID" json:"-"`
	Code      string         `gorm:"not null;uniqueIndex" json:"code"`
	Uses      int            `gorm:"default:0" json:"uses"`
	MaxUses   int            `gorm:"default:0" json:"max_uses"` // 0 = unlimited
	Active    bool           `gorm:"default:true" json:"active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Referral links a referrer to a referee they brought to the platform, tracking
// the reward state and the referral chain via the referee's own future
// referrals.
type Referral struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	ReferrerID uint           `gorm:"not null;index" json:"referrer_id"`
	Referrer   User           `gorm:"foreignKey:ReferrerID" json:"-"`
	RefereeID  uint           `gorm:"not null;uniqueIndex" json:"referee_id"` // a user can be referred once
	Referee    User           `gorm:"foreignKey:RefereeID" json:"-"`
	Code       string         `gorm:"not null;index" json:"code"`
	Status     ReferralStatus `gorm:"not null;default:'pending';index" json:"status"`
	// Tier is the referrer's tier at qualification time, driving the reward rate.
	Tier            int        `gorm:"default:1" json:"tier"`
	RewardAmountUSD int64      `gorm:"default:0" json:"reward_amount_usd"` // in cents
	SignupIP        string     `json:"signup_ip,omitempty"`
	QualifiedAt     *time.Time `json:"qualified_at,omitempty"`
	RewardedAt      *time.Time `json:"rewarded_at,omitempty"`
	RejectReason    string     `gorm:"type:text" json:"reject_reason,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// ReferralReward is a reward credited to a user as a result of a referral. Both
// the referrer (commission) and optionally the referee (signup bonus) may earn.
type ReferralReward struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ReferralID uint      `gorm:"not null;index" json:"referral_id"`
	UserID     uint      `gorm:"not null;index" json:"user_id"`
	AmountUSD  int64     `gorm:"not null" json:"amount_usd"` // in cents
	Type       string    `gorm:"not null" json:"type"`       // referrer_commission, referee_bonus
	Status     string    `gorm:"not null;default:'credited'" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}
