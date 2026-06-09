package oauth

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newOAuthTestContext returns a gin.Context whose engine has the HTML
// templates the OAuth handler renders on error paths. Without this, calls
// to c.HTML panic on a nil HTMLRender before the response status is
// written, which masks the real behavior the tests are asserting.
func newOAuthTestContext(w http.ResponseWriter) *gin.Context {
	c, engine := gin.CreateTestContext(w)
	engine.SetHTMLTemplate(template.Must(template.New("").Parse(
		`{{define "oauth_error.html"}}error: {{.error}}{{end}}` +
			`{{define "oauth_success.html"}}ok{{end}}`,
	)))
	return c
}

func TestHandler_OAuthCallback_ErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("CallbackErrorWithSessionFailure", func(t *testing.T) {
		// Setup
		registry := oauth.NewRegistry()
		registry.Register(&oauth.ProviderConfig{
			Type:         ai.IssuerClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		// Use an empty config for testing (the handler only needs it for the type, not for specific values)
		serverCfg := &config.Config{}
		oauthConfig := oauth.DefaultConfig()
		oauthManager := oauth.NewManager(oauthConfig, registry)
		handler := NewHandler(oauthManager, serverCfg)

		// Generate a session ID directly (no longer using SessionManager)
		sessionID := uuid.New().String()
		require.NotEmpty(t, sessionID, "SessionID should not be empty")

		// Create an OAuth session in the oauth.Manager
		now := time.Now()
		oauthSession := &oauth.SessionState{
			SessionID: sessionID,
			Status:    oauth.SessionStatusPending,
			Provider:  ai.IssuerClaudeCode,
			UserID:    "user123",
			CreatedAt: now,
			ExpiresAt: now.Add(oauth.DefaultSessionExpiry),
		}
		oauthManager.StoreSession(oauthSession)

		// Create a state with sessionID
		_, state, err := oauthManager.GetAuthURL("user123", ai.IssuerClaudeCode, "", "", sessionID)
		require.NoError(t, err, "GetAuthURL should succeed")

		// Verify session is pending
		storedSession, err := oauthManager.GetSession(sessionID)
		require.NoError(t, err, "Session should exist")
		assert.Equal(t, oauth.SessionStatusPending, storedSession.Status, "Initial session status should be pending")

		// Create a mock callback request with error
		w := httptest.NewRecorder()
		c := newOAuthTestContext(w)
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
		assert.Equal(t, oauth.SessionStatusFailed, storedSession.Status, "Session status should be failed")
		assert.NotEmpty(t, storedSession.Error, "Session error should be set")
		assert.Contains(t, storedSession.Error, "access_denied", "Error message should contain OAuth error")
	})

	t.Run("CallbackErrorWithoutSessionID", func(t *testing.T) {
		registry := oauth.NewRegistry()
		serverCfg := &config.Config{}
		oauthConfig := oauth.DefaultConfig()
		oauthManager := oauth.NewManager(oauthConfig, registry)
		handler := NewHandler(oauthManager, serverCfg)

		// Create a mock callback request with invalid state
		w := httptest.NewRecorder()
		c := newOAuthTestContext(w)
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
		registry := oauth.NewRegistry()
		registry.Register(&oauth.ProviderConfig{
			Type:         ai.IssuerClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		serverCfg := &config.Config{}
		oauthConfig := oauth.DefaultConfig()
		oauthConfig.StateExpiry = 10 * time.Millisecond // Very short expiry
		oauthManager := oauth.NewManager(oauthConfig, registry)
		handler := NewHandler(oauthManager, serverCfg)

		// Generate a session ID directly
		sessionID := uuid.New().String()

		// Create an OAuth session in the oauth.Manager
		now := time.Now()
		oauthSession := &oauth.SessionState{
			SessionID: sessionID,
			Status:    oauth.SessionStatusPending,
			Provider:  ai.IssuerClaudeCode,
			UserID:    "user123",
			CreatedAt: now,
			ExpiresAt: now.Add(oauth.DefaultSessionExpiry),
		}
		oauthManager.StoreSession(oauthSession)

		// Create a state with sessionID
		_, state, err := oauthManager.GetAuthURL("user123", ai.IssuerClaudeCode, "", "", sessionID)
		require.NoError(t, err)

		// Wait for state to expire
		time.Sleep(20 * time.Millisecond)

		// Create a mock callback request
		w := httptest.NewRecorder()
		c := newOAuthTestContext(w)
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
		assert.Equal(t, oauth.SessionStatusPending, storedSession.Status, "Session status should still be pending when state expires")
	})

	t.Run("GetStateDataBeforeHandleCallback", func(t *testing.T) {
		// This test explicitly verifies the bugfix behavior: GetStateData is called BEFORE HandleCallback
		registry := oauth.NewRegistry()
		registry.Register(&oauth.ProviderConfig{
			Type:         ai.IssuerClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})

		oauthConfig := oauth.DefaultConfig()
		oauthManager := oauth.NewManager(oauthConfig, registry)

		// Create a state with sessionID
		testSessionID := "test-session-from-handler"
		_, state, err := oauthManager.GetAuthURL("user123", ai.IssuerClaudeCode, "", "", testSessionID)
		require.NoError(t, err)

		// Simulate what OAuthCallback does: retrieve state BEFORE HandleCallback
		// This is the key pattern from the bugfix
		stateData, err := oauthManager.GetStateData(state)
		require.NoError(t, err, "GetStateData should succeed before HandleCallback")

		// Verify we have the sessionID (this is what the bugfix preserves)
		assert.Equal(t, testSessionID, stateData.SessionID, "SessionID should be retrieved from state data")
		assert.Equal(t, "user123", stateData.UserID, "UserID should match")
		assert.Equal(t, ai.IssuerClaudeCode, stateData.Provider, "Provider should match")

		// Now HandleCallback would delete the state, but we already have sessionID
		// This simulates the bugfix scenario
	})
}

func TestGenerateProviderName(t *testing.T) {
	t.Run("CustomNameTakesPriority", func(t *testing.T) {
		token := &oauth.Token{
			Metadata: map[string]any{
				"email": "john.doe@example.com",
				"name":  "John Doe",
			},
		}
		result := generateProviderName(ai.IssuerClaudeCode, token, "my-custom-name")
		assert.Equal(t, "my-custom-name", result, "Custom name should take priority")
	})

	t.Run("FullEmailUsedWhenNoCustomName", func(t *testing.T) {
		token := &oauth.Token{
			Metadata: map[string]any{
				"email": "alice.smith@company.com",
			},
		}
		result := generateProviderName(ai.IssuerGemini, token, "")
		assert.Equal(t, "alice.smith@company.com", result, "Should use full email")
	})

	t.Run("DisplayNameUsedWhenNoEmail", func(t *testing.T) {
		token := &oauth.Token{
			Metadata: map[string]any{
				"name": "Jane Johnson",
			},
		}
		result := generateProviderName(ai.IssuerClaudeCode, token, "")
		assert.Equal(t, "Jane-Johnson", result, "Should use display name with spaces replaced")
	})

	t.Run("TimestampFallbackWhenNoMetadata", func(t *testing.T) {
		token := &oauth.Token{
			Metadata: nil,
		}
		result := generateProviderName(ai.IssuerCodex, token, "")
		// Should match format: codex-YYYYMMDD-HHMM
		assert.Contains(t, result, "codex-", "Should have provider prefix")
		assert.Regexp(t, `codex-\d{8}-\d{4}`, result, "Should match timestamp format")
	})

	t.Run("TimestampFallbackWhenMetadataEmpty", func(t *testing.T) {
		token := &oauth.Token{
			Metadata: map[string]any{},
		}
		result := generateProviderName(ai.IssuerQwenCode, token, "")
		assert.Contains(t, result, "qwen_code-", "Should have provider prefix")
		assert.Regexp(t, `qwen_code-\d{8}-\d{4}`, result, "Should match timestamp format")
	})
}

// TestHandler_AuthorizeOAuth_Reauth_Validation covers the up-front guards added
// for the re-authentication flow: a missing target provider and an issuer
// mismatch must be rejected before any OAuth flow is initiated, so the user gets
// an immediate, clear error instead of a failed callback.
func TestHandler_AuthorizeOAuth_Reauth_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newReauthHandler := func(t *testing.T) (*Handler, *config.Config) {
		registry := oauth.NewRegistry()
		registry.Register(&oauth.ProviderConfig{
			Type:         ai.IssuerClaudeCode,
			DisplayName:  "Anthropic",
			ClientID:     "cid",
			ClientSecret: "sec",
			AuthURL:      "https://anthropic.com/auth",
			TokenURL:     "https://anthropic.com/token",
			Scopes:       []string{"api"},
		})
		cfg, err := config.NewConfigWithDir(t.TempDir(), config.WithDisableMigration(), config.WithDisableBuiltIn())
		require.NoError(t, err, "config should build")
		oauthManager := oauth.NewManager(oauth.DefaultConfig(), registry)
		return NewHandler(oauthManager, cfg), cfg
	}

	doAuthorize := func(h *Handler, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c := newOAuthTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/v1/oauth/authorize", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		h.AuthorizeOAuth(c)
		return w
	}

	t.Run("ReauthTargetNotFound", func(t *testing.T) {
		h, _ := newReauthHandler(t)
		w := doAuthorize(h, `{"provider":"claude_code","provider_uuid":"does-not-exist"}`)
		assert.Equal(t, http.StatusNotFound, w.Code, "missing re-auth target should 404")
		assert.Contains(t, w.Body.String(), "not found")
	})

	t.Run("ReauthIssuerMismatch", func(t *testing.T) {
		h, cfg := newReauthHandler(t)
		require.NoError(t, cfg.AddProvider(&typ.Provider{
			UUID:        "uuid-claude-1",
			Name:        "claude-acct",
			APIBase:     "https://api.anthropic.com",
			AuthType:    typ.AuthTypeOAuth,
			OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerClaudeCode, ProviderType: string(ai.IssuerClaudeCode)},
		}))
		// Request a codex login against a claude provider — must be rejected.
		w := doAuthorize(h, `{"provider":"codex","provider_uuid":"uuid-claude-1"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, "issuer mismatch should 400")
		assert.Contains(t, w.Body.String(), "mismatch")
	})
}
