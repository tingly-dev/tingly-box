package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestBuildContextWindowsFromRules(t *testing.T) {
	cfg := &Config{Rules: []typ.Rule{
		{UUID: "c1", Scenario: typ.ScenarioCodex, RequestModel: "gpt-5-codex", Active: true,
			Flags: typ.RuleFlags{Context1M: true}},
		// Keys stay verbatim — including a [1m]-suffixed name — so they line
		// up with the request models collectCodexRuleModels puts in the catalog.
		{UUID: "c2", Scenario: typ.ScenarioCodex, RequestModel: "team/coder[1m]", Active: true,
			Flags: typ.RuleFlags{Context1M: true}},
		// Flag off → no entry.
		{UUID: "c3", Scenario: typ.ScenarioCodex, RequestModel: "plain", Active: true},
		// Inactive → no entry.
		{UUID: "c4", Scenario: typ.ScenarioCodex, RequestModel: "off", Active: false,
			Flags: typ.RuleFlags{Context1M: true}},
		// Non-Codex scenario → no entry.
		{UUID: "cc", Scenario: typ.ScenarioClaudeCode, RequestModel: "haiku", Active: true,
			Flags: typ.RuleFlags{Context1M: true}},
	}}

	got := BuildContextWindowsFromRules(cfg)

	want := map[string]int{
		"gpt-5-codex":    codex1MContextWindow,
		"team/coder[1m]": codex1MContextWindow,
	}
	if len(got) != len(want) {
		t.Fatalf("context windows = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("contextWindows[%q] = %d, want %d", k, got[k], v)
		}
	}
}

// ============================================================================
// ApplyCodexConfig tests
//
// Contract: writing ~/.codex/config.toml must MERGE — only fields we manage
// are overwritten. Unrelated top-level keys, user-defined providers, and
// user-defined profiles must survive.
// ============================================================================

func loadCodexConfigForTest(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]interface{}{}
	if err := toml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal toml: %v\n--- content ---\n%s", err, data)
	}
	return out
}

func TestApplyCodexConfig_NewFile_WritesManagedFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	result, err := ApplyCodexConfig("http://localhost:12580/tingly/codex", []string{"tingly-codex", "tingly-gpt5"}, DefaultCodexPrefs(), true)
	if err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}
	if !result.Success || !result.Created {
		t.Fatalf("expected success+created, got %+v", result)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	if cfg["model"] != "tingly-codex" {
		t.Errorf("model = %v, want tingly-codex", cfg["model"])
	}
	if cfg["model_provider"] != "tingly-box" {
		t.Errorf("model_provider = %v, want tingly-box", cfg["model_provider"])
	}
	providers, _ := cfg["model_providers"].(map[string]interface{})
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if tb["base_url"] != "http://localhost:12580/tingly/codex" {
		t.Errorf("base_url = %v", tb["base_url"])
	}
	if tb["wire_api"] != "responses" {
		t.Errorf("wire_api = %v, want responses", tb["wire_api"])
	}
	profiles, _ := cfg["profiles"].(map[string]interface{})
	if len(profiles) != 2 {
		t.Fatalf("profiles = %d, want 2: %#v", len(profiles), profiles)
	}
	for _, model := range []string{"tingly-codex", "tingly-gpt5"} {
		p, ok := profiles[model].(map[string]interface{})
		if !ok {
			t.Fatalf("missing profile %q in %#v", model, profiles)
		}
		if p["model"] != model {
			t.Errorf("profile %s.model = %v", model, p["model"])
		}
		if p["model_provider"] != "tingly-box" {
			t.Errorf("profile %s.model_provider = %v", model, p["model_provider"])
		}
	}
}

func TestApplyCodexConfig_PreservesUserTopLevelFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `approval_policy = "untrusted"
disable_response_storage = true
model = "user-custom-model"
hide_agent_reasoning = false

[shell_environment_policy]
inherit = "all"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://example/tingly/codex", []string{"my-rule"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	if cfg["approval_policy"] != "untrusted" {
		t.Errorf("approval_policy lost: %v", cfg["approval_policy"])
	}
	if cfg["disable_response_storage"] != true {
		t.Errorf("disable_response_storage lost: %v", cfg["disable_response_storage"])
	}
	if cfg["hide_agent_reasoning"] != false {
		t.Errorf("hide_agent_reasoning lost: %v", cfg["hide_agent_reasoning"])
	}
	shell, _ := cfg["shell_environment_policy"].(map[string]interface{})
	if shell["inherit"] != "all" {
		t.Errorf("shell_environment_policy.inherit lost: %v", shell)
	}
	// Managed field overwritten with our default (first model)
	if cfg["model"] != "my-rule" {
		t.Errorf("model = %v, want my-rule (tingly should overwrite the default)", cfg["model"])
	}
}

func TestApplyCodexConfig_PreservesOtherProvidersAndProfiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `[model_providers.openai]
name = "OpenAI"
base_url = "https://api.openai.com/v1"
wire_api = "chat"

