package models

import (
	"time"

	"gorm.io/gorm"
)

// OAuthProvider identifies a supported social login provider.
type OAuthProvider string

const (
	ProviderGoogle   OAuthProvider = "google"
	ProviderGitHub   OAuthProvider = "github"
	ProviderFacebook OAuthProvider = "facebook"
)

// SocialAccount links a platform User to an external OAuth identity.
type SocialAccount struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	UserID          uint           `gorm:"not null;index" json:"user_id"`
	User            User           `gorm:"foreignKey:UserID" json:"-"`
	Provider        OAuthProvider  `gorm:"not null;index" json:"provider"`
	ProviderUserID  string         `gorm:"not null" json:"provider_user_id"`
	Email           string         `json:"email"`
	DisplayName     string         `json:"display_name"`
	AvatarURL       string         `json:"avatar_url"`
	AccessToken     string         `gorm:"type:text" json:"-"`
	RefreshToken    string         `gorm:"type:text" json:"-"`
	TokenExpiresAt  *time.Time     `json:"-"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SocialAccount) TableName() string {
	return "social_accounts"
}
