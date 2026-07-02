package agent

import (
	"os"
	"path/filepath"
	"testing"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

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

func TestSyncProfileNameSymlink_CreatesLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := syncProfileNameSymlink(dir, "p1", "ds", ""); err != nil {
		t.Fatalf("syncProfileNameSymlink: %v", err)
	}

	target, err := os.Readlink(filepath.Join(dir, "ds.json"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "p1.json" {
		t.Errorf("ds.json -> %q, want %q", target, "p1.json")
	}
}

func TestSyncProfileNameSymlink_NoOpWhenNameEmptyOrEqualsID(t *testing.T) {
	dir := t.TempDir()

	if err := syncProfileNameSymlink(dir, "p1", "", ""); err != nil {
		t.Fatalf("empty name: %v", err)
	}
	if err := syncProfileNameSymlink(dir, "p1", "p1", ""); err != nil {
		t.Fatalf("name == id: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files created, got %v", entries)
	}
}

func TestSyncProfileNameSymlink_RenameRemovesOldLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := syncProfileNameSymlink(dir, "p1", "ds", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := syncProfileNameSymlink(dir, "p1", "newname", "ds"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(dir, "ds.json")); !os.IsNotExist(err) {
		t.Errorf("expected ds.json to be removed after rename, lstat err = %v", err)
	}
	target, err := os.Readlink(filepath.Join(dir, "newname.json"))
	if err != nil {
		t.Fatalf("readlink newname.json: %v", err)
	}
	if target != "p1.json" {
		t.Errorf("newname.json -> %q, want %q", target, "p1.json")
	}
}

func TestSyncProfileNameSymlink_DoesNotClobberRealFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	// A real (non-symlink) file happens to occupy the alias name.
	if err := os.WriteFile(filepath.Join(dir, "ds.json"), []byte(`{"real":true}`), 0644); err != nil {
		t.Fatal(err)
	}

	err := syncProfileNameSymlink(dir, "p1", "ds", "")
	if err == nil {
		t.Fatal("expected error when alias name collides with a real file")
	}

	data, readErr := os.ReadFile(filepath.Join(dir, "ds.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != `{"real":true}` {
		t.Errorf("real file content was modified: %s", data)
	}
}

func TestSyncProfileNameSymlink_IdempotentWhenAlreadyCorrect(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := syncProfileNameSymlink(dir, "p1", "ds", ""); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if err := syncProfileNameSymlink(dir, "p1", "ds", ""); err != nil {
		t.Fatalf("second sync (idempotent): %v", err)
	}

	target, err := os.Readlink(filepath.Join(dir, "ds.json"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "p1.json" {
		t.Errorf("ds.json -> %q, want %q", target, "p1.json")
	}
}

func TestRemoveProfileNameSymlink_OnlyRemovesOwnSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := syncProfileNameSymlink(dir, "p1", "ds", ""); err != nil {
		t.Fatalf("create: %v", err)
	}

	removeProfileNameSymlink(dir, "p1", "ds")

	if _, err := os.Lstat(filepath.Join(dir, "ds.json")); !os.IsNotExist(err) {
		t.Errorf("expected ds.json removed, lstat err = %v", err)
	}

	// Removing again (already gone) and removing a name that maps to a real
	// file must not error or delete unrelated files.
	if err := os.WriteFile(filepath.Join(dir, "other.json"), []byte(`{"real":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	removeProfileNameSymlink(dir, "p1", "other")
	if _, err := os.Lstat(filepath.Join(dir, "other.json")); err != nil {
		t.Errorf("real file should not have been removed: %v", err)
	}
}
