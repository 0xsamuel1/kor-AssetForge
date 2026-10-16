package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/yourusername/kor-assetforge/models"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=password dbname=assetforge_test port=5432 sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	return db
}

type AuthTestSuite struct {
	suite.Suite
	db      *gorm.DB
	handler *AuthHandler
	router  *gin.Engine
	config  *AuthConfig
}

func (suite *AuthTestSuite) SetupSuite() {
	db := openTestDB(suite.T())

	if err := db.AutoMigrate(&models.User{}, &models.UserSession{}); err != nil {
		suite.T().Fatalf("AutoMigrate failed: %v", err)
	}

	config := &AuthConfig{
		JWTSecret:          "test-secret-key",
		JWTExpirationHours: 24,
		RefreshTokenHours:  168,
		EmailTokenHours:    24,
		PasswordResetMinutes: 30,
		BcryptCost:         4,
	}

	handler := NewAuthHandler(db, config, nil)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	authMiddleware := NewAuthMiddleware(config.JWTSecret)

	v1 := router.Group("/api/v1")
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/register", handler.Register)
		authGroup.POST("/login", handler.Login)
		authGroup.POST("/refresh", handler.RefreshToken)
		authGroup.POST("/verify-email", handler.VerifyEmail)
		authGroup.POST("/forgot-password", handler.ForgotPassword)
		authGroup.POST("/reset-password", handler.ResetPassword)
	}
	protected := v1.Group("")
	protected.Use(authMiddleware.JWTAuth())
	{
		protected.GET("/profile", handler.GetProfile)
		protected.POST("/logout", handler.Logout)
	}

	suite.db = db
	suite.handler = handler
	suite.router = router
	suite.config = config
}

func (suite *AuthTestSuite) TearDownSuite() {
	suite.db.Exec("DELETE FROM user_sessions")
	suite.db.Exec("DELETE FROM users")
	sqlDB, _ := suite.db.DB()
	sqlDB.Close()
}

func (suite *AuthTestSuite) newRequest(method, path string, body interface{}) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, path, &buf)
	r.Header.Set("Content-Type", "application/json")
	return r
}

func (suite *AuthTestSuite) TestRegister() {
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, suite.newRequest("POST", "/api/v1/auth/register", RegisterRequest{
		StellarAddress: "GA7QYNF7SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74P7UJVSGZ",
		Email:          "register@example.com",
		Username:       "registeruser",
		Password:       "password123",
	}))

	assert.Equal(suite.T(), http.StatusCreated, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Contains(suite.T(), resp, "message")
	assert.Contains(suite.T(), resp, "user_id")
}

func (suite *AuthTestSuite) TestRegisterDuplicateEmail() {
	suite.db.Create(&models.User{
		StellarAddress: "GBVNQGYOZQMXKSLGDSFWQ6TYU4KVWLTJJFC7MGXUA74P7UJVSGZ1234",
		Email:          "duplicate@example.com",
		Username:       "duplicateuser",
		PasswordHash:   "hash",
		EmailVerified:  true,
	})

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, suite.newRequest("POST", "/api/v1/auth/register", RegisterRequest{
		StellarAddress: "GA7QYNF7SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74P7UJVSGZ",
		Email:          "duplicate@example.com",
		Username:       "anotheruser",
		Password:       "password123",
	}))
	assert.Equal(suite.T(), http.StatusConflict, w.Code)
}

func (suite *AuthTestSuite) TestLogin() {
	hashed, _ := bcrypt.GenerateFromPassword([]byte("password123"), suite.config.BcryptCost)
	user := models.User{
		StellarAddress: "GCKIQYPTHQBE2B5NYNXCMVWI6WDXIJM46TIQO7OO4XJWNVQNQMM1234",
		Email:          "logintest@example.com",
		Username:       "logintest",
		PasswordHash:   string(hashed),
		EmailVerified:  true,
	}
	suite.db.Create(&user)

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, suite.newRequest("POST", "/api/v1/auth/login", LoginRequest{
		Email:    "logintest@example.com",
		Password: "password123",
	}))

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	var resp TokenResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEmpty(suite.T(), resp.AccessToken)
	assert.NotEmpty(suite.T(), resp.RefreshToken)
	assert.Equal(suite.T(), "Bearer", resp.TokenType)
}

