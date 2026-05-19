package agent

import (
	"encoding/json"
	"reflect"
	"strings"
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

// Empty prefs must still produce a usable env — the server-injected base
// URL and auth token are non-negotiable for Claude Code to function at all.
func TestClaudeCodePrefs_ToEnv_EmptyOnlyInjectsServerKeys(t *testing.T) {
	env, err := ClaudeCodePrefs{}.ToEnv("http://localhost:12580", "tok")
	if err != nil {
		t.Fatalf("ToEnv: %v", err)
	}
	if len(env) != 2 {
		t.Errorf("expected exactly 2 server-injected keys, got %d: %v", len(env), env)
	}
	mustEq(t, env, "ANTHROPIC_BASE_URL", "http://localhost:12580/tingly/claude_code")
	mustEq(t, env, "ANTHROPIC_AUTH_TOKEN", "tok")
}

// A fully-populated prefs should emit every typed env key. Catches future
// regressions where a new field is added without its JSON tag.
func TestClaudeCodePrefs_ToEnv_FullForm(t *testing.T) {
	p := ClaudeCodePrefs{
		AnthropicModel:                       "m-default",
		AnthropicDefaultHaikuModel:           "m-haiku",
		AnthropicDefaultSonnetModel:          "m-sonnet",
		AnthropicDefaultOpusModel:            "m-opus",
		ClaudeCodeSubagentModel:              "m-subagent",
		APITimeoutMs:                         "1",
		ClaudeCodeMaxOutputTokens:            "2",
		MaxThinkingTokens:                    "3",
		BashDefaultTimeoutMs:                 "4",
		BashMaxTimeoutMs:                     "5",
		McpTimeout:                           "6",
		McpToolTimeout:                       "7",
		MaxMcpOutputTokens:                   "8",
		DisableTelemetry:                     "1",
		DisableErrorReporting:                "1",
		ClaudeCodeDisableNonessentialTraffic: "1",
		DisableAutoupdater:                   "1",
		UseBuiltinRipgrep:                    "1",
		HTTPProxy:                            "http://proxy:8080",
		HTTPSProxy:                           "http://proxy:8443",
		NoProxy:                              "localhost",
	}
	env, err := p.ToEnv("http://localhost", "tok")
	if err != nil {
		t.Fatalf("ToEnv: %v", err)
	}

	wantKeys := []string{
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"CLAUDE_CODE_SUBAGENT_MODEL",
		"API_TIMEOUT_MS",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS",
		"MAX_THINKING_TOKENS",
		"BASH_DEFAULT_TIMEOUT_MS",
		"BASH_MAX_TIMEOUT_MS",
		"MCP_TIMEOUT",
		"MCP_TOOL_TIMEOUT",
		"MAX_MCP_OUTPUT_TOKENS",
		"DISABLE_TELEMETRY",
		"DISABLE_ERROR_REPORTING",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC",
		"DISABLE_AUTOUPDATER",
		"USE_BUILTIN_RIPGREP",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"NO_PROXY",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_AUTH_TOKEN",
	}
	for _, k := range wantKeys {
		if _, ok := env[k]; !ok {
			t.Errorf("missing key %s in env", k)
		}
	}
	if len(env) != len(wantKeys) {
		t.Errorf("env has %d keys, want %d. extra: %v", len(env), len(wantKeys), diffKeys(env, wantKeys))
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
			"SOME_NEW_ENV":    "value",
			"ANTHROPIC_MODEL": "override-via-extra",
		},
	}
	env, _ := p.ToEnv("http://localhost", "tok")
	mustEq(t, env, "SOME_NEW_ENV", "value")
	mustEq(t, env, "ANTHROPIC_MODEL", "override-via-extra")
}

