package ai

import (
	"encoding/json"
	"testing"
)

func TestIssuerConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant Issuer
		want     string
	}{
		{"IssuerClaudeCode", IssuerClaudeCode, "claude_code"},
		{"IssuerCodex", IssuerCodex, "codex"},
		{"IssuerGitHub", IssuerGitHub, "github"},
		{"IssuerGoogle", IssuerGoogle, "google"},
		{"IssuerOpenAI", IssuerOpenAI, "openai"},
		{"IssuerGemini", IssuerGemini, "gemini"},
		{"IssuerCopilot", IssuerCopilot, "copilot"},
		{"IssuerCursor", IssuerCursor, "cursor"},
		{"IssuerKimi", IssuerKimiCode, "kimi_code"},
		{"IssuerQwen", IssuerQwenCode, "qwen_code"},
		{"IssuerAntigravity", IssuerAntigravity, "antigravity"},
		{"IssuerIFlow", IssuerIFlow, "iflow"},
		{"IssuerAnthropic", IssuerAnthropic, "anthropic"},
		{"IssuerMock", IssuerMock, "mock"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.constant); got != tt.want {
				t.Errorf("Issuer constant = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOAuthDetail_GetIssuer(t *testing.T) {
	tests := []struct {
		name   string
		detail *OAuthDetail
		want   Issuer
	}{
		{
			name:   "nil detail",
			detail: nil,
			want:   "",
		},
		{
			name:   "empty detail",
			detail: &OAuthDetail{},
			want:   "",
		},
		{
			name: "only Issuer set",
			detail: &OAuthDetail{
				Issuer: IssuerClaudeCode,
			},
			want: IssuerClaudeCode,
		},
		{
			name: "only deprecated ProviderType set",
			detail: &OAuthDetail{
				ProviderType: "codex",
			},
			want: IssuerCodex,
		},
		{
			name: "both set - Issuer takes priority",
			detail: &OAuthDetail{
				Issuer:       IssuerGitHub,
				ProviderType: "claude_code",
			},
			want: IssuerGitHub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.detail.GetIssuer(); got != tt.want {
				t.Errorf("OAuthDetail.GetIssuer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOAuthDetail_UnmarshalJSON_BackwardCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantIss Issuer
		wantPT  string
	}{
		{
			name: "old format with provider_type",
			json: `{
				"access_token": "sk-test",
				"provider_type": "claude_code",
				"user_id": "user123"
			}`,
			wantIss: IssuerClaudeCode,
			wantPT:  "claude_code",
		},
		{
			name: "new format with issuer",
			json: `{
				"access_token": "sk-test",
				"issuer": "github",
				"user_id": "user456"
			}`,
			wantIss: IssuerGitHub,
			wantPT:  "",
		},
		{
			name: "both fields - issuer takes priority",
			json: `{
				"access_token": "sk-test",
				"issuer": "codex",
				"provider_type": "claude_code",
				"user_id": "user789"
			}`,
			wantIss: IssuerCodex,
			wantPT:  "claude_code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var detail OAuthDetail
			if err := json.Unmarshal([]byte(tt.json), &detail); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if detail.Issuer != tt.wantIss {
				t.Errorf("Issuer = %v, want %v", detail.Issuer, tt.wantIss)
			}

			if detail.ProviderType != tt.wantPT {
				t.Errorf("ProviderType = %v, want %v", detail.ProviderType, tt.wantPT)
			}

			if got := detail.GetIssuer(); got != tt.wantIss {
				t.Errorf("GetIssuer() = %v, want %v", got, tt.wantIss)
			}
		})
	}
}

func TestOAuthDetail_MarshalJSON_BackwardCompatibility(t *testing.T) {
	tests := []struct {
		name         string
		detail       OAuthDetail
		wantIssuerIn bool
		wantPTIn     bool
		wantPTValue  string
	}{
		{
			name: "Issuer set",
			detail: OAuthDetail{
				AccessToken: "sk-test",
				Issuer:      IssuerClaudeCode,
				UserID:      "user123",
			},
			wantIssuerIn: true,
			wantPTIn:     true,
			wantPTValue:  "claude_code",
		},
		{
			name: "ProviderType set (deprecated)",
			detail: OAuthDetail{
				AccessToken:  "sk-test",
				ProviderType: "codex",
				UserID:       "user456",
			},
			wantIssuerIn: false,
			wantPTIn:     true,
			wantPTValue:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(&tt.detail)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			jsonStr := string(data)

			// Check issuer field
			hasIssuer := contains(jsonStr, `"issuer":`)
			if tt.wantIssuerIn && !hasIssuer {
				t.Errorf("Expected issuer field in JSON, got: %s", jsonStr)
			}

			// Check provider_type field
			hasPT := contains(jsonStr, `"provider_type":`)
			if tt.wantPTIn != hasPT {
				t.Errorf("provider_type field presence = %v, want %v", hasPT, tt.wantPTIn)
			}

			// Check provider_type value when issuer is set
			if tt.detail.Issuer != "" && tt.wantPTIn {
				expectedPT := `"provider_type":"` + string(tt.detail.Issuer) + `"`
				if !contains(jsonStr, expectedPT) {
					t.Errorf("Expected %s in JSON, got: %s", expectedPT, jsonStr)
				}
			}
		})
	}
}

func TestProvider_IsClaudeCodeProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		want     bool
	}{
		{
			name:     "nil provider",
			provider: nil,
			want:     false,
		},
		{
			name:     "no OAuth detail",
			provider: &Provider{AuthType: AuthTypeAPIKey},
			want:     false,
		},
		{
			name: "API key auth with OAuth detail",
			provider: &Provider{
				AuthType: AuthTypeAPIKey,
				OAuthDetail: &OAuthDetail{
					Issuer: IssuerClaudeCode,
				},
			},
			want: false,
		},
		{
			name: "OAuth with Claude Code issuer",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					Issuer: IssuerClaudeCode,
				},
			},
			want: true,
		},
		{
			name: "OAuth with GitHub issuer",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					Issuer: IssuerGitHub,
				},
			},
			want: false,
		},
		{
			name: "OAuth with deprecated ProviderType",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					ProviderType: "claude_code",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.IsClaudeCodeProvider(); got != tt.want {
				t.Errorf("Provider.IsClaudeCodeProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_IsCodexProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		want     bool
	}{
		{
			name: "OAuth with Codex issuer",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					Issuer: IssuerCodex,
				},
			},
			want: true,
		},
		{
			name: "OAuth with Claude Code issuer",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					Issuer: IssuerClaudeCode,
				},
			},
			want: false,
		},
		{
			name: "API base matches Codex",
			provider: &Provider{
				AuthType: AuthTypeAPIKey,
				APIBase:  CodexAPIBase,
				Token:    "sk-test",
			},
			want: true,
		},
		{
			name: "OAuth with deprecated ProviderType codex",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					ProviderType: "codex",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.IsCodexProvider(); got != tt.want {
				t.Errorf("Provider.IsCodexProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_GetAccessToken(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		want     string
	}{
		{
			name: "API key auth",
			provider: &Provider{
				AuthType: AuthTypeAPIKey,
				Token:    "sk-api-key",
			},
			want: "sk-api-key",
		},
		{
			name: "OAuth auth",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					AccessToken: "sk-oauth-token",
				},
			},
			want: "sk-oauth-token",
		},
		{
			name: "Empty auth type (defaults to API key)",
			provider: &Provider{
				Token: "sk-default",
			},
			want: "sk-default",
		},
		{
			name: "OAuth with nil detail",
			provider: &Provider{
				AuthType:    AuthTypeOAuth,
				OAuthDetail: nil,
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.GetAccessToken(); got != tt.want {
				t.Errorf("Provider.GetAccessToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOAuthDetail_IsExpired(t *testing.T) {
	tests := []struct {
		name   string
		detail *OAuthDetail
		want   bool
	}{
		{
			name:   "nil detail",
			detail: nil,
			want:   false,
		},
		{
			name:   "empty detail",
			detail: &OAuthDetail{},
			want:   false,
		},
		{
			name:   "no expires at",
			detail: &OAuthDetail{AccessToken: "sk-test"},
			want:   false,
		},
		{
			name: "empty expires at",
			detail: &OAuthDetail{
				AccessToken: "sk-test",
				ExpiresAt:   "",
			},
			want: false,
		},
		{
			name: "invalid expires at",
			detail: &OAuthDetail{
				AccessToken: "sk-test",
				ExpiresAt:   "invalid",
			},
			want: false,
		},
		{
			name: "far future expires at",
			detail: &OAuthDetail{
				AccessToken: "sk-test",
				ExpiresAt:   "2099-12-31T23:59:59Z",
			},
			want: false,
		},
		{
			name: "past expires at",
			detail: &OAuthDetail{
				AccessToken: "sk-test",
				ExpiresAt:   "2020-01-01T00:00:00Z",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.detail.IsExpired(); got != tt.want {
				t.Errorf("OAuthDetail.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_IsOAuthToken(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		want     bool
	}{
		{
			name: "Claude OAuth token prefix",
			provider: &Provider{
				Token: "sk-ant-oat123456",
			},
			want: true,
		},
		{
			name: "Standard API key",
			provider: &Provider{
				Token: "sk-1234567890",
			},
			want: false,
		},
		{
			name: "Empty token",
			provider: &Provider{
				Token: "",
			},
			want: false,
		},
		{
			name: "OAuth token with extra characters",
			provider: &Provider{
				Token: "sk-ant-oat1234567890abcdef",
			},
			want: true,
		},
		{
			name: "Similar but not OAuth prefix",
			provider: &Provider{
				Token: "sk-ant-api123456",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.IsOAuthToken(); got != tt.want {
				t.Errorf("Provider.IsOAuthToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_IsOAuthExpired(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		want     bool
	}{
		{
			name: "not OAuth auth",
			provider: &Provider{
				AuthType: AuthTypeAPIKey,
			},
			want: false,
		},
		{
			name: "OAuth with nil detail",
			provider: &Provider{
				AuthType:    AuthTypeOAuth,
				OAuthDetail: nil,
			},
			want: false,
		},
		{
			name: "OAuth with no expires at",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					AccessToken: "sk-test",
				},
			},
			want: false,
		},
		{
			name: "OAuth with past expires at",
			provider: &Provider{
				AuthType: AuthTypeOAuth,
				OAuthDetail: &OAuthDetail{
					AccessToken: "sk-test",
					ExpiresAt:   "2020-01-01T00:00:00Z",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.IsOAuthExpired(); got != tt.want {
				t.Errorf("Provider.IsOAuthExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_ResolveEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		clientStyle APIStyle
		wantURL     string
		wantStyle   APIStyle
	}{
		{
			name: "dual provider, openai client picks openai dual URL",
			provider: &Provider{
				AuthType:         AuthTypeAPIKey,
				APIBase:          "https://primary.example.com/v1",
				APIStyle:         APIStyleOpenAI,
				APIBaseOpenAI:    "https://oai.example.com/v1",
				APIBaseAnthropic: "https://ant.example.com",
			},
			clientStyle: APIStyleOpenAI,
			wantURL:     "https://oai.example.com/v1",
			wantStyle:   APIStyleOpenAI,
		},
		{
			name: "dual provider, anthropic client picks anthropic dual URL",
			provider: &Provider{
				AuthType:         AuthTypeAPIKey,
				APIBase:          "https://primary.example.com/v1",
				APIStyle:         APIStyleOpenAI,
				APIBaseOpenAI:    "https://oai.example.com/v1",
				APIBaseAnthropic: "https://ant.example.com",
			},
			clientStyle: APIStyleAnthropic,
			wantURL:     "https://ant.example.com",
			wantStyle:   APIStyleAnthropic,
		},
		{
			name: "partial dual (only openai), anthropic client falls back to legacy",
			provider: &Provider{
				AuthType:      AuthTypeAPIKey,
				APIBase:       "https://primary.example.com",
				APIStyle:      APIStyleAnthropic,
				APIBaseOpenAI: "https://oai.example.com/v1",
			},
			clientStyle: APIStyleAnthropic,
			wantURL:     "https://primary.example.com",
			wantStyle:   APIStyleAnthropic,
		},
		{
			name: "partial dual (only openai), openai client uses dual URL",
			provider: &Provider{
				AuthType:      AuthTypeAPIKey,
				APIBase:       "https://primary.example.com",
				APIStyle:      APIStyleAnthropic,
				APIBaseOpenAI: "https://oai.example.com/v1",
			},
			clientStyle: APIStyleOpenAI,
			wantURL:     "https://oai.example.com/v1",
			wantStyle:   APIStyleOpenAI,
		},
		{
			name: "no dual fields, returns legacy",
			provider: &Provider{
				AuthType: AuthTypeAPIKey,
				APIBase:  "https://primary.example.com",
				APIStyle: APIStyleOpenAI,
			},
			clientStyle: APIStyleAnthropic,
			wantURL:     "https://primary.example.com",
			wantStyle:   APIStyleOpenAI,
		},
		{
			name: "OAuth provider ignores dual fields",
			provider: &Provider{
				AuthType:         AuthTypeOAuth,
				APIBase:          "https://primary.example.com",
				APIStyle:         APIStyleAnthropic,
				APIBaseOpenAI:    "https://oai.example.com/v1",
				APIBaseAnthropic: "https://ant.example.com",
			},
			clientStyle: APIStyleOpenAI,
			wantURL:     "https://primary.example.com",
			wantStyle:   APIStyleAnthropic,
		},
		{
			name:        "nil provider returns empty",
			provider:    nil,
			clientStyle: APIStyleOpenAI,
			wantURL:     "",
			wantStyle:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotStyle := tt.provider.ResolveEndpoint(tt.clientStyle)
			if gotURL != tt.wantURL {
				t.Errorf("ResolveEndpoint URL = %q, want %q", gotURL, tt.wantURL)
			}
			if gotStyle != tt.wantStyle {
				t.Errorf("ResolveEndpoint style = %q, want %q", gotStyle, tt.wantStyle)
			}
		})
	}
}

func TestProvider_IsDual(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		want     bool
	}{
		{
			name: "both dual URLs set, api_key auth",
			provider: &Provider{
				AuthType:         AuthTypeAPIKey,
				APIBaseOpenAI:    "https://oai.example.com/v1",
				APIBaseAnthropic: "https://ant.example.com",
			},
			want: true,
		},
		{
			name: "only openai dual URL set",
			provider: &Provider{
				AuthType:      AuthTypeAPIKey,
				APIBaseOpenAI: "https://oai.example.com/v1",
			},
			want: false,
		},
		{
			name: "both dual URLs set, oauth auth",
			provider: &Provider{
				AuthType:         AuthTypeOAuth,
				APIBaseOpenAI:    "https://oai.example.com/v1",
				APIBaseAnthropic: "https://ant.example.com",
			},
			want: false,
		},
		{
			name:     "no dual fields",
			provider: &Provider{AuthType: AuthTypeAPIKey, APIBase: "https://primary.example.com"},
			want:     false,
		},
		{
			name:     "nil provider",
			provider: nil,
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.IsDual(); got != tt.want {
				t.Errorf("IsDual() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
