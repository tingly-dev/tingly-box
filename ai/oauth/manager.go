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
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
)

// Manager handles OAuth flows
type Manager struct {
	config         *Config
	registry       *Registry
	tokenStorage   TokenStorage
	stateStorage   StateStorage
	sessionStorage SessionStorage
	Debug          bool
}

// StateData holds information about an OAuth state
type StateData struct {
	State         string
	UserID        string
	Issuer        ai.Issuer
	ExpiresAt     time.Time
	Timestamp     int64  // Unix timestamp when state was created
	ExpiresAtUnix int64  // Unix timestamp when state expires
	RedirectTo    string // Optional redirect URL after successful auth
	Name          string // Optional custom provider name
	CodeVerifier  string // PKCE code verifier (for PKCE flow)
	RedirectURI   string // Actual redirect_uri used in auth request (must match in token request)
	SessionID     string // Session ID for status tracking
}

type managerOptions struct {
	config   *Config
	registry *Registry
}

// ManagerOption configures an OAuth manager.
type ManagerOption func(*managerOptions)

// WithConfig sets the OAuth configuration used by the manager.
func WithConfig(config *Config) ManagerOption {
	return func(o *managerOptions) {
		o.config = config
	}
}

// WithRegistry sets the provider registry used by the manager.
func WithRegistry(registry *Registry) ManagerOption {
	return func(o *managerOptions) {
		o.registry = registry
	}
}

// NewManager creates a new OAuth manager.
func NewManager(opts ...ManagerOption) *Manager {
	options := &managerOptions{
		config:   DefaultConfig(),
		registry: DefaultRegistry(),
	}
	for _, opt := range opts {
		opt(options)
	}

	oauthConfig := options.config
	if oauthConfig == nil {
		oauthConfig = DefaultConfig()
	}
	registry := options.registry
	if registry == nil {
		registry = DefaultRegistry()
	}

	// Use storage from config, or create default memory storage
	tokenStorage := oauthConfig.TokenStorage
	if tokenStorage == nil {
		tokenStorage = NewMemoryTokenStorage()
	}

	stateStorage := oauthConfig.StateStorage
	if stateStorage == nil {
		stateStorage = NewMemoryStateStorage()
	}

	sessionStorage := oauthConfig.SessionStorage
	if sessionStorage == nil {
		sessionStorage = NewMemorySessionStorage()
	}

	// Keep the normalized storage on Config because manager methods use both the
	// direct fields below and m.config.*Storage.
	oauthConfig.TokenStorage = tokenStorage
	oauthConfig.StateStorage = stateStorage
	oauthConfig.SessionStorage = sessionStorage
	if oauthConfig.ProviderConfigs == nil {
		oauthConfig.ProviderConfigs = make(map[ai.Issuer]*ProviderConfig)
	}

	m := &Manager{
		config:         oauthConfig,
		registry:       registry,
		tokenStorage:   tokenStorage,
		stateStorage:   stateStorage,
		sessionStorage: sessionStorage,
	}

	// Start cleanup goroutine
	go m.cleanupPeriodically()

	return m
}

// generateState generates a secure random state parameter
func (m *Manager) generateState(encoding StateEncoding) (string, error) {
	var size int
	switch encoding {
	case StateEncodingBase64URL32:
		size = 32 // 32 bytes -> 43 chars in base64url (matches OpenAI Codex)
	case StateEncodingBase64URL:
		size = 16 // 16 bytes -> 22 chars in base64url
	default:
		size = 16 // 16 bytes -> 32 chars in hex
	}
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	switch encoding {
	case StateEncodingBase64URL, StateEncodingBase64URL32:
		return base64.RawURLEncoding.EncodeToString(b), nil
	default:
		return hex.EncodeToString(b), nil
	}
}

// generateCodeVerifier generates a PKCE code verifier
// 96 random bytes → 128 base64url chars
func (m *Manager) generateCodeVerifier() (string, error) {
	// Generate 96 random bytes (matches OpenAI Codex implementation)
	bytes := make([]byte, 96)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate code verifier: %w", err)
	}
	verifier := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)
	return verifier, nil
}

// generateCodeChallenge creates a PKCE code challenge from the verifier
// Uses SHA256 hash + base64url encoding
func (m *Manager) generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
	return challenge
}

