package services

import (
	"fmt"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

const (
	maxFailedAttempts = 5
	lockoutDuration   = 15 * time.Minute
)

// AuthService enforces account lockout after repeated failed login attempts (#330).
type AuthService struct {
	db           *gorm.DB
	emailService EmailService
}

// NewAuthService constructs an AuthService.
func NewAuthService(db *gorm.DB, emailService EmailService) *AuthService {
	return &AuthService{db: db, emailService: emailService}
}

// IsAccountLocked reports whether the account is currently locked out.
func (s *AuthService) IsAccountLocked(email string) (bool, error) {
	var user models.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		return false, nil // unknown account is never locked
	}
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return true, nil
	}
	return false, nil
}

// RecordFailedAttempt logs a failed attempt and locks the account once the
// threshold is reached, notifying the account owner by email.
func (s *AuthService) RecordFailedAttempt(email, ipAddress, userAgent string) (locked bool, err error) {
	return s.recordAttempt(email, ipAddress, userAgent, false)
}

// RecordSuccessfulAttempt clears any failed-attempt state for the account.
func (s *AuthService) RecordSuccessfulAttempt(email string) error {
	return s.db.Model(&models.User{}).Where("email = ?", email).Updates(map[string]interface{}{
		"failed_login_attempts": 0,
		"locked_until":          nil,
	}).Error
}

// UnlockAccount clears the lockout state for an account (admin action).
func (s *AuthService) UnlockAccount(email string) error {
	return s.RecordSuccessfulAttempt(email)
}

func (s *AuthService) recordAttempt(email, ipAddress, userAgent string, success bool) (locked bool, err error) {
	var user models.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		return false, nil
	}

	attempt := models.LoginAttempt{
		Email:        email,
		Success:      success,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		AttemptedAt:  time.Now(),
	}
	if err := s.db.Create(&attempt).Error; err != nil {
		return false, err
	}

	if success {
		return false, s.RecordSuccessfulAttempt(email)
	}

	failed := user.FailedLoginAttempts + 1
	updates := map[string]interface{}{"failed_login_attempts": failed}
	if failed >= maxFailedAttempts {
		lockedUntil := time.Now().Add(lockoutDuration)
		updates["locked_until"] = lockedUntil
		locked = true
	}
	if err := s.db.Model(&models.User{}).Where("email = ?", email).Updates(updates).Error; err != nil {
		return false, err
	}

	if locked && s.emailService != nil {
		_ = s.emailService.SendCustomEmail(user.Email, user.Username,
			"Your account has been temporarily locked",
			fmt.Sprintf("<p>Hi %s,</p><p>Your account was locked after %d failed login attempts.</p><p>It will unlock automatically in 15 minutes.</p>", user.Username, maxFailedAttempts),
			fmt.Sprintf("Hi %s,\n\nYour account was locked after %d failed login attempts.\nIt will unlock automatically in 15 minutes.\n", user.Username, maxFailedAttempts),
		)
	}
	return locked, nil
}
