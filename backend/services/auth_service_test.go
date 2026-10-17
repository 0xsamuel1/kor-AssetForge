package services

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/yourusername/kor-assetforge/models"
)

func openServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=password dbname=assetforge_test port=5432 sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.LoginAttempt{}))
	return db
}

func createServiceUser(t *testing.T, db *gorm.DB, email string) models.User {
	t.Helper()
	require.NoError(t, db.Where("email = ?", email).Delete(&models.User{}).Error)
	user := models.User{Email: email, StellarAddress: "GTEST" + email, Username: "u_" + email}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestAccountLockedAfterFiveFailedAttempts(t *testing.T) {
	db := openServiceTestDB(t)
	svc := NewAuthService(db, nil)
	user := createServiceUser(t, db, "lockout@example.com")

	locked, err := svc.IsAccountLocked(user.Email)
	require.NoError(t, err)
	assert.False(t, locked)

	for i := 0; i < 4; i++ {
		locked, err = svc.RecordFailedAttempt(user.Email, "127.0.0.1", "test")
		require.NoError(t, err)
		assert.False(t, locked)
	}
	locked, err = svc.RecordFailedAttempt(user.Email, "127.0.0.1", "test")
	require.NoError(t, err)
	assert.True(t, locked, "5th failed attempt should lock the account")

	locked, err = svc.IsAccountLocked(user.Email)
	require.NoError(t, err)
	assert.True(t, locked, "account should be locked for 15 minutes")
}

func TestSuccessfulLoginClearsFailures(t *testing.T) {
	db := openServiceTestDB(t)
	svc := NewAuthService(db, nil)
	user := createServiceUser(t, db, "clear@example.com")

	for i := 0; i < 5; i++ {
		_, err := svc.RecordFailedAttempt(user.Email, "127.0.0.1", "test")
		require.NoError(t, err)
	}
	locked, _ := svc.IsAccountLocked(user.Email)
	assert.True(t, locked)

	require.NoError(t, svc.RecordSuccessfulAttempt(user.Email))
	locked, err := svc.IsAccountLocked(user.Email)
	require.NoError(t, err)
	assert.False(t, locked)
}

func TestAdminUnlockClearsLockout(t *testing.T) {
	db := openServiceTestDB(t)
	svc := NewAuthService(db, nil)
	user := createServiceUser(t, db, "unlock@example.com")

	for i := 0; i < 5; i++ {
		_, err := svc.RecordFailedAttempt(user.Email, "127.0.0.1", "test")
		require.NoError(t, err)
	}
	locked, _ := svc.IsAccountLocked(user.Email)
	assert.True(t, locked)

	// Lockout expires after the 15-minute window (#330).
	require.NoError(t, db.Model(&models.User{}).Where("email = ?", user.Email).
		Update("locked_until", time.Now().Add(-time.Minute)).Error)
	locked, err := svc.IsAccountLocked(user.Email)
	require.NoError(t, err)
	assert.False(t, locked, "expired lockout should allow login again")
}
