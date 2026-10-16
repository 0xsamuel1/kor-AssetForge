package auth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
)

// OAuthHandler handles social login: starting a provider auth flow, handling
// the provider callback (logging in or creating an account), and linking or
// unlinking a social account on an existing, authenticated user.
type OAuthHandler struct {
	db           *gorm.DB
	config       *AuthConfig
	oauthService *services.OAuthService
}

// NewOAuthHandler creates a new OAuthHandler.
func NewOAuthHandler(db *gorm.DB, config *AuthConfig, oauthService *services.OAuthService) *OAuthHandler {
	return &OAuthHandler{db: db, config: config, oauthService: oauthService}
}

// OAuthCallbackRequest represents the authorization code returned by the provider.
type OAuthCallbackRequest struct {
	Code  string `json:"code" binding:"required"`
	State string `json:"state"`
}

// OAuthAuthURLResponse contains the URL the client should redirect the user to.
type OAuthAuthURLResponse struct {
	AuthURL string `json:"auth_url"`
	State   string `json:"state"`
}

var supportedOAuthProviders = map[string]models.OAuthProvider{
	"google":   models.ProviderGoogle,
	"github":   models.ProviderGitHub,
	"facebook": models.ProviderFacebook,
}

// GetAuthURL returns the provider authorization URL the frontend should redirect to.
// @Summary Start an OAuth login flow
// @Description Returns the provider's authorization URL for the requested OAuth provider
// @Tags oauth
// @Produce json
// @Param provider path string true "OAuth provider (google, github, facebook)"
// @Success 200 {object} auth.OAuthAuthURLResponse
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /auth/oauth/{provider}/url [get]
func (h *OAuthHandler) GetAuthURL(c *gin.Context) {
	provider, err := resolveOAuthProvider(c.Param("provider"))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError(err.Error()))
		return
	}

	if !h.oauthService.IsConfigured(provider) {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("OAuth provider is not configured"))
		return
	}

	state := uuid.New().String()
	authURL, err := h.oauthService.AuthURL(provider, state)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, OAuthAuthURLResponse{AuthURL: authURL, State: state})
}

// Callback handles the provider's redirect with an authorization code. If a
// social account already exists for this provider identity, the linked user
// is logged in. Otherwise, if the provider email matches an existing user,
// the social account is linked to that user. If no match is found, a new
// user account is created.
// @Summary Complete an OAuth login flow
// @Description Exchanges the authorization code for a token, fetches the provider profile, and logs the user in (creating or linking an account as needed)
// @Tags oauth
// @Accept json
// @Produce json
// @Param provider path string true "OAuth provider (google, github, facebook)"
// @Param callback body auth.OAuthCallbackRequest true "Authorization code from the provider"
// @Success 200 {object} auth.TokenResponse
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 502 {object} apperrors.ErrorResponse
// @Router /auth/oauth/{provider}/callback [post]
func (h *OAuthHandler) Callback(c *gin.Context) {
	provider, err := resolveOAuthProvider(c.Param("provider"))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError(err.Error()))
		return
	}

	var req OAuthCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	if !h.oauthService.IsConfigured(provider) {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("OAuth provider is not configured"))
		return
	}

	token, err := h.oauthService.ExchangeCode(provider, req.Code)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewExternalServiceError("Failed to exchange authorization code", err))
		return
	}

	profile, err := h.oauthService.FetchUserInfo(provider, token.AccessToken)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewExternalServiceError("Failed to fetch provider profile", err))
		return
	}
	if profile.ProviderUserID == "" {
		apperrors.AbortWithError(c, apperrors.NewExternalServiceError("Provider did not return a user identifier", nil))
		return
	}

	user, err := h.findOrCreateUserForProfile(provider, profile, token)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to authenticate with provider account"))
		return
	}

	now := time.Now()
	user.LastLoginAt = &now
	h.db.Save(user)

	accessToken, refreshToken, err := h.generateTokensForUser(user)
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
		User:         toUserInfo(user),
	})
}

// LinkAccount links a social account to the currently authenticated user.
// @Summary Link a social account
// @Description Links an OAuth provider identity to the authenticated user's account
// @Tags oauth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param provider path string true "OAuth provider (google, github, facebook)"
// @Param callback body auth.OAuthCallbackRequest true "Authorization code from the provider"
// @Success 200 {object} map[string]string
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 401 {object} apperrors.ErrorResponse
// @Failure 409 {object} apperrors.ErrorResponse
// @Router /auth/oauth/{provider}/link [post]
func (h *OAuthHandler) LinkAccount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	provider, err := resolveOAuthProvider(c.Param("provider"))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError(err.Error()))
		return
	}

	var req OAuthCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	token, err := h.oauthService.ExchangeCode(provider, req.Code)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewExternalServiceError("Failed to exchange authorization code", err))
		return
	}

	profile, err := h.oauthService.FetchUserInfo(provider, token.AccessToken)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewExternalServiceError("Failed to fetch provider profile", err))
		return
	}

	var existing models.SocialAccount
	err = h.db.Where("provider = ? AND provider_user_id = ?", provider, profile.ProviderUserID).First(&existing).Error
	if err == nil && existing.UserID != userID {
		apperrors.AbortWithError(c, apperrors.NewConflictError("This social account is already linked to another user"))
		return
	}
	if err == nil && existing.UserID == userID {
		apperrors.AbortWithError(c, apperrors.NewConflictError("This social account is already linked to your user"))
		return
	}

	account := models.SocialAccount{
		UserID:         userID.(uint),
		Provider:       provider,
		ProviderUserID: profile.ProviderUserID,
		Email:          profile.Email,
		DisplayName:    profile.DisplayName,
		AvatarURL:      profile.AvatarURL,
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
	}
	if err := h.db.Create(&account).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to link social account"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Social account linked successfully"})
}

