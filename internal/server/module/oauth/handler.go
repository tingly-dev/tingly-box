package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
	oauth2 "github.com/tingly-dev/tingly-box/pkg/oauth"
)

// CallbackServerManager manages dynamic callback servers for OAuth
type CallbackServerManager interface {
	StartDynamicCallbackServer(sessionID string, port int) error
	StopDynamicCallbackServer(sessionID string)
}

// Handler handles OAuth-related HTTP requests
type Handler struct {
	oauthManager          *oauth2.Manager
	config                *config.Config
	callbackServerManager CallbackServerManager
}

// NewHandler creates a new OAuth handler
func NewHandler(oauthManager *oauth2.Manager, cfg *config.Config) *Handler {
	return &Handler{
		oauthManager: oauthManager,
		config:       cfg,
	}
}

// SetCallbackServerManager sets the callback server manager (called by Server)
func (h *Handler) SetCallbackServerManager(csm CallbackServerManager) {
	h.callbackServerManager = csm
}

// CreateProviderFromToken is exported for use by the server's root OAuth callback
func (h *Handler) CreateProviderFromToken(token *oauth2.Token, providerType oauth2.ProviderType, customName, sessionID string) (string, error) {
	return h.createProviderFromToken(token, providerType, customName, sessionID)
}

// =============================================
// Provider Management Handlers
// =============================================

// ListOAuthProviders returns all available OAuth providers
// GET /api/v1/oauth/providers
func (h *Handler) ListOAuthProviders(c *gin.Context) {
	providers := h.oauthManager.GetRegistry().GetProviderInfo()
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
func (h *Handler) GetOAuthProvider(c *gin.Context) {
	providerType := oauth2.ProviderType(c.Param("type"))
	config, ok := h.oauthManager.GetRegistry().Get(providerType)
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
func (h *Handler) UpdateOAuthProvider(c *gin.Context) {
	providerType := oauth2.ProviderType(c.Param("type"))

	var req OAuthUpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return
	}

	config, ok := h.oauthManager.GetRegistry().Get(providerType)
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

	h.oauthManager.GetRegistry().Register(newConfig)

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action":   "update_oauth_provider",
		"provider": providerType,
	}).Info("OAuth provider updated")

	c.JSON(http.StatusOK, OAuthUpdateProviderResponse{
		Success: true,
		Message: "Provider configuration updated",
		Type:    string(providerType),
	})
}

// DeleteOAuthProvider deletes an OAuth provider configuration
// DELETE /api/v1/oauth/providers/:type
func (h *Handler) DeleteOAuthProvider(c *gin.Context) {
	providerType := oauth2.ProviderType(c.Param("type"))

	config, ok := h.oauthManager.GetRegistry().Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, OAuthErrorResponse{
			Success: false,
			Error:   "Provider not found",
		})
		return
	}

	// Clear credentials by registering with empty secrets
	h.oauthManager.GetRegistry().Register(&oauth2.ProviderConfig{
		Type:         config.Type,
		DisplayName:  config.DisplayName,
		ClientID:     "",
		ClientSecret: "",
		AuthURL:      config.AuthURL,
		TokenURL:     config.TokenURL,
		Scopes:       config.Scopes,
		AuthStyle:    config.AuthStyle,
		OAuthMethod:  config.OAuthMethod,
		RedirectURL:  config.RedirectURL,
		ConsoleURL:   config.ConsoleURL,
	})

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action":   "delete_oauth_provider",
		"provider": providerType,
	}).Info("OAuth provider deleted")

	c.JSON(http.StatusOK, OAuthUpdateProviderResponse{
		Success: true,
		Message: "Provider configuration deleted",
		Type:    string(providerType),
	})
}

// =============================================
// Authorization Handlers
// =============================================

