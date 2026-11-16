package db

import "testing"

// legacyImBotSettingsRecord is ImBotSettingsRecord as it was before
// auth_config and bash_allowlist moved to `serializer:json`: both were
// `string` columns holding hand-marshalled JSON, with "" meaning absent.
type legacyImBotSettingsRecord struct {
	BotUUID       string `gorm:"primaryKey;column:bot_uuid"`
	Name          string `gorm:"column:name"`
	Platform      string `gorm:"column:platform;index:idx_platform"`
	AuthType      string `gorm:"column:auth_type"`
	AuthConfig    string `gorm:"column:auth_config;type:text"`
	BashAllowlist string `gorm:"column:bash_allowlist;type:text"`
	Enabled       bool   `gorm:"column:enabled;index:idx_enabled"`
}

func (legacyImBotSettingsRecord) TableName() string { return "imbot_settings" }

func TestImBotSettingsStore_ReadsLegacyJSONStringRows(t *testing.T) {
	dir := t.TempDir()

	seedLegacyRows(t, dir, []legacyImBotSettingsRecord{
		{
			BotUUID: "full", Name: "full", Platform: "telegram", AuthType: "token",
			AuthConfig:    `{"token":"legacy-token","secret":"s3cr3t"}`,
			BashAllowlist: `["ls","cat"]`,
			Enabled:       true,
		},
		{
			// Both JSON columns in the legacy "absent" encoding.
			BotUUID: "bare", Name: "bare", Platform: "weixin",
			AuthConfig: "", BashAllowlist: "", Enabled: false,
		},
	})

	store, err := NewImBotSettingsStore(dir)
	if err != nil {
		t.Fatalf("NewImBotSettingsStore over legacy db: %v", err)
	}
	defer store.Close()

	t.Run("decodes legacy auth", func(t *testing.T) {
		got, err := store.GetSettingsByUUID("full")
		if err != nil {
			t.Fatalf("GetSettingsByUUID: %v", err)
		}
		if got.Auth["token"] != "legacy-token" || got.Auth["secret"] != "s3cr3t" {
			t.Errorf("Auth = %v, want token/secret", got.Auth)
		}
		// Token is mirrored out of Auth for backward compatibility.
		if got.Token != "legacy-token" {
			t.Errorf("Token = %q, want legacy-token", got.Token)
		}
	})

	t.Run("empty legacy columns read as absent, Auth stays non-nil", func(t *testing.T) {
		got, err := store.GetSettingsByUUID("bare")
		if err != nil {
			t.Fatalf("GetSettingsByUUID: %v", err)
		}
		if got.Auth == nil {
			t.Fatal("Auth = nil; callers index and assign into it")
		}
		if len(got.Auth) != 0 {
			t.Errorf("Auth = %v, want empty", got.Auth)
		}
	})

	t.Run("ListSettings decodes every row", func(t *testing.T) {
		all, err := store.ListSettings()
		if err != nil {
			t.Fatalf("ListSettings: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("ListSettings returned %d, want 2", len(all))
		}
		for _, s := range all {
			if s.Auth == nil {
				t.Errorf("%s: Auth = nil", s.UUID)
			}
		}
	})
}

func TestImBotSettingsStore_RoundTrip(t *testing.T) {
	store, err := NewImBotSettingsStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewImBotSettingsStore: %v", err)
	}
	defer store.Close()

	created, err := store.CreateSettings(Settings{
		Name: "rt", Platform: "telegram", AuthType: "token",
		Auth:    map[string]string{"token": "abc"},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSettings: %v", err)
	}

	got, err := store.GetSettingsByUUID(created.UUID)
	if err != nil {
		t.Fatalf("GetSettingsByUUID: %v", err)
	}
	if got.Auth["token"] != "abc" {
		t.Errorf("Auth = %v, want token=abc", got.Auth)
	}

	if err := store.UpdateSettings("no-such-uuid", Settings{Name: "x"}); err == nil {
		t.Error("UpdateSettings on a missing row returned nil, want an error")
	}
}

// TestImBotSettingsStore_UpdateIsPartial pins the rule UpdateSettings has
// always documented -- an empty/zero field leaves the stored value alone --
// across the move from Updates(map) to read-modify-write.
func TestImBotSettingsStore_UpdateIsPartial(t *testing.T) {
	store, err := NewImBotSettingsStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewImBotSettingsStore: %v", err)
	}
	defer store.Close()

	created, err := store.CreateSettings(Settings{
		Name: "orig", Platform: "telegram", AuthType: "token",
		Auth:         map[string]string{"token": "orig-token"},
		ProxyURL:     "http://proxy:8080",
		DefaultCwd:   "/orig",
		DefaultAgent: "agent-1",
		Scenarios:    `[{"scenario":"a"}]`,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("CreateSettings: %v", err)
	}

	// Update only the name; everything else is zero and must survive.
	if err := store.UpdateSettings(created.UUID, Settings{Name: "renamed", Enabled: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	got, err := store.GetSettingsByUUID(created.UUID)
	if err != nil {
		t.Fatalf("GetSettingsByUUID: %v", err)
	}
	if got.Name != "renamed" {
		t.Errorf("Name = %q, want renamed", got.Name)
	}
	if got.Auth["token"] != "orig-token" {
		t.Errorf("Auth = %v, want the original token preserved", got.Auth)
	}
	if got.ProxyURL != "http://proxy:8080" {
		t.Errorf("ProxyURL = %q, want preserved", got.ProxyURL)
	}
	if got.DefaultCwd != "/orig" || got.DefaultAgent != "agent-1" {
		t.Errorf("defaults not preserved: cwd=%q agent=%q", got.DefaultCwd, got.DefaultAgent)
	}
	if got.Platform != "telegram" || got.AuthType != "token" {
		t.Errorf("platform/authtype not preserved: %q/%q", got.Platform, got.AuthType)
	}

	t.Run("an unset Scenarios clears the stored value (the documented exception)", func(t *testing.T) {
		if err := store.UpdateSettings(created.UUID, Settings{Enabled: true}); err != nil {
			t.Fatalf("UpdateSettings: %v", err)
		}
		got, err := store.GetSettingsByUUID(created.UUID)
		if err != nil {
			t.Fatalf("GetSettingsByUUID: %v", err)
		}
		if got.Scenarios != "" {
			t.Errorf("Scenarios = %q, want cleared", got.Scenarios)
		}
	})

	t.Run("replacing auth works", func(t *testing.T) {
		if err := store.UpdateSettings(created.UUID, Settings{
			Auth:    map[string]string{"token": "new-token"},
			Enabled: true,
		}); err != nil {
			t.Fatalf("UpdateSettings: %v", err)
		}
		got, err := store.GetSettingsByUUID(created.UUID)
		if err != nil {
			t.Fatalf("GetSettingsByUUID: %v", err)
		}
		if got.Auth["token"] != "new-token" {
			t.Errorf("Auth = %v, want new-token", got.Auth)
		}
	})

	t.Run("columns Settings does not model survive an update", func(t *testing.T) {
		// debug exists on the record but not on Settings. The old
		// Updates(map) path never mentioned it; read-modify-write must not
		// reset it either.
		if err := store.db.Exec(
			"UPDATE imbot_settings SET debug = 1 WHERE bot_uuid = ?",
			created.UUID).Error; err != nil {
			t.Fatalf("seed debug: %v", err)
		}
		if err := store.UpdateSettings(created.UUID, Settings{Name: "again", Enabled: true}); err != nil {
			t.Fatalf("UpdateSettings: %v", err)
		}
		var rec ImBotSettingsRecord
		if err := store.db.Where("bot_uuid = ?", created.UUID).First(&rec).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if !rec.Debug {
			t.Error("debug was reset by UpdateSettings, want preserved")
		}
	})
}