[model_providers.tingly-box]
name = "Old Tingly"
base_url = "http://old-host/tingly/codex"
wire_api = "responses"

[profiles.work]
model = "gpt-5"
model_provider = "openai"

[profiles.legacy-tingly]
model = "tingly-legacy"
model_provider = "tingly-box"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://new-host/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	providers, _ := cfg["model_providers"].(map[string]interface{})

	// User's openai provider preserved
	openai, _ := providers["openai"].(map[string]interface{})
	if openai["base_url"] != "https://api.openai.com/v1" {
		t.Errorf("openai provider not preserved: %#v", openai)
	}

	// Our tingly-box provider overwritten with new base_url
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if tb["base_url"] != "http://new-host/tingly/codex" {
		t.Errorf("tingly-box.base_url = %v, want http://new-host/tingly/codex", tb["base_url"])
	}

	// User's [profiles.work] preserved
	profiles, _ := cfg["profiles"].(map[string]interface{})
	work, _ := profiles["work"].(map[string]interface{})
	if work["model"] != "gpt-5" || work["model_provider"] != "openai" {
		t.Errorf("profiles.work not preserved: %#v", work)
	}

	// User's [profiles.legacy-tingly] preserved (we don't garbage-collect
	// orphaned tingly profiles from previous applies — the user may want
	// to keep them).
	if _, ok := profiles["legacy-tingly"]; !ok {
		t.Errorf("profiles.legacy-tingly removed; expected preservation")
	}

	// Our new profile present
	tingly, _ := profiles["tingly-codex"].(map[string]interface{})
	if tingly["model"] != "tingly-codex" {
		t.Errorf("profiles.tingly-codex not written: %#v", profiles)
	}
}

func TestApplyCodexConfig_Idempotent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"a", "b"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(tempDir, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"a", "b"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatal(err)
	}
	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	profiles, _ := cfg["profiles"].(map[string]interface{})
	if len(profiles) != 2 {
		t.Errorf("idempotent apply added profiles: got %d, want 2 (%#v)", len(profiles), profiles)
	}
	// Sanity: at least the second run produced an updated file (backup exists)
	// but profile set shouldn't grow.
	_ = first
}

// When the sanitized profile key already exists, we overwrite. Backups
// (restored via `agent restore codex`) are the safety net — extra collision
// logic isn't worth the complexity for users who have explicitly opted in to
// tingly-box managing their codex config.
func TestApplyCodexConfig_OverwritesCollidingProfile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `[profiles.tingly-codex]
model = "stale-value"
model_provider = "openai"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatal(err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	profiles, _ := cfg["profiles"].(map[string]interface{})
	ours, _ := profiles["tingly-codex"].(map[string]interface{})
	if ours["model"] != "tingly-codex" || ours["model_provider"] != "tingly-box" {
		t.Errorf("expected colliding profile overwritten with tingly values, got %#v", ours)
	}
	if _, suffixed := profiles["tingly-codex-1"]; suffixed {
		t.Errorf("did not expect suffixed key; profiles=%#v", profiles)
	}
}

func TestApplyCodexConfig_WritesCatalogAndPointsConfigAtIt(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex", "tingly-gpt5"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	codexDir := filepath.Join(tempDir, ".codex")
	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))

	wantCatalog := filepath.Join(codexDir, "tingly-model-catalog.json")
	if cfg["model_catalog_json"] != wantCatalog {
		t.Errorf("model_catalog_json = %v, want %v", cfg["model_catalog_json"], wantCatalog)
	}

	data, err := os.ReadFile(wantCatalog)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		Schema string                   `json:"$schema"`
		Models []map[string]interface{} `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("unmarshal catalog: %v\n%s", err, data)
	}
	if catalog.Schema != codexModelCatalogSchema {
		t.Errorf("$schema = %q, want %q", catalog.Schema, codexModelCatalogSchema)
	}
	if len(catalog.Models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(catalog.Models))
	}
	gotSlugs := map[string]bool{}
	for _, m := range catalog.Models {
		gotSlugs[m["slug"].(string)] = true
		// Spot-check a few required ModelInfo fields so a future refactor
		// can't silently drop them and start tripping Codex's deserializer.
		for _, key := range []string{"display_name", "supported_reasoning_levels", "shell_type", "visibility", "truncation_policy", "input_modalities", "context_window", "max_context_window", "auto_compact_token_limit", "effective_context_window_percent"} {
			if _, ok := m[key]; !ok {
				t.Errorf("catalog entry %v missing required key %q", m["slug"], key)
			}
		}
	}
	if !gotSlugs["tingly-codex"] || !gotSlugs["tingly-gpt5"] {
		t.Errorf("catalog slugs = %v, want tingly-codex+tingly-gpt5", gotSlugs)
	}
}

