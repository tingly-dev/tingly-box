package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func requirePathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path to be absent: %s (err = %v)", path, err)
	}
}

func writeGeneratedArtifacts(t *testing.T, dir string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "settings.json"), "generated")
	writeTestFile(t, filepath.Join(dir, "statusline.sh"), "generated")
}

func TestGenerateCCEnv_ProfileSeparate_ResolvesModelsFromCanonicalRules(t *testing.T) {
	cfg := &serverconfig.Config{Rules: []typ.Rule{
		// Renamed request model — env must follow the rule, not the seeded name.
		{UUID: "builtin:claude_code:p1:haiku", Scenario: "claude_code:p1", RequestModel: "my-fast", Active: true},
		// Inactive rule → fall back to the seeded tier name.
		{UUID: "builtin:claude_code:p1:opus", Scenario: "claude_code:p1", RequestModel: "my-smart", Active: false},
		{UUID: "builtin:claude_code:p1:sonnet", Scenario: "claude_code:p1", RequestModel: "sonnet", Active: true},
		// default / subagent rules absent → fall back to the seeded tier name.
	}}

	env := GenerateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code:p1", false, true)

	wants := map[string]string{
		"ANTHROPIC_BASE_URL":             "http://localhost:12580/tingly/claude_code:p1",
		"TINGLY_API_URL":                 "http://localhost:12580",
		"ANTHROPIC_MODEL":                "default",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "my-fast",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "opus",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "sonnet",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "subagent",
	}
	for k, want := range wants {
		if env[k] != want {
			t.Errorf("env[%q] = %q, want %q", k, env[k], want)
		}
	}
}

func TestGenerateCCEnv_Profile_Context1MSuffix(t *testing.T) {
	cfg := &serverconfig.Config{Rules: []typ.Rule{
		// Flag set → env model advertises [1m] to Claude Code.
		{UUID: "builtin:claude_code:p1:haiku", Scenario: "claude_code:p1", RequestModel: "haiku",
			Flags: typ.RuleFlags{Context1M: true}, Active: true},
		// Already suffixed (e.g. user renamed it) → no double suffix.
		{UUID: "builtin:claude_code:p1:opus", Scenario: "claude_code:p1", RequestModel: "opus[1m]",
			Flags: typ.RuleFlags{Context1M: true}, Active: true},
		// Flag off → untouched.
		{UUID: "builtin:claude_code:p1:sonnet", Scenario: "claude_code:p1", RequestModel: "sonnet", Active: true},
	}}

	env := GenerateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code:p1", false, true)

	if got := env["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; got != "haiku[1m]" {
		t.Errorf("haiku model = %q, want %q", got, "haiku[1m]")
	}
	if got := env["ANTHROPIC_DEFAULT_OPUS_MODEL"]; got != "opus[1m]" {
		t.Errorf("opus model = %q, want %q (no double suffix)", got, "opus[1m]")
	}
	if got := env["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "sonnet" {
		t.Errorf("sonnet model = %q, want %q", got, "sonnet")
	}
}

func TestGenerateCCEnv_ProfileUnified_ResolvesCCRule(t *testing.T) {
	cfg := &serverconfig.Config{Rules: []typ.Rule{
		{UUID: "builtin:claude_code:p2:cc", Scenario: "claude_code:p2", RequestModel: "renamed-cc", Active: true},
	}}

	env := GenerateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code:p2", true, true)

	for _, k := range []string{
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"CLAUDE_CODE_SUBAGENT_MODEL",
	} {
		if env[k] != "renamed-cc" {
			t.Errorf("env[%q] = %q, want %q", k, env[k], "renamed-cc")
		}
	}
}

func TestGenerateCCEnv_Profile_Context1MAutoCompactWindow(t *testing.T) {
	// When any rule in the profile has Context1M=true, the auto-compact
	// window must be adjusted to 1M so Claude Code doesn't compact prematurely.
	cfg := &serverconfig.Config{Rules: []typ.Rule{
		{UUID: "builtin:claude_code:p1:haiku", Scenario: "claude_code:p1", RequestModel: "haiku",
			Flags: typ.RuleFlags{Context1M: true}, Active: true},
	}}

	env := GenerateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code:p1", false, true)

	if got := env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"]; got != "1000000" {
		t.Errorf("CLAUDE_CODE_AUTO_COMPACT_WINDOW = %q, want %q", got, "1000000")
	}
}