// UnlinkAccount removes a linked social account from the authenticated user.
// @Summary Unlink a social account
// @Description Removes a previously linked OAuth provider identity from the authenticated user's account
// @Tags oauth
// @Security BearerAuth
// @Produce json
// @Param provider path string true "OAuth provider (google, github, facebook)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 401 {object} apperrors.ErrorResponse
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /auth/oauth/{provider}/unlink [delete]
func (h *OAuthHandler) UnlinkAccount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	provider, err := resolveOAuthProvider(c.Param("provider"))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError(err.Error()))
		return
	}

	result := h.db.Where("user_id = ? AND provider = ?", userID, provider).Delete(&models.SocialAccount{})
	if result.Error != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to unlink social account"))
		return
	}
	if result.RowsAffected == 0 {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("No linked account found for this provider"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Social account unlinked successfully"})
}

// ListLinkedAccounts lists the providers currently linked to the authenticated user.
// @Summary List linked social accounts
// @Description Lists the OAuth providers currently linked to the authenticated user's account
// @Tags oauth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} apperrors.ErrorResponse
// @Router /auth/oauth/linked [get]
func (h *OAuthHandler) ListLinkedAccounts(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		apperrors.AbortWithError(c, apperrors.NewUnauthorizedError("User not authenticated"))
		return
	}

	var accounts []models.SocialAccount
	if err := h.db.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to list linked accounts"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"linked_accounts": accounts})
}

// findOrCreateUserForProfile resolves a User for the given provider profile:
// reusing the linked account, linking by matching email, or creating a new user.
func (h *OAuthHandler) findOrCreateUserForProfile(provider models.OAuthProvider, profile *services.OAuthUserInfo, token *services.OAuthTokenResponse) (*models.User, error) {
	var social models.SocialAccount
	err := h.db.Where("provider = ? AND provider_user_id = ?", provider, profile.ProviderUserID).First(&social).Error
	if err == nil {
		social.AccessToken = token.AccessToken
		social.RefreshToken = token.RefreshToken
		h.db.Save(&social)

		var user models.User
		if err := h.db.First(&user, social.UserID).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}

	var user models.User
	if profile.Email != "" {
		if err := h.db.Where("email = ?", profile.Email).First(&user).Error; err == nil {
			return h.linkNewSocialAccount(&user, provider, profile, token)
		}
	}

	user = models.User{
		StellarAddress: "oauth:" + string(provider) + ":" + profile.ProviderUserID,
		Email:          profile.Email,
		Username:       generateOAuthUsername(provider, profile),
		PasswordHash:   "",
		Role:           models.RoleUser,
		EmailVerified:  profile.Email != "",
	}
	if err := h.db.Create(&user).Error; err != nil {
		return nil, err
	}

	return h.linkNewSocialAccount(&user, provider, profile, token)
}

func (h *OAuthHandler) linkNewSocialAccount(user *models.User, provider models.OAuthProvider, profile *services.OAuthUserInfo, token *services.OAuthTokenResponse) (*models.User, error) {
	account := models.SocialAccount{
		UserID:         user.ID,
		Provider:       provider,
		ProviderUserID: profile.ProviderUserID,
		Email:          profile.Email,
		DisplayName:    profile.DisplayName,
		AvatarURL:      profile.AvatarURL,
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
	}
	if err := h.db.Create(&account).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func (h *OAuthHandler) generateTokensForUser(user *models.User) (string, string, error) {
	authHandler := &AuthHandler{db: h.db, config: h.config}
	return authHandler.generateTokens(user)
}

func generateOAuthUsername(provider models.OAuthProvider, profile *services.OAuthUserInfo) string {
	base := profile.DisplayName
	if base == "" {
		base = string(provider) + "_user"
	}
	return base + "_" + profile.ProviderUserID[:minInt(8, len(profile.ProviderUserID))]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func resolveOAuthProvider(raw string) (models.OAuthProvider, error) {
	provider, ok := supportedOAuthProviders[raw]
	if !ok {
		return "", fmt.Errorf("unsupported OAuth provider: %s", raw)
	}
	return provider, nil
}
