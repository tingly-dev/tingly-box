package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	State      string
	UserID     string
	Provider   ProviderType
	ExpiresAt  time.Time
	RedirectTo string // Optional redirect URL after successful auth
	Name       string // Optional custom provider name
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
	return uuid.New().String(), nil
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

	// Save state data
	if err := m.saveState(&StateData{
		State:      state,
		UserID:     userID,
		Provider:   providerType,
		RedirectTo: redirectTo,
		Name:       name,
	}); err != nil {
		return "", "", err
	}

	// Build authorization URL
	authURL, err := m.buildAuthURL(config, state)
	if err != nil {
		m.deleteState(state)
		return "", "", err
	}

	return authURL, state, nil
}

// buildAuthURL builds the authorization URL with all required parameters
func (m *Manager) buildAuthURL(config *ProviderConfig, state string) (string, error) {
	u, err := url.Parse(config.AuthURL)
	if err != nil {
		return "", err
	}

	redirectURL := config.RedirectURL
	if redirectURL == "" {
		redirectURL = fmt.Sprintf("%s/oauth/callback", m.config.BaseURL)
	}

	query := u.Query()
	query.Set("client_id", config.ClientID)
	query.Set("redirect_uri", redirectURL)
	query.Set("response_type", "code")
	query.Set("state", state)
	if len(config.Scopes) > 0 {
		query.Set("scope", strings.Join(config.Scopes, " "))
	}
	u.RawQuery = query.Encode()

	return u.String(), nil
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
	token, err := m.exchangeCodeForToken(ctx, config, code)
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
func (m *Manager) exchangeCodeForToken(ctx context.Context, config *ProviderConfig, code string) (*Token, error) {
	redirectURL := config.RedirectURL
	if redirectURL == "" {
		redirectURL = fmt.Sprintf("%s/oauth/callback", m.config.BaseURL)
	}

	// Build token request
	data := url.Values{}
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURL)
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenExchangeFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrTokenExchangeFailed, resp.StatusCode)
	}

	// Parse response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenExchangeFailed, err)
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

	// Build refresh request
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenExchangeFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrTokenExchangeFailed, resp.StatusCode)
	}

	// Parse response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenExchangeFailed, err)
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
