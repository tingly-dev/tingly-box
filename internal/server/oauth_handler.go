package server

import (
	"fmt"
	"html/template"
	"net/http"
	"time"
	oauth2 "tingly-box/pkg/oauth"
	"tingly-box/pkg/swagger"

	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
}

// OAuthAuthorizeResponse represents the response for OAuth authorization initiation
type OAuthAuthorizeResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message,omitempty" example:"Authorization initiated"`
	Data    struct {
		AuthURL string `json:"auth_url" example:"https://claude.ai/oauth/authorize?..."`
		State   string `json:"state" example:"random_state_string"`
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

	// OAuth Callback (no authentication required - called by OAuth provider)
	manager.GetEngine().GET("/oauth/callback", s.OAuthCallback)
}

// =============================================
// Handlers
// =============================================

// ListOAuthProviders returns all available OAuth providers
// GET /api/v1/oauth/providers
func (s *Server) ListOAuthProviders(c *gin.Context) {
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

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
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

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
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

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
		Type:               config.Type,
		DisplayName:        config.DisplayName,
		ClientID:           req.ClientID,
		ClientSecret:       req.ClientSecret,
		AuthURL:            config.AuthURL,
		TokenURL:           config.TokenURL,
		Scopes:             config.Scopes,
		AuthStyle:          config.AuthStyle,
		RedirectURL:        req.RedirectURL,
		ConsoleURL:         config.ConsoleURL,
		ClientIDEnvVar:     config.ClientIDEnvVar,
		ClientSecretEnvVar: config.ClientSecretEnvVar,
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
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

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
		Type:               config.Type,
		DisplayName:        config.DisplayName,
		ClientID:           "",
		ClientSecret:       "",
		AuthURL:            config.AuthURL,
		TokenURL:           config.TokenURL,
		Scopes:             config.Scopes,
		AuthStyle:          config.AuthStyle,
		ConsoleURL:         config.ConsoleURL,
		ClientIDEnvVar:     config.ClientIDEnvVar,
		ClientSecretEnvVar: config.ClientSecretEnvVar,
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
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

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

	userID := req.UserID
	if userID == "" {
		userID = oauth2.DefaultUserID
	}

	// Get auth URL
	authURL, state, err := s.oauthManager.GetAuthURL(c.Request.Context(), userID, providerType, req.Redirect)
	if err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Return JSON response
	resp := OAuthAuthorizeResponse{
		Success: true,
		Message: "Authorization initiated",
	}
	resp.Data.AuthURL = authURL
	resp.Data.State = state

	c.JSON(http.StatusOK, resp)
}

// GetOAuthToken returns the OAuth token for a user and provider
// GET /api/v1/oauth/token?provider=anthropic&user_id=xxx
func (s *Server) GetOAuthToken(c *gin.Context) {
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

	providerType := oauth2.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = oauth2.DefaultUserID
	}

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
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

	providerType := oauth2.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = oauth2.DefaultUserID
	}

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
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = oauth2.DefaultUserID
	}

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
	if s.oauthHandler == nil {
		c.JSON(http.StatusServiceUnavailable, OAuthErrorResponse{
			Success: false,
			Error:   "OAuth service not available",
		})
		return
	}

	// Delegate to the oauth handler's callback
	token, _, err := s.oauthManager.HandleCallback(c.Request.Context(), c.Request)
	if err != nil {
		c.HTML(http.StatusBadRequest, "oauth_error.html", gin.H{
			"error": err.Error(),
		})
		return
	}

	// Generate unique provider name
	providerType := string(token.Provider)
	timestamp := time.Now().Format("20060102-150405")
	providerName := fmt.Sprintf("oauth-%s-%s", providerType, timestamp)

	// Generate UUID for the provider
	providerUUID, err := uuid.NewUUID()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "oauth_error.html", gin.H{
			"error": fmt.Sprintf("Failed to generate provider UUID: %v", err),
		})
		return
	}

	// Determine API base and style based on provider type
	var apiBase string
	var apiStyle config.APIStyle
	switch token.Provider {
	case oauth2.ProviderAnthropic:
		apiBase = "https://api.anthropic.com"
		apiStyle = config.APIStyleAnthropic
	case oauth2.ProviderGoogle:
		apiBase = "https://generativelanguage.googleapis.com"
		apiStyle = config.APIStyleOpenAI
	case oauth2.ProviderOpenAI:
		apiBase = "https://api.openai.com/v1"
		apiStyle = config.APIStyleOpenAI
	default:
		// For mock and unknown providers
		apiBase = "mock"
		apiStyle = config.APIStyleOpenAI
	}

	// Build expires_at string
	var expiresAt string
	if !token.Expiry.IsZero() {
		expiresAt = token.Expiry.Format(time.RFC3339)
	}

	// Create Provider with OAuth credentials
	provider := &config.Provider{
		UUID:     providerUUID.String(),
		Name:     providerName,
		APIBase:  apiBase,
		APIStyle: apiStyle,
		Enabled:  true,
		AuthType: config.AuthTypeOAuth,
		OAuthDetail: &config.OAuthDetail{
			AccessToken:  token.AccessToken,
			ProviderType: string(token.Provider),
			UserID:       oauth2.DefaultUserID,
			RefreshToken: token.RefreshToken,
			ExpiresAt:    expiresAt,
		},
	}

	// Save provider to config
	if err := s.config.AddProvider(provider); err != nil {
		c.HTML(http.StatusInternalServerError, "oauth_error.html", gin.H{
			"error": fmt.Sprintf("Failed to save provider: %v", err),
		})
		return
	}

	// Log the successful provider creation
	if s.logger != nil {
		s.logger.LogAction("oauth_provider_created", map[string]interface{}{
			"provider_name": providerName,
			"provider_type": string(token.Provider),
			"uuid":          providerUUID.String(),
		}, true, "OAuth provider created successfully")
	}

	// Return success HTML page to inform the user
	c.HTML(http.StatusOK, "oauth_success.html", gin.H{
		"provider":      string(token.Provider),
		"provider_name": providerName,
		"access_token":  token.AccessToken[:20] + "...", // Partially show token
		"token_type":    token.TokenType,
	})
}
