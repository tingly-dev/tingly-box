package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

const (
	// DefaultUserID is the default user ID for single-user deployments
	DefaultUserID = "default"
)

// Handler provides HTTP handlers for OAuth endpoints
type Handler struct {
	manager *oauth.Manager
}

// NewHandler creates a new OAuth HTTP handler
func NewHandler(manager *oauth.Manager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// RegisterRoutes registers OAuth routes with a Gin router group
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	oauth := r.Group("/oauth")
	{
		oauth.GET("/providers", h.ListProviders)
		oauth.GET("/authorize", h.Authorize)
		oauth.GET("/callback", h.Callback)
		oauth.GET("/token", h.GetToken)
		oauth.DELETE("/token", h.RevokeToken)
		oauth.GET("/tokens", h.ListTokens)
	}

	// Register custom callback paths for providers that require specific routes
	// e.g., codex requires /auth/callback
	for _, providerType := range h.manager.GetRegistry().List() {
		if config, ok := h.manager.GetRegistry().Get(providerType); ok {
			if config.Callback != "" && config.Callback != "/callback" {
				r.GET(config.Callback, h.Callback)
			}
		}
	}
}

// ListProviders returns all available OAuth providers
// GET /oauth/providers
func (h *Handler) ListProviders(c *gin.Context) {
	providers := h.manager.GetRegistry().GetProviderInfo()
	c.JSON(http.StatusOK, gin.H{
		"providers": providers,
	})
}

// Authorize initiates the OAuth flow by redirecting to the provider's auth URL
// GET /oauth/authorize?provider=anthropic&user_id=xxx&redirect_to=xxx&name=xxx
func (h *Handler) Authorize(c *gin.Context) {
	providerType := oauth.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = DefaultUserID
	}

	redirectTo := c.Query("redirect_to")
	name := c.Query("name") // Optional custom provider name

	// Get auth URL
	sessionID := c.Query("session_id") // Optional session ID for tracking
	authURL, state, err := h.manager.GetAuthURL(userID, providerType, redirectTo, name, sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// For AJAX requests, return JSON with the auth URL
	if strings.Contains(c.GetHeader("Accept"), "application/json") {
		c.JSON(http.StatusOK, gin.H{
			"auth_url": authURL,
			"state":    state,
		})
		return
	}

	// For regular requests, redirect
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// Callback handles the OAuth callback from the provider
// GET /oauth/callback?code=xxx&state=xxx&proxy_url=xxx
func (h *Handler) Callback(c *gin.Context) {
	// Optional proxy URL from query parameter
	var opts []oauth.Option
	if proxyURL := c.Query("proxy_url"); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			opts = append(opts, oauth.WithProxyURL(u))
		}
	}

	token, err := h.manager.HandleCallback(c.Request.Context(), c.Request, opts...)
	if err != nil {
		c.HTML(http.StatusBadRequest, "oauth_error.html", gin.H{
			"error": err.Error(),
		})
		return
	}

	// If there's a redirect URL, redirect there with the token info
	if token.RedirectTo != "" {
		// Parse redirect URL and add token info
		redirectURL := token.RedirectTo
		if strings.Contains(redirectURL, "?") {
			redirectURL += "&token=" + token.AccessToken
		} else {
			redirectURL += "?token=" + token.AccessToken
		}
		c.Redirect(http.StatusTemporaryRedirect, redirectURL)
		return
	}

	// Otherwise, return JSON
	c.JSON(http.StatusOK, gin.H{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"token_type":    token.TokenType,
		"expires_at":    token.Expiry,
		"provider":      token.Provider,
	})
}

