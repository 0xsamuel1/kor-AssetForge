package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret          string
	JWTExpirationHours int
	RefreshTokenHours  int
	EmailTokenHours    int
	PasswordResetHours int
	BcryptCost         int
}

// AuthHandler handles authentication operations
type AuthHandler struct {
	db           *gorm.DB
	config       *AuthConfig
	emailService services.EmailService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(db *gorm.DB, config *AuthConfig, emailService services.EmailService) *AuthHandler {
	return &AuthHandler{db: db, config: config, emailService: emailService}
}

// RegisterRequest represents user registration request
type RegisterRequest struct {
	StellarAddress string `json:"stellar_address" binding:"required"`
	Email          string `json:"email" binding:"required,email"`
	Username       string `json:"username" binding:"required,min=3,max=50"`
	Password       string `json:"password" binding:"required,min=8"`
}

// LoginRequest represents user login request
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// TokenResponse represents JWT token response
type TokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int64    `json:"expires_in"`
	User         UserInfo `json:"user"`
}

// UserInfo represents user information in responses
type UserInfo struct {
	ID             uint       `json:"id"`
	StellarAddress string     `json:"stellar_address"`
	Email          string     `json:"email"`
	Username       string     `json:"username"`
	Role           string     `json:"role"`
	EmailVerified  bool       `json:"email_verified"`
	KYCVerified    bool       `json:"kyc_verified"`
	TOTPEnabled    bool       `json:"totp_enabled"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
}

// RefreshTokenRequest represents token refresh request
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// VerifyEmailRequest represents email verification request
type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// ForgotPasswordRequest represents forgot password request
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ResetPasswordRequest represents password reset request
type ResetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

// Setup2FARequest represents 2FA setup request
type Setup2FARequest struct {
	Token string `json:"token" binding:"required"`
}

// Disable2FARequest represents 2FA disable request
type Disable2FARequest struct {
	Password  string `json:"password" binding:"required"`
	TOTPToken string `json:"totp_token" binding:"required"`
}

// Verify2FARequest represents 2FA verification during login
type Verify2FARequest struct {
	UserID    uint   `json:"user_id" binding:"required"`
	TOTPToken string `json:"totp_token" binding:"required"`
}

// Setup2FAResponse represents the response for 2FA setup
type Setup2FAResponse struct {
	Secret      string   `json:"secret"`
	QRURL       string   `json:"qr_url"`
	BackupCodes []string `json:"backup_codes"`
}

// RegenerateRecoveryCodesRequest represents a request to regenerate 2FA recovery codes
type RegenerateRecoveryCodesRequest struct {
	Password  string `json:"password" binding:"required"`
	TOTPToken string `json:"totp_token" binding:"required"`
}

// RecoveryCodesResponse represents a freshly generated set of recovery codes
type RecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
	GeneratedAt   string   `json:"generated_at"`
}

// RecoveryCodeLoginRequest represents 2FA login using a recovery code
type RecoveryCodeLoginRequest struct {
	UserID       uint   `json:"user_id" binding:"required"`
	RecoveryCode string `json:"recovery_code" binding:"required"`
}

// RecoveryCodesStatusResponse reports how many unused recovery codes remain
type RecoveryCodesStatusResponse struct {
	RemainingCodes int    `json:"remaining_codes"`
	GeneratedAt    string `json:"generated_at,omitempty"`
}

// Register handles user registration
// @Summary Register a new user
// @Description Create a new user account with Stellar address and email
// @Tags auth
// @Accept json
// @Produce json
// @Param register body auth.RegisterRequest true "Registration details"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 409 {object} apperrors.ErrorResponse
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var existing models.User
	if err := h.db.Where("email = ? OR username = ? OR stellar_address = ?",
		req.Email, req.Username, req.StellarAddress).First(&existing).Error; err == nil {
		apperrors.AbortWithError(c, apperrors.NewConflictError("User already exists with this email, username, or Stellar address"))
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), h.config.BcryptCost)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to hash password"))
		return
	}

	emailToken := generateSecureToken()
	user := models.User{
		StellarAddress:    req.StellarAddress,
		Email:             req.Email,
		Username:          req.Username,
		PasswordHash:      string(hashedPassword),
		Role:              models.RoleUser,
		EmailVerified:     false,
		EmailToken:        emailToken,
		EmailTokenExpires: time.Now().Add(time.Hour * time.Duration(h.config.EmailTokenHours)),
	}

	if err := h.db.Create(&user).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to create user"))
		return
	}

	if h.emailService != nil {
		if err := h.emailService.SendVerificationEmail(user.Email, user.Username, emailToken); err != nil {
			log.Printf("failed to queue verification email: %v", err)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User registered successfully. Please check your email for verification.",
		"user_id": user.ID,
	})
}

// Login handles user authentication
// @Summary Login a user
// @Description Authenticate a user and receive access and refresh tokens
// @Tags auth
// @Accept json
// @Produce json
// @Param login body auth.LoginRequest true "Login credentials"
// @Success 200 {object} auth.TokenResponse
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 401 {object} apperrors.ErrorResponse
// @Failure 403 {object} apperrors.ErrorResponse
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Invalid email or password"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Invalid email or password"))
		return
	}

	if !user.EmailVerified {
		apperrors.AbortWithError(c, apperrors.NewForbiddenError("Please verify your email before logging in"))
		return
	}

	if user.TOTPEnabled {
		c.JSON(http.StatusOK, gin.H{
			"requires_2fa": true,
			"user_id":      user.ID,
			"message":      "Please complete two-factor authentication",
		})
		return
	}

	now := time.Now()
	user.LastLoginAt = &now
	h.db.Save(&user)

	accessToken, refreshToken, err := h.generateTokens(&user)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to generate tokens"))
		return
	}

	session := models.UserSession{
		UserID:       user.ID,
		SessionToken: generateSecureToken(),
		IPAddress:    c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
		ExpiresAt:    time.Now().Add(time.Hour * time.Duration(h.config.RefreshTokenHours)),
	}
	if err := h.db.Create(&session).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to create session"))
		return
	}

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.config.JWTExpirationHours * 3600),
		User:         toUserInfo(&user),
	})
}

// RefreshToken handles token refresh
// @Summary Refresh access token
// @Description Use a refresh token to obtain a new access token
// @Tags auth
// @Accept json
// @Produce json
// @Param refresh body auth.RefreshTokenRequest true "Refresh token request"
// @Success 200 {object} auth.TokenResponse
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 401 {object} apperrors.ErrorResponse
// @Router /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.config.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Invalid refresh token"))
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Invalid token claims"))
		return
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Invalid token claims"))
		return
	}

	var user models.User
	if err := h.db.First(&user, uint(userIDFloat)).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not found"))
		return
	}

	accessToken, refreshToken, err := h.generateTokens(&user)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to generate tokens"))
		return
	}

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.config.JWTExpirationHours * 3600),
		User:         toUserInfo(&user),
	})
}

// VerifyEmail handles email verification
// @Summary Verify email address
// @Description Verify a user's email address using a provided token
// @Tags auth
// @Accept json
// @Produce json
// @Param verify body auth.VerifyEmailRequest true "Email verification token"
// @Success 200 {object} map[string]string
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /auth/verify-email [post]
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.Where("email_token = ? AND email_token_expires > ?", req.Token, time.Now()).First(&user).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid or expired verification token"))
		return
	}

	user.EmailVerified = true
	user.EmailToken = ""
	user.EmailTokenExpires = time.Time{}
	if err := h.db.Save(&user).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to verify email"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

// ForgotPassword handles forgot password requests
// @Summary Request password reset
// @Description Request a password reset link to be sent to the user's email
// @Tags auth
// @Accept json
// @Produce json
// @Param forgot_password body auth.ForgotPasswordRequest true "Email for password reset"
// @Success 200 {object} map[string]string
// @Router /auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// Don't reveal whether the email exists
		c.JSON(http.StatusOK, gin.H{"message": "If an account with this email exists, a password reset link has been sent."})
		return
	}

	user.PasswordResetToken = generateSecureToken()
	user.PasswordResetExpires = time.Now().Add(time.Hour * time.Duration(h.config.PasswordResetHours))
	if err := h.db.Save(&user).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to generate reset token"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "If an account with this email exists, a password reset link has been sent."})
}

// ResetPassword handles password reset
// @Summary Reset password
// @Description Reset user password using a provided token
// @Tags auth
// @Accept json
// @Produce json
// @Param reset_password body auth.ResetPasswordRequest true "Password reset details"
// @Success 200 {object} map[string]string
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.Where("password_reset_token = ? AND password_reset_expires > ?", req.Token, time.Now()).First(&user).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid or expired reset token"))
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), h.config.BcryptCost)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to hash password"))
		return
	}

	user.PasswordHash = string(hashedPassword)
	user.PasswordResetToken = ""
	user.PasswordResetExpires = time.Time{}
	if err := h.db.Save(&user).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to reset password"))
		return
	}

	// Invalidate all sessions for security
	h.db.Where("user_id = ?", user.ID).Delete(&models.UserSession{})

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully"})
}

// Logout handles user logout
// @Summary Logout user
// @Description Invalidate the current user's session
// @Tags auth
// @Security BearerAuth
// @Success 200 {object} map[string]string
// @Failure 401 {object} apperrors.ErrorResponse
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	if err := h.db.Where("user_id = ?", userID).Delete(&models.UserSession{}).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to logout"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// GetProfile returns the current user's profile
// @Summary Get user profile
// @Description Get the profile information of the currently authenticated user
// @Tags auth
// @Security BearerAuth
// @Success 200 {object} auth.UserInfo
// @Failure 401 {object} apperrors.ErrorResponse
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /profile [get]
func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("User not found"))
		return
	}

	c.JSON(http.StatusOK, toUserInfo(&user))
}

// Setup2FA initiates 2FA setup for the authenticated user
func (h *AuthHandler) Setup2FA(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("User not found"))
		return
	}

	if user.TOTPEnabled {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("2FA is already enabled"))
		return
	}

	secret := generateTOTPSecret()
	issuer := "kor-AssetForge"
	qrURL := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		issuer, user.Email, secret, issuer)

	backupCodes := make([]string, 8)
	for i := range backupCodes {
		backupCodes[i] = generateSecureToken()[:10]
	}
	codesJSON, _ := json.Marshal(backupCodes)

	user.TOTPSecret = secret
	user.TOTPVerified = false
	user.BackupCodes = string(codesJSON)
	h.db.Save(&user)

	c.JSON(http.StatusOK, Setup2FAResponse{
		Secret:      secret,
		QRURL:       qrURL,
		BackupCodes: backupCodes,
	})
}

// Verify2FA verifies and enables 2FA for the authenticated user
func (h *AuthHandler) Verify2FA(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	var req Setup2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("User not found"))
		return
	}

	if user.TOTPEnabled {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("2FA is already enabled"))
		return
	}

	if !validateTOTP(user.TOTPSecret, req.Token) {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid TOTP token"))
		return
	}

	user.TOTPEnabled = true
	user.TOTPVerified = true
	h.db.Save(&user)

	c.JSON(http.StatusOK, gin.H{"message": "2FA enabled successfully"})
}

// Disable2FA disables 2FA for the authenticated user with verification
func (h *AuthHandler) Disable2FA(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	var req Disable2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("User not found"))
		return
	}

	if !user.TOTPEnabled {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("2FA is not enabled"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Invalid password"))
		return
	}

	validTOTP := validateTOTP(user.TOTPSecret, req.TOTPToken)
	validBackup := false
	if !validTOTP {
		var codes []string
		if err := json.Unmarshal([]byte(user.BackupCodes), &codes); err == nil {
			for _, code := range codes {
				if code == req.TOTPToken {
					validBackup = true
					break
				}
			}
		}
	}

	if !validTOTP && !validBackup {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid TOTP token or backup code"))
		return
	}

	user.TOTPSecret = ""
	user.TOTPEnabled = false
	user.TOTPVerified = false
	user.BackupCodes = ""
	h.db.Save(&user)

	c.JSON(http.StatusOK, gin.H{"message": "2FA disabled successfully"})
}

// GenerateRecoveryCodes creates a fresh set of hashed, single-use 2FA recovery
// codes for the authenticated user, replacing any existing codes. The plaintext
// codes are only ever returned in this response and cannot be retrieved again.
// @Summary Generate 2FA recovery codes
// @Description Generate a new set of backup recovery codes, invalidating any previous codes
// @Tags auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body auth.RegenerateRecoveryCodesRequest true "Password and current TOTP token"
// @Success 200 {object} auth.RecoveryCodesResponse
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 401 {object} apperrors.ErrorResponse
// @Router /auth/2fa/recovery-codes [post]
func (h *AuthHandler) GenerateRecoveryCodes(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	var req RegenerateRecoveryCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("User not found"))
		return
	}

	if !user.TOTPEnabled {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("2FA must be enabled before generating recovery codes"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("Invalid password"))
		return
	}

	if !validateTOTP(user.TOTPSecret, req.TOTPToken) {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid TOTP token"))
		return
	}

	plainCodes, hashedCodes, err := generateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to generate recovery codes"))
		return
	}

	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.RecoveryCode{}).Error; err != nil {
			return err
		}
		codes := make([]models.RecoveryCode, len(hashedCodes))
		for i, hash := range hashedCodes {
			codes[i] = models.RecoveryCode{UserID: user.ID, CodeHash: hash}
		}
		if err := tx.Create(&codes).Error; err != nil {
			return err
		}
		return tx.Model(&user).Update("recovery_codes_generated_at", now).Error
	})
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to store recovery codes"))
		return
	}

	c.JSON(http.StatusOK, RecoveryCodesResponse{
		RecoveryCodes: plainCodes,
		GeneratedAt:   now.Format(time.RFC3339),
	})
}

// GetRecoveryCodesStatus reports how many unused recovery codes remain for the
// authenticated user, without exposing the codes themselves.
// @Summary Get 2FA recovery code status
// @Description Returns the number of unused recovery codes remaining
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} auth.RecoveryCodesStatusResponse
// @Failure 401 {object} apperrors.ErrorResponse
// @Router /auth/2fa/recovery-codes [get]
func (h *AuthHandler) GetRecoveryCodesStatus(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("User not found"))
		return
	}

	var remaining int64
	h.db.Model(&models.RecoveryCode{}).Where("user_id = ? AND used_at IS NULL", user.ID).Count(&remaining)

	resp := RecoveryCodesStatusResponse{RemainingCodes: int(remaining)}
	if user.RecoveryCodesGeneratedAt != nil {
		resp.GeneratedAt = user.RecoveryCodesGeneratedAt.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, resp)
}

// LoginWithRecoveryCode completes login using a single-use recovery code when
// the user's 2FA device (TOTP) is unavailable. The code is consumed on success.
// @Summary Login with a 2FA recovery code
// @Description Complete authentication using a backup recovery code instead of a TOTP token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.RecoveryCodeLoginRequest true "User ID and recovery code"
// @Success 200 {object} auth.TokenResponse
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 401 {object} apperrors.ErrorResponse
// @Router /auth/2fa/recovery-codes/login [post]
func (h *AuthHandler) LoginWithRecoveryCode(c *gin.Context) {
	var req RecoveryCodeLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.First(&user, req.UserID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not found"))
		return
	}

	if !user.TOTPEnabled {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("2FA is not enabled for this user"))
		return
	}

	normalized := normalizeRecoveryCode(req.RecoveryCode)

	var codes []models.RecoveryCode
	if err := h.db.Where("user_id = ? AND used_at IS NULL", user.ID).Find(&codes).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to verify recovery code"))
		return
	}

	var matched *models.RecoveryCode
	for i := range codes {
		if bcrypt.CompareHashAndPassword([]byte(codes[i].CodeHash), []byte(normalized)) == nil {
			matched = &codes[i]
			break
		}
	}

	if matched == nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid or already used recovery code"))
		return
	}

	now := time.Now()
	matched.UsedAt = &now
	if err := h.db.Save(matched).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to consume recovery code"))
		return
	}

	user.LastLoginAt = &now
	h.db.Save(&user)

	accessToken, refreshToken, err := h.generateTokens(&user)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to generate tokens"))
		return
	}

	session := models.UserSession{
		UserID:       user.ID,
		SessionToken: generateSecureToken(),
		IPAddress:    c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
		ExpiresAt:    time.Now().Add(time.Hour * time.Duration(h.config.RefreshTokenHours)),
	}
	h.db.Create(&session)

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.config.JWTExpirationHours * 3600),
		User:         toUserInfo(&user),
	})
}

// LoginWith2FA handles the second step of 2FA login
func (h *AuthHandler) LoginWith2FA(c *gin.Context) {
	var req Verify2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	var user models.User
	if err := h.db.First(&user, req.UserID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not found"))
		return
	}

	if !user.TOTPEnabled {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("2FA is not enabled for this user"))
		return
	}

	validTOTP := validateTOTP(user.TOTPSecret, req.TOTPToken)
	validBackup := false
	if !validTOTP {
		var codes []string
		if err := json.Unmarshal([]byte(user.BackupCodes), &codes); err == nil {
			for i, code := range codes {
				if code == req.TOTPToken {
					codes = append(codes[:i], codes[i+1:]...)
					updatedCodes, _ := json.Marshal(codes)
					user.BackupCodes = string(updatedCodes)
					validBackup = true
					break
				}
			}
		}
	}

	if !validTOTP && !validBackup {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid TOTP token or backup code"))
		return
	}

	now := time.Now()
	user.LastLoginAt = &now
	h.db.Save(&user)

	accessToken, refreshToken, err := h.generateTokens(&user)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to generate tokens"))
		return
	}

	session := models.UserSession{
		UserID:       user.ID,
		SessionToken: generateSecureToken(),
		IPAddress:    c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
		ExpiresAt:    time.Now().Add(time.Hour * time.Duration(h.config.RefreshTokenHours)),
	}
	h.db.Create(&session)

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.config.JWTExpirationHours * 3600),
		User:         toUserInfo(&user),
	})
}

func (h *AuthHandler) generateTokens(user *models.User) (string, string, error) {
	accessClaims := jwt.MapClaims{
		"user_id":  user.ID,
		"email":    user.Email,
		"username": user.Username,
		"role":     string(user.Role),
		"exp":      time.Now().Add(time.Hour * time.Duration(h.config.JWTExpirationHours)).Unix(),
		"iat":      time.Now().Unix(),
		"type":     "access",
		"jti":      uuid.New().String(),
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessStr, err := accessToken.SignedString([]byte(h.config.JWTSecret))
	if err != nil {
		return "", "", err
	}

	refreshClaims := jwt.MapClaims{
		"user_id": user.ID,
		"exp":     time.Now().Add(time.Hour * time.Duration(h.config.RefreshTokenHours)).Unix(),
		"iat":     time.Now().Unix(),
		"type":    "refresh",
		"jti":     uuid.New().String(),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshStr, err := refreshToken.SignedString([]byte(h.config.JWTSecret))
	if err != nil {
		return "", "", err
	}

	return accessStr, refreshStr, nil
}

// recoveryCodeCount is the number of single-use recovery codes issued per generation.
const recoveryCodeCount = 10

func generateSecureToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// generateRecoveryCodes creates n random recovery codes formatted as
// "XXXX-XXXX-XXXX" and returns both the plaintext values (shown once to the
// user) and their bcrypt hashes (persisted for later verification).
func generateRecoveryCodes(n int) (plain []string, hashed []string, err error) {
	plain = make([]string, n)
	hashed = make([]string, n)
	for i := 0; i < n; i++ {
		code, genErr := generateRecoveryCode()
		if genErr != nil {
			return nil, nil, genErr
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(normalizeRecoveryCode(code)), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, nil, hashErr
		}
		plain[i] = code
		hashed[i] = string(hash)
	}
	return plain, hashed, nil
}

func generateRecoveryCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	chars := make([]byte, 12)
	for i, v := range b {
		chars[i] = alphabet[int(v)%len(alphabet)]
	}
	return fmt.Sprintf("%s-%s-%s", chars[0:4], chars[4:8], chars[8:12]), nil
}

// normalizeRecoveryCode strips formatting and standardizes case so that
// user-entered codes match what was stored regardless of separators/case.
func normalizeRecoveryCode(code string) string {
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	return strings.ToUpper(strings.TrimSpace(code))
}

func generateTOTPSecret() string {
	secret := make([]byte, 20)
	rand.Read(secret)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

func validateTOTP(secret string, token string) bool {
	if secret == "" || token == "" {
		return false
	}

	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	for i := int64(-1); i <= 1; i++ {
		counter := uint64(now/30 + i)
		if computeTOTP(key, counter) == token {
			return true
		}
	}
	return false
}

func computeTOTP(key []byte, counter uint64) string {
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(counterBytes)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0xf
	code := (int(hash[offset]&0x7f) << 24) |
		(int(hash[offset+1]) << 16) |
		(int(hash[offset+2]) << 8) |
		int(hash[offset+3]&0xff)

	code = code % 1000000
	return fmt.Sprintf("%06d", code)
}

func toUserInfo(u *models.User) UserInfo {
	return UserInfo{
		ID:             u.ID,
		StellarAddress: u.StellarAddress,
		Email:          u.Email,
		Username:       u.Username,
		Role:           string(u.Role),
		EmailVerified:  u.EmailVerified,
		KYCVerified:    u.KYCVerified,
		TOTPEnabled:    u.TOTPEnabled,
		LastLoginAt:    u.LastLoginAt,
	}
}