// AuthorizeOAuth initiates OAuth authorization flow
// POST /api/v1/oauth/authorize
func (h *Handler) AuthorizeOAuth(c *gin.Context) {
	var req OAuthAuthorizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Also support query parameters for backward compatibility
		if req.Provider == "" {
			req.Provider = c.Query("provider")
		}
		if req.UserID == "" {
			req.UserID = c.Query("user_id")
		}
		if req.Redirect == "" {
			req.Redirect = c.Query("redirect")
		}
		if req.ResponseType == "" {
			req.ResponseType = c.Query("response_type")
		}
		if req.Name == "" {
			req.Name = c.Query("name")
		}
		if req.ProxyURL == "" {
			req.ProxyURL = c.Query("proxy_url")
		}

		// If still no provider, return error
		if req.Provider == "" {
			c.JSON(http.StatusBadRequest, OAuthErrorResponse{
				Success: false,
				Error:   "Invalid request: provider is required",
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
	config, ok := h.oauthManager.GetRegistry().Get(providerType)
	if !ok {
		c.JSON(http.StatusNotFound, OAuthErrorResponse{
			Success: false,
			Error:   "Provider not found",
		})
		return
	}

	userID := req.UserID
	proxyURL := req.ProxyURL
	log.Printf("[OAuth] Received proxy_url from request: '%s'", req.ProxyURL)

	// If no manual proxy URL, try to auto-detect from existing providers
	if proxyURL == "" {
		providerToBase := map[oauth2.ProviderType]string{
			oauth2.ProviderCodex:       "openai",
			oauth2.ProviderOpenAI:      "openai",
			oauth2.ProviderClaudeCode:  "anthropic",
			oauth2.ProviderGemini:      "google",
			oauth2.ProviderAntigravity: "google",
		}

		if baseProvider, ok := providerToBase[providerType]; ok {
			providers := h.config.ListProviders()
			for _, p := range providers {
				isOpenAIProvider := p.APIStyle == protocol.APIStyleOpenAI ||
					strings.Contains(p.APIBase, "openai.com") ||
					(p.Models != nil && len(p.Models) > 0 && strings.HasPrefix(p.Models[0], "gpt"))
				isAnthropicProvider := p.APIStyle == protocol.APIStyleAnthropic ||
					strings.Contains(p.APIBase, "anthropic.com")

				var matches bool
				switch baseProvider {
				case "openai":
					matches = isOpenAIProvider
				case "anthropic":
					matches = isAnthropicProvider
				}

				if matches && p.ProxyURL != "" {
					proxyURL = p.ProxyURL
					log.Printf("[OAuth] Auto-detected proxy URL from provider %s: %s", p.Name, proxyURL)
					break
				}
			}
		}
	}

	// Set proxy URL on OAuth manager for token exchange
	// This must be done before the callback happens so the token exchange uses the proxy
	if proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			c.JSON(http.StatusBadRequest, OAuthErrorResponse{
				Success: false,
				Error:   "Invalid proxy URL: " + err.Error(),
			})
			return
		}
		h.oauthManager.SetProxyURL(parsedURL)
		log.Printf("[OAuth] Proxy URL configured: %s", proxyURL)
	}

	// Generate session ID for this OAuth flow
	sessionID := uuid.New().String()

	// Create OAuth session for token exchange using the generated sessionID
	// This ensures frontend polling and OAuth callback use the same session
	now := time.Now()
	oauthSession := &oauth2.SessionState{
		SessionID: sessionID,
		Status:    oauth2.SessionStatusPending,
		Provider:  providerType,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(oauth2.DefaultSessionExpiry), // Use unified session expiration
	}
	// Store proxy URL if provided
	if proxyURL != "" {
		oauthSession.ProxyURL = proxyURL
	}
	h.oauthManager.StoreSession(oauthSession)

	// Start dynamic callback server if provider has port constraints (like Codex)
	if len(config.CallbackPorts) > 0 && h.callbackServerManager != nil {
		callbackPort := config.CallbackPorts[0]
		if err := h.callbackServerManager.StartDynamicCallbackServer(sessionID, callbackPort); err != nil {
			c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
				Success: false,
				Error:   "Failed to start callback server: " + err.Error(),
			})
			return
		}

		// Update OAuth manager's BaseURL to use the callback server's port
		h.oauthManager.SetBaseURL(fmt.Sprintf("http://localhost:%d", callbackPort))
	}

	// Handle device code flow
	if config.OAuthMethod == oauth2.OAuthMethodDeviceCode || config.OAuthMethod == oauth2.OAuthMethodDeviceCodePKCE {
		deviceCodeData, err := h.oauthManager.InitiateDeviceCodeFlow(c.Request.Context(), userID, providerType, req.Redirect, req.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, OAuthErrorResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		// Start polling for token in background
		go h.pollForDeviceCodeToken(c.Request.Context(), deviceCodeData, providerType, req.Name, sessionID)

		// Return device code flow response
		resp := OAuthAuthorizeResponse{
			Success: true,
			Message: "Device code flow initiated",
		}
		resp.Data.SessionID = sessionID
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
	authURL, state, err := h.oauthManager.GetAuthURL(userID, providerType, req.Redirect, req.Name, sessionID)
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
	resp.Data.SessionID = sessionID

	c.JSON(http.StatusOK, resp)
}

// pollForDeviceCodeToken polls for token in background after device code flow initiation
func (h *Handler) pollForDeviceCodeToken(ctx context.Context, deviceCodeData *oauth2.DeviceCodeData, providerType oauth2.ProviderType, customName, sessionID string) {
	fmt.Printf("[OAuth] Starting device code polling for %s in background\n", providerType)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	token, err := h.oauthManager.PollForToken(ctx, deviceCodeData, nil)
	if err != nil {
		fmt.Printf("[OAuth] Device code polling failed for %s: %v\n", providerType, err)
		_ = h.oauthManager.UpdateSessionStatus(sessionID, oauth2.SessionStatusFailed, "", err.Error())
		return
	}

	fmt.Printf("[OAuth] Device code polling succeeded for %s, creating provider\n", providerType)
	providerUUID, err := h.createProviderFromToken(token, providerType, customName, sessionID)
	if err != nil {
		fmt.Printf("[OAuth] Failed to create provider for %s: %v\n", providerType, err)
		_ = h.oauthManager.UpdateSessionStatus(sessionID, oauth2.SessionStatusFailed, "", err.Error())
		return
	}

	// Update session status to success
	_ = h.oauthManager.UpdateSessionStatus(sessionID, oauth2.SessionStatusSuccess, providerUUID, "")
}

// =============================================
// Token Management Handlers
// =============================================

// GetOAuthToken returns the OAuth token for a user and provider
// GET /api/v1/oauth/token?provider_uuid=xxx OR ?provider=xxx&user_id=xxx (deprecated)
func (h *Handler) GetOAuthToken(c *gin.Context) {
	// Try new API first (provider_uuid)
	providerUUID := c.Query("provider_uuid")
	if providerUUID != "" {
		provider, err := h.config.GetProviderByUUID(providerUUID)
		if err != nil {
			c.JSON(http.StatusNotFound, OAuthErrorResponse{
				Success: false,
				Error:   "Provider not found",
			})
			return
		}

		if provider.OAuthDetail == nil {
			c.JSON(http.StatusBadRequest, OAuthErrorResponse{
				Success: false,
				Error:   "Provider is not an OAuth provider",
			})
			return
		}

		resp := OAuthTokenResponse{Success: true}
		resp.Data.AccessToken = provider.OAuthDetail.AccessToken
		resp.Data.RefreshToken = provider.OAuthDetail.RefreshToken
		resp.Data.TokenType = "Bearer"
		resp.Data.ExpiresAt = provider.OAuthDetail.ExpiresAt
		resp.Data.Provider = provider.OAuthDetail.ProviderType

		// Check if token is valid
		if provider.OAuthDetail.ExpiresAt != "" {
			expiresAt, err := time.Parse(time.RFC3339, provider.OAuthDetail.ExpiresAt)
			if err == nil {
				resp.Data.Valid = time.Now().Before(expiresAt)
			}
		}

		c.JSON(http.StatusOK, resp)
		return
	}

	// Fall back to old API (provider + user_id) for backward compatibility
	providerType := oauth2.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")

	token, err := h.oauthManager.GetToken(c.Request.Context(), userID, providerType)
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

// RefreshOAuthToken refreshes an OAuth token using refresh token
// POST /api/v1/oauth/refresh
func (h *Handler) RefreshOAuthToken(c *gin.Context) {
	var req OAuthRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return
	}

	provider, err := h.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, OAuthErrorResponse{
			Success: false,
			Error:   "Provider not found",
		})
		return
	}

	if provider.OAuthDetail == nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Provider is not an OAuth provider",
		})
		return
	}

	if provider.OAuthDetail.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "No refresh token available",
		})
		return
	}

	providerType, err := oauth2.ParseProviderType(provider.OAuthDetail.ProviderType)
	if err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Invalid provider type: " + err.Error(),
		})
		return
	}

	// Refresh token
	token, err := h.oauthManager.RefreshToken(
		c.Request.Context(),
		provider.OAuthDetail.UserID,
		providerType,
		provider.OAuthDetail.RefreshToken,
		oauth2.WithProxyString(provider.ProxyURL),
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   "Failed to refresh token: " + err.Error(),
		})
		return
	}

	// Update provider with new token
	provider.OAuthDetail.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		provider.OAuthDetail.RefreshToken = token.RefreshToken
	}
	provider.OAuthDetail.ExpiresAt = token.Expiry.Format(time.RFC3339)

	if err := h.config.UpdateProvider(provider.UUID, provider); err != nil {
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   "Failed to update provider: " + err.Error(),
		})
		return
	}

	// Build response
	resp := OAuthRefreshTokenResponse{
		Success: true,
		Message: "Token refreshed successfully",
	}
	resp.Data.ProviderUUID = provider.UUID
	resp.Data.AccessToken = token.AccessToken
	resp.Data.RefreshToken = token.RefreshToken
	resp.Data.TokenType = "Bearer"
	resp.Data.ExpiresAt = token.Expiry.Format(time.RFC3339)
	resp.Data.ProviderType = string(token.Provider)

	c.JSON(http.StatusOK, resp)
}

