package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// SessionStatus represents the status of an OAuth session
type SessionStatus string

const (
	SessionStatusPending SessionStatus = "pending" // Authorization initiated
	SessionStatusSuccess SessionStatus = "success" // Provider created successfully
	SessionStatusFailed  SessionStatus = "failed"  // Authorization failed
)

// SessionState holds information about an OAuth session
type SessionState struct {
	SessionID    string        `json:"session_id"`
	Status       SessionStatus `json:"status"`
	Provider     ProviderType  `json:"provider"`
	UserID       string        `json:"user_id"`
	CreatedAt    time.Time     `json:"created_at"`
	ExpiresAt    time.Time     `json:"expires_at"`
	ProviderUUID string        `json:"provider_uuid,omitempty"` // Set when success
	Error        string        `json:"error,omitempty"`         // Set when failed
}

// Manager handles OAuth flows
type Manager struct {
	config   *Config
	registry *Registry

	// State management for OAuth flow
	states map[string]*StateData
	mu     sync.RWMutex

	// Session management for OAuth authorization tracking
	sessions   map[string]*SessionState
	sessionsMu sync.RWMutex
}

// StateData holds information about an OAuth state
type StateData struct {
	State         string
	UserID        string
	Provider      ProviderType
	ExpiresAt     time.Time
	Timestamp     int64  // Unix timestamp when state was created
	ExpiresAtUnix int64  // Unix timestamp when state expires
	RedirectTo    string // Optional redirect URL after successful auth
	Name          string // Optional custom provider name
	CodeVerifier  string // PKCE code verifier (for PKCE flow)
	RedirectURI   string // Actual redirect_uri used in auth request (must match in token request)
	SessionID     string // Session ID for status tracking
}

// NewManager creates a new OAuth manager
func NewManager(config *Config, registry *Registry) *Manager {
	if config == nil {
		config = DefaultConfig()
	}
	if registry == nil {
		registry = DefaultRegistry()
	}

	m := &Manager{
		config:   config,
		registry: registry,
		states:   make(map[string]*StateData),
		sessions: make(map[string]*SessionState),
	}

	// Start cleanup goroutine
	go m.cleanupExpiredStates()
	go m.cleanupExpiredSessions()

	return m
}

// generateState generates a secure random state parameter using UUID
func (m *Manager) generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// generateCodeVerifier generates a PKCE code verifier (43-128 characters)
// Uses cryptographically secure random bytes encoded as base64url
func (m *Manager) generateCodeVerifier() (string, error) {
	// Generate 32 random bytes (256 bits), which when base64url-encoded
	// gives us 43 characters (minimum required by PKCE spec)
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate code verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge creates a PKCE code challenge from the verifier
// Uses SHA256 and base64url encoding (S256 method)
func (m *Manager) generateCodeChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// generateStateKey generates a key for storing state data
func (m *Manager) stateKey(state string) string {
	return state
}

// saveState saves state data with expiration
func (m *Manager) saveState(data *StateData) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	data.Timestamp = now.Unix()
	data.ExpiresAt = now.Add(m.config.StateExpiry)
	data.ExpiresAtUnix = data.ExpiresAt.Unix()
	m.states[m.stateKey(data.State)] = data
	return nil
}

// getState retrieves and validates state data
func (m *Manager) getState(state string) (*StateData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.states[m.stateKey(state)]
	if !ok {
		return nil, ErrInvalidState
	}

	if time.Now().After(data.ExpiresAt) {
		return nil, ErrStateExpired
	}

	return data, nil
}

// deleteState removes state data
func (m *Manager) deleteState(state string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, m.stateKey(state))
}

// cleanupExpiredStates removes expired states periodically
func (m *Manager) cleanupExpiredStates() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for key, data := range m.states {
			if now.After(data.ExpiresAt) {
				delete(m.states, key)
			}
		}
		m.mu.Unlock()
	}
}

