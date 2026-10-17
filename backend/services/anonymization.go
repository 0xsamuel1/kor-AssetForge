package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// AnonymizationService irreversibly removes direct identifiers while retaining
// records and their numeric foreign keys for accounting and audit purposes.
type AnonymizationService struct{ db *gorm.DB }

func NewAnonymizationService(db *gorm.DB) *AnonymizationService {
	return &AnonymizationService{db: db}
}

func (s *AnonymizationService) AnonymizeUser(userID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Unscoped().First(&user, userID).Error; err != nil {
			return err
		}
		identity := anonymizedIdentity(userID)
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"stellar_address": "ANON-" + identity,
			"email":            identity + "@anonymized.invalid",
			"username":         "anon-" + identity,
			"password_hash":    "",
			"email_token":      "",
			"password_reset_token": "",
			"totp_secret":      "",
			"backup_codes":     "",
			"deleted_at":       time.Now().UTC(),
		}).Error; err != nil { return err }

		updates := map[string]interface{}{"ip_address": "0.0.0.0", "user_agent": "anonymized"}
		for _, model := range []interface{}{&models.UserSession{}, &models.UserActivity{}} {
			if tx.Migrator().HasTable(model) {
				if err := tx.Model(model).Where("user_id = ?", userID).Updates(updates).Error; err != nil { return err }
			}
		}
		if tx.Migrator().HasTable(&models.ComplianceAuditLog{}) {
			if err := tx.Model(&models.ComplianceAuditLog{}).Where("user_id = ?", userID).Update("ip_address", "0.0.0.0").Error; err != nil { return err }
		}
		if tx.Migrator().HasTable(&models.AuditLog{}) {
			if err := tx.Model(&models.AuditLog{}).Where("user_id = ?", userID).Update("ip_address", "0.0.0.0").Error; err != nil { return err }
		}
		if tx.Migrator().HasTable(&models.Referral{}) {
			if err := tx.Model(&models.Referral{}).Where("referrer_id = ? OR referee_id = ?", userID, userID).Update("signup_ip", "0.0.0.0").Error; err != nil { return err }
		}
		if tx.Migrator().HasTable(&models.KYCRecord{}) {
			if err := tx.Model(&models.KYCRecord{}).Where("user_id = ?", userID).Updates(map[string]interface{}{"full_name": "Anonymized User", "date_of_birth": "", "nationality": "", "provider_record_id": "", "review_notes": ""}).Error; err != nil { return err }
		}
		return nil
	})
}

func anonymizedIdentity(userID uint) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("assetforge-anonymized-user:%d", userID)))
	return hex.EncodeToString(digest[:])[:32]
}