func TestGenerateCCEnv_Profile_No1M_NoAutoCompactWindow(t *testing.T) {
	// Without the 1M flag, the env should NOT include the auto-compact
	// window override — let Claude Code use its own default.
	cfg := &serverconfig.Config{Rules: []typ.Rule{
		{UUID: "builtin:claude_code:p1:haiku", Scenario: "claude_code:p1", RequestModel: "haiku",
			Flags: typ.RuleFlags{Context1M: false}, Active: true},
	}}

	env := GenerateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code:p1", false, true)

	if _, ok := env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"]; ok {
		t.Errorf("CLAUDE_CODE_AUTO_COMPACT_WINDOW should not be set without Context1M, got %q", env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"])
	}
}

func TestGenerateCCEnv_MainScenario_ResolvesModernBuiltins(t *testing.T) {
	cfg := &serverconfig.Config{Rules: []typ.Rule{
		{UUID: serverconfig.RuleUUIDCCHaiku, Scenario: typ.ScenarioClaudeCode, RequestModel: "vendor/fast", Active: true},
	}}

	env := GenerateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code", false, false)

	if got := env["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; got != "vendor/fast" {
		t.Errorf("haiku model = %q, want %q", got, "vendor/fast")
	}
}

func TestGenerateCCEnv_MainScenario_ResolvesLegacyBuiltins(t *testing.T) {
	// Pre-migration configs still carry the legacy built-in-cc-* UUIDs.
	cfg := &serverconfig.Config{Rules: []typ.Rule{
		{UUID: serverconfig.RuleUUIDBuiltinCCHaiku, Scenario: typ.ScenarioClaudeCode, RequestModel: "vendor/fast", Active: true},
	}}

	env := GenerateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code", false, false)

	if got := env["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; got != "vendor/fast" {
		t.Errorf("haiku model = %q, want %q", got, "vendor/fast")
	}
	// Missing rules keep the canonical tingly/* fallbacks.
	if got := env["ANTHROPIC_MODEL"]; got != "tingly/cc-default" {
		t.Errorf("default model = %q, want %q", got, "tingly/cc-default")
	}
}

