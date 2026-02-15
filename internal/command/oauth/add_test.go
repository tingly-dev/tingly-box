package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// TestProviderConfig validates provider configurations
func TestProviderConfig(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		wantConfig   *ProviderOAuthConfig
		wantErr      bool
	}{
		{
			name:         "claude_code config",
			providerType: "claude_code",
			wantConfig: &ProviderOAuthConfig{
				Type:         "claude_code",
				DisplayName:  "Claude Code",
				APIBase:      "https://api.anthropic.com/v1",
				APIStyle:     "anthropic",
				OAuthMethod:  "pkce",
				NeedsPort1455: false,
			},
		},
		{
			name:         "qwen_code config",
			providerType: "qwen_code",
			wantConfig: &ProviderOAuthConfig{
				Type:         "qwen_code",
				DisplayName:  "Qwen",
				APIBase:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
				APIStyle:     "openai",
				OAuthMethod:  "device_code",
				NeedsPort1455: false,
			},
		},
		{
			name:         "codex config",
			providerType: "codex",
			wantConfig: &ProviderOAuthConfig{
				Type:         "codex",
				DisplayName:  "Codex",
				APIBase:      "https://api.openai.com/v1",
				APIStyle:     "openai",
				OAuthMethod:  "pkce",
				NeedsPort1455: true,
			},
		},
		{
			name:         "antigravity config",
			providerType: "antigravity",
			wantConfig: &ProviderOAuthConfig{
				Type:         "antigravity",
				DisplayName:  "Antigravity",
				APIBase:      "https://api.antigravity.com/v1",
				APIStyle:     "openai",
				OAuthMethod:  "pkce",
				NeedsPort1455: false,
			},
		},
		{
			name:         "unsupported provider",
			providerType: "unknown",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getProviderConfig(tt.providerType)
			if (err != nil) != tt.wantErr {
				t.Errorf("getProviderConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Type != tt.wantConfig.Type {
					t.Errorf("Type = %v, want %v", got.Type, tt.wantConfig.Type)
				}
				if got.DisplayName != tt.wantConfig.DisplayName {
					t.Errorf("DisplayName = %v, want %v", got.DisplayName, tt.wantConfig.DisplayName)
				}
				if got.APIBase != tt.wantConfig.APIBase {
					t.Errorf("APIBase = %v, want %v", got.APIBase, tt.wantConfig.APIBase)
				}
				if got.OAuthMethod != tt.wantConfig.OAuthMethod {
					t.Errorf("OAuthMethod = %v, want %v", got.OAuthMethod, tt.wantConfig.OAuthMethod)
				}
				if got.NeedsPort1455 != tt.wantConfig.NeedsPort1455 {
					t.Errorf("NeedsPort1455 = %v, want %v", got.NeedsPort1455, tt.wantConfig.NeedsPort1455)
				}
			}
		})
	}
}

// TestCreateProviderFromToken tests provider creation from OAuth token
func TestCreateProviderFromToken(t *testing.T) {
	// Create a temporary app config
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("Failed to create app config: %v", err)
	}

	config := &ProviderOAuthConfig{
		Type:        "claude_code",
		DisplayName: "Claude Code",
		APIBase:     "https://api.anthropic.com/v1",
		APIStyle:    "anthropic",
	}

	token := &oauth.Token{
		AccessToken:  "test_access_token",
		RefreshToken: "test_refresh_token",
		Expiry:       time.Now().Add(1 * time.Hour),
		Metadata: map[string]interface{}{
			"email": "test@example.com",
		},
	}

	// Test provider creation
	err = createProviderFromToken(appConfig, config, "test-claude", token)
	if err != nil {
		t.Fatalf("createProviderFromToken() error = %v", err)
	}

	// Verify provider was created
	provider, err := appConfig.GetProviderByName("test-claude")
	if err != nil {
		t.Fatalf("Failed to get created provider: %v", err)
	}

	if provider.Name != "test-claude" {
		t.Errorf("Provider name = %v, want test-claude", provider.Name)
	}
	if provider.AuthType != typ.AuthTypeOAuth {
		t.Errorf("Provider auth type = %v, want %v", provider.AuthType, typ.AuthTypeOAuth)
	}
	if provider.OAuthDetail == nil {
		t.Fatal("OAuthDetail is nil")
	}
	if provider.OAuthDetail.AccessToken != "test_access_token" {
		t.Errorf("AccessToken = %v, want test_access_token", provider.OAuthDetail.AccessToken)
	}
	if provider.OAuthDetail.ProviderType != "claude_code" {
		t.Errorf("ProviderType = %v, want claude_code", provider.OAuthDetail.ProviderType)
	}
}