func TestApplyCodexConfig_CatalogContextMetadataIsExplicit(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex", "tingly-gpt5"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, ".codex", "tingly-model-catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		Models []struct {
			Slug                          string `json:"slug"`
			ContextWindow                 int    `json:"context_window"`
			MaxContextWindow              int    `json:"max_context_window"`
			AutoCompactTokenLimit         int    `json:"auto_compact_token_limit"`
			EffectiveContextWindowPercent int    `json:"effective_context_window_percent"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if len(catalog.Models) != 2 {
		t.Fatalf("models len = %d, want 2", len(catalog.Models))
	}

	bySlug := map[string]struct {
		Slug                          string `json:"slug"`
		ContextWindow                 int    `json:"context_window"`
		MaxContextWindow              int    `json:"max_context_window"`
		AutoCompactTokenLimit         int    `json:"auto_compact_token_limit"`
		EffectiveContextWindowPercent int    `json:"effective_context_window_percent"`
	}{}
	for _, model := range catalog.Models {
		bySlug[model.Slug] = model
		if model.ContextWindow != codexDefaultContextWindow {
			t.Errorf("%s context_window = %d, want %d", model.Slug, model.ContextWindow, codexDefaultContextWindow)
		}
		if model.AutoCompactTokenLimit != codexAutoCompactTokenLimit(model.ContextWindow) {
			t.Errorf("%s auto_compact_token_limit = %d, want configured percentage of context_window", model.Slug, model.AutoCompactTokenLimit)
		}
		if model.EffectiveContextWindowPercent != codexEffectiveContextWindowPercent {
			t.Errorf("%s effective_context_window_percent = %d, want %d", model.Slug, model.EffectiveContextWindowPercent, codexEffectiveContextWindowPercent)
		}
	}
	if bySlug["tingly-codex"].MaxContextWindow != codexDefaultMaxContextWindow {
		t.Errorf("tingly-codex max_context_window = %d, want %d", bySlug["tingly-codex"].MaxContextWindow, codexDefaultMaxContextWindow)
	}
	if bySlug["tingly-gpt5"].MaxContextWindow != codexDefaultMaxContextWindow {
		t.Errorf("tingly-gpt5 max_context_window = %d, want %d", bySlug["tingly-gpt5"].MaxContextWindow, codexDefaultMaxContextWindow)
	}
}

// supported_reasoning_levels deserializes into Vec<ReasoningEffortPreset>
// upstream — a list of {effort, description} objects. Regression test for a
// bug where we emitted bare strings and Codex rejected the catalog at startup
// with "invalid type: string ..., expected struct ReasoningEffortPreset".
func TestApplyCodexConfig_CatalogReasoningPresetsAreObjects(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, ".codex", "tingly-model-catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		Models []struct {
			SupportedReasoningLevels []struct {
				Effort      string `json:"effort"`
				Description string `json:"description"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if len(catalog.Models) != 1 {
		t.Fatalf("models len = %d, want 1", len(catalog.Models))
	}
	levels := catalog.Models[0].SupportedReasoningLevels
	if len(levels) == 0 {
		t.Fatal("supported_reasoning_levels empty")
	}
	for i, lvl := range levels {
		if lvl.Effort == "" {
			t.Errorf("levels[%d].effort empty (likely emitted as bare string)", i)
		}
		if lvl.Description == "" {
			t.Errorf("levels[%d].description empty", i)
		}
	}
}

func TestApplyCodexConfig_NoModels_SkipsCatalog(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	if _, err := ApplyCodexConfig("http://h/tingly/codex", nil, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	if _, ok := cfg["model_catalog_json"]; ok {
		t.Errorf("model_catalog_json should not be set when no models: %v", cfg["model_catalog_json"])
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".codex", "tingly-model-catalog.json")); !os.IsNotExist(err) {
		t.Errorf("catalog file should not exist when no models, err=%v", err)
	}
}

func TestApplyCodexConfig_WriteCatalogFalse_SkipsCatalog(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	result, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), false)
	if err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success: %s", result.Message)
	}

	codexDir := filepath.Join(tempDir, ".codex")
	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	if _, ok := cfg["model_catalog_json"]; ok {
		t.Errorf("model_catalog_json should be absent when writeCatalog=false, got %v", cfg["model_catalog_json"])
	}
	if _, err := os.Stat(filepath.Join(codexDir, "tingly-model-catalog.json")); !os.IsNotExist(err) {
		t.Errorf("catalog file should not exist when writeCatalog=false")
	}
}