func TestCCProfileArtifactDir(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name        string
		profileID   string
		profileName string
		want        string
		wantErr     bool
	}{
		{name: "local default", profileID: "default", want: filepath.Join(root, "default")},
		{name: "named profile", profileID: "p1", profileName: "work", want: filepath.Join(root, "p1--work")},
		{name: "unsafe legacy name falls back to ID", profileID: "p1", profileName: "../legacy", want: filepath.Join(root, "p1")},
		{name: "unsafe ID rejected", profileID: "../p1", profileName: "work", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ccProfileArtifactDir(root, tt.profileID, tt.profileName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("artifact dir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildCCProfileSettings_MaterializesReadableDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	localClaudeDir := filepath.Join(home, ".claude")
	writeTestFile(t, filepath.Join(localClaudeDir, "settings.json"), `{"theme":"dark"}`)

	root := filepath.Join(home, ".tingly-box", "claude")
	for _, path := range []string{filepath.Join(root, "p1.json"), filepath.Join(root, "statusline-p1.sh")} {
		writeTestFile(t, path, "legacy")
	}
	legacyAliasPath := filepath.Join(root, "work.json")
	legacyAliasCreated := os.Symlink("p1.json", legacyAliasPath) == nil

	settingsPath, err := BuildCCProfileSettings("p1", "claude_code:p1", "work", map[string]string{"TINGLY_TEST": "yes"})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "p1--work", "settings.json")
	if settingsPath != wantPath {
		t.Fatalf("settings path = %q, want %q", settingsPath, wantPath)
	}

	settings := readJSONMap(t, settingsPath)
	if settings["theme"] != "dark" {
		t.Errorf("local setting was not preserved: %#v", settings)
	}
	env, ok := settings["env"].(map[string]any)
	if !ok || env["TINGLY_TEST"] != "yes" {
		t.Errorf("generated env missing: %#v", settings["env"])
	}
	statusLine, ok := settings["statusLine"].(map[string]any)
	if !ok || statusLine["command"] != filepath.Join(root, "p1--work", "statusline.sh") {
		t.Errorf("status line does not point inside profile directory: %#v", settings["statusLine"])
	}
	for _, legacyPath := range []string{filepath.Join(root, "p1.json"), filepath.Join(root, "statusline-p1.sh")} {
		requirePathMissing(t, legacyPath)
	}
	if legacyAliasCreated {
		requirePathMissing(t, legacyAliasPath)
	}

	defaultPath, err := BuildCCProfileSettings("default", "claude_code", "", map[string]string{"TINGLY_DEFAULT": "yes"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "default", "settings.json"); defaultPath != want {
		t.Errorf("default settings path = %q, want %q", defaultPath, want)
	}

	if err := os.Remove(filepath.Join(localClaudeDir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildCCProfileSettings("default", "claude_code", "", map[string]string{"TINGLY_DEFAULT": "refreshed"}); err != nil {
		t.Fatal(err)
	}
	settings = readJSONMap(t, defaultPath)
	if _, exists := settings["theme"]; exists {
		t.Errorf("removed local settings leaked from an older generated artifact: %#v", settings)
	}
}

func TestRemoveCCProfileArtifacts_PreservesUserFiles(t *testing.T) {
	root := t.TempDir()
	artifactDir := filepath.Join(root, "p1--work")
	writeGeneratedArtifacts(t, artifactDir)
	writeTestFile(t, filepath.Join(artifactDir, "notes.txt"), "user-owned")
	for _, path := range []string{filepath.Join(root, "p1.json"), filepath.Join(root, "statusline-p1.sh")} {
		writeTestFile(t, path, "legacy")
	}

	if err := removeCCProfileArtifacts(root, "p1", "work"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.json", "statusline.sh"} {
		requirePathMissing(t, filepath.Join(artifactDir, name))
	}
	if _, err := os.Stat(filepath.Join(artifactDir, "notes.txt")); err != nil {
		t.Errorf("user file should be preserved: %v", err)
	}
	for _, legacyPath := range []string{filepath.Join(root, "p1.json"), filepath.Join(root, "statusline-p1.sh")} {
		requirePathMissing(t, legacyPath)
	}
}

func TestRemoveCCProfileArtifacts_RemovesEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	artifactDir := filepath.Join(root, "p1--work")
	writeGeneratedArtifacts(t, artifactDir)

	if err := removeCCProfileArtifacts(root, "p1", "work"); err != nil {
		t.Fatal(err)
	}
	requirePathMissing(t, artifactDir)
}

func TestProfileArtifactOperations_RejectSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	artifactDir := filepath.Join(root, "p1--work")
	if err := os.Symlink(outside, artifactDir); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	outsideSettings := filepath.Join(outside, "settings.json")
	writeTestFile(t, outsideSettings, "outside")

	if err := ensureCCProfileArtifactDir(artifactDir); err == nil {
		t.Fatal("expected symlink artifact directory to be rejected for writes")
	}
	if err := removeCCProfileArtifacts(root, "p1", "work"); err == nil {
		t.Fatal("expected symlink artifact directory to be rejected for cleanup")
	}
	if data, err := os.ReadFile(outsideSettings); err != nil || string(data) != "outside" {
		t.Fatalf("outside file was changed: data = %q, err = %v", data, err)
	}
}

func TestRemoveRenamedCCProfileArtifacts_PreservesSameDirectory(t *testing.T) {
	root := t.TempDir()
	artifactDir := filepath.Join(root, "p1--work")
	settingsPath := filepath.Join(artifactDir, "settings.json")
	writeTestFile(t, settingsPath, "generated")

	if err := removeRenamedCCProfileArtifacts(root, "p1", "work", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("same artifact directory should be preserved: %v", err)
	}
}

func TestRemoveRenamedCCProfileArtifacts_RemovesOnlyOldDirectory(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "p1--old")
	newDir := filepath.Join(root, "p1--new")
	for _, dir := range []string{oldDir, newDir} {
		writeGeneratedArtifacts(t, dir)
	}

	if err := removeRenamedCCProfileArtifacts(root, "p1", "old", "new"); err != nil {
		t.Fatal(err)
	}
	requirePathMissing(t, oldDir)
	if _, err := os.Stat(filepath.Join(newDir, "settings.json")); err != nil {
		t.Fatalf("new directory should be preserved: %v", err)
	}
}