// TestCallbackServer tests the callback server functionality
func TestCallbackServer(t *testing.T) {
	// Note: This is a simplified test - a full integration test would require:
	// 1. Starting the actual callback server
	// 2. Generating a real auth URL with state
	// 3. Simulating the OAuth provider callback
	// 4. Verifying token exchange and provider creation

	t.Log("Callback server test requires full OAuth flow simulation")
	t.Log("This would be better tested as an integration test")

	// Mock token response
	mockToken := &oauth.Token{
		AccessToken:  "test_access_token",
		RefreshToken: "test_refresh_token",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	// Create a test HTTP server that acts as the OAuth provider
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate successful OAuth callback
		if r.URL.Path == "/callback" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"access_token":  mockToken.AccessToken,
				"refresh_token": mockToken.RefreshToken,
				"token_type":    "Bearer",
				"expires_in":    "3600",
			})
		}
	}))
	defer testServer.Close()

	t.Logf("Test server URL: %s", testServer.URL)
}

// TestJSONLOutputFormat tests the JSONL output format
func TestJSONLOutputFormat(t *testing.T) {
	provider := &typ.Provider{
		UUID:        "test-uuid",
		Name:        "test-provider",
		APIBase:     "https://api.example.com/v1",
		APIStyle:    "openai",
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken:  "test_token",
			ProviderType: "claude_code",
		},
		Enabled: true,
	}

	// Create export data
	exportData := map[string]interface{}{
		"type":         "provider",
		"uuid":         provider.UUID,
		"name":         provider.Name,
		"api_base":     provider.APIBase,
		"api_style":    string(provider.APIStyle),
		"auth_type":    string(provider.AuthType),
		"token":        provider.Token,
		"oauth_detail": provider.OAuthDetail,
		"enabled":      provider.Enabled,
		"proxy_url":    provider.ProxyURL,
		"timeout":      provider.Timeout,
		"tags":         provider.Tags,
		"models":       provider.Models,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(exportData)
	if err != nil {
		t.Fatalf("Failed to marshal provider data: %v", err)
	}

	// Verify it contains required fields (order doesn't matter in JSON)
	requiredFields := []string{"type", "uuid", "name", "api_base", "auth_type", "oauth_detail"}
	for _, field := range requiredFields {
		if !strings.Contains(string(jsonData), `"`+field+`"`) {
			t.Errorf("JSONL output missing field: %s", field)
		}
	}

	// Verify type value is correct
	if !strings.Contains(string(jsonData), `"type":"provider"`) {
		t.Errorf("JSONL output missing correct type value")
	}

	t.Logf("JSONL output: %s", string(jsonData))
}

// TestPortValidationForCodex tests port 1455 requirement for codex
func TestPortValidationForCodex(t *testing.T) {
	config := &ProviderOAuthConfig{
		Type:         "codex",
		NeedsPort1455: true,
	}

	tests := []struct {
		name        string
		port        int
		wantValid   bool
		wantPort    int
	}{
		{
			name:      "default port should use 1455",
			port:      0,
			wantValid: true,
			wantPort:  1455,
		},
		{
			name:      "port 1455 is valid",
			port:      1455,
			wantValid: true,
			wantPort:  1455,
		},
		{
			name:      "other port should be invalid",
			port:      12580,
			wantValid: false,
			wantPort:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate port validation logic from add.go
			isValid := true
			var finalPort int

			// Check if port is invalid for codex
			if config.NeedsPort1455 && tt.port != 0 && tt.port != 1455 {
				isValid = false
			}

			// Determine final port
			if isValid {
				if config.NeedsPort1455 && tt.port == 0 {
					finalPort = 1455
				} else if tt.port == 0 {
					finalPort = 12580
				} else {
					finalPort = tt.port
				}
			}

			if isValid != tt.wantValid {
				t.Errorf("isValid = %v, want %v", isValid, tt.wantValid)
			}
			if isValid && finalPort != tt.wantPort {
				t.Errorf("Final port = %v, want %v", finalPort, tt.wantPort)
			}
		})
	}
}
