package services

import (
	"testing"

	"github.com/yourusername/kor-assetforge/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAnonymizeUserPreservesForeignKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.UserSession{}, &models.KYCRecord{}))

	user := models.User{StellarAddress: "G-ORIGINAL", Email: "person@example.com", Username: "person", PasswordHash: "hash"}
	require.NoError(t, db.Create(&user).Error)
	session := models.UserSession{UserID: user.ID, SessionToken: "session", IPAddress: "127.0.0.1", UserAgent: "browser"}
	require.NoError(t, db.Create(&session).Error)

	require.NoError(t, NewAnonymizationService(db).AnonymizeUser(user.ID))
	var anonymized models.User
	require.NoError(t, db.Unscoped().First(&anonymized, user.ID).Error)
	var preserved models.UserSession
	require.NoError(t, db.First(&preserved, session.ID).Error)
	require.NotEqual(t, user.Email, anonymized.Email)
	require.Equal(t, user.ID, preserved.UserID)
	require.Equal(t, "0.0.0.0", preserved.IPAddress)
	require.NotEqual(t, "hash", anonymized.PasswordHash)
}