func TestApplyCodexConfig_BacksUpExistingCatalog(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(codexDir, "tingly-model-catalog.json")
	stale := []byte(`{"models":[{"slug":"old"}]}`)
	if err := os.WriteFile(catalogPath, stale, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"new-model"}, DefaultCodexPrefs(), true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	backupDir := filepath.Join(codexDir, "backup")
	matches, err := filepath.Glob(filepath.Join(backupDir, "tingly-model-catalog.json.bak-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Errorf("expected backup of existing catalog in %s, none found", backupDir)
	}
}

func TestApplyCodexConfig_NoModels_OnlyTouchesManagedFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `model = "user-custom"
some_user_flag = true
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyCodexConfig("http://h/tingly/codex", nil, DefaultCodexPrefs(), true); err != nil {
		t.Fatal(err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	// model untouched because we have nothing to put there
	if cfg["model"] != "user-custom" {
		t.Errorf("model should be untouched when no models given, got %v", cfg["model"])
	}
	if cfg["some_user_flag"] != true {
		t.Errorf("some_user_flag lost: %v", cfg["some_user_flag"])
	}
	// Provider still installed so codex can talk to tingly-box
	providers, _ := cfg["model_providers"].(map[string]interface{})
	if _, ok := providers["tingly-box"]; !ok {
		t.Errorf("tingly-box provider should still be installed: %#v", providers)
	}
}

func TestApplyCodexConfig_PrefsAppliedTopLevelAndPerProfile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	prefs := &CodexPrefs{
		ModelReasoningEffort:  "high",
		ModelReasoningSummary: "detailed",
		ModelVerbosity:        "low",
	}
	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"tingly-codex"}, prefs, true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	// Top-level
	if cfg["model_reasoning_effort"] != "high" {
		t.Errorf("top model_reasoning_effort = %v, want high", cfg["model_reasoning_effort"])
	}
	if cfg["model_reasoning_summary"] != "detailed" {
		t.Errorf("top model_reasoning_summary = %v, want detailed", cfg["model_reasoning_summary"])
	}
	if cfg["model_verbosity"] != "low" {
		t.Errorf("top model_verbosity = %v, want low", cfg["model_verbosity"])
	}
	// Per-profile (self-contained)
	profiles, _ := cfg["profiles"].(map[string]interface{})
	p, _ := profiles["tingly-codex"].(map[string]interface{})
	if p["model_reasoning_effort"] != "high" {
		t.Errorf("profile model_reasoning_effort = %v, want high", p["model_reasoning_effort"])
	}
	if p["model_verbosity"] != "low" {
		t.Errorf("profile model_verbosity = %v, want low", p["model_verbosity"])
	}
}

func TestApplyCodexConfig_PrefsRejectInvalidEnumAndCannotClobberManaged(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	prefs := &CodexPrefs{
		ModelReasoningEffort: "bogus", // invalid enum -> dropped
	}
	if _, err := ApplyCodexConfig("http://h/tingly/codex", []string{"m1"}, prefs, true); err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}

	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	if _, ok := cfg["model_reasoning_effort"]; ok {
		t.Errorf("invalid enum should be dropped, got %v", cfg["model_reasoning_effort"])
	}
	// Managed fields remain controlled by tingly-box.
	if cfg["model_provider"] != "tingly-box" {
		t.Errorf("model_provider = %v, want tingly-box", cfg["model_provider"])
	}
	if cfg["model"] != "m1" {
		t.Errorf("model = %v, want m1", cfg["model"])
	}
}

// CodexPrefsFromConfig is the inverse of toConfig: only whitelisted keys are
// read, enum values validated, and the bool normalized to "true"/"".
func TestCodexPrefsFromConfig(t *testing.T) {
	cfg := map[string]interface{}{
		"model_reasoning_effort":             "high",
		"model_reasoning_summary":            "detailed",
		"model_verbosity":                    "low",
		"model_supports_reasoning_summaries": true,
		// Unrelated keys are ignored — they must not leak into prefs.
		"model":            "tingly/codex",
		"model_provider":   "tingly-box",
		"approval_policy":  "on-request",
		"unknown_user_key": "whatever",
	}
	prefs := CodexPrefsFromConfig(cfg)
	assert.Equal(t, "high", prefs.ModelReasoningEffort)
	assert.Equal(t, "detailed", prefs.ModelReasoningSummary)
	assert.Equal(t, "low", prefs.ModelVerbosity)
	assert.Equal(t, "true", prefs.SupportsReasoningSummaries) // legacy key still read
}

// Enum values outside the allowed set are dropped (forward-compatible,
// injection-safe) — they do not surface as an invalid option in the form.
func TestCodexPrefsFromConfig_DropsInvalidEnum(t *testing.T) {
	cfg := map[string]interface{}{
		"model_reasoning_effort":  "bogus", // not a valid effort
		"model_reasoning_summary": "concise",
		"model_verbosity":         7, // wrong type
		// false / non-"true" → empty (not opted in)
		"model_supports_reasoning_summaries": false,
	}
	prefs := CodexPrefsFromConfig(cfg)
	assert.Empty(t, prefs.ModelReasoningEffort)
	assert.Equal(t, "concise", prefs.ModelReasoningSummary)
	assert.Empty(t, prefs.ModelVerbosity)
	assert.Empty(t, prefs.SupportsReasoningSummaries)
}

// max / ultra are advertised by current Codex models (models.json
// supported_reasoning_levels), so they must round-trip through the prefs.
func TestCodexPrefsFromConfig_AcceptsMaxUltraEffort(t *testing.T) {
	for _, effort := range []string{"max", "ultra"} {
		prefs := CodexPrefsFromConfig(map[string]interface{}{"model_reasoning_effort": effort})
		assert.Equal(t, effort, prefs.ModelReasoningEffort)
	}
}

func TestCodexPrefsFromConfig_Empty(t *testing.T) {
	prefs := CodexPrefsFromConfig(map[string]interface{}{})
	require.NotNil(t, prefs)
	assert.Empty(t, prefs.ModelReasoningEffort)
}

// toConfig and CodexPrefsFromConfig must round-trip: any prefs we can write
// must read back identical. Pins the forward/inverse pair against drift.
func TestCodexPrefs_RoundTrip(t *testing.T) {
	cases := []*CodexPrefs{
		{},
		DefaultCodexPrefs(),
		{ModelReasoningEffort: "high", ModelReasoningSummary: "detailed", ModelVerbosity: "low"},
		{ModelReasoningEffort: "none", ModelReasoningSummary: "none"},
	}
	for i, original := range cases {
		out := CodexPrefsFromConfig(original.toConfig())
		assert.Equal(t, original, out, "round-trip mismatch at case %d", i)
	}
}

// ReadCodexConfig reports the tingly-managed state and infers writeCatalog from
// the presence of model_catalog_json.
func TestReadCodexConfig_AppliedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`
model_provider = "tingly-box"
model_catalog_json = "/x/tingly-model-catalog.json"
model_reasoning_effort = "high"
`), 0644))

	prefs, writeCatalog, exists, err := ReadCodexConfig()
	require.NoError(t, err)
	assert.True(t, exists, "tingly-managed config should report exists=true")
	assert.True(t, writeCatalog, "model_catalog_json present → writeCatalog=true")
	assert.Equal(t, "high", prefs.ModelReasoningEffort)
}

// A config.toml with no tingly footprint reads as not-applied, even if it has
// reasoning prefs from some other setup — those are not tingly-owned state.
func TestReadCodexConfig_NonTinglyNotApplied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`
model = "gpt-5"
model_reasoning_effort = "high"
`), 0644))

	_, writeCatalog, exists, err := ReadCodexConfig()
	require.NoError(t, err)
	assert.False(t, exists, "expected exists=false for a non-tingly config")
	assert.False(t, writeCatalog, "expected writeCatalog=false when no model_catalog_json")
}

// Missing file is not an error — first-time setup returns defaults + not-applied.
func TestReadCodexConfig_MissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	prefs, writeCatalog, exists, err := ReadCodexConfig()
	require.NoError(t, err)
	assert.False(t, exists, "expected exists=false when no config.toml")
	assert.False(t, writeCatalog, "expected writeCatalog=false when no config.toml")
	// Defaults returned so the form has a starting value.
	assert.Equal(t, "medium", prefs.ModelReasoningEffort)
}

// Hybrid mode keeps requests on the tingly-box gateway while leaving
// ~/.codex/auth.json free to hold a native ChatGPT login. The gateway token
// therefore rides in config.toml's provider stanza as experimental_bearer_token
// (with requires_openai_auth=true) rather than in auth.json.

func TestRenderCodexConfigTOML_HybridEmbedsBearerToken(t *testing.T) {
	tomlBytes, err := RenderCodexConfigTOML("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), false, "tingly-box-secret")
	if err != nil {
		t.Fatalf("RenderCodexConfigTOML: %v", err)
	}
	cfg := map[string]interface{}{}
	if err := toml.Unmarshal(tomlBytes, &cfg); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, tomlBytes)
	}
	providers, _ := cfg["model_providers"].(map[string]interface{})
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if tb["experimental_bearer_token"] != "tingly-box-secret" {
		t.Errorf("experimental_bearer_token = %v, want tingly-box-secret", tb["experimental_bearer_token"])
	}
	if tb["requires_openai_auth"] != true {
		t.Errorf("requires_openai_auth = %v, want true", tb["requires_openai_auth"])
	}
	// Managed routing fields are still present so requests go through tingly.
	if cfg["model_provider"] != "tingly-box" {
		t.Errorf("model_provider = %v, want tingly-box", cfg["model_provider"])
	}
}

func TestRenderCodexConfigTOML_GatewayProviderShape(t *testing.T) {
	// Classic gateway path (bearerToken == ""): no hybrid bearer token, the key
	// is sourced from auth.json (requires_openai_auth=true), and the removed
	// `preferred_auth_method` field must not reappear (config-schema.json is
	// additionalProperties:false and rejects it).
	tomlBytes, err := RenderCodexConfigTOML("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), false, "")
	if err != nil {
		t.Fatalf("RenderCodexConfigTOML: %v", err)
	}
	cfg := map[string]interface{}{}
	if err := toml.Unmarshal(tomlBytes, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	providers, _ := cfg["model_providers"].(map[string]interface{})
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if _, ok := tb["experimental_bearer_token"]; ok {
		t.Errorf("gateway config unexpectedly carries experimental_bearer_token: %#v", tb)
	}
	if tb["requires_openai_auth"] != true {
		t.Errorf("requires_openai_auth = %v, want true (gateway sources key from auth.json)", tb["requires_openai_auth"])
	}
	if _, ok := tb["preferred_auth_method"]; ok {
		t.Errorf("provider carries preferred_auth_method, which is not a valid Codex config-schema field: %#v", tb)
	}
}

func TestApplyCodexConfigWithContextWindows_HybridWritesBearerToken(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	result, err := ApplyCodexConfigWithContextWindows("http://h/tingly/codex", []string{"tingly-codex"}, DefaultCodexPrefs(), true, nil, "tok-123")
	if err != nil {
		t.Fatalf("ApplyCodexConfigWithContextWindows: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	cfg := loadCodexConfigForTest(t, filepath.Join(tempDir, ".codex", "config.toml"))
	providers, _ := cfg["model_providers"].(map[string]interface{})
	tb, _ := providers["tingly-box"].(map[string]interface{})
	if tb["experimental_bearer_token"] != "tok-123" {
		t.Errorf("experimental_bearer_token = %v, want tok-123", tb["experimental_bearer_token"])
	}
}

func TestApplyCodexAuth_HybridWithoutTokens_LeavesAuthJSONUntouched(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	codexDir := filepath.Join(tempDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(codexDir, "auth.json")
	original := `{
  "auth_mode": "chatgpt",
  "tokens": {
    "access_token": "existing-access",
    "refresh_token": "existing-refresh"
  }
}`
	if err := os.WriteFile(authPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ApplyCodexAuth(CodexAuthHybrid, "should-be-ignored", nil)
	if err != nil {
		t.Fatalf("ApplyCodexAuth: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	got, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	if string(got) != original {
		t.Errorf("auth.json was modified.\n--- got ---\n%s\n--- want ---\n%s", got, original)
	}
}

func TestApplyCodexAuth_HybridWithTokens_WritesChatGPTLogin(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	tokens := &CodexChatGPTTokens{
		AccessToken:  "acc",
		RefreshToken: "ref",
		IDToken:      "idt",
		AccountID:    "acct-1",
	}
	result, err := ApplyCodexAuth(CodexAuthHybrid, "gateway-key", tokens)
	if err != nil {
		t.Fatalf("ApplyCodexAuth: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, ".codex", "auth.json"))
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal auth.json: %v", err)
	}
	if payload["auth_mode"] != "chatgpt" {
		t.Errorf("auth_mode = %v, want chatgpt", payload["auth_mode"])
	}
	// The gateway key must NOT leak into auth.json in hybrid mode.
	if _, ok := payload["OPENAI_API_KEY"]; ok {
		t.Errorf("auth.json unexpectedly carries OPENAI_API_KEY: %#v", payload)
	}
	tok, _ := payload["tokens"].(map[string]interface{})
	if tok["access_token"] != "acc" || tok["refresh_token"] != "ref" {
		t.Errorf("tokens block wrong: %#v", tok)
	}
}

// The catalog's headroom percentages are user-tunable via prefs; empty or
// out-of-range values fall back to the defaults, and compaction is clamped to
// the effective window.
func TestRenderCodexModelCatalog_HeadroomFromPrefs(t *testing.T) {
	render := func(p *CodexPrefs) (effective, compactLimit, window int) {
		raw, err := RenderCodexModelCatalogWithPrefs([]string{"m"}, nil, p)
		if err != nil {
			t.Fatal(err)
		}
		var c struct {
			Models []struct {
				ContextWindow int `json:"context_window"`
				Compact       int `json:"auto_compact_token_limit"`
				Effective     int `json:"effective_context_window_percent"`
			} `json:"models"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		return c.Models[0].Effective, c.Models[0].Compact, c.Models[0].ContextWindow
	}

	eff, compact, win := render(nil)
	assert.Equal(t, 82, eff)
	assert.Equal(t, win*75/100, compact)

	eff, compact, win = render(&CodexPrefs{EffectiveContextWindowPercent: "90", AutoCompactPercent: "70"})
	assert.Equal(t, 90, eff)
	assert.Equal(t, win*70/100, compact)

	// out of range / non-numeric → defaults
	eff, compact, win = render(&CodexPrefs{EffectiveContextWindowPercent: "5", AutoCompactPercent: "abc"})
	assert.Equal(t, 82, eff)
	assert.Equal(t, win*75/100, compact)

	// compact above effective is clamped down to it
	eff, compact, win = render(&CodexPrefs{EffectiveContextWindowPercent: "60", AutoCompactPercent: "95"})
	assert.Equal(t, 60, eff)
	assert.Equal(t, win*60/100, compact)
}