// RevokeOAuthToken revokes an OAuth token
// DELETE /api/v1/oauth/token?provider_uuid=xxx OR ?provider=xxx&user_id=xxx (deprecated)
func (h *Handler) RevokeOAuthToken(c *gin.Context) {
	// Try new API first (provider_uuid)
	providerUUID := c.Query("provider_uuid")
	if providerUUID != "" {
		provider, err := h.config.GetProviderByUUID(providerUUID)
		if err != nil {
			c.JSON(http.StatusNotFound, OAuthErrorResponse{
				Success: false,
				Error:   "Provider not found",
			})
			return
		}

		// Clear OAuth credentials
		provider.OAuthDetail = nil

		if err := h.config.UpdateProvider(provider.UUID, provider); err != nil {
			c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
				Success: false,
				Error:   "Failed to update provider: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, OAuthMessageResponse{
			Success: true,
			Message: "Token revoked successfully",
		})
		return
	}

	// Fall back to old API (provider + user_id) for backward compatibility
	providerType := oauth2.ProviderType(c.Query("provider"))
	if providerType == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "provider parameter is required",
		})
		return
	}

	userID := c.Query("user_id")

	err := h.oauthManager.RevokeToken(userID, providerType)
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

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action":   "revoke_oauth_token",
		"provider": providerType,
	}).Info("OAuth token revoked")

	c.JSON(http.StatusOK, OAuthMessageResponse{
		Success: true,
		Message: "Token revoked successfully",
	})
}