// GetToken returns the OAuth token for a user and provider
// GET /oauth/token?provider=anthropic&user_id=xxx&proxy_url=xxx
func (h *Handler) GetToken(c *gin.Context) {
	providerType := oauth.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = DefaultUserID
	}

	// Optional proxy URL from query parameter
	var opts []oauth.Option
	if proxyURL := c.Query("proxy_url"); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			opts = append(opts, oauth.WithProxyURL(u))
		}
	}

	token, err := h.manager.GetToken(c.Request.Context(), userID, providerType, opts...)
	if err != nil {
		if err == oauth.ErrTokenNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "no token found for this provider",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": token.AccessToken,
		"token_type":   token.TokenType,
		"expires_at":   token.Expiry,
		"provider":     token.Provider,
		"valid":        token.Valid(),
	})
}

// RevokeToken removes the OAuth token for a user and provider
// DELETE /oauth/token?provider=anthropic&user_id=xxx
func (h *Handler) RevokeToken(c *gin.Context) {
	providerType := oauth.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = DefaultUserID
	}

	err := h.manager.RevokeToken(userID, providerType)
	if err != nil {
		if err == oauth.ErrTokenNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "no token found for this provider",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "token revoked successfully",
	})
}

// ListTokens returns all tokens for a user
// GET /oauth/tokens?user_id=xxx
func (h *Handler) ListTokens(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		userID = DefaultUserID
	}

	providers, err := h.manager.ListProviders(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	type TokenInfo struct {
		Provider  oauth.ProviderType `json:"provider"`
		Valid     bool               `json:"valid"`
		ExpiresAt string             `json:"expires_at,omitempty"`
	}

	tokens := make([]TokenInfo, 0, len(providers))
	for _, provider := range providers {
		token, err := h.manager.GetToken(c.Request.Context(), userID, provider)
		if err == nil && token != nil {
			expiresAt := ""
			if !token.Expiry.IsZero() {
				expiresAt = token.Expiry.Format("2006-01-02T15:04:05Z07:00")
			}
			tokens = append(tokens, TokenInfo{
				Provider:  provider,
				Valid:     token.Valid(),
				ExpiresAt: expiresAt,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tokens": tokens,
	})
}

// ConfigHandler handles OAuth configuration updates
type ConfigHandler struct {
	manager  *oauth.Manager
	registry *oauth.Registry
}

// NewConfigHandler creates a new OAuth configuration handler
func NewConfigHandler(manager *oauth.Manager, registry *oauth.Registry) *ConfigHandler {
	return &ConfigHandler{
		manager:  manager,
		registry: registry,
	}
}

// RegisterConfigRoutes registers OAuth configuration routes
func (h *ConfigHandler) RegisterConfigRoutes(r *gin.Engine) {
	config := r.Group("/api/v1/oauth")
	{
		config.GET("/providers", h.ListProviders)
		config.GET("/providers/:type", h.GetProvider)
		config.PUT("/providers/:type", h.UpdateProvider)
		config.DELETE("/providers/:type", h.DeleteProvider)
	}
}

// ListProviders returns all registered OAuth providers
// GET /api/v1/oauth/providers
func (h *ConfigHandler) ListProviders(c *gin.Context) {
	providers := h.registry.GetProviderInfo()
	c.JSON(http.StatusOK, providers)
}

// GetProvider returns a specific OAuth provider configuration
// GET /api/v1/oauth/providers/:type
func (h *ConfigHandler) GetProvider(c *gin.Context) {
	providerType := oauth.ProviderType(c.Param("type"))
	config, ok := h.registry.Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("provider %s not found", providerType),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":         config.Type,
		"display_name": config.DisplayName,
		"auth_url":     config.AuthURL,
		"token_url":    config.TokenURL,
		"scopes":       config.Scopes,
		"configured":   config.ClientID != "" && config.ClientSecret != "",
	})
}

