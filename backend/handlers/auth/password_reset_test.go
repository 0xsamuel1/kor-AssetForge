package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/yourusername/kor-assetforge/models"
)

func newPasswordResetHandler(t *testing.T, db *gorm.DB) *AuthHandler {
	t.Helper()
	config := &AuthConfig{
		JWTSecret:           "test-secret-key",
		JWTExpirationHours:  24,
		RefreshTokenHours:   168,
		EmailTokenHours:     24,
		PasswordResetMinutes: 30,
		BcryptCost:          4,
	}
	return NewAuthHandler(db, config, nil)
}

func createPasswordResetUser(t *testing.T, db *gorm.DB, email string) models.User {
	t.Helper()
	_ = db.Where("email = ?", email).Delete(&models.User{})
	hashed, err := bcrypt.GenerateFromPassword([]byte("password123"), 4)
	require.NoError(t, err)
	user := models.User{
		StellarAddress: "GTEST" + email,
		Email:          email,
		Username:       "user_" + email,
		PasswordHash:   string(hashed),
		EmailVerified:  true,
		Role:           models.RoleUser,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func issueResetToken(t *testing.T, db *gorm.DB, user *models.User, offset time.Duration) {
	t.Helper()
	user.PasswordResetToken = uuid.New().String()
	user.PasswordResetExpires = time.Now().Add(offset)
	require.NoError(t, db.Save(user).Error)
}

func TestResetPasswordExpiredTokenRejected(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		t.Skip("test database not available")
	}
	gin.SetMode(gin.TestMode)
	handler := newPasswordResetHandler(t, db)
	router := gin.New()
	router.POST("/api/v1/auth/reset-password", handler.ResetPassword)

	user := createPasswordResetUser(t, db, "expired-reset@example.com")
	issueResetToken(t, db, &user, -10*time.Minute) // already expired

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password",
		strings.NewReader(`{"token":"`+user.PasswordResetToken+`","password":"newPassword123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "expired")
}

func TestResetPasswordValidTokenSucceeds(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		t.Skip("test database not available")
	}
	gin.SetMode(gin.TestMode)
	handler := newPasswordResetHandler(t, db)
	router := gin.New()
	router.POST("/api/v1/auth/reset-password", handler.ResetPassword)

	user := createPasswordResetUser(t, db, "valid-reset@example.com")
	issueResetToken(t, db, &user, 10*time.Minute) // not yet expired

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password",
		strings.NewReader(`{"token":"`+user.PasswordResetToken+`","password":"newPassword123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Token must be single-use: cleared after successful reset.
	var persisted models.User
	require.NoError(t, db.First(&persisted, user.ID).Error)
	assert.Empty(t, persisted.PasswordResetToken)
	assert.True(t, persisted.PasswordResetExpires.IsZero())
}

func TestResetPasswordTokenCannotBeReused(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		t.Skip("test database not available")
	}
	gin.SetMode(gin.TestMode)
	handler := newPasswordResetHandler(t, db)
	router := gin.New()
	router.POST("/api/v1/auth/reset-password", handler.ResetPassword)

	user := createPasswordResetUser(t, db, "reuse-reset@example.com")
	issueResetToken(t, db, &user, 10*time.Minute)

	token := user.PasswordResetToken
	payload := `{"token":"` + token + `","password":"newPassword123"}`

	first := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(payload))
	first.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, first)
	assert.Equal(t, http.StatusOK, w1.Code)

	second := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(payload))
	second.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, second)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}