// ListOAuthTokens lists all OAuth tokens for a user
// GET /api/v1/oauth/tokens?user_id=xxx (deprecated parameter, now lists all if not provided)
func (h *Handler) ListOAuthTokens(c *gin.Context) {
	// For backward compatibility, check if user_id is provided
	userID := c.Query("user_id")
	if userID != "" {
		// Use old API: List providers for specific user via oauthManager
		providers, err := h.oauthManager.ListProviders(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		tokens := make([]TokenInfo, 0)

		for _, provider := range providers {
			token, err := h.oauthManager.GetToken(c.Request.Context(), userID, provider)
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
		return
	}

	// New API: List all OAuth providers from config
	providers, err := h.config.ListOAuthProviders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{
			Success: false,
			Error:   "Failed to list providers: " + err.Error(),
		})
		return
	}

	data := make([]TokenInfo, 0, len(providers))
	for _, provider := range providers {
		if provider.OAuthDetail == nil {
			continue
		}

		tokenInfo := TokenInfo{
			Provider:  provider.Name,
			ExpiresAt: provider.OAuthDetail.ExpiresAt,
		}

		// Check if token is valid
		if provider.OAuthDetail.ExpiresAt != "" {
			expiresAt, err := time.Parse(time.RFC3339, provider.OAuthDetail.ExpiresAt)
			if err == nil {
				tokenInfo.Valid = time.Now().Before(expiresAt)
			}
		}

		data = append(data, tokenInfo)
	}

	c.JSON(http.StatusOK, OAuthTokensResponse{
		Success: true,
		Data:    data,
	})
}

