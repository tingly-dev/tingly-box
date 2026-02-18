package oauth

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestMemoryTokenStorage tests the in-memory token storage implementation
func TestMemoryTokenStorage(t *testing.T) {
	storage := NewMemoryTokenStorage()

	userID := "test-user"
	provider := ProviderClaudeCode

	token := &Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour),
		Provider:     provider,
	}

	// Test SaveToken
	t.Run("SaveToken", func(t *testing.T) {
		err := storage.SaveToken(userID, provider, token)
		if err != nil {
			t.Fatalf("SaveToken failed: %v", err)
		}
	})

	// Test GetToken
	t.Run("GetToken", func(t *testing.T) {
		retrieved, err := storage.GetToken(userID, provider)
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}
		if retrieved.AccessToken != token.AccessToken {
			t.Errorf("Expected access token %s, got %s", token.AccessToken, retrieved.AccessToken)
		}
		if retrieved.RefreshToken != token.RefreshToken {
			t.Errorf("Expected refresh token %s, got %s", token.RefreshToken, retrieved.RefreshToken)
		}
	})

	// Test GetToken not found
	t.Run("GetTokenNotFound", func(t *testing.T) {
		_, err := storage.GetToken("unknown-user", provider)
		if err != ErrTokenNotFound {
			t.Errorf("Expected ErrTokenNotFound, got %v", err)
		}
	})

	// Test ListProviders
	t.Run("ListProviders", func(t *testing.T) {
		providers, err := storage.ListProviders(userID)
		if err != nil {
			t.Fatalf("ListProviders failed: %v", err)
		}
		if len(providers) != 1 {
			t.Errorf("Expected 1 provider, got %d", len(providers))
		}
		if providers[0] != provider {
			t.Errorf("Expected provider %s, got %s", provider, providers[0])
		}
	})

	// Test DeleteToken
	t.Run("DeleteToken", func(t *testing.T) {
		err := storage.DeleteToken(userID, provider)
		if err != nil {
			t.Fatalf("DeleteToken failed: %v", err)
		}

		// Verify token is deleted
		_, err = storage.GetToken(userID, provider)
		if err != ErrTokenNotFound {
			t.Errorf("Expected ErrTokenNotFound after delete, got %v", err)
		}
	})
}

// TestTokenValid tests the Token.Valid method
func TestTokenValid(t *testing.T) {
	t.Run("ValidToken", func(t *testing.T) {
		token := &Token{
			AccessToken: "test-token",
			Expiry:      time.Now().Add(1 * time.Hour),
		}
		if !token.Valid() {
			t.Error("Expected token to be valid")
		}
	})

	t.Run("NilToken", func(t *testing.T) {
		var token *Token
		if token.Valid() {
			t.Error("Expected nil token to be invalid")
		}
	})

	t.Run("EmptyAccessToken", func(t *testing.T) {
		token := &Token{
			AccessToken: "",
			Expiry:      time.Now().Add(1 * time.Hour),
		}
		if token.Valid() {
			t.Error("Expected token with empty access token to be invalid")
		}
	})

	t.Run("ExpiredToken", func(t *testing.T) {
		token := &Token{
			AccessToken: "test-token",
			Expiry:      time.Now().Add(-1 * time.Hour),
		}
		if token.Valid() {
			t.Error("Expected expired token to be invalid")
		}
	})

	t.Run("NoExpiry", func(t *testing.T) {
		token := &Token{
			AccessToken: "test-token",
			Expiry:      time.Time{}, // Zero time
		}
		if !token.Valid() {
			t.Error("Expected token with no expiry to be valid")
		}
	})
}