// GetAuthURL generates the OAuth authorization URL for a provider
func (m *Manager) GetAuthURL(userID string, providerType ProviderType, redirectTo string, name string, sessionID string) (string, string, error) {
	config, ok := m.registry.Get(providerType)
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrInvalidProvider, providerType)
	}

	if config.ClientID == "" {
		return "", "", fmt.Errorf("%w: %s", ErrProviderNotConfigured, providerType)
	}

	// Generate state
	state, err := m.generateState()
	if err != nil {
		return "", "", err
	}

	// Generate PKCE code verifier if provider uses PKCE
	var codeVerifier string
	if config.OAuthMethod == OAuthMethodPKCE {
		codeVerifier, err = m.generateCodeVerifier()
		if err != nil {
			return "", "", fmt.Errorf("failed to generate code verifier: %w", err)
		}
	}

	// Build authorization URL
	authURL, redirectURI, err := m.buildAuthURL(config, state, codeVerifier)
	if err != nil {
		m.deleteState(state)
		return "", "", err
	}

	// Update state with actual redirect_uri used
	if err := m.saveState(&StateData{
		State:        state,
		UserID:       userID,
		Provider:     providerType,
		RedirectTo:   redirectTo,
		Name:         name,
		CodeVerifier: codeVerifier,
		RedirectURI:  redirectURI,
		SessionID:    sessionID,
	}); err != nil {
		return "", "", err
	}

	return authURL, state, nil
}

// buildAuthURL builds the authorization URL with all required parameters
// Returns the auth URL and the actual redirect_uri used
func (m *Manager) buildAuthURL(config *ProviderConfig, state string, codeVerifier string) (string, string, error) {
	u, err := url.Parse(config.AuthURL)
	if err != nil {
		return "", "", err
	}

	//redirectURL := config.RedirectURL
	//if redirectURL == "" {
	redirectURL := fmt.Sprintf("%s/callback", m.config.BaseURL)
	//}

	query := u.Query()
	query.Set("client_id", config.ClientID)
	query.Set("redirect_uri", redirectURL)
	query.Set("response_type", "code")
	query.Set("state", state)
	if len(config.Scopes) > 0 {
		query.Set("scope", strings.Join(config.Scopes, " "))
	}

	// Add PKCE parameters if provider uses PKCE
	if config.OAuthMethod == OAuthMethodPKCE && codeVerifier != "" {
		query.Set("code_challenge", m.generateCodeChallenge(codeVerifier))
		query.Set("code_challenge_method", "S256")
	}

	// Call provider's auth hook if present
	if config.Hook != nil {
		// Convert query to map for hook
		params := make(map[string]string)
		for k, v := range query {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
		if err := config.Hook.BeforeAuth(params); err != nil {
			return "", "", err
		}
		// Convert back to query
		for k, v := range params {
			query.Set(k, v)
		}
	}

	u.RawQuery = query.Encode()

	return u.String(), redirectURL, nil
}

// HandleCallback handles the OAuth callback request
func (m *Manager) HandleCallback(ctx context.Context, r *http.Request) (*Token, error) {
	// Parse callback parameters
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		return nil, fmt.Errorf("oauth error: %s", errorParam)
	}

	if code == "" {
		return nil, ErrInvalidCode
	}

	// Validate state
	stateData, err := m.getState(state)
	if err != nil {
		return nil, err
	}
	defer m.deleteState(state)

	// Get provider config
	config, ok := m.registry.Get(stateData.Provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, stateData.Provider)
	}

	// Exchange code for token
	// For PKCE providers, include the code verifier; for standard OAuth, omit it
	var codeVerifier string
	if config.OAuthMethod == OAuthMethodPKCE {
		codeVerifier = stateData.CodeVerifier
	}

	token, err := m.exchangeCodeForToken(ctx, config, state, code, codeVerifier, stateData.RedirectURI)
	if err != nil {
		return nil, err
	}
	token.Provider = stateData.Provider
	token.RedirectTo = stateData.RedirectTo
	token.Name = stateData.Name
	token.SessionID = stateData.SessionID

	// Save token
	if err := m.config.TokenStorage.SaveToken(stateData.UserID, stateData.Provider, token); err != nil {
		return nil, err
	}

	return token, nil
}