// UpdateProvider updates an OAuth provider configuration
// PUT /api/v1/oauth/providers/:type
func (h *ConfigHandler) UpdateProvider(c *gin.Context) {
	providerType := oauth.ProviderType(c.Param("type"))

	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURL  string `json:"redirect_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	config, ok := h.registry.Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("provider %s not found", providerType),
		})
		return
	}

	// Update configuration (create a copy)
	newConfig := &oauth.ProviderConfig{
		Type:               config.Type,
		DisplayName:        config.DisplayName,
		ClientID:           req.ClientID,
		ClientSecret:       req.ClientSecret,
		AuthURL:            config.AuthURL,
		TokenURL:           config.TokenURL,
		Scopes:             config.Scopes,
		AuthStyle:          config.AuthStyle,
		OAuthMethod:        config.OAuthMethod,
		TokenRequestFormat: config.TokenRequestFormat,
		RedirectURL:        req.RedirectURL,
		Callback:           config.Callback, // Preserve original callback
		ConsoleURL:         config.ConsoleURL,
		DeviceCodeURL:      config.DeviceCodeURL,
		GrantType:          config.GrantType,
		Hook:               config.Hook,
	}

	h.registry.Register(newConfig)

	c.JSON(http.StatusOK, gin.H{
		"message": "provider configuration updated",
		"type":    providerType,
	})
}

// DeleteProvider removes an OAuth provider configuration
// DELETE /api/v1/oauth/providers/:type
func (h *ConfigHandler) DeleteProvider(c *gin.Context) {
	providerType := oauth.ProviderType(c.Param("type"))

	config, ok := h.registry.Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("provider %s not found", providerType),
		})
		return
	}

	// Clear credentials by registering with empty values
	h.registry.Register(&oauth.ProviderConfig{
		Type:               config.Type,
		DisplayName:        config.DisplayName,
		ClientID:           "",
		ClientSecret:       "",
		AuthURL:            config.AuthURL,
		TokenURL:           config.TokenURL,
		Scopes:             config.Scopes,
		AuthStyle:          config.AuthStyle,
		OAuthMethod:        config.OAuthMethod,
		TokenRequestFormat: config.TokenRequestFormat,
		Callback:           config.Callback, // Preserve original callback
		ConsoleURL:         config.ConsoleURL,
		DeviceCodeURL:      config.DeviceCodeURL,
		GrantType:          config.GrantType,
		Hook:               config.Hook,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "provider configuration cleared",
		"type":    providerType,
	})
}

// ProviderConfigInput represents provider configuration from environment or file
type ProviderConfigInput struct {
	ProviderType oauth.ProviderType `json:"provider_type" yaml:"provider_type"`
	ClientID     string             `json:"client_id" yaml:"client_id"`
	ClientSecret string             `json:"client_secret" yaml:"client_secret"`
	RedirectURL  string             `json:"redirect_url,omitempty" yaml:"redirect_url,omitempty"`
}

// LoadProviderConfigs loads provider configurations from a list
func LoadProviderConfigs(registry *oauth.Registry, configs []ProviderConfigInput) {
	for _, cfg := range configs {
		existing, ok := registry.Get(cfg.ProviderType)
		if ok {
			// Update existing config
			registry.Register(&oauth.ProviderConfig{
				Type:               existing.Type,
				DisplayName:        existing.DisplayName,
				ClientID:           cfg.ClientID,
				ClientSecret:       cfg.ClientSecret,
				AuthURL:            existing.AuthURL,
				TokenURL:           existing.TokenURL,
				Scopes:             existing.Scopes,
				AuthStyle:          existing.AuthStyle,
				OAuthMethod:        existing.OAuthMethod,
				TokenRequestFormat: existing.TokenRequestFormat,
				RedirectURL:        cfg.RedirectURL,
				Callback:           existing.Callback, // Preserve original callback
				ConsoleURL:         existing.ConsoleURL,
				DeviceCodeURL:      existing.DeviceCodeURL,
				GrantType:          existing.GrantType,
				Hook:               existing.Hook,
			})
		}
	}
}

// MarshalProviderConfigs marshals provider configs for storage (without secrets)
func MarshalProviderConfigs(registry *oauth.Registry) ([]byte, error) {
	providers := registry.GetProviderInfo()
	return json.Marshal(providers)
}