// TestTokenExpiredIn tests the Token.ExpiredIn method
func TestTokenExpiredIn(t *testing.T) {
	t.Run("WillExpireSoon", func(t *testing.T) {
		token := &Token{
			AccessToken: "test-token",
			Expiry:      time.Now().Add(1 * time.Minute),
		}
		if !token.ExpiredIn(5 * time.Minute) {
			t.Error("Expected token to expire within 5 minutes")
		}
	})

	t.Run("WillNotExpireSoon", func(t *testing.T) {
		token := &Token{
			AccessToken: "test-token",
			Expiry:      time.Now().Add(10 * time.Minute),
		}
		if token.ExpiredIn(5 * time.Minute) {
			t.Error("Expected token not to expire within 5 minutes")
		}
	})

	t.Run("NoExpiry", func(t *testing.T) {
		token := &Token{
			AccessToken: "test-token",
			Expiry:      time.Time{},
		}
		if token.ExpiredIn(5 * time.Minute) {
			t.Error("Expected token with no expiry to not expire")
		}
	})

	t.Run("NilToken", func(t *testing.T) {
		var token *Token
		if token.ExpiredIn(5 * time.Minute) {
			t.Error("Expected nil token to not expire")
		}
	})
}

// TestProviderRegistry tests the provider registry
func TestProviderRegistry(t *testing.T) {
	t.Run("DefaultRegistry", func(t *testing.T) {
		registry := DefaultRegistry()

		// Check that default providers are registered
		providers := []ProviderType{
			ProviderClaudeCode,
			ProviderOpenAI,
			ProviderGoogle,
			ProviderGitHub,
		}

		for _, p := range providers {
			if !registry.IsRegistered(p) {
				t.Errorf("Expected provider %s to be registered", p)
			}
		}
	})

	t.Run("RegisterAndGet", func(t *testing.T) {
		registry := NewRegistry()

		config := &ProviderConfig{
			Type:         ProviderType("test"),
			DisplayName:  "Test Provider",
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			AuthURL:      "https://example.com/auth",
			TokenURL:     "https://example.com/token",
			Scopes:       []string{"scope1", "scope2"},
		}

		registry.Register(config)

		retrieved, ok := registry.Get(ProviderType("test"))
		if !ok {
			t.Fatal("Expected provider to be registered")
		}

		if retrieved.DisplayName != "Test Provider" {
			t.Errorf("Expected display name 'Test Provider', got '%s'", retrieved.DisplayName)
		}
	})

	t.Run("Unregister", func(t *testing.T) {
		registry := NewRegistry()

		config := &ProviderConfig{
			Type:        ProviderType("test"),
			DisplayName: "Test Provider",
		}

		registry.Register(config)
		registry.Unregister(ProviderType("test"))

		_, ok := registry.Get(ProviderType("test"))
		if ok {
			t.Error("Expected provider to be unregistered")
		}
	})

	t.Run("GetProviderInfo", func(t *testing.T) {
		registry := NewRegistry()

		registry.Register(&ProviderConfig{
			Type:         ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			Scopes:       []string{"api"},
		})

		registry.Register(&ProviderConfig{
			Type:        ProviderOpenAI,
			DisplayName: "OpenAI",
			// No credentials
			AuthURL: "https://openai.com/auth",
			Scopes:  []string{"api"},
		})

		info := registry.GetProviderInfo()
		if len(info) != 2 {
			t.Errorf("Expected 2 providers, got %d", len(info))
		}

		// Check configured status
		anthropicConfigured := false
		openaiConfigured := false
		for _, p := range info {
			if p.Type == ProviderClaudeCode && p.Configured {
				anthropicConfigured = true
			}
			if p.Type == ProviderOpenAI && !p.Configured {
				openaiConfigured = true
			}
		}

		if !anthropicConfigured {
			t.Error("Expected Anthropic to be marked as configured")
		}
		if !openaiConfigured {
			t.Error("Expected OpenAI to be marked as not configured")
		}
	})
}