// Even if Extra carries reserved server keys, the server-injected values
// must win — otherwise a misconfigured prefs blob could redirect the proxy
// URL or fake the auth token.
func TestClaudeCodePrefs_ToEnv_ExtraCannotOverrideServerKeys(t *testing.T) {
	p := ClaudeCodePrefs{
		Extra: map[string]string{
			"ANTHROPIC_BASE_URL":   "http://evil.example.com",
			"ANTHROPIC_AUTH_TOKEN": "leaked-token",
		},
	}
	env, _ := p.ToEnv("http://localhost:12580", "real-token")
	mustEq(t, env, "ANTHROPIC_BASE_URL", "http://localhost:12580/tingly/claude_code")
	mustEq(t, env, "ANTHROPIC_AUTH_TOKEN", "real-token")
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

// Round-trip: a prefs JSON payload written by the frontend (env-name keys)
// must deserialize into the typed struct without losing any field.
func TestClaudeCodePrefs_UnmarshalsEnvNameKeys(t *testing.T) {
	payload := `{
		"ANTHROPIC_MODEL": "m1",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": "m-haiku",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "m-sonnet[1m]",
		"ANTHROPIC_DEFAULT_OPUS_MODEL": "m-opus",
		"CLAUDE_CODE_SUBAGENT_MODEL": "m-sub",
		"API_TIMEOUT_MS": "3000000",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32000",
		"DISABLE_TELEMETRY": "1"
	}`

	var p ClaudeCodePrefs
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.AnthropicModel != "m1" {
		t.Errorf("AnthropicModel = %q", p.AnthropicModel)
	}
	if p.AnthropicDefaultSonnetModel != "m-sonnet[1m]" {
		t.Errorf("AnthropicDefaultSonnetModel = %q", p.AnthropicDefaultSonnetModel)
	}
	if p.ClaudeCodeSubagentModel != "m-sub" {
		t.Errorf("ClaudeCodeSubagentModel = %q", p.ClaudeCodeSubagentModel)
	}
	if p.APITimeoutMs != "3000000" {
		t.Errorf("APITimeoutMs = %q", p.APITimeoutMs)
	}
	if p.DisableTelemetry != "1" {
		t.Errorf("DisableTelemetry = %q", p.DisableTelemetry)
	}
}

// Marshal → Unmarshal round-trip preserves every set field. Catches the
// failure mode where a new Go field is added without a json tag (or with a
// typo) — the data would silently round-trip-loss otherwise.
func TestClaudeCodePrefs_JSONRoundTripPreservesFields(t *testing.T) {
	in := ClaudeCodePrefs{
		AnthropicModel:                       "x",
		AnthropicDefaultHaikuModel:           "x-haiku",
		AnthropicDefaultSonnetModel:          "x-sonnet",
		AnthropicDefaultOpusModel:            "x-opus",
		ClaudeCodeSubagentModel:              "x-sub",
		APITimeoutMs:                         "1",
		ClaudeCodeMaxOutputTokens:            "2",
		MaxThinkingTokens:                    "3",
		BashDefaultTimeoutMs:                 "4",
		BashMaxTimeoutMs:                     "5",
		McpTimeout:                           "6",
		McpToolTimeout:                       "7",
		MaxMcpOutputTokens:                   "8",
		DisableTelemetry:                     "1",
		DisableErrorReporting:                "1",
		ClaudeCodeDisableNonessentialTraffic: "1",
		DisableAutoupdater:                   "1",
		UseBuiltinRipgrep:                    "1",
		HTTPProxy:                            "http://p",
		HTTPSProxy:                           "https://p",
		NoProxy:                              "localhost",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ClaudeCodePrefs
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Extra is never serialized (json:"-"), so the input had it nil; output
	// is also nil. Comparing the rest with reflect.DeepEqual is safe.
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch:\nin:  %+v\nout: %+v", in, out)
	}
}

// Sanity check: every Go field that maps to an env should use omitempty.
// Without it, the marshaled map would contain "FOO":"" for unset fields
// and ToEnv would write spurious blank envs into settings.json.
func TestClaudeCodePrefs_AllTypedFieldsUseOmitempty(t *testing.T) {
	rt := reflect.TypeOf(ClaudeCodePrefs{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue // Extra map (json:"-") is excluded by design.
		}
		if !strings.Contains(tag, "omitempty") {
			t.Errorf("field %s json tag %q is missing omitempty", f.Name, tag)
		}
	}
}

// Unified default: every model slot points at the same canonical model,
// plus tb's privacy/timeout opinions.
func TestDefaultClaudeCodePrefs_Unified(t *testing.T) {
	env, err := DefaultClaudeCodePrefs(true).ToEnv("http://localhost:12580", "test-token")
	if err != nil {
		t.Fatalf("ToEnv: %v", err)
	}
	want := map[string]string{
		"ANTHROPIC_MODEL":                          "tingly/cc",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":            "tingly/cc",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":           "tingly/cc",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":             "tingly/cc",
		"CLAUDE_CODE_SUBAGENT_MODEL":               "tingly/cc",
		"API_TIMEOUT_MS":                           "3000000",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":            "32000",
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ANTHROPIC_BASE_URL":                       "http://localhost:12580/tingly/claude_code",
		"ANTHROPIC_AUTH_TOKEN":                     "test-token",
	}
	assertEnvMapsEqual(t, want, env)
}

// Separate default: each slot gets its own dedicated tingly/cc-* model so
// users can route different Claude Code workloads to distinct rules.
func TestDefaultClaudeCodePrefs_Separate(t *testing.T) {
	env, _ := DefaultClaudeCodePrefs(false).ToEnv("http://localhost:12580", "test-token")
	want := map[string]string{
		"ANTHROPIC_MODEL":                          "tingly/cc-default",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":            "tingly/cc-haiku",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":           "tingly/cc-sonnet",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":             "tingly/cc-opus",
		"CLAUDE_CODE_SUBAGENT_MODEL":               "tingly/cc-subagent",
		"API_TIMEOUT_MS":                           "3000000",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":            "32000",
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ANTHROPIC_BASE_URL":                       "http://localhost:12580/tingly/claude_code",
		"ANTHROPIC_AUTH_TOKEN":                     "test-token",
	}
	assertEnvMapsEqual(t, want, env)
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

func diffKeys(env map[string]string, wantKeys []string) []string {
	want := make(map[string]struct{}, len(wantKeys))
	for _, k := range wantKeys {
		want[k] = struct{}{}
	}
	var extra []string
	for k := range env {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	return extra
}