// exchangeCodeForToken exchanges the authorization code for an access token
func (m *Manager) exchangeCodeForToken(ctx context.Context, config *ProviderConfig, state string, code string, codeVerifier string, redirectURI string) (*Token, error) {
	useJSON := config.TokenRequestFormat == TokenRequestFormatJSON

	var reqBody io.Reader
	var contentType string

	// Build common parameters
	params := map[string]string{
		"grant_type":   "authorization_code",
		"client_id":    config.ClientID,
		"redirect_uri": redirectURI,
		"code":         code,
		"state":        state,
	}

	// Add client_secret if possible
	if config.ClientSecret != "" {
		params["client_secret"] = config.ClientSecret
	}

	// Add code_verifier for PKCE
	if config.OAuthMethod == OAuthMethodPKCE && codeVerifier != "" {
		params["code_verifier"] = codeVerifier
	}

	reqBody, contentType, err := buildRequestBody(params, useJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to build request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.TokenURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	// Call provider's token hook if present
	if config.Hook != nil {
		if err := config.Hook.BeforeToken(params, req.Header); err != nil {
			return nil, err
		}
		// Rebuild body in case hook modified params
		reqBody, contentType, err = buildRequestBody(params, useJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to rebuild request body: %w", err)
		}
		req.Body = io.NopCloser(reqBody)
		req.Header.Set("Content-Type", contentType)
	}

	// Debug: print request details
	m.debugRequest(req, useJSON)

	// Send request with optional proxy support
	// Proxy is read from HTTP_PROXY/HTTPS_PROXY environment variables
	client := &http.Client{Timeout: 60 * time.Second}

	// Check if proxy is configured
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		// Use proxy from environment
		client.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}
		fmt.Printf("[OAuth] Using proxy from environment\n")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client error: %w: %v", ErrTokenExchangeFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: status %d, body: %d", resp.StatusCode, len(string(body)))
	}

	// Parse response directly into Token
	token := &Token{}
	if err := json.NewDecoder(resp.Body).Decode(token); err != nil {
		return nil, fmt.Errorf("data decode: %w: %v", ErrTokenExchangeFailed, err)
	}

	// Convert ExpiresIn to Expiry
	if token.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	// Call provider's after-token hook to fetch additional metadata
	if config.Hook != nil && token.AccessToken != "" {
		metadata, err := config.Hook.AfterToken(ctx, token.AccessToken, client)
		if err != nil {
			fmt.Printf("[OAuth] AfterToken hook failed: %v\n", err)
			// Continue even if AfterToken fails, as we already have the token
		}
		if metadata != nil {
			token.Metadata = metadata
		}
	}

	return token, nil
}

// GetToken retrieves a token for a user and provider, refreshing if necessary
func (m *Manager) GetToken(ctx context.Context, userID string, providerType ProviderType) (*Token, error) {
	token, err := m.config.TokenStorage.GetToken(userID, providerType)
	if err != nil {
		return nil, err
	}

	// Check if token needs refresh
	if token.ExpiredIn(m.config.TokenExpiryBuffer) {
		if token.RefreshToken != "" {
			refreshed, err := m.refreshToken(ctx, providerType, token.RefreshToken)
			if err == nil {
				refreshed.Provider = providerType
				if err := m.config.TokenStorage.SaveToken(userID, providerType, refreshed); err == nil {
					return refreshed, nil
				}
			}
		}
		// If refresh failed, return the existing token if still valid
		if token.Valid() {
			return token, nil
		}
		return nil, fmt.Errorf("token expired and refresh failed")
	}

	return token, nil
}

// refreshToken refreshes an access token using a refresh token
func (m *Manager) refreshToken(ctx context.Context, providerType ProviderType, refreshToken string) (*Token, error) {
	config, ok := m.registry.Get(providerType)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, providerType)
	}

	useJSON := config.TokenRequestFormat == TokenRequestFormatJSON

	// Build common parameters
	params := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     config.ClientID,
		"client_secret": config.ClientSecret,
	}

	reqBody, contentType, err := buildRequestBody(params, useJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to build request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.TokenURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	// Call provider's token hook if present
	if config.Hook != nil {
		if err := config.Hook.BeforeToken(params, req.Header); err != nil {
			return nil, err
		}
		// Rebuild body in case hook modified params
		reqBody, contentType, err = buildRequestBody(params, useJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to rebuild request body: %w", err)
		}
		req.Body = io.NopCloser(reqBody)
		req.Header.Set("Content-Type", contentType)
	}

	// Debug: print request details
	m.debugRequest(req, useJSON)

	// Send request with optional proxy support
	// Proxy is read from HTTP_PROXY/HTTPS_PROXY environment variables
	client := &http.Client{Timeout: 30 * time.Second}

	// Check if proxy is configured
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		// Use proxy from environment
		client.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}
		fmt.Printf("[OAuth] Using proxy from environment\n")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w: %v", ErrTokenExchangeFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh token failed: status %d, body: %d", resp.StatusCode, len(string(body)))
	}

	// Parse response directly into Token
	token := &Token{}
	if err := json.NewDecoder(resp.Body).Decode(token); err != nil {
		return nil, fmt.Errorf("decode error: %w: %v", ErrTokenExchangeFailed, err)
	}

	// Convert ExpiresIn to Expiry
	if token.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return token, nil
}