// =============================================
// Session Management Handlers
// =============================================

// GetOAuthSessionStatus returns the status of an OAuth session
// GET /api/v1/oauth/status?session_id=xxx
func (h *Handler) GetOAuthSessionStatus(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "session_id parameter is required",
		})
		return
	}

	// Use oauth.Manager's session storage (the source of truth for OAuth session status)
	session, err := h.oauthManager.GetSession(sessionID)
	if err != nil {
		// Session not found or expired
		log.Printf("[OAuth] Session status request: sessionID=%s, not found (err=%v)", sessionID, err)
		resp := OAuthSessionStatusResponse{
			Success: true,
		}
		resp.Data.SessionID = sessionID
		resp.Data.Status = "not_found"
		c.JSON(http.StatusOK, resp)
		return
	}

	// Check expiration
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		log.Printf("[OAuth] Session status request: sessionID=%s, expired", sessionID)
		resp := OAuthSessionStatusResponse{
			Success: true,
		}
		resp.Data.SessionID = sessionID
		resp.Data.Status = "expired"
		c.JSON(http.StatusOK, resp)
		return
	}

	log.Printf("[OAuth] Session status request: sessionID=%s, status=%s, providerUUID=%s", sessionID, session.Status, session.ProviderUUID)

	resp := OAuthSessionStatusResponse{
		Success: true,
	}
	resp.Data.SessionID = session.SessionID
	resp.Data.Status = string(session.Status)
	resp.Data.ProviderUUID = session.ProviderUUID
	resp.Data.Error = session.Error

	c.JSON(http.StatusOK, resp)
}

// CancelOAuthSession cancels an in-progress OAuth session
// POST /api/v1/oauth/cancel
func (h *Handler) CancelOAuthSession(c *gin.Context) {
	var req OAuthCancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return
	}

	// Use oauth.Manager's session storage to mark session as failed
	// Note: We use "failed" status for cancelled sessions since there's no "cancelled" status in oauth.Manager
	if err := h.oauthManager.UpdateSessionStatus(req.SessionID, oauth2.SessionStatusFailed, "", "User cancelled"); err != nil {
		log.Printf("[OAuth] Failed to cancel session %s: %v", req.SessionID, err)
		// Don't return error - session might not exist, which is fine
	} else {
		log.Printf("[OAuth] Cancelled session %s", req.SessionID)
	}

	// Stop the callback server if it exists for this session
	if h.callbackServerManager != nil {
		h.callbackServerManager.StopDynamicCallbackServer(req.SessionID)
	}

	c.JSON(http.StatusOK, OAuthMessageResponse{
		Success: true,
		Message: "OAuth session cancelled",
	})
}

// =============================================
// Callback Handler
// =============================================

