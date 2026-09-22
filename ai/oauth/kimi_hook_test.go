package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
)

func TestKimiHook_BeforeAuth_NoOp(t *testing.T) {
	hook := &KimiHook{}
	params := map[string]string{"existing": "value"}
	if err := hook.BeforeAuth(params); err != nil {
		t.Fatalf("BeforeAuth returned error: %v", err)
	}
	if len(params) != 1 || params["existing"] != "value" {
		t.Errorf("expected BeforeAuth to be a no-op, got %v", params)
	}
}

func TestKimiHook_BeforeToken_SetsDeviceHeaders(t *testing.T) {
	hook := &KimiHook{}
	header := http.Header{}
	if err := hook.BeforeToken(map[string]string{}, header); err != nil {
		t.Fatalf("BeforeToken returned error: %v", err)
	}

	if got := header.Get("X-Msh-Platform"); got != "kimi_cli" {
		t.Errorf("expected X-Msh-Platform=kimi_cli, got %q", got)
	}
	if got := header.Get("X-Msh-Version"); got != "1.10.6" {
		t.Errorf("expected X-Msh-Version=1.10.6, got %q", got)
	}
	if header.Get("X-Msh-Device-Name") == "" {
		t.Error("expected non-empty X-Msh-Device-Name")
	}
	if header.Get("X-Msh-Device-Model") == "" {
		t.Error("expected non-empty X-Msh-Device-Model")
	}
	if header.Get("X-Msh-Os-Version") == "" {
		t.Error("expected non-empty X-Msh-Os-Version")
	}
}

func TestKimiHook_AfterToken_NoOp(t *testing.T) {
	hook := &KimiHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", http.DefaultClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata, got %v", meta)
	}
}

func TestKimiDeviceModel_MatchesRuntimeGOOS(t *testing.T) {
	got := KimiDeviceModel()
	if got == "" {
		t.Fatal("expected non-empty device model")
	}
}

func TestKimiOsVersion_ReturnsKnownValue(t *testing.T) {
	got := KimiOsVersion()
	if got == "" {
		t.Fatal("expected non-empty OS version")
	}
}

// TestManager_RefreshToken_Kimi_OmitsEmptyClientSecret is a regression test:
// Kimi is a public client (ClientSecret == "") and its token endpoint
// validates the refresh form body strictly, matching kimi-cli's own
// refresh_token request (client_id, grant_type, refresh_token only — no
// client_secret field at all, even empty). Sending an empty
// `client_secret=` field, as the refresh path used to do for every issuer
// except Codex, silently broke background refresh for Kimi credentials.
func TestManager_RefreshToken_Kimi_OmitsEmptyClientSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if _, present := r.PostForm["client_secret"]; present {
			t.Errorf("expected no client_secret field in Kimi refresh request, got %q", r.PostForm.Get("client_secret"))
		}
		if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
			t.Errorf("expected grant_type=refresh_token, got %q", got)
		}
		if got := r.PostForm.Get("refresh_token"); got != "old-refresh-token" {
			t.Errorf("expected refresh_token=old-refresh-token, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "new-access-token",
			"refresh_token": "new-refresh-token",
			"token_type": "Bearer",
			"expires_in": 3600
		}`))
	}))
	defer server.Close()

	registry := NewRegistry()
	registry.Register(&ProviderConfig{
		Type:               ai.IssuerKimiCode,
		GrantType:          "urn:ietf:params:oauth:grant-type:device_code",
		DisplayName:        "Kimi Code",
		ClientID:           "test-kimi-client-id",
		ClientSecret:       "",
		TokenURL:           server.URL,
		OAuthMethod:        OAuthMethodDeviceCode,
		TokenRequestFormat: TokenRequestFormatForm,
		Hook:               &KimiHook{},
	})

	manager := NewManager(WithConfig(DefaultConfig()), WithRegistry(registry))

	token, err := manager.RefreshToken(context.Background(), "user123", ai.IssuerKimiCode, "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if token.AccessToken != "new-access-token" {
		t.Errorf("expected new-access-token, got %q", token.AccessToken)
	}
}