// RefreshToken refreshes an access token using a refresh token
// This is a public method that can be called from HTTP handlers
func (m *Manager) RefreshToken(ctx context.Context, userID string, providerType ProviderType, refreshToken string) (*Token, error) {
	// Refresh the token
	token, err := m.refreshToken(ctx, providerType, refreshToken)
	if err != nil {
		return nil, err
	}

	token.Provider = providerType

	// Save the refreshed token
	if err := m.config.TokenStorage.SaveToken(userID, providerType, token); err != nil {
		return nil, fmt.Errorf("failed to save refreshed token: %w", err)
	}

	return token, nil
}

// RevokeToken removes a token for a user and provider
func (m *Manager) RevokeToken(userID string, providerType ProviderType) error {
	return m.config.TokenStorage.DeleteToken(userID, providerType)
}

// ListProviders returns all providers that have valid tokens for the user
func (m *Manager) ListProviders(userID string) ([]ProviderType, error) {
	return m.config.TokenStorage.ListProviders(userID)
}

// GetRegistry returns the provider registry
func (m *Manager) GetRegistry() *Registry {
	return m.registry
}

// GetConfig returns the OAuth configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// InitiateDeviceCodeFlow initiates the Device Code flow and returns device code data
// RFC 8628: OAuth 2.0 Device Authorization Grant
func (m *Manager) InitiateDeviceCodeFlow(ctx context.Context, userID string, providerType ProviderType, redirectTo string, name string) (*DeviceCodeData, error) {
	config, ok := m.registry.Get(providerType)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, providerType)
	}

	if config.ClientID == "" {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotConfigured, providerType)
	}

	if config.DeviceCodeURL == "" {
		return nil, fmt.Errorf("provider %s does not support device code flow", providerType)
	}

	// Generate PKCE code verifier if provider uses Device Code PKCE
	var codeVerifier string
	var codeChallenge string
	if config.OAuthMethod == OAuthMethodDeviceCodePKCE {
		var err error
		codeVerifier, err = m.generateCodeVerifier()
		if err != nil {
			return nil, fmt.Errorf("failed to generate code verifier: %w", err)
		}
		codeChallenge = m.generateCodeChallenge(codeVerifier)
	}

	// Build device authorization request
	useJSON := config.TokenRequestFormat == TokenRequestFormatJSON

	// Build common parameters
	params := map[string]string{
		"client_id": config.ClientID,
		"scope":     strings.Join(config.Scopes, " "),
	}
	// Add PKCE parameters for Device Code PKCE flow
	if config.OAuthMethod == OAuthMethodDeviceCodePKCE {
		params["code_challenge"] = codeChallenge
		params["code_challenge_method"] = "S256"
	}

	reqBody, contentType, err := buildRequestBody(params, useJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to build request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.DeviceCodeURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	// Call provider's token hook if present
	if config.Hook != nil {
		if err := config.Hook.BeforeToken(params, req.Header); err != nil {
			return nil, err
		}
		// Rebuild body in case hook modified params
		reqBody, contentType, err = buildRequestBody(params, useJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to rebuild request body: %w", err)
		}
		req.Body = io.NopCloser(reqBody)
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed: status %d, body: %d", resp.StatusCode, len(string(body)))
	}

	// Parse device code response
	var deviceResp DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return nil, fmt.Errorf("failed to decode device code response: %w", err)
	}

	// Create device code data
	now := time.Now()
	data := &DeviceCodeData{
		DeviceCodeResponse: &deviceResp,
		Provider:           providerType,
		UserID:             userID,
		RedirectTo:         redirectTo,
		Name:               name,
		InitiatedAt:        now,
		ExpiresAt:          now.Add(time.Duration(deviceResp.ExpiresIn) * time.Second),
		CodeVerifier:       codeVerifier, // Store PKCE verifier for token polling
	}

	return data, nil
}