// TestManager tests the OAuth manager
func TestManager(t *testing.T) {
	t.Run("NewManager", func(t *testing.T) {
		config := DefaultConfig()
		registry := NewRegistry()

		manager := NewManager(config, registry)

		if manager == nil {
			t.Fatal("Expected manager to be created")
		}

		if manager.GetRegistry() != registry {
			t.Error("Expected registry to be set")
		}

		if manager.GetConfig() != config {
			t.Error("Expected config to be set")
		}
	})

	t.Run("GetAuthURL", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(&ProviderConfig{
			Type:         ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		config := DefaultConfig()
		config.BaseURL = "http://localhost:8080"
		manager := NewManager(config, registry)

		authURL, state, err := manager.GetAuthURL("user123", ProviderClaudeCode, "", "", "")
		if err != nil {
			t.Fatalf("GetAuthURL failed: %v", err)
		}

		if authURL == "" {
			t.Error("Expected auth URL to be generated")
		}

		if state == "" {
			t.Error("Expected state to be generated")
		}

		// Verify state is saved
		stateData, err := manager.getState(state)
		if err != nil {
			t.Fatalf("getState failed: %v", err)
		}

		if stateData.UserID != "user123" {
			t.Errorf("Expected userID 'user123', got '%s'", stateData.UserID)
		}

		if stateData.Provider != ProviderClaudeCode {
			t.Errorf("Expected provider %s, got %s", ProviderClaudeCode, stateData.Provider)
		}
	})

	t.Run("GetAuthURLProviderNotConfigured", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(&ProviderConfig{
			Type:        ProviderClaudeCode,
			DisplayName: "Anthropic",
			// No client credentials
			AuthURL:  "https://anthropic.com/auth",
			TokenURL: "https://anthropic.com/token",
			Scopes:   []string{"api"},
		})

		config := DefaultConfig()
		manager := NewManager(config, registry)

		_, _, err := manager.GetAuthURL("user123", ProviderClaudeCode, "", "", "")
		if err == nil {
			t.Error("Expected error for unconfigured provider")
		}
		// Check if the error message contains "not configured"
		if err != nil && !strings.Contains(err.Error(), "not configured") {
			t.Errorf("Expected 'not configured' error, got %v", err)
		}
	})

	t.Run("GetAuthURLInvalidProvider", func(t *testing.T) {
		registry := NewRegistry()
		config := DefaultConfig()
		manager := NewManager(config, registry)

		_, _, err := manager.GetAuthURL("user123", ProviderType("invalid"), "", "", "")
		if err == nil {
			t.Error("Expected error for invalid provider")
		}
	})

	t.Run("StateExpiry", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(&ProviderConfig{
			Type:         ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		config := DefaultConfig()
		config.StateExpiry = 10 * time.Millisecond
		manager := NewManager(config, registry)

		authURL, state, err := manager.GetAuthURL("user123", ProviderClaudeCode, "", "", "")
		if err != nil {
			t.Fatalf("GetAuthURL failed: %v", err)
		}

		if authURL == "" {
			t.Error("Expected auth URL to be generated")
		}

		// Wait for state to expire
		time.Sleep(20 * time.Millisecond)

		// Try to get the expired state
		_, err = manager.getState(state)
		if err != ErrStateExpired {
			t.Errorf("Expected ErrStateExpired, got %v", err)
		}
	})
}

// TestHandleCallback tests the callback handling with mock HTTP server
func TestHandleCallback(t *testing.T) {
	// Create a test HTTP server to mock OAuth provider
	server := &http.Server{
		Addr: ":12845", // Use a different port to avoid conflicts
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/token" {
				// Mock token endpoint
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"access_token": "test-access-token",
					"refresh_token": "test-refresh-token",
					"token_type": "Bearer",
					"expires_in": 3600
				}`))
			}
		}),
	}

	go func() {
		server.ListenAndServe()
	}()
	defer server.Shutdown(context.Background())

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	t.Run("SuccessfulCallback", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(&ProviderConfig{
			Type:         ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "http://localhost:12845/token",
			Scopes:       []string{"api"},
		})

		config := DefaultConfig()
		config.BaseURL = "http://localhost:8080"
		manager := NewManager(config, registry)

		// First, get auth URL to create a state
		_, state, err := manager.GetAuthURL("user123", ProviderClaudeCode, "", "", "")
		if err != nil {
			t.Fatalf("GetAuthURL failed: %v", err)
		}

		// Create a mock callback request
		reqURL, _ := url.Parse("http://localhost:8080/oauth/callback")
		query := reqURL.Query()
		query.Set("code", "test-auth-code")
		query.Set("state", state)
		reqURL.RawQuery = query.Encode()

		req := &http.Request{
			URL: reqURL,
		}

		token, err := manager.HandleCallback(context.Background(), req)
		if err != nil {
			t.Fatalf("HandleCallback failed: %v", err)
		}

		if token.AccessToken != "test-access-token" {
			t.Errorf("Expected access token 'test-access-token', got '%s'", token.AccessToken)
		}

		if token.RefreshToken != "test-refresh-token" {
			t.Errorf("Expected refresh token 'test-refresh-token', got '%s'", token.RefreshToken)
		}

		if token.Provider != ProviderClaudeCode {
			t.Errorf("Expected provider %s, got %s", ProviderClaudeCode, token.Provider)
		}

		// Verify token was saved
		savedToken, err := config.TokenStorage.GetToken("user123", ProviderClaudeCode)
		if err != nil {
			t.Fatalf("Failed to get saved token: %v", err)
		}

		if savedToken.AccessToken != token.AccessToken {
			t.Errorf("Saved token access token mismatch")
		}
	})

	t.Run("CallbackWithError", func(t *testing.T) {
		registry := NewRegistry()
		config := DefaultConfig()
		manager := NewManager(config, registry)

		reqURL, _ := url.Parse("http://localhost:8080/oauth/callback")
		query := reqURL.Query()
		query.Set("error", "access_denied")
		reqURL.RawQuery = query.Encode()

		req := &http.Request{
			URL: reqURL,
		}

		_, err := manager.HandleCallback(context.Background(), req)
		if err == nil {
			t.Error("Expected error for callback with error parameter")
		}
	})

	t.Run("CallbackWithInvalidState", func(t *testing.T) {
		registry := NewRegistry()
		config := DefaultConfig()
		manager := NewManager(config, registry)

		reqURL, _ := url.Parse("http://localhost:8080/oauth/callback")
		query := reqURL.Query()
		query.Set("code", "test-code")
		query.Set("state", "invalid-state")
		reqURL.RawQuery = query.Encode()

		req := &http.Request{
			URL: reqURL,
		}

		_, err := manager.HandleCallback(context.Background(), req)
		if err != ErrInvalidState {
			t.Errorf("Expected ErrInvalidState, got %v", err)
		}
	})
}

// TestGetToken tests token retrieval with automatic refresh
func TestGetToken(t *testing.T) {
	t.Run("GetValidToken", func(t *testing.T) {
		storage := NewMemoryTokenStorage()
		token := &Token{
			AccessToken: "test-token",
			Expiry:      time.Now().Add(1 * time.Hour),
			Provider:    ProviderClaudeCode,
		}
		storage.SaveToken("user123", ProviderClaudeCode, token)

		config := DefaultConfig()
		config.TokenStorage = storage
		manager := NewManager(config, nil)

		retrieved, err := manager.GetToken(context.Background(), "user123", ProviderClaudeCode)
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}

		if retrieved.AccessToken != "test-token" {
			t.Errorf("Expected access token 'test-token', got '%s'", retrieved.AccessToken)
		}
	})

	t.Run("GetTokenNotFound", func(t *testing.T) {
		storage := NewMemoryTokenStorage()
		config := DefaultConfig()
		config.TokenStorage = storage
		manager := NewManager(config, nil)

		_, err := manager.GetToken(context.Background(), "user123", ProviderClaudeCode)
		if err != ErrTokenNotFound {
			t.Errorf("Expected ErrTokenNotFound, got %v", err)
		}
	})
}

// TestRevokeToken tests token revocation
func TestRevokeToken(t *testing.T) {
	storage := NewMemoryTokenStorage()
	token := &Token{
		AccessToken: "test-token",
		Provider:    ProviderClaudeCode,
	}
	storage.SaveToken("user123", ProviderClaudeCode, token)

	config := DefaultConfig()
	config.TokenStorage = storage
	manager := NewManager(config, nil)

	err := manager.RevokeToken("user123", ProviderClaudeCode)
	if err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	// Verify token is deleted
	_, err = storage.GetToken("user123", ProviderClaudeCode)
	if err != ErrTokenNotFound {
		t.Errorf("Expected ErrTokenNotFound after revoke, got %v", err)
	}
}

// TestListProviders tests listing providers with tokens
func TestListProviders(t *testing.T) {
	storage := NewMemoryTokenStorage()

	// Add tokens for multiple providers
	storage.SaveToken("user123", ProviderClaudeCode, &Token{
		AccessToken: "anthropic-token",
		Provider:    ProviderClaudeCode,
	})
	storage.SaveToken("user123", ProviderOpenAI, &Token{
		AccessToken: "openai-token",
		Provider:    ProviderOpenAI,
	})

	config := DefaultConfig()
	config.TokenStorage = storage
	manager := NewManager(config, nil)

	providers, err := manager.ListProviders("user123")
	if err != nil {
		t.Fatalf("ListProviders failed: %v", err)
	}

	if len(providers) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(providers))
	}
}

// TestOptions tests the options pattern for OAuth operations
func TestOptions(t *testing.T) {
	t.Run("applyOptions", func(t *testing.T) {
		// Test empty options
		opts := applyOptions()
		if opts.ProxyURL != nil {
			t.Error("Expected nil ProxyURL for empty options")
		}
		if opts.HTTPClient != nil {
			t.Error("Expected nil HTTPClient for empty options")
		}

		// Test WithProxyURL
		proxyURL, _ := url.Parse("http://proxy.example.com:8080")
		opts = applyOptions(WithProxyURL(proxyURL))
		if opts.ProxyURL == nil {
			t.Error("Expected ProxyURL to be set")
		}
		if opts.ProxyURL.String() != "http://proxy.example.com:8080" {
			t.Errorf("Unexpected ProxyURL: %s", opts.ProxyURL.String())
		}

		// Test WithProxyURLString
		opts = applyOptions(WithProxyURLString("http://proxy2.example.com:9090"))
		if opts.ProxyURL == nil {
			t.Error("Expected ProxyURL to be set")
		}
		if opts.ProxyURL.String() != "http://proxy2.example.com:9090" {
			t.Errorf("Unexpected ProxyURL: %s", opts.ProxyURL.String())
		}

		// Test WithProxyURLString with empty string
		opts = applyOptions(WithProxyURLString(""))
		if opts.ProxyURL != nil {
			t.Error("Expected nil ProxyURL for empty string")
		}

		// Test WithProxyURLString with invalid URL
		opts = applyOptions(WithProxyURLString("://invalid"))
		if opts.ProxyURL != nil {
			t.Error("Expected nil ProxyURL for invalid URL")
		}

		// Test WithHTTPClient
		customClient := &http.Client{}
		opts = applyOptions(WithHTTPClient(customClient))
		if opts.HTTPClient == nil {
			t.Error("Expected HTTPClient to be set")
		}

		// Test multiple options
		opts = applyOptions(
			WithProxyURL(proxyURL),
			WithHTTPClient(customClient),
		)
		if opts.ProxyURL == nil {
			t.Error("Expected ProxyURL to be set")
		}
		if opts.HTTPClient == nil {
			t.Error("Expected HTTPClient to be set")
		}
	})

	t.Run("getHTTPClient", func(t *testing.T) {
		config := DefaultConfig()
		manager := NewManager(config, nil)

		// Test with no options (should use config's client)
		opts := applyOptions()
		client := manager.getHTTPClient(opts)
		if client == nil {
			t.Error("Expected non-nil HTTP client")
		}

		// Test with proxy URL option
		proxyURL, _ := url.Parse("http://proxy.example.com:8080")
		opts = applyOptions(WithProxyURL(proxyURL))
		client = manager.getHTTPClient(opts)
		if client == nil {
			t.Error("Expected non-nil HTTP client")
		}
		// Verify transport has proxy
		transport, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Error("Expected http.Transport")
		}
		if transport == nil {
			t.Error("Expected non-nil transport")
		}

		// Test with custom HTTP client option (should take precedence)
		customClient := &http.Client{Timeout: 10 * time.Second}
		opts = applyOptions(WithHTTPClient(customClient))
		client = manager.getHTTPClient(opts)
		if client != customClient {
			t.Error("Expected custom HTTP client to be used")
		}
	})
}
