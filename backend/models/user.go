package models

import (
	"time"

	"gorm.io/gorm"
)

// UserRole represents user roles for RBAC
type UserRole string

const (
	RoleUser      UserRole = "user"
	RoleAdmin     UserRole = "admin"
	RoleModerator UserRole = "moderator"
)

// User represents a platform user
type User struct {
	ID                   uint           `gorm:"primaryKey" json:"id"`
	StellarAddress       string         `gorm:"uniqueIndex;not null" json:"stellar_address"`
	Email                string         `gorm:"uniqueIndex" json:"email"`
	Username             string         `gorm:"uniqueIndex" json:"username"`
	PasswordHash         string         `gorm:"not null" json:"-"`
	Role                 UserRole       `gorm:"default:'user'" json:"role"`
	KYCVerified          bool           `gorm:"default:false" json:"kyc_verified"`
	AccreditedInvestor   bool           `gorm:"default:false" json:"accredited_investor"`
	EmailVerified        bool           `gorm:"default:false" json:"email_verified"`
	EmailToken           string         `gorm:"index" json:"-"`
	EmailTokenExpires    time.Time      `json:"-"`
	PasswordResetToken   string         `gorm:"index" json:"-"`
	PasswordResetExpires time.Time      `json:"-"`
	TOTPSecret           string         `gorm:"default:''" json:"-"`
	TOTPEnabled          bool           `gorm:"default:false" json:"totp_enabled"`
	TOTPVerified         bool           `gorm:"default:false" json:"-"`
	BackupCodes              string         `gorm:"type:text" json:"-"`
	RecoveryCodesGeneratedAt *time.Time     `json:"-"`
	LastLoginAt              *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`
}

// RecoveryCode represents a single-use 2FA backup recovery code.
// Codes are stored as salted hashes; the plaintext value is only ever
// shown to the user once, at generation time.
type RecoveryCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"not null;index" json:"user_id"`
	User      User       `gorm:"foreignKey:UserID" json:"-"`
	CodeHash  string     `gorm:"not null" json:"-"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (RecoveryCode) TableName() string {
	return "user_recovery_codes"
}

// UserSession tracks active login sessions
type UserSession struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"not null;index" json:"user_id"`
	User         User      `gorm:"foreignKey:UserID" json:"-"`
	SessionToken string    `gorm:"uniqueIndex;not null" json:"-"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// UserBalance represents a user's token balance
type UserBalance struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"not null" json:"user_id"`
	User          User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	AssetID       uint      `gorm:"not null" json:"asset_id"`
	Asset         Asset     `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	Balance       int64     `gorm:"not null;default:0" json:"balance"`
	LockedBalance int64     `gorm:"not null;default:0" json:"locked_balance"`
	UpdatedAt     time.Time `json:"updated_at"`
}
