package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tingly-box/internal/typ"
	oauth2 "tingly-box/pkg/oauth"
	"tingly-box/pkg/swagger"
)

// =============================================
// OAuth Provider Models
// =============================================

// OAuthProviderInfo represents OAuth provider information
type OAuthProviderInfo struct {
	Type        string   `json:"type" example:"anthropic"`
	DisplayName string   `json:"display_name" example:"Anthropic Claude"`
	AuthURL     string   `json:"auth_url,omitempty" example:"https://claude.ai/oauth/authorize"`
	Scopes      []string `json:"scopes,omitempty" example:"api"`
	Configured  bool     `json:"configured" example:"true"`
}

// OAuthProvidersResponse represents the response for listing OAuth providers
type OAuthProvidersResponse struct {
	Success bool                `json:"success" example:"true"`
	Data    []OAuthProviderInfo `json:"data"`
}

// =============================================
// OAuth Authorize Models
// =============================================

// OAuthAuthorizeRequest represents the request to initiate OAuth flow
type OAuthAuthorizeRequest struct {
	Provider     string `json:"provider" binding:"required" description:"OAuth provider type" example:"anthropic"`
	UserID       string `json:"user_id" description:"User ID for the OAuth flow" example:"user123"`
	Redirect     string `json:"redirect" description:"URL to redirect after OAuth completion" example:"http://localhost:3000/callback"`
	ResponseType string `json:"response_type" description:"Response type: 'redirect' or 'json'" example:"json"`
	Name         string `json:"name" description:"Custom name for the provider (optional, auto-generated if empty)" example:"my-claude-account"`
}

// OAuthAuthorizeResponse represents the response for OAuth authorization initiation
type OAuthAuthorizeResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message,omitempty" example:"Authorization initiated"`
	Data    struct {
		AuthURL   string `json:"auth_url,omitempty" example:"https://claude.ai/oauth/authorize?..."`
		State     string `json:"state,omitempty" example:"random_state_string"`
		SessionID string `json:"session_id,omitempty" example:"abc123def456"` // For status tracking
		// Device code flow fields
		DeviceCode              string `json:"device_code,omitempty" example:"MN-12345678-abcdef"`
		UserCode                string `json:"user_code,omitempty" example:"ABCD-EFGH"`
		VerificationURI         string `json:"verification_uri,omitempty" example:"https://chat.qwen.ai/activate"`
		VerificationURIComplete string `json:"verification_uri_complete,omitempty" example:"https://chat.qwen.ai/activate?user_code=ABCD-EFGH"`
		ExpiresIn               int64  `json:"expires_in,omitempty" example:"1800"`
		Interval                int64  `json:"interval,omitempty" example:"5"`
		Provider                string `json:"provider,omitempty" example:"qwen_code"`
	} `json:"data"`
}

// =============================================
// OAuth Token Models
// =============================================

// TokenInfo represents OAuth token information
type TokenInfo struct {
	Provider  string `json:"provider" example:"anthropic"`
	Valid     bool   `json:"valid" example:"true"`
	ExpiresAt string `json:"expires_at,omitempty" example:"2024-01-01T12:00:00Z"`
}

// OAuthTokenResponse represents the OAuth token response
type OAuthTokenResponse struct {
	Success bool `json:"success" example:"true"`
	Data    struct {
		AccessToken  string `json:"access_token" example:"sk-ant-..."`
		RefreshToken string `json:"refresh_token,omitempty" example:"refresh_..."`
		TokenType    string `json:"token_type" example:"Bearer"`
		ExpiresAt    string `json:"expires_at,omitempty" example:"2024-01-01T12:00:00Z"`
		Provider     string `json:"provider" example:"anthropic"`
		Valid        bool   `json:"valid" example:"true"`
	} `json:"data"`
}

// OAuthTokensResponse represents the response for listing all user tokens
type OAuthTokensResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    []TokenInfo `json:"data"`
}

// =============================================
// OAuth Refresh Token Models
// =============================================