// A rule's codex_compact_percent overrides the shared default for that model
// only; results stay inside the usable window.
func TestRenderCodexModelCatalog_PerModelCompactPercent(t *testing.T) {
	raw, err := RenderCodexModelCatalogWithPrefs(
		[]string{"plain", "tuned", "high", "tooLow"},
		map[string]int{"tuned": codex1MContextWindow},
		&CodexPrefs{
			EffectiveContextWindowPercent: "95",
			ModelAutoCompactPercent:       map[string]int{"tuned": 60, "high": 100, "tooLow": 10},
		})
	require.NoError(t, err)
	var c struct {
		Models []struct {
			Slug          string `json:"slug"`
			ContextWindow int    `json:"context_window"`
			Compact       int    `json:"auto_compact_token_limit"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(raw, &c))
	got := map[string]int{}
	for _, m := range c.Models {
		got[m.Slug] = m.Compact * 100 / m.ContextWindow
	}
	assert.Equal(t, 75, got["plain"], "no override follows the shared default")
	assert.Equal(t, 60, got["tuned"])
	assert.Equal(t, 95, got["high"], "capped at the usable window")
	assert.Equal(t, 75, got["tooLow"], "out-of-range override is ignored")
}

func TestBuildCompactPercentsFromRules(t *testing.T) {
	cfg := &Config{Rules: []typ.Rule{
		{UUID: "a", Scenario: typ.ScenarioCodex, RequestModel: "m1", Active: true, Flags: typ.RuleFlags{CodexCompactPercent: 70}},
		{UUID: "b", Scenario: typ.ScenarioCodex, RequestModel: "m2", Active: true},
		{UUID: "c", Scenario: typ.ScenarioCodex, RequestModel: "m3", Active: false, Flags: typ.RuleFlags{CodexCompactPercent: 70}},
		{UUID: "d", Scenario: typ.ScenarioClaudeCode, RequestModel: "m4", Active: true, Flags: typ.RuleFlags{CodexCompactPercent: 70}},
	}}
	assert.Equal(t, map[string]int{"m1": 70}, BuildCompactPercentsFromRules(cfg))
}

// With per-model overrides in the catalog, the shared default read back is the
// most common value, not whichever model happens to come first.
func TestReadCodexConfig_RestoresSharedCompactDefaultDespiteOverrides(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	_, err := ApplyCodexConfig("http://h/tingly/codex", []string{"first", "b", "c"},
		&CodexPrefs{AutoCompactPercent: "70", ModelAutoCompactPercent: map[string]int{"first": 55}}, true)
	require.NoError(t, err)
	prefs, _, _, err := ReadCodexConfig()
	require.NoError(t, err)
	assert.Equal(t, "70", prefs.AutoCompactPercent)
}

// ReadCodexConfig restores non-default headroom percentages from the catalog.
func TestReadCodexConfig_RestoresCatalogHeadroom(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	_, err := ApplyCodexConfig("http://h/tingly/codex", []string{"m"}, &CodexPrefs{EffectiveContextWindowPercent: "90", AutoCompactPercent: "70"}, true)
	require.NoError(t, err)
	prefs, _, _, err := ReadCodexConfig()
	require.NoError(t, err)
	assert.Equal(t, "90", prefs.EffectiveContextWindowPercent)
	assert.Equal(t, "70", prefs.AutoCompactPercent)

	// defaults stay empty
	_, err = ApplyCodexConfig("http://h/tingly/codex", []string{"m"}, DefaultCodexPrefs(), true)
	require.NoError(t, err)
	prefs, _, _, err = ReadCodexConfig()
	require.NoError(t, err)
	assert.Empty(t, prefs.EffectiveContextWindowPercent)
	assert.Empty(t, prefs.AutoCompactPercent)
}

// Codex's config.toml schema no longer has model_supports_reasoning_summaries:
// the toggle now lands on each catalog entry, and a legacy key left by older
// versions is removed on the next apply.
func TestApplyCodexConfig_SupportsSummariesMovesToCatalog(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	codexDir := filepath.Join(tempDir, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("model_supports_reasoning_summaries = true\n"), 0644))

	_, err := ApplyCodexConfig("http://h/tingly/codex", []string{"m"}, &CodexPrefs{SupportsReasoningSummaries: "true"}, true)
	require.NoError(t, err)

	cfg := loadCodexConfigForTest(t, filepath.Join(codexDir, "config.toml"))
	_, hasLegacy := cfg["model_supports_reasoning_summaries"]
	assert.False(t, hasLegacy, "legacy config.toml key must be removed")
	profiles, _ := cfg["profiles"].(map[string]interface{})
	profile, _ := profiles["m"].(map[string]interface{})
	_, profileHasLegacy := profile["model_supports_reasoning_summaries"]
	assert.False(t, profileHasLegacy)

	raw, err := os.ReadFile(filepath.Join(codexDir, "tingly-model-catalog.json"))
	require.NoError(t, err)
	var catalog struct {
		Models []struct {
			Supports bool `json:"supports_reasoning_summaries"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(raw, &catalog))
	require.Len(t, catalog.Models, 1)
	assert.True(t, catalog.Models[0].Supports)

	// Reads back from the catalog even without the legacy key.
	prefs, _, _, err := ReadCodexConfig()
	require.NoError(t, err)
	assert.Equal(t, "true", prefs.SupportsReasoningSummaries)
}

func TestCodexConfigDefaultsToV2ForGatewayAndHybrid(t *testing.T) {
	for _, bearer := range []string{"", "scoped-token"} {
		raw, err := RenderCodexConfigTOML("http://localhost:12580/tingly/codex", []string{"mixed-providers"}, DefaultCodexPrefs(), false, bearer)
		require.NoError(t, err)
		var cfg map[string]interface{}
		require.NoError(t, toml.Unmarshal(raw, &cfg))
		providers := cfg["model_providers"].(map[string]interface{})
		provider := providers[codexGatewayProviderName].(map[string]interface{})
		require.Equal(t, "v2", provider["capabilities"].(map[string]interface{})["remote_compaction"])
		require.Equal(t, "responses", provider["wire_api"])
		require.Equal(t, true, provider["requires_openai_auth"])
		if bearer != "" {
			require.Equal(t, bearer, provider["experimental_bearer_token"])
		}
		require.NotContains(t, string(raw), "remote_compaction_v2")
	}
}

func TestCodexConfigReapplyPreservesCapabilityOverrides(t *testing.T) {
	for _, override := range []string{"", "unsupported", "v2"} {
		caps := map[string]interface{}{"external_web_access": false}
		if override != "" {
			caps["remote_compaction"] = override
		}
		cfg := map[string]interface{}{"model_providers": map[string]interface{}{
			codexGatewayProviderName: map[string]interface{}{"base_url": "http://old", "capabilities": caps},
			"other":                  map[string]interface{}{"name": "other", "capabilities": map[string]interface{}{"remote_compaction": "unsupported"}},
		}}
		for range 2 {
			mergeCodexConfig(cfg, "http://new/tingly/codex", []string{"mixed"}, "", DefaultCodexPrefs(), "")
		}
		providers := cfg["model_providers"].(map[string]interface{})
		provider := providers[codexGatewayProviderName].(map[string]interface{})
		got := provider["capabilities"].(map[string]interface{})
		want := override
		if want == "" {
			want = "v2"
		}
		require.Equal(t, want, got["remote_compaction"])
		require.Equal(t, false, got["external_web_access"])
		require.Equal(t, "http://new/tingly/codex", provider["base_url"])
		require.Equal(t, "unsupported", providers["other"].(map[string]interface{})["capabilities"].(map[string]interface{})["remote_compaction"])
	}
}
