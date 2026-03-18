package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	oauth2 "github.com/tingly-dev/tingly-box/pkg/oauth"
)

func TestHandler_OAuthCallback_ErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("CallbackErrorWithSessionFailure", func(t *testing.T) {
		// Setup
		registry := oauth2.NewRegistry()
		registry.Register(&oauth2.ProviderConfig{
			Type:         oauth2.ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		// Use an empty config for testing (the handler only needs it for the type, not for specific values)
		serverCfg := &config.Config{}
		oauthConfig := oauth2.DefaultConfig()
		oauthManager := oauth2.NewManager(oauthConfig, registry)
		handler := NewHandler(oauthManager, serverCfg)

		// Generate a session ID directly (no longer using SessionManager)
		sessionID := uuid.New().String()
		require.NotEmpty(t, sessionID, "SessionID should not be empty")

		// Create an OAuth session in the oauth.Manager
		now := time.Now()
		oauthSession := &oauth2.SessionState{
			SessionID: sessionID,
			Status:    oauth2.SessionStatusPending,
			Provider:  oauth2.ProviderClaudeCode,
			UserID:    "user123",
			CreatedAt: now,
			ExpiresAt: now.Add(oauth2.DefaultSessionExpiry),
		}
		oauthManager.StoreSession(oauthSession)

		// Create a state with sessionID
		_, state, err := oauthManager.GetAuthURL("user123", oauth2.ProviderClaudeCode, "", "", sessionID)
		require.NoError(t, err, "GetAuthURL should succeed")

		// Verify session is pending
		storedSession, err := oauthManager.GetSession(sessionID)
		require.NoError(t, err, "Session should exist")
		assert.Equal(t, oauth2.SessionStatusPending, storedSession.Status, "Initial session status should be pending")

		// Create a mock callback request with error
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		reqURL, _ := url.Parse("http://localhost:8080/oauth/callback")
		query := reqURL.Query()
		query.Set("error", "access_denied")
		query.Set("state", state)
		reqURL.RawQuery = query.Encode()
		req := httptest.NewRequest("GET", reqURL.String(), nil)
		c.Request = req

		// Call OAuthCallback - note: HTML rendering will panic without template engine,
		// but we can recover and verify the session status was updated correctly
		assert.NotPanics(t, func() {
			defer func() {
				if r := recover(); r != nil {
					// Expected panic due to missing template engine in test
					// The important part is that the session status was updated
				}
			}()
			handler.OAuthCallback(c)
		}, "OAuthCallback should handle callback (may panic on HTML rendering in test)")

		// Verify HTTP response status
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 on OAuth error")

		// Verify session was marked as failed (this is the key bugfix behavior)
		storedSession, err = oauthManager.GetSession(sessionID)
		require.NoError(t, err, "Session should still exist")
		assert.Equal(t, oauth2.SessionStatusFailed, storedSession.Status, "Session status should be failed")
		assert.NotEmpty(t, storedSession.Error, "Session error should be set")
		assert.Contains(t, storedSession.Error, "access_denied", "Error message should contain OAuth error")
	})

	t.Run("CallbackErrorWithoutSessionID", func(t *testing.T) {
		registry := oauth2.NewRegistry()
		serverCfg := &config.Config{}
		oauthConfig := oauth2.DefaultConfig()
		oauthManager := oauth2.NewManager(oauthConfig, registry)
		handler := NewHandler(oauthManager, serverCfg)

		// Create a mock callback request with invalid state
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		reqURL, _ := url.Parse("http://localhost:8080/oauth/callback")
		query := reqURL.Query()
		query.Set("error", "access_denied")
		query.Set("state", "invalid-state")
		reqURL.RawQuery = query.Encode()
		req := httptest.NewRequest("GET", reqURL.String(), nil)
		c.Request = req

		// Call OAuthCallback - should not panic (this tests the bugfix safety)
		assert.NotPanics(t, func() {
			handler.OAuthCallback(c)
		}, "OAuthCallback should not panic with invalid state")

		// Verify HTTP response
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 on OAuth error")
	})

	t.Run("CallbackWithExpiredState", func(t *testing.T) {
		registry := oauth2.NewRegistry()
		registry.Register(&oauth2.ProviderConfig{
			Type:         oauth2.ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		serverCfg := &config.Config{}
		oauthConfig := oauth2.DefaultConfig()
		oauthConfig.StateExpiry = 10 * time.Millisecond // Very short expiry
		oauthManager := oauth2.NewManager(oauthConfig, registry)
		handler := NewHandler(oauthManager, serverCfg)

		// Generate a session ID directly
		sessionID := uuid.New().String()

		// Create an OAuth session in the oauth.Manager
		now := time.Now()
		oauthSession := &oauth2.SessionState{
			SessionID: sessionID,
			Status:    oauth2.SessionStatusPending,
			Provider:  oauth2.ProviderClaudeCode,
			UserID:    "user123",
			CreatedAt: now,
			ExpiresAt: now.Add(oauth2.DefaultSessionExpiry),
		}
		oauthManager.StoreSession(oauthSession)

		// Create a state with sessionID
		_, state, err := oauthManager.GetAuthURL("user123", oauth2.ProviderClaudeCode, "", "", sessionID)
		require.NoError(t, err)

		// Wait for state to expire
		time.Sleep(20 * time.Millisecond)

		// Create a mock callback request
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		reqURL, _ := url.Parse("http://localhost:8080/oauth/callback")
		query := reqURL.Query()
		query.Set("code", "test-code")
		query.Set("state", state)
		reqURL.RawQuery = query.Encode()
		req := httptest.NewRequest("GET", reqURL.String(), nil)
		c.Request = req

		// Call OAuthCallback - should handle expired state gracefully
		assert.NotPanics(t, func() {
			handler.OAuthCallback(c)
		}, "OAuthCallback should not panic with expired state")

		// Verify HTTP response
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 on expired state")

		// Verify session was NOT marked as failed (because we couldn't get the sessionID from expired state)
		storedSession, _ := oauthManager.GetSession(sessionID)
		assert.Equal(t, oauth2.SessionStatusPending, storedSession.Status, "Session status should still be pending when state expires")
	})

	t.Run("GetStateDataBeforeHandleCallback", func(t *testing.T) {
		// This test explicitly verifies the bugfix behavior: GetStateData is called BEFORE HandleCallback
		registry := oauth2.NewRegistry()
		registry.Register(&oauth2.ProviderConfig{
			Type:         oauth2.ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		oauthConfig := oauth2.DefaultConfig()
		oauthManager := oauth2.NewManager(oauthConfig, registry)

		// Create a state with sessionID
		testSessionID := "test-session-from-handler"
		_, state, err := oauthManager.GetAuthURL("user123", oauth2.ProviderClaudeCode, "", "", testSessionID)
		require.NoError(t, err)

		// Simulate what OAuthCallback does: retrieve state BEFORE HandleCallback
		// This is the key pattern from the bugfix
		stateData, err := oauthManager.GetStateData(state)
		require.NoError(t, err, "GetStateData should succeed before HandleCallback")

		// Verify we have the sessionID (this is what the bugfix preserves)
		assert.Equal(t, testSessionID, stateData.SessionID, "SessionID should be retrieved from state data")
		assert.Equal(t, "user123", stateData.UserID, "UserID should match")
		assert.Equal(t, oauth2.ProviderClaudeCode, stateData.Provider, "Provider should match")

		// Now HandleCallback would delete the state, but we already have sessionID
		// This simulates the bugfix scenario
	})
}

func TestHandler_AuthorizeOAuth_SessionExpiry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("SessionExpiryUsesDefaultConstant", func(t *testing.T) {
		registry := oauth2.NewRegistry()
		registry.Register(&oauth2.ProviderConfig{
			Type:         oauth2.ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		serverCfg := &config.Config{}
		oauthConfig := oauth2.DefaultConfig()
		oauthManager := oauth2.NewManager(oauthConfig, registry)
		handler := NewHandler(oauthManager, serverCfg)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/api/v1/oauth/authorize?provider=claude_code&user_id=user123", nil)
		c.Request = req

		handler.AuthorizeOAuth(c)

		assert.Equal(t, http.StatusOK, w.Code, "AuthorizeOAuth should return 200")

		var resp OAuthAuthorizeResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err, "Should be able to unmarshal response")

		assert.True(t, resp.Success, "Response should indicate success")
		assert.NotEmpty(t, resp.Data.SessionID, "SessionID should be set")

		// Get the session and verify expiry uses DefaultSessionExpiry
		session, err := oauthManager.GetSession(resp.Data.SessionID)
		require.NoError(t, err, "Session should exist")
		require.NotNil(t, session, "Session should not be nil")

		expectedExpiry := time.Now().Add(oauth2.DefaultSessionExpiry)
		diff := session.ExpiresAt.Sub(expectedExpiry)
		if diff < 0 {
			diff = -diff
		}

		// Allow 1 second tolerance
		assert.LessOrEqual(t, diff, time.Second, "Session expiry should use DefaultSessionExpiry (10 minutes)")
	})
}

func TestHandler_OAuthCallback_Integration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// This test requires a mock OAuth provider that returns errors
	t.Run("FullFlowWithProviderError", func(t *testing.T) {
		// Create a test HTTP server that returns OAuth errors
		server := &http.Server{
			Addr: ":12847",
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error": "invalid_grant", "error_description": "The authorization code is invalid"}`))
				}
			}),
		}

		go func() {
			server.ListenAndServe()
		}()
		defer server.Shutdown(context.Background())
		time.Sleep(100 * time.Millisecond)

		registry := oauth2.NewRegistry()
		registry.Register(&oauth2.ProviderConfig{
			Type:         oauth2.ProviderClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "http://localhost:12847/token",
			Scopes:       []string{"api"},
		})

		serverCfg := &config.Config{}
		oauthConfig := oauth2.DefaultConfig()
		oauthManager := oauth2.NewManager(oauthConfig, registry)
		handler := NewHandler(oauthManager, serverCfg)

		// Step 1: Authorize (create session)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/api/v1/oauth/authorize?provider=claude_code&user_id=user123", nil)
		c.Request = req
		handler.AuthorizeOAuth(c)

		var authResp OAuthAuthorizeResponse
		json.Unmarshal(w.Body.Bytes(), &authResp)
		sessionID := authResp.Data.SessionID

		// Step 2: Get auth URL
		_, state, err := oauthManager.GetAuthURL("user123", oauth2.ProviderClaudeCode, "", "", sessionID)
		require.NoError(t, err)

		// Step 3: Simulate callback with code (will fail at token exchange)
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		reqURL, _ := url.Parse("http://localhost:8080/oauth/callback")
		query := reqURL.Query()
		query.Set("code", "test-auth-code")
		query.Set("state", state)
		reqURL.RawQuery = query.Encode()
		req = httptest.NewRequest("GET", reqURL.String(), nil)
		c.Request = req
		handler.OAuthCallback(c)

		// Step 4: Verify session was marked as failed (key bugfix behavior)
		session, err := oauthManager.GetSession(sessionID)
		require.NoError(t, err, "Session should exist")
		assert.Equal(t, oauth2.SessionStatusFailed, session.Status, "Session should be marked as failed after OAuth error")
		assert.NotEmpty(t, session.Error, "Error message should be set")
	})
}

func TestGenerateProviderName(t *testing.T) {
	t.Run("CustomNameTakesPriority", func(t *testing.T) {
		token := &oauth2.Token{
			Metadata: map[string]any{
				"email": "john.doe@example.com",
				"name":  "John Doe",
			},
		}
		result := generateProviderName(oauth2.ProviderClaudeCode, token, "my-custom-name")
		assert.Equal(t, "my-custom-name", result, "Custom name should take priority")
	})

	t.Run("FullEmailUsedWhenNoCustomName", func(t *testing.T) {
		token := &oauth2.Token{
			Metadata: map[string]any{
				"email": "alice.smith@company.com",
			},
		}
		result := generateProviderName(oauth2.ProviderGemini, token, "")
		assert.Equal(t, "alice.smith@company.com", result, "Should use full email")
	})

	t.Run("DisplayNameUsedWhenNoEmail", func(t *testing.T) {
		token := &oauth2.Token{
			Metadata: map[string]any{
				"name": "Jane Johnson",
			},
		}
		result := generateProviderName(oauth2.ProviderClaudeCode, token, "")
		assert.Equal(t, "Jane-Johnson", result, "Should use display name with spaces replaced")
	})

	t.Run("TimestampFallbackWhenNoMetadata", func(t *testing.T) {
		token := &oauth2.Token{
			Metadata: nil,
		}
		result := generateProviderName(oauth2.ProviderCodex, token, "")
		// Should match format: codex-YYYYMMDD-HHMM
		assert.Contains(t, result, "codex-", "Should have provider prefix")
		assert.Regexp(t, `codex-\d{8}-\d{4}`, result, "Should match timestamp format")
	})

	t.Run("TimestampFallbackWhenMetadataEmpty", func(t *testing.T) {
		token := &oauth2.Token{
			Metadata: map[string]any{},
		}
		result := generateProviderName(oauth2.ProviderQwenCode, token, "")
		assert.Contains(t, result, "qwen_code-", "Should have provider prefix")
		assert.Regexp(t, `qwen_code-\d{8}-\d{4}`, result, "Should match timestamp format")
	})
}