// OAuthRefreshTokenRequest represents the request to refresh an OAuth token
type OAuthRefreshTokenRequest struct {
	ProviderUUID string `json:"provider_uuid" binding:"required" description:"Provider UUID to refresh token for" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// OAuthRefreshTokenResponse represents the response for refreshing an OAuth token
type OAuthRefreshTokenResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message,omitempty" example:"Token refreshed successfully"`
	Data    struct {
		ProviderUUID string `json:"provider_uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
		AccessToken  string `json:"access_token" example:"sk-ant-..."`
		RefreshToken string `json:"refresh_token,omitempty" example:"refresh_..."`
		TokenType    string `json:"token_type" example:"Bearer"`
		ExpiresAt    string `json:"expires_at,omitempty" example:"2024-01-01T12:00:00Z"`
		ProviderType string `json:"provider_type" example:"claude_code"`
	} `json:"data"`
}

// =============================================
// OAuth Config Models
// =============================================

// OAuthUpdateProviderRequest represents the request to update OAuth provider config
type OAuthUpdateProviderRequest struct {
	ClientID     string `json:"client_id" binding:"required" description:"OAuth client ID" example:"your_client_id"`
	ClientSecret string `json:"client_secret" description:"OAuth client secret" example:"your_client_secret"`
	RedirectURL  string `json:"redirect_url" description:"OAuth redirect URI" example:"http://localhost:12580/oauth/callback"`
}

// OAuthUpdateProviderResponse represents the response for updating provider config
type OAuthUpdateProviderResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Provider configuration updated"`
	Type    string `json:"type,omitempty" example:"anthropic"`
}

// OAuthErrorResponse represents a standard error response
type OAuthErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Error   string `json:"error" example:"Error message"`
}

// OAuthMessageResponse represents a simple success message response
type OAuthMessageResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Operation successful"`
}

// =============================================
// OAuth Device Code Flow Models
// =============================================

// OAuthDeviceCodeResponse represents the response for device code flow initiation
type OAuthDeviceCodeResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message,omitempty" example:"Device code flow initiated"`
	Data    struct {
		DeviceCode              string `json:"device_code" example:"MN-12345678-abcdef"`
		UserCode                string `json:"user_code" example:"ABCD-EFGH"`
		VerificationURI         string `json:"verification_uri" example:"https://chat.qwen.ai/activate"`
		VerificationURIComplete string `json:"verification_uri_complete,omitempty" example:"https://chat.qwen.ai/activate?user_code=ABCD-EFGH"`
		ExpiresIn               int64  `json:"expires_in" example:"1800"`
		Interval                int64  `json:"interval" example:"5"`
		Provider                string `json:"provider" example:"qwen_code"`
	} `json:"data"`
}

// OAuthProviderDataResponse represents a single provider data response
type OAuthProviderDataResponse struct {
	Success bool              `json:"success" example:"true"`
	Data    OAuthProviderInfo `json:"data"`
}

// OAuthCallbackDataResponse represents the OAuth callback response with token data
type OAuthCallbackDataResponse struct {
	Success      bool   `json:"success" example:"true"`
	AccessToken  string `json:"access_token,omitempty" example:"sk-ant-..."`
	RefreshToken string `json:"refresh_token,omitempty" example:"refresh_..."`
	TokenType    string `json:"token_type,omitempty" example:"Bearer"`
	ExpiresAt    string `json:"expires_at,omitempty" example:"2024-01-01T12:00:00Z"`
	Provider     string `json:"provider,omitempty" example:"anthropic"`
}