// saveState saves state data with expiration
func (m *Manager) saveState(data *StateData) error {
	// Set expiration time based on config
	now := time.Now()
	data.Timestamp = now.Unix()
	data.ExpiresAt = now.Add(m.config.StateExpiry)
	data.ExpiresAtUnix = data.ExpiresAt.Unix()

	return m.stateStorage.SaveState(data.State, data)
}

// getState retrieves and validates state data
func (m *Manager) getState(state string) (*StateData, error) {
	return m.stateStorage.GetState(state)
}

// GetStateData retrieves state data by state parameter (public method for external access)
func (m *Manager) GetStateData(state string) (*StateData, error) {
	return m.stateStorage.GetState(state)
}

// deleteState removes state data
func (m *Manager) deleteState(state string) {
	_ = m.stateStorage.DeleteState(state)
}

// cleanupExpiredStates is removed - now handled by cleanupPeriodically

// GetAuthURL generates the OAuth authorization URL for a provider
func (m *Manager) GetAuthURL(userID string, issuer ai.Issuer, redirectTo string, name string, sessionID string, opts ...Option) (string, string, error) {
	options := applyOptions(opts...)
	config, ok := m.registry.Get(issuer)
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrInvalidProvider, issuer)
	}

	if config.ClientID == "" {
		return "", "", fmt.Errorf("%w: %s", ErrProviderNotConfigured, issuer)
	}

	// Generate state
	state, err := m.generateState(config.StateEncoding)
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
	authURL, redirectURI, err := m.buildAuthURL(config, state, codeVerifier, options)
	if err != nil {
		m.deleteState(state)
		return "", "", err
	}

	// Update state with actual redirect_uri used
	if err := m.saveState(&StateData{
		State:        state,
		UserID:       userID,
		Issuer:       issuer,
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
func (m *Manager) buildAuthURL(config *ProviderConfig, state string, codeVerifier string, opts *Options) (string, string, error) {
	u, err := url.Parse(config.AuthURL)
	if err != nil {
		return "", "", err
	}

	// Validate port constraint if specified
	if len(config.CallbackPorts) > 0 {
		baseURL, err := url.Parse(m.callbackBaseURL(opts))
		if err == nil {
			port := baseURL.Port()
			if port == "" {
				// Default to port 80 for http, 443 for https
				if baseURL.Scheme == "https" {
					port = "443"
				} else {
					port = "80"
				}
			}
			portInt := 0
			if port != "" {
				_, err := fmt.Sscanf(port, "%d", &portInt)
				if err != nil {
					return "", "", fmt.Errorf("invalid port in BaseURL: %w", err)
				}
			}
			allowed := false
			for _, allowedPort := range config.CallbackPorts {
				if portInt == allowedPort {
					allowed = true
					break
				}
			}
			if !allowed {
				return "", "", fmt.Errorf("port %d is not allowed for provider %s (allowed ports: %v)", portInt, config.Type, config.CallbackPorts)
			}
		}
	}

	// Use hardcoded RedirectURL if provided (for providers requiring specific redirect URIs)
	callbackPath := config.Callback
	if callbackPath == "" {
		callbackPath = "/callback"
	}
	redirectURL := fmt.Sprintf("%s%s", m.callbackBaseURL(opts), callbackPath)

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
		challenge := m.generateCodeChallenge(codeVerifier)
		query.Set("code_challenge", challenge)
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

// callbackBaseURL returns the per-request callback base URL or the manager default.
func (m *Manager) callbackBaseURL(opts *Options) string {
	if opts != nil && opts.BaseURL != "" {
		return opts.BaseURL
	}
	return m.config.BaseURL
}

// getHTTPClient returns appropriate HTTP client based on options and config
func (m *Manager) getHTTPClient(opts *Options) *http.Client {
	// 1. Use explicit HTTPClient from options if provided
	if opts.HTTPClient != nil {
		return opts.HTTPClient
	}

	// 2. Use proxy from options if provided
	if opts.ProxyURL != nil {
		transport := &http.Transport{
			Proxy: http.ProxyURL(opts.ProxyURL),
		}
		return &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}
	}

	// 3. Fall back to config's HTTP client (which may have proxy)
	return m.config.GetHTTPClient()
}

// buildTokenRequest constructs a POST request to endpoint carrying params in the
// provider's configured body format. It sets the default Content-Type/Accept
// headers, runs the provider hook (which may mutate params and headers),
// rebuilds the body if the hook changed params, and merges per-flow extra
// headers. This is the single construction path shared by token exchange,
// refresh, device-code initiation, and device-code polling.
func (m *Manager) buildTokenRequest(ctx context.Context, config *ProviderConfig, endpoint string, params map[string]string, opts *Options) (*http.Request, error) {
	reqBody, contentType, err := buildRequestBody(params, config.TokenRequestFormat)
	if err != nil {
		return nil, fmt.Errorf("failed to build request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	// Call provider's token hook if present. It may mutate params and headers.
	if config.Hook != nil {
		if err := config.Hook.BeforeToken(params, req.Header); err != nil {
			return nil, err
		}
		// Rebuild the body in case the hook mutated params.
		reqBody, _, err = buildRequestBody(params, config.TokenRequestFormat)
		if err != nil {
			return nil, fmt.Errorf("failed to rebuild request body: %w", err)
		}
		req.Body = io.NopCloser(reqBody)
	}

	applyExtraHeaders(req.Header, opts.ExtraHeaders)
	return req, nil
}

// sendTokenRequest builds and sends a token request, applying the debug hook and
// the per-request timeout. Callers own response reading and status handling
// because their success/error decoding differs.
func (m *Manager) sendTokenRequest(ctx context.Context, config *ProviderConfig, endpoint string, params map[string]string, timeout time.Duration, opts *Options) (*http.Response, error) {
	req, err := m.buildTokenRequest(ctx, config, endpoint, params, opts)
	if err != nil {
		return nil, err
	}

	m.debugRequest(req, config.TokenRequestFormat)

	client := m.getHTTPClient(opts)
	client.Timeout = timeout
	return client.Do(req)
}

// populateAnthropicMetadata extracts organization / account identity from an
// Anthropic (Claude) token response body into the token metadata. A response
// body that does not decode is ignored — the token itself is still usable.
func populateAnthropicMetadata(token *Token, rawBody []byte) {
	var resp AnthropicTokenResponse
	if json.Unmarshal(rawBody, &resp) != nil {
		return
	}
	token.putMetadata("organization_id", resp.Organization.UUID)
	token.putMetadata("organization_name", resp.Organization.Name)
	token.putMetadata("account_id", resp.Account.UUID)
	token.putMetadata("email", resp.Account.EmailAddress)
}

// populateCodexMetadata extracts email / account_id / name from a Codex ID
// token (JWT) into the token metadata. Returns false if there is no ID token or
// it could not be parsed.
func populateCodexMetadata(token *Token) bool {
	if token.IDToken == "" {
		return false
	}
	claims := parseIDToken(token.IDToken)
	if claims == nil {
		return false
	}
	token.putMetadata("email", claims.Email)
	token.putMetadata("account_id", claims.GetAccountID())
	token.putMetadata("name", claims.Name)
	return true
}

// HandleCallback handles the OAuth callback request
func (m *Manager) HandleCallback(ctx context.Context, r *http.Request, opts ...Option) (*Token, error) {
	options := applyOptions(opts...)

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
	config, ok := m.registry.Get(stateData.Issuer)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, stateData.Issuer)
	}

	// Exchange code for token
	// For PKCE providers, include the code verifier; for standard OAuth, omit it
	var codeVerifier string
	if config.OAuthMethod == OAuthMethodPKCE {
		codeVerifier = stateData.CodeVerifier
	}

	token, err := m.exchangeCodeForToken(ctx, config, state, code, codeVerifier, stateData.RedirectURI, options)
	if err != nil {
		return nil, err
	}
	token.Issuer = stateData.Issuer
	token.RedirectTo = stateData.RedirectTo
	token.Name = stateData.Name
	token.SessionID = stateData.SessionID

	// Save token
	if err := m.config.TokenStorage.SaveToken(stateData.UserID, stateData.Issuer, token); err != nil {
		return nil, err
	}

	return token, nil
}

// exchangeCodeForToken exchanges the authorization code for an access token
func (m *Manager) exchangeCodeForToken(ctx context.Context, config *ProviderConfig, state string, code string, codeVerifier string, redirectURI string, opts *Options) (*Token, error) {
	// Build common parameters
	params := map[string]string{
		"grant_type":   "authorization_code",
		"client_id":    config.ClientID,
		"code":         code,
		"redirect_uri": redirectURI,
	}

	// Add client_secret if possible

	switch config.Type {
	case ai.IssuerCodex:
		// ignore client secret for codex
	case ai.IssuerClaudeCode:
		// require state for claude code
		params["state"] = state
		if config.ClientSecret != "" {
			params["client_secret"] = config.ClientSecret
		}
	default:
		if config.ClientSecret != "" {
			params["client_secret"] = config.ClientSecret
		}
	}

	// Add code_verifier for PKCE
	if config.OAuthMethod == OAuthMethodPKCE && codeVerifier != "" {
		params["code_verifier"] = codeVerifier
	}

	logrus.WithFields(logrus.Fields{
		"state":                state,
		"provider":             config.Type,
		"code_verifier_length": len(codeVerifier),
		"redirect_uri":         redirectURI,
		"grant_type":           params["grant_type"],
		"has_client_secret":    config.ClientSecret != "",
		"token_url":            config.TokenURL,
		"request_format":       map[TokenRequestFormat]string{TokenRequestFormatJSON: "JSON", TokenRequestFormatForm: "Form"}[config.TokenRequestFormat],
	}).Info("[OAuth] Exchanging authorization code for token")

	resp, err := m.sendTokenRequest(ctx, config, config.TokenURL, params, 60*time.Second, opts)
	if err != nil {
		return nil, fmt.Errorf("client error: %w: %v", ErrTokenExchangeFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		// Log the body for debugging (truncate if too long)
		if len(bodyStr) > 500 {
			logrus.Debugf("token exchange failed: status %d, body: %s...", resp.StatusCode, bodyStr[:500])
		} else {
			logrus.Debugf("token exchange failed: status %d, body: %s", resp.StatusCode, bodyStr)
		}
		return nil, fmt.Errorf("token exchange failed: status %d, body: %s", resp.StatusCode, bodyStr)
	}

	// Read response body so we can both decode it and log it on failure.
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	token := &Token{}
	if err := json.Unmarshal(rawBody, token); err != nil {
		return nil, fmt.Errorf("data decode: %w: %v", ErrTokenExchangeFailed, err)
	}

	// Convert ExpiresIn to Expiry
	token.setExpiryFromExpiresIn()

	// Extract provider-specific identity metadata from the token response.
	switch config.Type {
	case ai.IssuerClaudeCode, ai.IssuerAnthropic:
		// Anthropic/Claude return organization/account info in the token response.
		populateAnthropicMetadata(token, rawBody)
	case ai.IssuerCodex:
		// Codex carries user info in the ID token (JWT).
		if token.IDToken != "" {
			if !populateCodexMetadata(token) {
				logrus.Warnf("[OAuth] Failed to parse ID token for Codex provider")
			}
		} else {
			// Log only the key set so we can tell whether OpenAI omitted id_token
			// or returned it under a different key — never the values, which
			// contain access_token / refresh_token.
			var raw map[string]json.RawMessage
			if jsonErr := json.Unmarshal(rawBody, &raw); jsonErr == nil {
				keys := make([]string, 0, len(raw))
				for k := range raw {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				logrus.Warnf("[OAuth] Codex token exchange returned no id_token; response field names: %v", keys)
			} else {
				logrus.Warnf("[OAuth] Codex token exchange returned no id_token; could not inspect response: %v", jsonErr)
			}
		}
	}

	// Call provider's after-token hook to fetch additional metadata
	if config.Hook != nil && token.AccessToken != "" {
		metadata, err := config.Hook.AfterToken(ctx, token.AccessToken, m.getHTTPClient(opts))
		if err != nil {
			logrus.Warnf("[OAuth] AfterToken hook failed: %v", err)
			// Continue even if AfterToken fails, as we already have the token
		}
		token.mergeMetadata(metadata)
	}

	return token, nil
}

// GetToken retrieves a token for a user and provider, refreshing if necessary
func (m *Manager) GetToken(ctx context.Context, userID string, issuer ai.Issuer, opts ...Option) (*Token, error) {
	options := applyOptions(opts...)
	token, err := m.config.TokenStorage.GetToken(userID, issuer)
	if err != nil {
		return nil, err
	}

	// Check if token needs refresh
	if token.ExpiredIn(m.config.TokenExpiryBuffer) {
		if token.RefreshToken != "" {
			refreshed, err := m.refreshToken(ctx, issuer, token.RefreshToken, options)
			if err == nil {
				refreshed.Issuer = issuer
				// Preserve old refresh token if new one is not returned
				if refreshed.RefreshToken == "" {
					refreshed.RefreshToken = token.RefreshToken
				}
				if err := m.config.TokenStorage.SaveToken(userID, issuer, refreshed); err == nil {
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
func (m *Manager) refreshToken(ctx context.Context, issuer ai.Issuer, refreshToken string, opts *Options) (*Token, error) {
	config, ok := m.registry.Get(issuer)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, issuer)
	}

	// Build common parameters
	params := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     config.ClientID,
	}

	// ref: https://github.com/openai/codex/blob/d807d44a/codex-rs/core/tests/suite/auth_refresh.rs#L35-L94
	// codex DO NOT require client_secret
	if issuer != ai.IssuerCodex {
		params["client_secret"] = config.ClientSecret
	}

	resp, err := m.sendTokenRequest(ctx, config, config.TokenURL, params, 30*time.Second, opts)
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
	token.setExpiryFromExpiresIn()

	// For Codex provider, parse ID token to extract user info
	if issuer == ai.IssuerCodex {
		populateCodexMetadata(token)
	}

	return token, nil
}

// RefreshToken refreshes an access token using a refresh token
// This is a public method that can be called from HTTP handlers
func (m *Manager) RefreshToken(ctx context.Context, userID string, issuer ai.Issuer, refreshToken string, opts ...Option) (*Token, error) {
	options := applyOptions(opts...)
	// Refresh the token
	token, err := m.refreshToken(ctx, issuer, refreshToken, options)
	if err != nil {
		return nil, err
	}

	token.Issuer = issuer

	// Preserve old refresh token if new one is not returned
	// Some OAuth providers don't return a new refresh token on each refresh
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}

	// Save the refreshed token
	if err := m.config.TokenStorage.SaveToken(userID, issuer, token); err != nil {
		return nil, fmt.Errorf("failed to save refreshed token: %w", err)
	}

	return token, nil
}

// RevokeToken removes a token for a user and provider
func (m *Manager) RevokeToken(userID string, issuer ai.Issuer) error {
	return m.config.TokenStorage.DeleteToken(userID, issuer)
}

// ListIssuers returns all providers that have valid tokens for the user
func (m *Manager) ListIssuers(userID string) ([]ai.Issuer, error) {
	return m.config.TokenStorage.ListIssuers(userID)
}

// GetRegistry returns the provider registry
func (m *Manager) GetRegistry() *Registry {
	return m.registry
}

// GetConfig returns the OAuth configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// SetBaseURL updates the BaseURL in the OAuth configuration
// This is used when starting a dynamic callback server on a specific port
func (m *Manager) SetBaseURL(baseURL string) {
	m.config.BaseURL = baseURL
}

// SetProxyURL updates the ProxyURL in the OAuth configuration
// This is used to temporarily set a proxy for a specific OAuth flow
func (m *Manager) SetProxyURL(proxyURL *url.URL) {
	m.config.ProxyURL = proxyURL
	if proxyURL != nil {
		logrus.Infof("[OAuth] Set proxy URL: %s", proxyURL.String())
	}
}

// ResetProxyURL clears the ProxyURL in the OAuth configuration
// This should be called after OAuth flow completes
func (m *Manager) ResetProxyURL() {
	m.config.ProxyURL = nil
	logrus.Info("[OAuth] Reset proxy URL")
}

// applyExtraHeaders lets callers inject provider-specific header state
// (e.g. Kimi's X-Msh-Device-Id) without the manager knowing the provider.
func applyExtraHeaders(dst http.Header, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Set(k, v)
		}
	}
}

// debugRequest prints HTTP request details for debugging
func (m *Manager) debugRequest(req *http.Request, format TokenRequestFormat) {
	if !m.Debug {
		return
	}
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
			switch format {
			case TokenRequestFormatJSON:
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
			default:
				logrus.Debugf("%s", string(bodyBytes))
			}
			// Restore body for actual request
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}
	logrus.Debug("================================")
}

// cleanupPeriodically removes expired states, sessions, and tokens
func (m *Manager) cleanupPeriodically() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.stateStorage.CleanupExpired()
		m.sessionStorage.CleanupExpired()
		m.tokenStorage.CleanupExpired()
	}
}
