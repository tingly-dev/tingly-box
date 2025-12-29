package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager handles OAuth flows
type Manager struct {
	config   *Config
	registry *Registry

	// State management for OAuth flow
	states map[string]*StateData
	mu     sync.RWMutex
}

// StateData holds information about an OAuth state
type StateData struct {
	State        string
	UserID       string
	Provider     ProviderType
	ExpiresAt    time.Time
	RedirectTo   string // Optional redirect URL after successful auth
	Name         string // Optional custom provider name
	CodeVerifier string // PKCE code verifier (for PKCE flow)
	RedirectURI  string // Actual redirect_uri used in auth request (must match in token request)
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
	}

	// Start cleanup goroutine
	go m.cleanupExpiredStates()

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

	data.ExpiresAt = time.Now().Add(m.config.StateExpiry)
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
func (m *Manager) GetAuthURL(ctx context.Context, userID string, providerType ProviderType, redirectTo string, name string) (string, string, error) {
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

	redirectURL := config.RedirectURL
	if redirectURL == "" {
		redirectURL = fmt.Sprintf("%s/callback", m.config.BaseURL)
	}

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

	// Add provider-specific extra parameters
	for key, value := range config.AuthExtraParams {
		query.Set(key, value)
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

	token, err := m.exchangeCodeForToken(ctx, config, code, codeVerifier, stateData.RedirectURI)
	if err != nil {
		return nil, err
	}
	token.Provider = stateData.Provider
	token.RedirectTo = stateData.RedirectTo
	token.Name = stateData.Name

	// Save token
	if err := m.config.TokenStorage.SaveToken(stateData.UserID, stateData.Provider, token); err != nil {
		return nil, err
	}

	return token, nil
}

// exchangeCodeForToken exchanges the authorization code for an access token
func (m *Manager) exchangeCodeForToken(ctx context.Context, config *ProviderConfig, code string, codeVerifier string, redirectURI string) (*Token, error) {
	useJSON := config.TokenRequestFormat == TokenRequestFormatJSON

	var reqBody io.Reader
	var contentType string

	if useJSON {
		// Build JSON request body
		jsonData := map[string]any{
			"grant_type":   "authorization_code",
			"client_id":    config.ClientID,
			"redirect_uri": redirectURI,
			"code":         code,
		}

		// Add client_secret
		jsonData["client_secret"] = config.ClientSecret

		// Add code_verifier for PKCE flow
		if config.OAuthMethod == OAuthMethodPKCE && codeVerifier != "" {
			jsonData["code_verifier"] = codeVerifier
		}

		// Add provider-specific extra parameters
		for key, value := range config.TokenExtraParams {
			jsonData[key] = value
		}

		bodyBytes, err := json.Marshal(jsonData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON request: %w", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
		contentType = "application/json"
	} else {
		// Build form-encoded request body
		data := url.Values{}
		data.Set("grant_type", "authorization_code")
		data.Set("client_id", config.ClientID)
		data.Set("redirect_uri", redirectURI)
		data.Set("code", code)

		// PKCE flow: use code_verifier instead of client_secret
		// Standard OAuth: use client_secret for authentication
		if config.OAuthMethod == OAuthMethodPKCE {
			if codeVerifier != "" {
				data.Set("code_verifier", codeVerifier)
			}
		}
		data.Set("client_secret", config.ClientSecret)

		// Add provider-specific extra parameters
		for key, value := range config.TokenExtraParams {
			data.Set(key, value)
		}

		reqBody = strings.NewReader(data.Encode())
		contentType = "application/x-www-form-urlencoded"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.TokenURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	// Add provider-specific extra headers (may override Content-Type above)
	for key, value := range config.TokenExtraHeaders {
		req.Header.Set(key, value)
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
		return nil, fmt.Errorf("token exchange failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("data decode: %w: %v", ErrTokenExchangeFailed, err)
	}

	token := &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
	}

	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
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

	var reqBody io.Reader
	var contentType string

	if useJSON {
		// Build JSON request body
		jsonData := map[string]any{
			"grant_type":    "refresh_token",
			"refresh_token": refreshToken,
			"client_id":     config.ClientID,
		}

		if config.ClientSecret != "" {
			jsonData["client_secret"] = config.ClientSecret
		}

		// Add provider-specific extra parameters
		for key, value := range config.TokenExtraParams {
			jsonData[key] = value
		}

		bodyBytes, err := json.Marshal(jsonData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON request: %w", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
		contentType = "application/json"
	} else {
		// Build form-encoded request body
		data := url.Values{}
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", refreshToken)
		data.Set("client_id", config.ClientID)
		data.Set("client_secret", config.ClientSecret)

		// Add provider-specific extra parameters
		for key, value := range config.TokenExtraParams {
			data.Set(key, value)
		}

		reqBody = strings.NewReader(data.Encode())
		contentType = "application/x-www-form-urlencoded"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.TokenURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	// Add provider-specific extra headers (may override Content-Type above)
	for key, value := range config.TokenExtraHeaders {
		req.Header.Set(key, value)
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
		return nil, fmt.Errorf("refresh token failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode error: %w: %v", ErrTokenExchangeFailed, err)
	}

	token := &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
	}

	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
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

// debugRequest prints HTTP request details for debugging
func (m *Manager) debugRequest(req *http.Request, isJSON bool) {
	fmt.Printf("\n=== OAuth Debug: HTTP Request ===\n")
	fmt.Printf("Method: %s\n", req.Method)
	fmt.Printf("URL: %s\n", req.URL.String())
	fmt.Printf("\nHeaders:\n")
	for key, values := range req.Header {
		for _, value := range values {
			// Mask sensitive headers
			if strings.EqualFold(key, "Authorization") {
				value = "***REDACTED***"
			}
			fmt.Printf("  %s: %s\n", key, value)
		}
	}

	if req.Body != nil && req.Body != http.NoBody {
		fmt.Printf("\nBody:\n")
		// Read body to print it (but we need to restore it for the actual request)
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil {
			// Try to format JSON for readability
			if isJSON {
				var formatted any
				if json.Unmarshal(bodyBytes, &formatted) == nil {
					if pretty, err := json.MarshalIndent(formatted, "", "  "); err == nil {
						fmt.Printf("%s\n", string(pretty))
					} else {
						fmt.Printf("%s\n", string(bodyBytes))
					}
				} else {
					fmt.Printf("%s\n", string(bodyBytes))
				}
			} else {
				fmt.Printf("%s\n", string(bodyBytes))
			}
			// Restore body for actual request
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}
	fmt.Printf("================================\n\n")
}
