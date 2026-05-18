package agent

import (
	"encoding/json"
	"testing"
)

func TestClaudeCodePrefs_ToEnv_OmitsEmpty(t *testing.T) {
	// Sparse prefs: only one model + one limit, everything else empty.
	p := ClaudeCodePrefs{
		AnthropicModel: "tingly/cc",
		APITimeoutMs:   "60000",
	}
	env, err := p.ToEnv("http://localhost:12580", "tok")
	if err != nil {
		t.Fatalf("ToEnv: %v", err)
	}

	mustEq(t, env, "ANTHROPIC_MODEL", "tingly/cc")
	mustEq(t, env, "API_TIMEOUT_MS", "60000")
	mustEq(t, env, "ANTHROPIC_BASE_URL", "http://localhost:12580/tingly/claude_code")
	mustEq(t, env, "ANTHROPIC_AUTH_TOKEN", "tok")

	for _, k := range []string{
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS",
		"DISABLE_TELEMETRY",
		"HTTP_PROXY",
	} {
		if _, ok := env[k]; ok {
			t.Errorf("expected %s to be omitted, got %q", k, env[k])
		}
	}
}

func TestClaudeCodePrefs_ToEnv_StripsTrailingSlashOnBaseURL(t *testing.T) {
	env, _ := ClaudeCodePrefs{}.ToEnv("http://localhost:12580/", "tok")
	mustEq(t, env, "ANTHROPIC_BASE_URL", "http://localhost:12580/tingly/claude_code")
}

func TestClaudeCodePrefs_ToEnv_ExtraMerges(t *testing.T) {
	p := ClaudeCodePrefs{
		AnthropicModel: "tingly/cc",
		Extra: map[string]string{
			"SOME_NEW_ENV":   "value",
			"ANTHROPIC_MODEL": "override-via-extra",
		},
	}
	env, _ := p.ToEnv("http://localhost", "tok")
	mustEq(t, env, "SOME_NEW_ENV", "value")
	mustEq(t, env, "ANTHROPIC_MODEL", "override-via-extra")
}

func TestClaudeCodePrefs_ToEnv_OneMillionSuffixPassesThrough(t *testing.T) {
	// 1M is part of the model string. ToEnv doesn't synthesize it — the
	// caller (UI) is responsible for appending [1m] before sending prefs.
	p := ClaudeCodePrefs{
		AnthropicDefaultSonnetModel: "tingly/cc-sonnet[1m]",
	}
	env, _ := p.ToEnv("http://localhost", "tok")
	mustEq(t, env, "ANTHROPIC_DEFAULT_SONNET_MODEL", "tingly/cc-sonnet[1m]")
}

func TestDefaultClaudeCodePrefs_Unified_MatchesLegacyEnv(t *testing.T) {
	prefsEnv, err := DefaultClaudeCodePrefs(true).ToEnv("http://localhost:12580", "test-token")
	if err != nil {
		t.Fatalf("ToEnv: %v", err)
	}
	legacy := BuildClaudeCodeEnv("http://localhost:12580", "test-token", true)
	assertEnvMapsEqual(t, legacy, prefsEnv)
}

func TestDefaultClaudeCodePrefs_Separate_MatchesLegacyEnv(t *testing.T) {
	prefsEnv, _ := DefaultClaudeCodePrefs(false).ToEnv("http://localhost:12580", "test-token")
	legacy := BuildClaudeCodeEnv("http://localhost:12580", "test-token", false)
	assertEnvMapsEqual(t, legacy, prefsEnv)
}

func TestClaudeCodePrefs_JSONShapeUsesEnvNames(t *testing.T) {
	b, _ := json.Marshal(ClaudeCodePrefs{
		AnthropicDefaultHaikuModel: "tingly/cc-haiku",
		DisableTelemetry:           "1",
	})
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mustEq(t, m, "ANTHROPIC_DEFAULT_HAIKU_MODEL", "tingly/cc-haiku")
	mustEq(t, m, "DISABLE_TELEMETRY", "1")
	if _, ok := m["AnthropicDefaultHaikuModel"]; ok {
		t.Error("Go field name leaked into JSON output")
	}
}

func mustEq(t *testing.T, env map[string]string, key, want string) {
	t.Helper()
	if got := env[key]; got != want {
		t.Errorf("env[%s] = %q, want %q", key, got, want)
	}
}

func assertEnvMapsEqual(t *testing.T, want, got map[string]string) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("env[%s] = %q, want %q", k, got[k], v)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected key %s = %q in prefs env", k, got[k])
		}
	}
}