// OAuthCallback handles OAuth callback from providers
// GET /oauth/callback and /callback
func (h *Handler) OAuthCallback(c *gin.Context) {
	// Get state parameter first for error handling
	state := c.Query("state")

	// Retrieve state data BEFORE calling HandleCallback, because HandleCallback
	// deletes the state data after validation. We need sessionID for error handling.
	var sessionID string
	if stateData, err := h.oauthManager.GetStateData(state); err == nil {
		sessionID = stateData.SessionID
	}

	// Delegate to the oauth handler's callback, now returns name in token
	token, err := h.oauthManager.HandleCallback(c.Request.Context(), c.Request)
	if err != nil {
		// Fail the session if we have a sessionID from the state data
		if sessionID != "" {
			log.Printf("[OAuth] Callback failed, failing session %s: %v", sessionID, err)
			_ = h.oauthManager.UpdateSessionStatus(sessionID, oauth2.SessionStatusFailed, "", err.Error())
		}
		c.HTML(http.StatusBadRequest, "oauth_error.html", gin.H{
			"error": err.Error(),
		})
		return
	}

	log.Printf("[OAuth] Callback successful, token.SessionID=%s, state sessionID=%s", token.SessionID, sessionID)

	// Use createProviderFromToken to create the provider
	// Pass session ID to retrieve proxy URL from the session
	providerUUID, err := h.createProviderFromToken(token, token.Provider, "", token.SessionID)
	if err != nil {
		log.Printf("[OAuth] Failed to create provider for token.SessionID %s: %v", token.SessionID, err)
		_ = h.oauthManager.UpdateSessionStatus(token.SessionID, oauth2.SessionStatusFailed, "", err.Error())
		c.HTML(http.StatusInternalServerError, "oauth_error.html", gin.H{
			"error": fmt.Sprintf("Failed to create provider: %v", err),
		})
		return
	}

	log.Printf("[OAuth] Provider created successfully, UUID=%s, token.SessionID=%s", providerUUID, token.SessionID)

	// Update session status to success if session ID exists
	if token.SessionID != "" {
		log.Printf("[OAuth] Completing session %s with provider UUID %s", token.SessionID, providerUUID)
		_ = h.oauthManager.UpdateSessionStatus(token.SessionID, oauth2.SessionStatusSuccess, providerUUID, "")
	} else {
		log.Printf("[OAuth] WARNING: token.SessionID is empty, cannot complete session!")
	}

	// Stop the dynamic callback server if this session had one
	// This is done after successful OAuth completion
	if h.callbackServerManager != nil && token.SessionID != "" {
		go func() {
			time.Sleep(1 * time.Second) // Give time for the response to be sent
			h.callbackServerManager.StopDynamicCallbackServer(token.SessionID)
		}()
	}

	// Return success HTML page to inform the user
	c.HTML(http.StatusOK, "oauth_success.html", gin.H{
		"provider":      string(token.Provider),
		"provider_name": "",                             // Will be shown with UUID in the page
		"access_token":  token.AccessToken[:20] + "...", // Partially show token
		"token_type":    token.TokenType,
	})
}

// =============================================
// Provider Creation Helper
// =============================================