// OAuthSessionStatusResponse represents the session status check response
type OAuthSessionStatusResponse struct {
	Success bool `json:"success" example:"true"`
	Data    struct {
		SessionID    string `json:"session_id" example:"abc123def456"`
		Status       string `json:"status" example:"success"`
		ProviderUUID string `json:"provider_uuid,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
		Error        string `json:"error,omitempty" example:"Authorization failed"`
	} `json:"data"`
}

// generateRandomSuffix generates a random alphanumeric suffix of specified length
func generateRandomSuffix(length int) string {
	bytes := make([]byte, length/2+1)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

// normalizeAPIBase normalizes an API base URL by ensuring it has protocol and path suffix
// Examples:
//   - normalizeAPIBase("portal.qwen.ai", "/v1") => "https://portal.qwen.ai/v1"
//   - normalizeAPIBase("https://api.example.com", "/v1") => "https://api.example.com/v1"
//   - normalizeAPIBase("api.example.com/v1", "/v1") => "https://api.example.com/v1"
//   - normalizeAPIBase("https://api.example.com/", "/v1") => "https://api.example.com/v1"
//   - normalizeAPIBase("https://api.example.com/v1", "/v1") => "https://api.example.com/v1"
//   - normalizeAPIBase("https://api.example.com/v2", "/v1") => "https://api.example.com/v2"
func normalizeAPIBase(baseURL, pathSuffix string) string {
	// Ensure URL has a protocol scheme
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	// Remove trailing slash from base URL
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Remove leading slash from path suffix
	pathSuffix = strings.TrimPrefix(pathSuffix, "/")

	// Check if base URL already ends with the path suffix
	if strings.HasSuffix(baseURL, pathSuffix) {
		return baseURL
	}

	// Check if base URL already has a version path (e.g., /v2, /v3)
	// In that case, keep the existing version
	if strings.Contains(baseURL, "/v") {
		parts := strings.Split(baseURL, "/")
		for _, part := range parts {
			if strings.HasPrefix(part, "v") && len(part) > 1 && part[1] >= '0' && part[1] <= '9' {
				// Found a version path like v1, v2, etc., keep it
				return baseURL
			}
		}
	}

	// Add the path suffix
	return fmt.Sprintf("%s/%s", baseURL, pathSuffix)
}

func (s *Server) useOAuthEndpoints(manager *swagger.RouteManager) {
	// Register simple HTML templates for OAuth callback
	tmpl := template.Must(template.New("oauth").Parse(`
{{ define "oauth_success.html" }}
<!DOCTYPE html>
<html>
<head>
    <title>OAuth Success - Tingly Box</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5; }
        .container { text-align: center; padding: 40px; background: white; border-radius: 12px; box-shadow: 0 4px 20px rgba(0,0,0,0.1); }
        .icon { font-size: 64px; margin-bottom: 20px; }
        h1 { color: #10b981; margin: 0 0 10px; }
        p { color: #666; margin: 8px 0; }
        .token { background: #f3f4f6; padding: 12px; border-radius: 6px; font-family: monospace; margin: 20px auto; max-width: 400px; }
        .provider-name { background: #e0f2fe; color: #0369a1; padding: 8px 16px; border-radius: 6px; font-weight: 500; margin: 10px auto; max-width: 400px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">✅</div>
        <h1>OAuth Authorization Successful</h1>
        <p><strong>Provider:</strong> {{ .provider }}</p>
        <div class="provider-name">{{ .provider_name }}</div>
        <p><strong>Token Type:</strong> {{ .token_type }}</p>
        <div class="token">{{ .access_token }}</div>
        <p style="font-size: 14px; color: #999;">Provider has been created. You can close this window and return to the application.</p>
    </div>
</body>
</html>
{{ end }}

{{ define "oauth_error.html" }}
<!DOCTYPE html>
<html>
<head>
    <title>OAuth Error - Tingly Box</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5; }
        .container { text-align: center; padding: 40px; background: white; border-radius: 12px; box-shadow: 0 4px 20px rgba(0,0,0,0.1); }
        .icon { font-size: 64px; margin-bottom: 20px; }
        h1 { color: #ef4444; margin: 0 0 10px; }
        p { color: #666; margin: 8px 0; }
        .error { background: #fef2f2; color: #dc2626; padding: 16px; border-radius: 6px; margin: 20px auto; max-width: 500px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">❌</div>
        <h1>OAuth Authorization Failed</h1>
        <div class="error">{{ .error }}</div>
        <p style="font-size: 14px; color: #999;">Please try again or contact support if the issue persists.</p>
    </div>
</body>
</html>
{{ end }}
`))
	manager.GetEngine().SetHTMLTemplate(tmpl)

	// Create authenticated API group
	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.authMW.UserAuthMiddleware())

	// OAuth Provider Management
	//apiV1.GET("/oauth/providers", s.ListOAuthProviders,
	//	swagger.WithTags("oauth"),
	//	swagger.WithDescription("List all available OAuth providers"),
	//	swagger.WithResponseModel(OAuthProvidersResponse{}),
	//)
	//
	//apiV1.GET("/oauth/providers/:type", s.GetOAuthProvider,
	//	swagger.WithTags("oauth"),
	//	swagger.WithDescription("Get specific OAuth provider configuration"),
	//	swagger.WithResponseModel(OAuthProviderDataResponse{}),
	//)
	//
	//apiV1.PUT("/oauth/providers/:type", s.UpdateOAuthProvider,
	//	swagger.WithTags("oauth"),
	//	swagger.WithDescription("Update OAuth provider configuration"),
	//	swagger.WithRequestModel(OAuthUpdateProviderRequest{}),
	//	swagger.WithResponseModel(OAuthUpdateProviderResponse{}),
	//)
	//
	//apiV1.DELETE("/oauth/providers/:type", s.DeleteOAuthProvider,
	//	swagger.WithTags("oauth"),
	//	swagger.WithDescription("Delete OAuth provider configuration (clears credentials)"),
	//	swagger.WithResponseModel(OAuthUpdateProviderResponse{}),
	//)

	// OAuth Authorization Flow
	apiV1.POST("/oauth/authorize", s.AuthorizeOAuth,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Initiate OAuth authorization flow"),
		swagger.WithRequestModel(OAuthAuthorizeRequest{}),
		swagger.WithResponseModel(OAuthAuthorizeResponse{}),
	)

	// OAuth Token Management
	apiV1.GET("/oauth/token", s.GetOAuthToken,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Get OAuth token for a user and provider"),
		swagger.WithResponseModel(OAuthTokenResponse{}),
	)

	apiV1.POST("/oauth/refresh", s.RefreshOAuthToken,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Refresh OAuth token using refresh token"),
		swagger.WithRequestModel(OAuthRefreshTokenRequest{}),
		swagger.WithResponseModel(OAuthRefreshTokenResponse{}),
	)

	apiV1.DELETE("/oauth/token", s.RevokeOAuthToken,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Revoke OAuth token for a user and provider"),
		swagger.WithResponseModel(OAuthMessageResponse{}),
	)

	apiV1.GET("/oauth/tokens", s.ListOAuthTokens,
		swagger.WithTags("oauth"),
		swagger.WithDescription("List all OAuth tokens for a user"),
		swagger.WithResponseModel(OAuthTokensResponse{}),
	)

	// OAuth Session Status
	apiV1.GET("/oauth/status", s.GetOAuthSessionStatus,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Get OAuth session status"),
		swagger.WithQueryRequired("session_id", "string", "session id to check oauth status"),
		swagger.WithResponseModel(OAuthSessionStatusResponse{}),
	)

	// OAuth Callback (no authentication required - called by OAuth provider)
	manager.GetEngine().GET("/oauth/callback", s.OAuthCallback)
	manager.GetEngine().GET("/callback", s.OAuthCallback)
}

// =============================================
// Handlers
// =============================================

// ListOAuthProviders returns all available OAuth providers
// GET /api/v1/oauth/providers
func (s *Server) ListOAuthProviders(c *gin.Context) {

	providers := s.oauthManager.GetRegistry().GetProviderInfo()
	data := make([]OAuthProviderInfo, len(providers))
	for i, p := range providers {
		data[i] = OAuthProviderInfo{
			Type:        string(p.Type),
			DisplayName: p.DisplayName,
			AuthURL:     p.AuthURL,
			Scopes:      p.Scopes,
			Configured:  p.Configured,
		}
	}

	c.JSON(http.StatusOK, OAuthProvidersResponse{
		Success: true,
		Data:    data,
	})
}

// GetOAuthProvider returns a specific OAuth provider configuration
// GET /api/v1/oauth/providers/:type
func (s *Server) GetOAuthProvider(c *gin.Context) {

	providerType := oauth2.ProviderType(c.Param("type"))
	config, ok := s.oauthManager.GetRegistry().Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, OAuthErrorResponse{
			Success: false,
			Error:   "Provider not found",
		})
		return
	}

	c.JSON(http.StatusOK, OAuthProviderDataResponse{
		Success: true,
		Data: OAuthProviderInfo{
			Type:        string(config.Type),
			DisplayName: config.DisplayName,
			AuthURL:     config.AuthURL,
			Scopes:      config.Scopes,
			Configured:  config.ClientID != "" && config.ClientSecret != "",
		},
	})
}

// UpdateOAuthProvider updates an OAuth provider configuration
// PUT /api/v1/oauth/providers/:type
func (s *Server) UpdateOAuthProvider(c *gin.Context) {
	providerType := oauth2.ProviderType(c.Param("type"))

	var req OAuthUpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return
	}

	config, ok := s.oauthManager.GetRegistry().Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, OAuthErrorResponse{
			Success: false,
			Error:   "Provider not found",
		})
		return
	}

	// Update configuration
	newConfig := &oauth2.ProviderConfig{
		Type:         config.Type,
		DisplayName:  config.DisplayName,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		AuthURL:      config.AuthURL,
		TokenURL:     config.TokenURL,
		Scopes:       config.Scopes,
		AuthStyle:    config.AuthStyle,
		OAuthMethod:  config.OAuthMethod,
		RedirectURL:  req.RedirectURL,
		ConsoleURL:   config.ConsoleURL,
	}

	s.oauthManager.GetRegistry().Register(newConfig)

	if s.logger != nil {
		s.logger.LogAction("update_oauth_provider", map[string]interface{}{
			"provider": providerType,
		}, true, "OAuth provider updated")
	}

	c.JSON(http.StatusOK, OAuthUpdateProviderResponse{
		Success: true,
		Message: "Provider configuration updated",
		Type:    string(providerType),
	})
}

// DeleteOAuthProvider removes an OAuth provider configuration (clears credentials)
// DELETE /api/v1/oauth/providers/:type
func (s *Server) DeleteOAuthProvider(c *gin.Context) {

	providerType := oauth2.ProviderType(c.Param("type"))

	config, ok := s.oauthManager.GetRegistry().Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, OAuthErrorResponse{
			Success: false,
			Error:   "Provider not found",
		})
		return
	}

	// Clear credentials
	s.oauthManager.GetRegistry().Register(&oauth2.ProviderConfig{
		Type:         config.Type,
		DisplayName:  config.DisplayName,
		ClientID:     "",
		ClientSecret: "",
		AuthURL:      config.AuthURL,
		TokenURL:     config.TokenURL,
		Scopes:       config.Scopes,
		AuthStyle:    config.AuthStyle,
		OAuthMethod:  config.OAuthMethod,
		ConsoleURL:   config.ConsoleURL,
	})

	if s.logger != nil {
		s.logger.LogAction("delete_oauth_provider", map[string]interface{}{
			"provider": providerType,
		}, true, "OAuth provider credentials cleared")
	}

	c.JSON(http.StatusOK, OAuthUpdateProviderResponse{
		Success: true,
		Message: "Provider configuration cleared",
		Type:    string(providerType),
	})
}

// AuthorizeOAuth initiates the OAuth flow
// POST /api/v1/oauth/authorize
func (s *Server) AuthorizeOAuth(c *gin.Context) {

	var req OAuthAuthorizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Also support query parameters for backward compatibility
		req.Provider = c.Query("provider")
		req.UserID = c.Query("user_id")
		req.Redirect = c.Query("redirect")
		req.ResponseType = c.Query("response_type")

		if req.Provider == "" {
			c.JSON(http.StatusBadRequest, OAuthErrorResponse{
				Success: false,
				Error:   "provider is required",
			})
			return
		}
	}

	providerType, err := oauth2.ParseProviderType(req.Provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Invalid provider: " + err.Error(),
		})
		return
	}

	// Check if provider uses device code flow
	config, ok := s.oauthManager.GetRegistry().Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, OAuthErrorResponse{
			Success: false,
			Error:   "Provider not found",
		})
		return
	}

	userID := req.UserID

	// Create session for status tracking
	session, err := s.oauthManager.CreateSession(userID, providerType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   "Failed to create session: " + err.Error(),
		})
		return
	}

	// Handle device code flow (OAuthMethodDeviceCode or OAuthMethodDeviceCodePKCE)
	if config.OAuthMethod == oauth2.OAuthMethodDeviceCode || config.OAuthMethod == oauth2.OAuthMethodDeviceCodePKCE {
		deviceCodeData, err := s.oauthManager.InitiateDeviceCodeFlow(c.Request.Context(), userID, providerType, req.Redirect, req.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, OAuthErrorResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		// Start polling for token in background
		go func() {
			fmt.Printf("[OAuth] Starting device code polling for %s in background\n", providerType)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			token, err := s.oauthManager.PollForToken(ctx, deviceCodeData, nil)
			if err != nil {
				fmt.Printf("[OAuth] Device code polling failed for %s: %v\n", providerType, err)
				// Update session status to failed
				_ = s.oauthManager.UpdateSessionStatus(session.SessionID, oauth2.SessionStatusFailed, "", err.Error())
				return
			}

			fmt.Printf("[OAuth] Device code polling succeeded for %s, creating provider\n", providerType)
			// Create provider with OAuth credentials after successful polling
			providerUUID, err := s.createProviderFromToken(token, providerType, req.Name)
			if err != nil {
				fmt.Printf("[OAuth] Failed to create provider for %s: %v\n", providerType, err)
				// Update session status to failed
				_ = s.oauthManager.UpdateSessionStatus(session.SessionID, oauth2.SessionStatusFailed, "", err.Error())
				return
			}

			// Update session status to success
			_ = s.oauthManager.UpdateSessionStatus(session.SessionID, oauth2.SessionStatusSuccess, providerUUID, "")
		}()

		// Return device code flow response with session_id
		resp := OAuthAuthorizeResponse{
			Success: true,
			Message: "Device code flow initiated",
		}
		resp.Data.SessionID = session.SessionID
		resp.Data.DeviceCode = deviceCodeData.DeviceCode
		resp.Data.UserCode = deviceCodeData.UserCode
		resp.Data.VerificationURI = deviceCodeData.VerificationURI
		resp.Data.VerificationURIComplete = deviceCodeData.VerificationURIComplete
		resp.Data.ExpiresIn = deviceCodeData.ExpiresIn
		resp.Data.Interval = deviceCodeData.Interval
		resp.Data.Provider = string(providerType)

		c.JSON(http.StatusOK, resp)
		return
	}

	// Handle standard authorization code flow
	authURL, state, err := s.oauthManager.GetAuthURL(c.Request.Context(), userID, providerType, req.Redirect, req.Name, session.SessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Return JSON response with session_id
	resp := OAuthAuthorizeResponse{
		Success: true,
		Message: "Authorization initiated",
	}
	resp.Data.AuthURL = authURL
	resp.Data.State = state
	resp.Data.SessionID = session.SessionID

	c.JSON(http.StatusOK, resp)
}

// GetOAuthToken returns the OAuth token for a user and provider
// GET /api/v1/oauth/token?provider=anthropic&user_id=xxx
func (s *Server) GetOAuthToken(c *gin.Context) {

	providerType := oauth2.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")

	token, err := s.oauthManager.GetToken(c.Request.Context(), userID, providerType)
	if err != nil {
		if err == oauth2.ErrTokenNotFound {
			c.JSON(http.StatusNotFound, OAuthErrorResponse{
				Success: false,
				Error:   "No token found for this provider",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	resp := OAuthTokenResponse{Success: true}
	resp.Data.AccessToken = token.AccessToken
	resp.Data.RefreshToken = token.RefreshToken
	resp.Data.TokenType = token.TokenType
	resp.Data.Provider = string(token.Provider)
	resp.Data.Valid = token.Valid()

	if !token.Expiry.IsZero() {
		resp.Data.ExpiresAt = token.Expiry.Format("2006-01-02T15:04:05Z07:00")
	}

	c.JSON(http.StatusOK, resp)
}

// RevokeOAuthToken removes the OAuth token for a user and provider
// DELETE /api/v1/oauth/token?provider=anthropic&user_id=xxx
func (s *Server) RevokeOAuthToken(c *gin.Context) {

	providerType := oauth2.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")

	err := s.oauthManager.RevokeToken(userID, providerType)
	if err != nil {
		if err == oauth2.ErrTokenNotFound {
			c.JSON(http.StatusNotFound, OAuthErrorResponse{
				Success: false,
				Error:   "No token found for this provider",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if s.logger != nil {
		s.logger.LogAction("revoke_oauth_token", map[string]interface{}{
			"user_id":  userID,
			"provider": providerType,
		}, true, "OAuth token revoked")
	}

	c.JSON(http.StatusOK, OAuthMessageResponse{
		Success: true,
		Message: "Token revoked successfully",
	})
}

// ListOAuthTokens returns all tokens for a user
// GET /api/v1/oauth/tokens?user_id=xxx
func (s *Server) ListOAuthTokens(c *gin.Context) {

	userID := c.Query("user_id")

	providers, err := s.oauthManager.ListProviders(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	tokens := make([]TokenInfo, 0)

	for _, provider := range providers {
		token, err := s.oauthManager.GetToken(c.Request.Context(), userID, provider)
		if err == nil && token != nil {
			expiresAt := ""
			if !token.Expiry.IsZero() {
				expiresAt = token.Expiry.Format("2006-01-02T15:04:05Z07:00")
			}
			tokens = append(tokens, TokenInfo{
				Provider:  string(provider),
				Valid:     token.Valid(),
				ExpiresAt: expiresAt,
			})
		}
	}

	c.JSON(http.StatusOK, OAuthTokensResponse{
		Success: true,
		Data:    tokens,
	})
}

// OAuthCallback handles the OAuth callback from the provider
// This is typically called by the OAuth provider redirect
// GET /oauth/callback?code=xxx&state=xxx
func (s *Server) OAuthCallback(c *gin.Context) {
	// Delegate to the oauth handler's callback, now returns name in token
	token, err := s.oauthManager.HandleCallback(c.Request.Context(), c.Request)
	if err != nil {
		c.HTML(http.StatusBadRequest, "oauth_error.html", gin.H{
			"error": err.Error(),
		})
		return
	}

	// Use createProviderFromToken to create the provider
	providerUUID, err := s.createProviderFromToken(token, token.Provider, "")
	if err != nil {
		c.HTML(http.StatusInternalServerError, "oauth_error.html", gin.H{
			"error": fmt.Sprintf("Failed to create provider: %v", err),
		})
		return
	}

	// Update session status to success if session ID exists
	if token.SessionID != "" {
		_ = s.oauthManager.UpdateSessionStatus(token.SessionID, oauth2.SessionStatusSuccess, providerUUID, "")
	}

	// Return success HTML page to inform the user
	c.HTML(http.StatusOK, "oauth_success.html", gin.H{
		"provider":      string(token.Provider),
		"provider_name": "",                             // Will be shown with UUID in the page
		"access_token":  token.AccessToken[:20] + "...", // Partially show token
		"token_type":    token.TokenType,
	})
}

// RefreshOAuthToken refreshes an OAuth token using a refresh token
// POST /api/v1/oauth/refresh
func (s *Server) RefreshOAuthToken(c *gin.Context) {
	var req OAuthRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return
	}

	// Get provider by UUID
	provider, err := s.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, OAuthErrorResponse{
			Success: false,
			Error:   "Provider not found: " + err.Error(),
		})
		return
	}

	// Check if provider uses OAuth
	if provider.AuthType != typ.AuthTypeOAuth || provider.OAuthDetail == nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Provider does not use OAuth authentication",
		})
		return
	}

	// Check if provider has refresh token
	if provider.OAuthDetail.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Provider does not have a refresh token",
		})
		return
	}

	// Parse provider type
	providerType, err := oauth2.ParseProviderType(provider.OAuthDetail.ProviderType)
	if err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Invalid provider type: " + err.Error(),
		})
		return
	}

	// Refresh the token
	token, err := s.oauthManager.RefreshToken(c.Request.Context(), provider.OAuthDetail.UserID, providerType, provider.OAuthDetail.RefreshToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   "Failed to refresh token: " + err.Error(),
		})
		return
	}

	// Update provider with new token
	var expiresAt string
	if !token.Expiry.IsZero() {
		expiresAt = token.Expiry.Format(time.RFC3339)
	}

	provider.OAuthDetail.AccessToken = token.AccessToken
	provider.OAuthDetail.RefreshToken = token.RefreshToken
	provider.OAuthDetail.ExpiresAt = expiresAt

	if err := s.config.UpdateProvider(provider.UUID, provider); err != nil {
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   "Failed to update provider: " + err.Error(),
		})
		return
	}

	// Log the successful token refresh
	if s.logger != nil {
		s.logger.LogAction("refresh_oauth_token", map[string]interface{}{
			"provider_uuid": provider.UUID,
			"provider_name": provider.Name,
			"provider_type": providerType,
		}, true, "OAuth token refreshed successfully")
	}

	// Build response
	resp := OAuthRefreshTokenResponse{
		Success: true,
		Message: "Token refreshed successfully",
	}
	resp.Data.ProviderUUID = provider.UUID
	resp.Data.AccessToken = token.AccessToken
	resp.Data.RefreshToken = token.RefreshToken
	resp.Data.TokenType = token.TokenType
	resp.Data.ProviderType = string(token.Provider)

	if !token.Expiry.IsZero() {
		resp.Data.ExpiresAt = token.Expiry.Format("2006-01-02T15:04:05Z07:00")
	}

	c.JSON(http.StatusOK, resp)
}

// createProviderFromToken creates a provider from an OAuth token
// This helper is used by both OAuthCallback and device code flow
// Returns the provider UUID or an error
func (s *Server) createProviderFromToken(token *oauth2.Token, providerType oauth2.ProviderType, customName string) (string, error) {
	// Get custom name from token (stored in state during authorize)
	if customName == "" {
		customName = token.Name
	}

	// Generate unique provider name with random suffix
	pType := string(providerType)
	var providerName string
	if customName != "" {
		// Use custom name from state
		providerName = customName
	} else {
		// Auto-generate name with 6-char random suffix
		randomSuffix := generateRandomSuffix(6)
		providerName = fmt.Sprintf("%s-%s", pType, randomSuffix)
	}

	// Generate UUID for the provider
	providerUUID, err := uuid.NewUUID()
	if err != nil {
		return "", fmt.Errorf("failed to generate provider UUID: %w", err)
	}

	// Determine API base and style based on provider type
	// Priority: token.ResourceURL > provider default > mock
	var apiBase string
	var apiStyle typ.APIStyle

	// If token contains ResourceURL from OAuth response, use it
	{
		// Otherwise use provider default
		switch providerType {
		case oauth2.ProviderClaudeCode:
			apiBase = "https://api.anthropic.com"
			apiStyle = typ.APIStyleAnthropic
		case oauth2.ProviderQwenCode:
			if token.ResourceURL != "" {
				// Normalize ResourceURL: ensure it has /v1 suffix for OpenAI-compatible API
				apiBase = normalizeAPIBase(token.ResourceURL, "/v1")
				fmt.Printf("[OAuth] Using ResourceURL from token: %s (normalized to: %s)\n", token.ResourceURL, apiBase)
			} else {
				apiBase = "https://portal.qwen.ai/v1"
			}
			apiStyle = typ.APIStyleOpenAI
		case oauth2.ProviderGoogle:
			apiBase = "https://generativelanguage.googleapis.com"
			apiStyle = typ.APIStyleOpenAI
		case oauth2.ProviderOpenAI:
			apiBase = "https://api.openai.com/v1"
			apiStyle = typ.APIStyleOpenAI
		default:
			// For mock and unknown providers
			apiBase = "mock"
			apiStyle = typ.APIStyleOpenAI
		}
	}

	// For providers without ResourceURL, determine APIStyle if not set
	if apiStyle == "" {
		apiStyle = typ.APIStyleOpenAI
	}

	// Build expires_at string
	var expiresAt string
	if !token.Expiry.IsZero() {
		expiresAt = token.Expiry.Format(time.RFC3339)
	}

	// Create Provider with OAuth credentials
	provider := &typ.Provider{
		UUID:     providerUUID.String(),
		Name:     providerName,
		APIBase:  apiBase,
		APIStyle: apiStyle,
		Enabled:  true,
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken:  token.AccessToken,
			ProviderType: string(token.Provider),
			UserID:       uuid.New().String(),
			RefreshToken: token.RefreshToken,
			ExpiresAt:    expiresAt,
		},
	}

	// Save provider to config
	if err := s.config.AddProvider(provider); err != nil {
		return "", fmt.Errorf("failed to save provider: %w", err)
	}

	// Log the successful provider creation
	if s.logger != nil {
		s.logger.LogAction("oauth_provider_created", map[string]interface{}{
			"provider_name": providerName,
			"provider_type": string(token.Provider),
			"uuid":          providerUUID.String(),
		}, true, "OAuth provider created successfully")
	}

	return providerUUID.String(), nil
}

// GetOAuthSessionStatus returns the status of an OAuth session
// GET /api/v1/oauth/status?session_id=xxx
func (s *Server) GetOAuthSessionStatus(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "session_id parameter is required",
		})
		return
	}

	session, err := s.oauthManager.GetSession(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, OAuthSessionStatusResponse{
			Success: false,
		})
		return
	}

	resp := OAuthSessionStatusResponse{
		Success: true,
	}
	resp.Data.SessionID = session.SessionID
	resp.Data.Status = string(session.Status)
	resp.Data.ProviderUUID = session.ProviderUUID
	resp.Data.Error = session.Error

	c.JSON(http.StatusOK, resp)
}