// PollForToken polls the token endpoint until the user completes authentication
// or the device code expires
// Polling timeout is limited to 5 minutes (user needs time to complete auth)
func (m *Manager) PollForToken(ctx context.Context, data *DeviceCodeData, callback func(*Token)) (*Token, error) {
	config, ok := m.registry.Get(data.Provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, data.Provider)
	}

	// Default interval is 5 seconds if not specified
	interval := time.Duration(data.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Create a timeout context with 2 minute limit for polling
	// User needs time to: open link, enter code, and complete authorization
	const pollTimeout = 2 * time.Minute
	timeoutCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	fmt.Printf("[OAuth] Device code polling started for %s, timeout: %v\n", data.Provider, pollTimeout)

	for {
		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("authentication timed out after %v", pollTimeout)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			fmt.Printf("[OAuth] Polling token endpoint for %s...\n", data.Provider)
			token, err := m.pollTokenRequest(ctx, config, data.DeviceCode, data.CodeVerifier)
			if err != nil {
				// Check if error is a transient error that we should retry
				if isTransientDeviceCodeError(err) {
					fmt.Printf("[OAuth] Authorization pending for %s, continuing poll...\n", data.Provider)
					time.Sleep(interval)
					continue
				}
				fmt.Printf("[OAuth] Polling error for %s: %v\n", data.Provider, err)
				return nil, err
			}

			fmt.Printf("[OAuth] Successfully obtained token for %s\n", data.Provider)
			// Successfully got token
			token.Provider = data.Provider
			token.RedirectTo = data.RedirectTo
			token.Name = data.Name

			// Save token
			if err := m.config.TokenStorage.SaveToken(data.UserID, data.Provider, token); err != nil {
				return nil, fmt.Errorf("failed to save token: %w", err)
			}

			// Call callback if provided
			if callback != nil {
				callback(token)
			}

			return token, nil
		}
	}
}

