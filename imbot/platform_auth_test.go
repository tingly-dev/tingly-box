package imbot

import "testing"

// TestBuildAuthConfigLark is the regression test for a defect the table
// replaced: "lark" was present in PlatformConfigs but missing from both
// hand-written switches in the bot manager, so Lark bots were rejected as
// having no valid credentials, and would have been handed a token-type auth
// config that the Feishu client rejects even if they had got past that.
func TestBuildAuthConfigLark(t *testing.T) {
	auth := map[string]string{"clientId": "cli_x", "clientSecret": "sec"}

	cfg := BuildAuthConfig("lark", auth)
	if cfg.Type != "oauth" {
		t.Errorf("Type = %q, want oauth — the Feishu client rejects anything else", cfg.Type)
	}
	if cfg.ClientID != "cli_x" || cfg.ClientSecret != "sec" {
		t.Errorf("credentials not mapped: %+v", cfg)
	}
	if missing := MissingAuthKeys("lark", auth); len(missing) != 0 {
		t.Errorf("a fully configured Lark bot reports missing keys: %v", missing)
	}
}

func TestBuildAuthConfigPerPlatform(t *testing.T) {
	tests := []struct {
		platform string
		auth     map[string]string
		wantType string
		check    func(*testing.T, AuthConfig)
	}{
		{
			platform: "telegram",
			auth:     map[string]string{"token": "t"},
			wantType: "token",
			check: func(t *testing.T, c AuthConfig) {
				if c.Token != "t" {
					t.Errorf("Token = %q", c.Token)
				}
			},
		},
		{
			platform: "feishu",
			auth:     map[string]string{"clientId": "id", "clientSecret": "sec"},
			wantType: "oauth",
			check: func(t *testing.T, c AuthConfig) {
				if c.ClientID != "id" || c.ClientSecret != "sec" {
					t.Errorf("oauth fields not mapped: %+v", c)
				}
			},
		},
		{
			platform: "whatsapp",
			auth:     map[string]string{"token": "t", "phoneNumberId": "p"},
			wantType: "token",
			check: func(t *testing.T, c AuthConfig) {
				if c.AccountID != "p" {
					t.Errorf("AccountID = %q, want the phone number id", c.AccountID)
				}
			},
		},
		{
			platform: "weixin",
			auth:     map[string]string{"token": "t", "bot_id": "b", "user_id": "u", "base_url": "https://x"},
			wantType: "qr",
			check: func(t *testing.T, c AuthConfig) {
				if c.AccountID != "b" {
					t.Errorf("AccountID = %q, want bot_id", c.AccountID)
				}
				if c.AuthDir != "u" {
					t.Errorf("AuthDir = %q, want user_id — Weixin reuses this field", c.AuthDir)
				}
			},
		},
		{
			platform: "tingly",
			auth:     map[string]string{},
			wantType: "none",
			check:    func(t *testing.T, c AuthConfig) {},
		},
		{
			// A platform with no table entry falls back to a bot token, which
			// is the least surprising guess for a new IM platform.
			platform: "some-future-platform",
			auth:     map[string]string{"token": "t"},
			wantType: "token",
			check: func(t *testing.T, c AuthConfig) {
				if c.Token != "t" {
					t.Errorf("Token = %q", c.Token)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			cfg := BuildAuthConfig(tt.platform, tt.auth)
			if cfg.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", cfg.Type, tt.wantType)
			}
			tt.check(t, cfg)
			if missing := MissingAuthKeys(tt.platform, tt.auth); len(missing) != 0 {
				t.Errorf("unexpected missing keys: %v", missing)
			}
		})
	}
}

// TestMissingAuthKeysNamesThem covers the operator-facing half: a bot that
// cannot start should say which credential it lacks, not just "no valid
// credentials".
func TestMissingAuthKeysNamesThem(t *testing.T) {
	missing := MissingAuthKeys("feishu", map[string]string{"clientId": "id"})
	if len(missing) != 1 || missing[0] != "clientSecret" {
		t.Errorf("missing = %v, want [clientSecret]", missing)
	}

	if missing := MissingAuthKeys("tingly", nil); len(missing) != 0 {
		t.Errorf("tingly needs no credentials, got %v", missing)
	}
}

// TestAuthOptionsWeixin covers credentials that travel as connection options
// rather than as auth fields.
func TestAuthOptionsWeixin(t *testing.T) {
	opts := AuthOptions("weixin", map[string]string{
		"token": "t", "bot_id": "b", "user_id": "u", "base_url": "https://x",
	})
	if opts["user_id"] != "u" || opts["base_url"] != "https://x" {
		t.Errorf("weixin options = %v", opts)
	}
	if _, leaked := opts["token"]; leaked {
		t.Error("token belongs in AuthConfig, not in connection options")
	}

	if opts := AuthOptions("telegram", map[string]string{"token": "t"}); opts != nil {
		t.Errorf("telegram has no option-carried credentials, got %v", opts)
	}
}

// TestAuthMappingCoversEveryConfiguredPlatform guards the table against the
// exact drift that broke Lark: an entry that describes a form but never says
// how those fields reach the client.
func TestAuthMappingCoversEveryConfiguredPlatform(t *testing.T) {
	for id, cfg := range PlatformConfigs {
		if cfg.Auth.Type == "" {
			t.Errorf("platform %q has no auth mapping", id)
			continue
		}
		if cfg.Auth.Type != cfg.AuthType {
			t.Errorf("platform %q: auth mapping type %q disagrees with form AuthType %q",
				id, cfg.Auth.Type, cfg.AuthType)
		}
	}
}
