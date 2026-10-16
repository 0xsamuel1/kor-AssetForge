package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/yourusername/kor-assetforge/models"
)

// OAuthUserInfo is the normalized profile returned by every supported provider.
type OAuthUserInfo struct {
	ProviderUserID string
	Email          string
	DisplayName    string
	AvatarURL      string
}

// OAuthTokenResponse is the normalized token exchange result.
type OAuthTokenResponse struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// OAuthProviderConfig holds the client credentials and redirect URI for one provider.
type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// OAuthService exchanges authorization codes for tokens and fetches the
// authenticated user's profile from Google, GitHub, or Facebook.
type OAuthService struct {
	client  *http.Client
	configs map[models.OAuthProvider]OAuthProviderConfig
}

// NewOAuthService builds an OAuthService using provider credentials from the environment:
// GOOGLE_OAUTH_CLIENT_ID/SECRET/REDIRECT_URL, GITHUB_OAUTH_..., FACEBOOK_OAUTH_....
func NewOAuthService() *OAuthService {
	return &OAuthService{
		client: &http.Client{Timeout: 15 * time.Second},
		configs: map[models.OAuthProvider]OAuthProviderConfig{
			models.ProviderGoogle: {
				ClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
				ClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
				RedirectURL:  os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
			},
			models.ProviderGitHub: {
				ClientID:     os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
				ClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
				RedirectURL:  os.Getenv("GITHUB_OAUTH_REDIRECT_URL"),
			},
			models.ProviderFacebook: {
				ClientID:     os.Getenv("FACEBOOK_OAUTH_CLIENT_ID"),
				ClientSecret: os.Getenv("FACEBOOK_OAUTH_CLIENT_SECRET"),
				RedirectURL:  os.Getenv("FACEBOOK_OAUTH_REDIRECT_URL"),
			},
		},
	}
}

// IsConfigured reports whether client credentials are present for the provider.
func (s *OAuthService) IsConfigured(provider models.OAuthProvider) bool {
	cfg, ok := s.configs[provider]
	return ok && cfg.ClientID != "" && cfg.ClientSecret != ""
}

// AuthURL builds the provider's authorization URL the client should redirect the user to.
func (s *OAuthService) AuthURL(provider models.OAuthProvider, state string) (string, error) {
	cfg, ok := s.configs[provider]
	if !ok {
		return "", fmt.Errorf("unsupported oauth provider: %s", provider)
	}

	switch provider {
	case models.ProviderGoogle:
		q := url.Values{
			"client_id":     {cfg.ClientID},
			"redirect_uri":  {cfg.RedirectURL},
			"response_type": {"code"},
			"scope":         {"openid email profile"},
			"state":         {state},
		}
		return "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode(), nil
	case models.ProviderGitHub:
		q := url.Values{
			"client_id":    {cfg.ClientID},
			"redirect_uri": {cfg.RedirectURL},
			"scope":        {"read:user user:email"},
			"state":        {state},
		}
		return "https://github.com/login/oauth/authorize?" + q.Encode(), nil
	case models.ProviderFacebook:
		q := url.Values{
			"client_id":     {cfg.ClientID},
			"redirect_uri":  {cfg.RedirectURL},
			"response_type": {"code"},
			"scope":         {"email public_profile"},
			"state":         {state},
		}
		return "https://www.facebook.com/v18.0/dialog/oauth?" + q.Encode(), nil
	default:
		return "", fmt.Errorf("unsupported oauth provider: %s", provider)
	}
}

// ExchangeCode exchanges an authorization code for an access token.
func (s *OAuthService) ExchangeCode(provider models.OAuthProvider, code string) (*OAuthTokenResponse, error) {
	cfg, ok := s.configs[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported oauth provider: %s", provider)
	}

	switch provider {
	case models.ProviderGoogle:
		return s.exchangeGoogle(cfg, code)
	case models.ProviderGitHub:
		return s.exchangeGitHub(cfg, code)
	case models.ProviderFacebook:
		return s.exchangeFacebook(cfg, code)
	default:
		return nil, fmt.Errorf("unsupported oauth provider: %s", provider)
	}
}

