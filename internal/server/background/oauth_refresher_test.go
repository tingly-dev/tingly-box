package background

import (
	"context"
	"testing"
	"time"

	"tingly-box/internal/config"
	oauth2 "tingly-box/pkg/oauth"
)

// tokenRefresher is a minimal interface for the refresh functionality
type tokenRefresher interface {
	RefreshToken(ctx context.Context, userID string, providerType oauth2.ProviderType, refreshToken string) (*oauth2.Token, error)
}

// mockTokenRefresher tracks RefreshToken calls for testing
type mockTokenRefresher struct {
	refreshCalled bool
	refreshToken  string
	userID        string
	providerType  oauth2.ProviderType
}

func (m *mockTokenRefresher) RefreshToken(ctx context.Context, userID string, providerType oauth2.ProviderType, refreshToken string) (*oauth2.Token, error) {
	m.refreshCalled = true
	m.refreshToken = refreshToken
	m.userID = userID
	m.providerType = providerType
	// Return a dummy token with extended expiry
	return &oauth2.Token{
		AccessToken:  "new_access_token",
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(1 * time.Hour),
	}, nil
}

// TestNewOAuthRefresher tests creating a new OAuth refresher
func TestNewOAuthRefresher(t *testing.T) {
	cfg := &config.Config{}
	registry := oauth2.DefaultRegistry()
	oauthConfig := &oauth2.Config{
		BaseURL:           "http://localhost:8080",
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	manager := oauth2.NewManager(oauthConfig, registry)

	refresher := NewTokenRefresher(manager, cfg)

	if refresher == nil {
		t.Fatal("Expected non-nil refresher")
	}

	if refresher.manager != manager {
		t.Error("Manager not set correctly")
	}

	if refresher.serverConfig != cfg {
		t.Error("ServerConfig not set correctly")
	}

	if refresher.checkInterval != 10*time.Minute {
		t.Errorf("Expected check interval 10m, got %v", refresher.checkInterval)
	}

	if refresher.refreshBuffer != 30*time.Minute {
		t.Errorf("Expected refresh buffer 30m, got %v", refresher.refreshBuffer)
	}
}

// TestOAuthRefresherSetters tests setting check interval and refresh buffer
func TestOAuthRefresherSetters(t *testing.T) {
	cfg := &config.Config{}
	registry := oauth2.DefaultRegistry()
	oauthConfig := &oauth2.Config{
		BaseURL:           "http://localhost:8080",
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	manager := oauth2.NewManager(oauthConfig, registry)

	refresher := NewTokenRefresher(manager, cfg)

	// Test SetCheckInterval
	newInterval := 5 * time.Minute
	refresher.SetCheckInterval(newInterval)
	if refresher.checkInterval != newInterval {
		t.Errorf("Expected check interval %v, got %v", newInterval, refresher.checkInterval)
	}

	// Test SetRefreshBuffer
	newBuffer := 10 * time.Minute
	refresher.SetRefreshBuffer(newBuffer)
	if refresher.refreshBuffer != newBuffer {
		t.Errorf("Expected refresh buffer %v, got %v", newBuffer, refresher.refreshBuffer)
	}
}

// TestOAuthRefresherStartStop tests starting and stopping the refresher
func TestOAuthRefresherStartStop(t *testing.T) {
	cfg := &config.Config{}
	registry := oauth2.DefaultRegistry()
	oauthConfig := &oauth2.Config{
		BaseURL:           "http://localhost:8080",
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	manager := oauth2.NewManager(oauthConfig, registry)

	refresher := NewTokenRefresher(manager, cfg)
	// Set short interval for testing
	refresher.SetCheckInterval(100 * time.Millisecond)

	// Initially not running
	if refresher.Running() {
		t.Error("Expected refresher to not be running initially")
	}

	// Start refresher in background
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool)
	go func() {
		refresher.Start(ctx)
		done <- true
	}()

	// Wait a bit for it to start
	time.Sleep(50 * time.Millisecond)

	// Should be running now
	if !refresher.Running() {
		t.Error("Expected refresher to be running after Start")
	}

	// Stop the refresher
	cancel()

	// Wait for it to stop
	select {
	case <-done:
		// OK, stopped successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Refresher did not stop within timeout")
	}

	// Should not be running anymore
	if refresher.Running() {
		t.Error("Expected refresher to not be running after Stop")
	}
}

// TestOAuthRefresherStop tests stopping an idle refresher
func TestOAuthRefresherStopIdle(t *testing.T) {
	cfg := &config.Config{}
	registry := oauth2.DefaultRegistry()
	oauthConfig := &oauth2.Config{
		BaseURL:           "http://localhost:8080",
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	manager := oauth2.NewManager(oauthConfig, registry)

	refresher := NewTokenRefresher(manager, cfg)

	// Stop should not panic on idle refresher
	refresher.Stop()
}

// TestOAuthRefresherCheckAndRefreshTokens tests checking tokens with empty config
func TestOAuthRefresherCheckAndRefreshTokens(t *testing.T) {
	cfg := &config.Config{}
	registry := oauth2.DefaultRegistry()
	oauthConfig := &oauth2.Config{
		BaseURL:           "http://localhost:8080",
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	manager := oauth2.NewManager(oauthConfig, registry)

	refresher := NewTokenRefresher(manager, cfg)

	// Should not panic with empty config
	refresher.CheckAndRefreshTokens()
}

// TestOAuthRefresherStartTwice tests that starting twice doesn't cause issues
func TestOAuthRefresherStartTwice(t *testing.T) {
	cfg := &config.Config{}
	registry := oauth2.DefaultRegistry()
	oauthConfig := &oauth2.Config{
		BaseURL:           "http://localhost:8080",
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	manager := oauth2.NewManager(oauthConfig, registry)

	refresher := NewTokenRefresher(manager, cfg)
	refresher.SetCheckInterval(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First start
	go refresher.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	if !refresher.Running() {
		t.Error("Expected refresher to be running")
	}

	// Second start should be no-op (already running)
	go refresher.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	if !refresher.Running() {
		t.Error("Expected refresher to still be running")
	}

	// Cleanup
	cancel()
	time.Sleep(200 * time.Millisecond)
}

// TestOAuthRefresherActualRefresh tests that refresh is actually triggered for expiring tokens
func TestOAuthRefresherActualRefresh(t *testing.T) {
	cfg := &config.Config{}

	// Create mock refresher
	mockMgr := &mockTokenRefresher{}

	// Create a provider with an expiring token (expires in 2 minutes)
	provider := &config.Provider{
		UUID:     "test-provider-uuid",
		Name:     "TestOAuthProvider",
		APIBase:  "https://api.test.com",
		APIStyle: config.APIStyleOpenAI,
		AuthType: config.AuthTypeOAuth,
		OAuthDetail: &config.OAuthDetail{
			AccessToken:  "old_access_token",
			RefreshToken: "refresh_token_123",
			ProviderType: "claude_code", // Valid provider type
			UserID:       "test-user",
			ExpiresAt:    time.Now().Add(2 * time.Minute).Format(time.RFC3339), // Expires in 2 min
		},
	}

	// Manually add provider to config
	cfg.Providers = append(cfg.Providers, provider)

	// Create refresher with mock manager and short buffer
	refresher := &OAuthRefresher{
		manager:       mockMgr,
		serverConfig:  cfg,
		checkInterval: 10 * time.Minute,
		refreshBuffer: 5 * time.Minute, // Should refresh if expires within 5 min
		stopChan:      make(chan struct{}),
	}

	// Call CheckAndRefreshTokens directly
	refresher.CheckAndRefreshTokens()

	// Verify RefreshToken was called
	if !mockMgr.refreshCalled {
		t.Error("Expected RefreshToken to be called for expiring token")
	}

	if mockMgr.refreshToken != "refresh_token_123" {
		t.Errorf("Expected refresh token 'refresh_token_123', got '%s'", mockMgr.refreshToken)
	}

	if mockMgr.userID != "test-user" {
		t.Errorf("Expected userID 'test-user', got '%s'", mockMgr.userID)
	}

	// Verify provider was updated with new token
	updatedProvider, err := cfg.GetProviderByUUID("test-provider-uuid")
	if err != nil {
		t.Fatalf("Failed to get updated provider: %v", err)
	}

	if updatedProvider.OAuthDetail.AccessToken != "new_access_token" {
		t.Errorf("Expected access token 'new_access_token', got '%s'", updatedProvider.OAuthDetail.AccessToken)
	}
}

// TestOAuthRefresherSkipValidTokens tests that valid tokens are not refreshed
func TestOAuthRefresherSkipValidTokens(t *testing.T) {
	cfg := &config.Config{}

	// Create mock refresher
	mockMgr := &mockTokenRefresher{}

	// Create a provider with a valid token (expires in 1 hour)
	provider := &config.Provider{
		UUID:     "test-provider-uuid",
		Name:     "TestOAuthProvider",
		APIBase:  "https://api.test.com",
		APIStyle: config.APIStyleOpenAI,
		AuthType: config.AuthTypeOAuth,
		OAuthDetail: &config.OAuthDetail{
			AccessToken:  "valid_access_token",
			RefreshToken: "refresh_token_123",
			ProviderType: "claude_code", // Valid provider type
			UserID:       "test-user",
			ExpiresAt:    time.Now().Add(1 * time.Hour).Format(time.RFC3339), // Expires in 1 hour
		},
	}

	// Manually add provider to config
	cfg.Providers = append(cfg.Providers, provider)

	// Create refresher with mock manager and 5 min buffer
	refresher := &OAuthRefresher{
		manager:       mockMgr,
		serverConfig:  cfg,
		checkInterval: 10 * time.Minute,
		refreshBuffer: 5 * time.Minute, // Should only refresh if expires within 5 min
		stopChan:      make(chan struct{}),
	}

	// Call CheckAndRefreshTokens directly
	refresher.CheckAndRefreshTokens()

	// Verify RefreshToken was NOT called (token is still valid)
	if mockMgr.refreshCalled {
		t.Error("Expected RefreshToken NOT to be called for valid token")
	}
}