// createProviderFromToken creates a provider from OAuth token
func (h *Handler) createProviderFromToken(token *oauth2.Token, providerType oauth2.ProviderType, customName, sessionID string) (string, error) {
	// Get custom name from token (stored in state during authorize)
	if customName == "" {
		customName = token.Name
	}

	// Retrieve proxy URL from session if available
	var proxyURL string
	if sessionID != "" {
		session, err := h.oauthManager.GetSession(sessionID)
		if err == nil && session != nil {
			proxyURL = session.ProxyURL
			if proxyURL != "" {
				log.Printf("[OAuth] Using proxy URL from session: %s", proxyURL)
			}
		}
	}

	// Generate provider name using smart naming strategy
	// Priority: customName > email username > display name > timestamp
	providerName := generateProviderName(providerType, token, customName)

	// Generate UUID for the provider
	providerUUID, err := uuid.NewUUID()
	if err != nil {
		return "", fmt.Errorf("failed to generate provider UUID: %w", err)
	}

	// Determine API base and style based on provider type
	var apiBase string
	var apiStyle protocol.APIStyle

	// If token contains ResourceURL from OAuth response, use it
	if token.ResourceURL != "" && providerType == oauth2.ProviderQwenCode {
		apiBase = normalizeAPIBase(token.ResourceURL, "/v1")
		apiStyle = protocol.APIStyleOpenAI
	} else {
		switch providerType {
		case oauth2.ProviderClaudeCode:
			apiBase = "https://api.anthropic.com"
			apiStyle = protocol.APIStyleAnthropic
		case oauth2.ProviderQwenCode:
			apiBase = "https://portal.qwen.ai/v1"
			apiStyle = protocol.APIStyleOpenAI
		case oauth2.ProviderGoogle:
			apiBase = "https://generativelanguage.googleapis.com"
			apiStyle = protocol.APIStyleOpenAI
		case oauth2.ProviderAntigravity:
			apiBase = "https://cloudcode-pa.googleapis.com"
			apiStyle = protocol.APIStyleGoogle
		case oauth2.ProviderOpenAI:
			apiBase = "https://api.openai.com/v1"
			apiStyle = protocol.APIStyleOpenAI
		case oauth2.ProviderCodex:
			apiBase = protocol.CodexAPIBase
			apiStyle = protocol.APIStyleOpenAI
		default:
			apiBase = "mock"
			apiStyle = protocol.APIStyleOpenAI
		}
	}

	// Build expires_at string
	var expiresAt string
	if !token.Expiry.IsZero() {
		expiresAt = token.Expiry.Format(time.RFC3339)
	}

	provider := &typ.Provider{
		UUID:     providerUUID.String(),
		Name:     providerName,
		APIBase:  apiBase,
		APIStyle: apiStyle,
		Enabled:  true,
		ProxyURL: proxyURL,
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken:  token.AccessToken,
			ProviderType: string(providerType),
			UserID:       uuid.New().String(),
			RefreshToken: token.RefreshToken,
			ExpiresAt:    expiresAt,
			ExtraFields:  make(map[string]interface{}),
		},
	}

	// Store account_id from token metadata for ChatGPT API
	if token.Metadata != nil {
		for k, v := range token.Metadata {
			provider.OAuthDetail.ExtraFields[k] = v
		}
	}

	// Save provider to config
	if err := h.config.AddProvider(provider); err != nil {
		return "", fmt.Errorf("failed to save provider: %w", err)
	}

	// Fetch models for the newly created OAuth provider
	log.Printf("[OAuth] Fetching models for OAuth provider %s (%s)", providerName, providerUUID.String())
	if err := h.config.FetchAndSaveProviderModels(providerUUID.String()); err != nil {
		log.Printf("[OAuth] Warning: Failed to fetch models for OAuth provider %s: %v", providerName, err)
	} else {
		modelManager := h.config.GetModelManager()
		models := modelManager.GetModels(providerUUID.String())
		log.Printf("[OAuth] Successfully fetched %d models for OAuth provider %s", len(models), providerName)
	}

	// Log the successful provider creation
	logrus.WithFields(logrus.Fields{
		"action":        "oauth_provider_created",
		"provider_name": providerName,
		"provider_type": string(token.Provider),
		"uuid":          providerUUID.String(),
	}).Info("OAuth provider created successfully")

	return providerUUID.String(), nil
}

// generateProviderName generates a human-readable provider name from token metadata
// Priority: customName > email > display name > timestamp
// Note: Account ID is NOT used for naming (sensitive information)
func generateProviderName(providerType oauth2.ProviderType, token *oauth2.Token, customName string) string {
	// Priority 1: Custom name from user
	if customName != "" {
		return customName
	}

	// Extract metadata
	var email, displayName string
	if token.Metadata != nil {
		email, _ = token.Metadata["email"].(string)
		displayName, _ = token.Metadata["name"].(string)
	}

	// Priority 2: Use full email
	if email != "" {
		return email
	}

	// Priority 3: Use display name (spaces to hyphens)
	if displayName != "" {
		return strings.ReplaceAll(displayName, " ", "-")
	}

	// Priority 4: Timestamp-based fallback (format: YYYYMMDD-HHMM)
	timestamp := time.Now().Format("20060102-1504")
	return fmt.Sprintf("%s-%s", providerType, timestamp)
}

// generateRandomSuffix generates a random alphanumeric suffix of specified length
func generateRandomSuffix(length int) string {
	bytes := make([]byte, length/2+1)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

// normalizeAPIBase normalizes an API base URL
func normalizeAPIBase(baseURL, pathSuffix string) string {
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	baseURL = strings.TrimSuffix(baseURL, "/")
	pathSuffix = strings.TrimPrefix(pathSuffix, "/")

	if strings.HasSuffix(baseURL, pathSuffix) {
		return baseURL
	}

	if strings.Contains(baseURL, "/v") {
		parts := strings.Split(baseURL, "/")
		for _, part := range parts {
			if strings.HasPrefix(part, "v") && len(part) > 1 && part[1] >= '0' && part[1] <= '9' {
				return baseURL
			}
		}
	}

	return fmt.Sprintf("%s/%s", baseURL, pathSuffix)
}