// FetchUserInfo retrieves the authenticated user's normalized profile from the provider.
func (s *OAuthService) FetchUserInfo(provider models.OAuthProvider, accessToken string) (*OAuthUserInfo, error) {
	switch provider {
	case models.ProviderGoogle:
		return s.userInfoGoogle(accessToken)
	case models.ProviderGitHub:
		return s.userInfoGitHub(accessToken)
	case models.ProviderFacebook:
		return s.userInfoFacebook(accessToken)
	default:
		return nil, fmt.Errorf("unsupported oauth provider: %s", provider)
	}
}

func (s *OAuthService) exchangeGoogle(cfg OAuthProviderConfig, code string) (*OAuthTokenResponse, error) {
	form := url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURL},
		"grant_type":    {"authorization_code"},
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := s.postForm("https://oauth2.googleapis.com/token", form, &body); err != nil {
		return nil, err
	}
	return &OAuthTokenResponse{AccessToken: body.AccessToken, RefreshToken: body.RefreshToken, ExpiresIn: body.ExpiresIn}, nil
}

func (s *OAuthService) exchangeGitHub(cfg OAuthProviderConfig, code string) (*OAuthTokenResponse, error) {
	form := url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURL},
	}
	req, err := http.NewRequest(http.MethodPost, "https://github.com/login/oauth/access_token", nil)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = form.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode github token response: %w", err)
	}
	if body.Error != "" {
		return nil, fmt.Errorf("github token exchange failed: %s", body.Error)
	}
	return &OAuthTokenResponse{AccessToken: body.AccessToken}, nil
}

func (s *OAuthService) exchangeFacebook(cfg OAuthProviderConfig, code string) (*OAuthTokenResponse, error) {
	q := url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURL},
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := s.getJSON("https://graph.facebook.com/v18.0/oauth/access_token?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	return &OAuthTokenResponse{AccessToken: body.AccessToken, ExpiresIn: body.ExpiresIn}, nil
}

func (s *OAuthService) userInfoGoogle(accessToken string) (*OAuthUserInfo, error) {
	var body struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := s.getJSONWithAuth("https://www.googleapis.com/oauth2/v3/userinfo", accessToken, &body); err != nil {
		return nil, err
	}
	return &OAuthUserInfo{ProviderUserID: body.Sub, Email: body.Email, DisplayName: body.Name, AvatarURL: body.Picture}, nil
}

func (s *OAuthService) userInfoGitHub(accessToken string) (*OAuthUserInfo, error) {
	var body struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := s.getJSONWithAuth("https://api.github.com/user", accessToken, &body); err != nil {
		return nil, err
	}

	email := body.Email
	if email == "" {
		email = s.fetchGitHubPrimaryEmail(accessToken)
	}

	name := body.Name
	if name == "" {
		name = body.Login
	}

	return &OAuthUserInfo{
		ProviderUserID: fmt.Sprintf("%d", body.ID),
		Email:          email,
		DisplayName:    name,
		AvatarURL:      body.AvatarURL,
	}, nil
}

func (s *OAuthService) fetchGitHubPrimaryEmail(accessToken string) string {
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := s.getJSONWithAuth("https://api.github.com/user/emails", accessToken, &emails); err != nil {
		return ""
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	if len(emails) > 0 {
		return emails[0].Email
	}
	return ""
}

func (s *OAuthService) userInfoFacebook(accessToken string) (*OAuthUserInfo, error) {
	q := url.Values{
		"fields":       {"id,name,email,picture"},
		"access_token": {accessToken},
	}
	var body struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		} `json:"picture"`
	}
	if err := s.getJSON("https://graph.facebook.com/v18.0/me?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	return &OAuthUserInfo{ProviderUserID: body.ID, Email: body.Email, DisplayName: body.Name, AvatarURL: body.Picture.Data.URL}, nil
}

func (s *OAuthService) postForm(endpoint string, form url.Values, out interface{}) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = form.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("oauth request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("oauth request to %s returned status %d: %s", endpoint, resp.StatusCode, string(data))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *OAuthService) getJSON(endpoint string, out interface{}) error {
	resp, err := s.client.Get(endpoint)
	if err != nil {
		return fmt.Errorf("oauth request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("oauth request to %s returned status %d: %s", endpoint, resp.StatusCode, string(data))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *OAuthService) getJSONWithAuth(endpoint, accessToken string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("oauth request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("oauth request to %s returned status %d: %s", endpoint, resp.StatusCode, string(data))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
