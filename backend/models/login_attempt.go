package models

import "time"

// LoginAttempt records a single authentication attempt for an account,
// used to enforce lockout after repeated failed logins (#330).
type LoginAttempt struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Email      string    `gorm:"not null;index" json:"email"`
	Success    bool      `gorm:"not null" json:"success"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	AttemptedAt time.Time `gorm:"not null;index" json:"attempted_at"`
}

func (LoginAttempt) TableName() string {
	return "login_attempts"
}