// pollTokenRequest makes a single token polling request
func (m *Manager) pollTokenRequest(ctx context.Context, config *ProviderConfig, deviceCode string, codeVerifier string) (*Token, error) {
	useJSON := config.TokenRequestFormat == TokenRequestFormatJSON

	// Build common parameters
	params := map[string]string{
		"grant_type":  config.GrantType,
		"client_id":   config.ClientID,
		"device_code": deviceCode,
	}
	if config.ClientSecret != "" {
		params["client_secret"] = config.ClientSecret
	}
	// Add PKCE code_verifier for Device Code PKCE flow
	if config.OAuthMethod == OAuthMethodDeviceCodePKCE && codeVerifier != "" {
		params["code_verifier"] = codeVerifier
	}

	reqBody, contentType, err := buildRequestBody(params, useJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to build request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.TokenURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	// Call provider's token hook if present
	if config.Hook != nil {
		if err := config.Hook.BeforeToken(params, req.Header); err != nil {
			return nil, err
		}
		// Rebuild body in case hook modified params
		reqBody, contentType, err = buildRequestBody(params, useJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to rebuild request body: %w", err)
		}
		req.Body = io.NopCloser(reqBody)
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token poll request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for authorization pending (should retry)
	if resp.StatusCode == http.StatusBadRequest {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil {
			switch errResp.Error {
			case "authorization_pending", "slow_down":
				return nil, &DeviceCodePendingError{Message: errResp.Error}
			case "access_denied", "expired_token":
				return nil, fmt.Errorf("device code error: %s", errResp.Error)
			}
			// Unknown error in 400 response
			return nil, fmt.Errorf("device code error (400): %s - body: %s", errResp.Error, string(body))
		}
		// 400 but no valid error response
		return nil, fmt.Errorf("device code error (400): body: %s", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token poll failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse token response directly into Token
	token := &Token{}
	if err := json.Unmarshal(body, token); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	// Convert ExpiresIn to Expiry
	if token.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return token, nil
}

// DeviceCodePendingError represents a pending device code authorization
type DeviceCodePendingError struct {
	Message string
}

func (e *DeviceCodePendingError) Error() string {
	return e.Message
}

// isTransientDeviceCodeError checks if an error is a transient device code error
func isTransientDeviceCodeError(err error) bool {
	if _, ok := err.(*DeviceCodePendingError); ok {
		return true
	}
	return false
}

// debugRequest prints HTTP request details for debugging
func (m *Manager) debugRequest(req *http.Request, isJSON bool) {
	logrus.Debug("=== OAuth Debug: HTTP Request ===")
	logrus.Debugf("Method: %s", req.Method)
	logrus.Debugf("URL: %s", req.URL.String())
	logrus.Debug("Headers:")
	for key, values := range req.Header {
		for _, value := range values {
			// Mask sensitive headers
			if strings.EqualFold(key, "Authorization") {
				value = "***REDACTED***"
			}
			logrus.Debugf("  %s: %s", key, value)
		}
	}

	if req.Body != nil && req.Body != http.NoBody {
		logrus.Debug("Body:")
		// Read body to print it (but we need to restore it for the actual request)
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil {
			// Try to format JSON for readability
			if isJSON {
				var formatted any
				if json.Unmarshal(bodyBytes, &formatted) == nil {
					if pretty, err := json.MarshalIndent(formatted, "", "  "); err == nil {
						logrus.Debugf("%s", string(pretty))
					} else {
						logrus.Debugf("%s", string(bodyBytes))
					}
				} else {
					logrus.Debugf("%s", string(bodyBytes))
				}
			} else {
				logrus.Debugf("%s", string(bodyBytes))
			}
			// Restore body for actual request
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}
	logrus.Debug("================================")
}

// =============================================
// Session Management for OAuth Status Tracking
// =============================================

// generateSessionID generates a unique session ID
func (m *Manager) generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateSession creates a new OAuth session with pending status
func (m *Manager) CreateSession(userID string, provider ProviderType) (*SessionState, error) {
	sessionID, err := m.generateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &SessionState{
		SessionID: sessionID,
		Status:    SessionStatusPending,
		Provider:  provider,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute), // Session expires after 10 minutes
	}

	m.sessionsMu.Lock()
	m.sessions[sessionID] = session
	m.sessionsMu.Unlock()

	logrus.WithFields(logrus.Fields{
		"session_id": sessionID,
		"provider":   provider,
		"user_id":    userID,
		"status":     SessionStatusPending,
	}).Info("[OAuth] Session created")

	return session, nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(sessionID string) (*SessionState, error) {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

// UpdateSessionStatus updates the status of a session
func (m *Manager) UpdateSessionStatus(sessionID string, status SessionStatus, providerUUID string, errMsg string) error {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		logrus.WithField("session_id", sessionID).Warn("[OAuth] Failed to update session: not found")
		return fmt.Errorf("session not found")
	}

	session.Status = status
	if providerUUID != "" {
		session.ProviderUUID = providerUUID
	}
	if errMsg != "" {
		session.Error = errMsg
	}

	// Log session status change
	logEntry := logrus.WithFields(logrus.Fields{
		"session_id":    sessionID,
		"provider":      session.Provider,
		"new_status":    status,
		"provider_uuid": providerUUID,
	})

	if status == SessionStatusSuccess {
		logEntry.Info("[OAuth] Session completed successfully")
	} else if status == SessionStatusFailed {
		logEntry.WithField("error", errMsg).Error("[OAuth] Session failed")
	} else {
		logEntry.Debug("[OAuth] Session status updated")
	}

	return nil
}

// cleanupExpiredSessions removes expired sessions periodically
func (m *Manager) cleanupExpiredSessions() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.sessionsMu.Lock()
		now := time.Now()
		for key, session := range m.sessions {
			if now.After(session.ExpiresAt) {
				delete(m.sessions, key)
			}
		}
		m.sessionsMu.Unlock()
	}
}
