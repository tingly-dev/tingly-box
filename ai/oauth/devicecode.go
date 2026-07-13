package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

// DeviceCodeResponse represents the response from the device authorization endpoint
// RFC 8628: OAuth 2.0 Device Authorization Grant
type DeviceCodeResponse struct {
	// DeviceCode is the device verification code
	DeviceCode string `json:"device_code"`

	// UserCode is the end-user verification code
	UserCode string `json:"user_code"`

	// VerificationURI is the end-user verification URI where user enters the user code
	VerificationURI string `json:"verification_uri"`

	// VerificationURIComplete is the end-user verification URI with user_code pre-filled
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`

	// ExpiresIn is the lifetime in seconds of the device_code and user_code
	ExpiresIn int64 `json:"expires_in"`

	// Interval is the minimum amount of time in seconds that the client SHOULD wait
	// between polling requests to the token endpoint
	Interval int64 `json:"interval,omitempty"`
}

// DeviceCodeData holds device code information with metadata
type DeviceCodeData struct {
	*DeviceCodeResponse
	Issuer       ai.Issuer
	UserID       string
	RedirectTo   string
	Name         string
	ExpiresAt    time.Time
	InitiatedAt  time.Time
	CodeVerifier string // PKCE code verifier (for Device Code PKCE flow)
}

// InitiateDeviceCodeFlow initiates the Device Code flow and returns device code data
// RFC 8628: OAuth 2.0 Device Authorization Grant
func (m *Manager) InitiateDeviceCodeFlow(ctx context.Context, userID string, issuer ai.Issuer, redirectTo string, name string, opts ...Option) (*DeviceCodeData, error) {
	options := applyOptions(opts...)
	config, ok := m.registry.Get(issuer)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, issuer)
	}

	if config.ClientID == "" {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotConfigured, issuer)
	}

	if config.DeviceCodeURL == "" {
		return nil, fmt.Errorf("provider %s does not support device code flow", issuer)
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
	// Build common parameters
	params := map[string]string{
		"client_id": config.ClientID,
	}
	if len(config.Scopes) > 0 {
		params["scope"] = strings.Join(config.Scopes, " ")
	}
	// Add PKCE parameters for Device Code PKCE flow
	if config.OAuthMethod == OAuthMethodDeviceCodePKCE {
		params["code_challenge"] = codeChallenge
		params["code_challenge_method"] = "S256"
	}

	resp, err := m.sendTokenRequest(ctx, config, config.DeviceCodeURL, params, 30*time.Second, options)
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
		Issuer:             issuer,
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
// or the device code expires.
func (m *Manager) PollForToken(ctx context.Context, data *DeviceCodeData, callback func(*Token), opts ...Option) (*Token, error) {
	options := applyOptions(opts...)
	config, ok := m.registry.Get(data.Issuer)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, data.Issuer)
	}

	// Default interval is 5 seconds if not specified
	interval := time.Duration(data.Interval) * time.Second
	if interval == 0 {
		interval = 2 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Poll until the provider's device code expires, falling back to the
	// standard OAuth session expiry when the provider did not return one.
	pollDeadline := data.ExpiresAt
	if pollDeadline.IsZero() {
		pollDeadline = time.Now().Add(DefaultSessionExpiry)
	}
	timeoutCtx, cancel := context.WithDeadline(ctx, pollDeadline)
	defer cancel()

	fmt.Printf("[OAuth] Device code polling started for %s, expires at: %s\n", data.Issuer, pollDeadline.Format(time.RFC3339))

	for {
		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("authentication timed out at %s", pollDeadline.Format(time.RFC3339))
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			fmt.Printf("[OAuth] Polling token endpoint for %s...\n", data.Issuer)
			token, err := m.pollTokenRequest(ctx, config, data.DeviceCode, data.CodeVerifier, options)
			if err != nil {
				// Check if error is a transient error that we should retry
				if isTransientDeviceCodeError(err) {
					fmt.Printf("[OAuth] Authorization pending for %s, continuing poll...\n", data.Issuer)
					time.Sleep(interval)
					continue
				}
				fmt.Printf("[OAuth] Polling error for %s: %v\n", data.Issuer, err)
				return nil, err
			}

			fmt.Printf("[OAuth] Successfully obtained token for %s\n", data.Issuer)
			// Successfully got token
			token.Issuer = data.Issuer
			token.RedirectTo = data.RedirectTo
			token.Name = data.Name

			// Save token
			if err := m.config.TokenStorage.SaveToken(data.UserID, data.Issuer, token); err != nil {
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
func (m *Manager) pollTokenRequest(ctx context.Context, config *ProviderConfig, deviceCode string, codeVerifier string, opts *Options) (*Token, error) {
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

	resp, err := m.sendTokenRequest(ctx, config, config.TokenURL, params, 30*time.Second, opts)
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
	token.setExpiryFromExpiresIn()

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