func (suite *AuthTestSuite) TestLoginUnverifiedEmail() {
	hashed, _ := bcrypt.GenerateFromPassword([]byte("password123"), suite.config.BcryptCost)
	suite.db.Create(&models.User{
		StellarAddress: "GCYXCGHQMX8SLGDSFWQ6TYU4KVWLTJJFC7MGXUA74P7UJVSGZ1234AB",
		Email:          "unverified@example.com",
		Username:       "unverifiedtest",
		PasswordHash:   string(hashed),
		EmailVerified:  false,
	})

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, suite.newRequest("POST", "/api/v1/auth/login", LoginRequest{
		Email:    "unverified@example.com",
		Password: "password123",
	}))
	assert.Equal(suite.T(), http.StatusForbidden, w.Code)
}

func (suite *AuthTestSuite) TestGetProfile() {
	user := models.User{
		StellarAddress: "GDPROFILETEST7SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74AB",
		Email:          "profile@example.com",
		Username:       "profiletest",
		PasswordHash:   "hash",
		Role:           models.RoleUser,
		EmailVerified:  true,
		KYCVerified:    true,
	}
	suite.db.Create(&user)

	token, _, _ := suite.handler.generateTokens(&user)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/profile", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	suite.router.ServeHTTP(w, r)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	var resp UserInfo
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(suite.T(), "profiletest", resp.Username)
	assert.True(suite.T(), resp.KYCVerified)
}

func (suite *AuthTestSuite) TestVerifyEmail() {
	token := generateSecureToken()
	suite.db.Create(&models.User{
		StellarAddress:    "GDEMAILVERIFY7SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74AB",
		Email:             "verifyemail@example.com",
		Username:          "verifyemailtest",
		PasswordHash:      "hash",
		EmailToken:        token,
		EmailTokenExpires: time.Now().Add(time.Hour),
	})

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, suite.newRequest("POST", "/api/v1/auth/verify-email", VerifyEmailRequest{Token: token}))
	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

func (suite *AuthTestSuite) TestForgotPassword() {
	suite.db.Create(&models.User{
		StellarAddress: "GDFORGOTPWD7SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74AB1",
		Email:          "forgot@example.com",
		Username:       "forgottest",
		PasswordHash:   "hash",
		EmailVerified:  true,
	})

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, suite.newRequest("POST", "/api/v1/auth/forgot-password", ForgotPasswordRequest{Email: "forgot@example.com"}))
	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

func (suite *AuthTestSuite) TestResetPassword() {
	resetToken := generateSecureToken()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("oldpass123"), suite.config.BcryptCost)
	user := models.User{
		StellarAddress:      "GDRESETPWD17SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74AB1",
		Email:               "resetpwd@example.com",
		Username:            "resetpwdtest",
		PasswordHash:        string(hashed),
		EmailVerified:       true,
		PasswordResetToken:  resetToken,
		PasswordResetExpires: time.Now().Add(time.Hour),
	}
	suite.db.Create(&user)

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, suite.newRequest("POST", "/api/v1/auth/reset-password", ResetPasswordRequest{
		Token:    resetToken,
		Password: "newpassword123",
	}))
	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var updated models.User
	suite.db.First(&updated, user.ID)
	assert.Empty(suite.T(), updated.PasswordResetToken)
	assert.NoError(suite.T(), bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("newpassword123")))
}

func TestAuthTestSuite(t *testing.T) {
	suite.Run(t, new(AuthTestSuite))
}
