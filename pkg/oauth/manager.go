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

	token, err := m.exchangeCodeForToken(ctx, config, state, code, codeVerifier, stateData.RedirectURI)
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

	// Add provider-specific extra parameters
	for key, value := range config.TokenExtraParams {
		params[key] = value
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

	// Parse response directly into Token
	token := &Token{}
	if err := json.NewDecoder(resp.Body).Decode(token); err != nil {
		return nil, fmt.Errorf("data decode: %w: %v", ErrTokenExchangeFailed, err)
	}

	// Convert ExpiresIn to Expiry
	if token.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
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

	// Add provider-specific extra parameters
	for key, value := range config.TokenExtraParams {
		params[key] = value
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

	// Add provider-specific extra headers
	for key, value := range config.AuthExtraParams {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed: status %d, body: %s", resp.StatusCode, string(body))
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
// Polling timeout is limited to 1 minute
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

	// Create a timeout context with 1 minute limit for polling
	const pollTimeout = 1 * time.Minute
	timeoutCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	for {
		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("authentication timed out after %v", pollTimeout)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			token, err := m.pollTokenRequest(ctx, config, data.DeviceCode, data.CodeVerifier)
			if err != nil {
				// Check if error is a transient error that we should retry
				if isTransientDeviceCodeError(err) {
					time.Sleep(interval)
					continue
				}
				return nil, err
			}

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

	// Add provider-specific extra headers
	for key, value := range config.TokenExtraHeaders {
		req.Header.Set(key, value)
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
		}
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
