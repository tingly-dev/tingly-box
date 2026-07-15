package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/constant"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GenerateCCEnv builds the env map for Claude Code settings.json.
//
// scenarioPath is "claude_code" for the main scenario or "claude_code:p1" for
// a profile. isProfile=true → tier models resolved from profile-scoped built-in
// UUIDs; isProfile=false → resolved from main-scenario built-in UUIDs (with
// legacy-UUID fallback for pre-migration configs).
//
// Reading the rule's request_model (instead of assuming the seeded name) keeps
// the env aligned when a user renames a rule's model; the seeded name is the
// fallback when the rule is missing or inactive.
func GenerateCCEnv(cfg *serverconfig.Config, baseURL, apiKey, scenarioPath string, unified, isProfile bool) map[string]string {
	env := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       baseURL + "/tingly/" + scenarioPath,
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"TINGLY_API_URL":                           baseURL,
	}

	// Track whether any resolved rule has the 1M context flag so we can
	// mirror the frontend quick-config's auto-compact window adjustment.
	context1M := false

	ruleModel := func(fallback string, uuids ...string) string {
		if cfg != nil {
			for _, uuid := range uuids {
				if r := cfg.GetRuleByUUID(uuid); r != nil && r.Active {
					if m := strings.TrimSpace(r.RequestModel); m != "" {
						// Mirror the frontend quick-config: a rule with the 1M context
						// flag advertises itself to Claude Code via the [1m] suffix (the
						// client strips it back and sends the context-1m beta header).
						if r.Flags.Context1M {
							context1M = true
							if !strings.HasSuffix(m, serverconfig.Context1MSuffix) {
								m += serverconfig.Context1MSuffix
							}
						}
						return m
					}
				}
			}
		}
		return fallback
	}

	// tierModel resolves one tier slot: profile rules by canonical profiled UUID
	// with the short tier name as fallback, main-scenario rules by the modern
	// built-in UUID (legacy UUID as a compat fallback) with canonical tingly/*
	// name as the final fallback.
	tierModel := func(tier, legacyUUID, legacyFallback string) string {
		if isProfile {
			return ruleModel(tier, serverconfig.BuiltinRuleUUID(typ.RuleScenario(scenarioPath), tier))
		}
		return ruleModel(legacyFallback, serverconfig.BuiltinRuleUUID(typ.ScenarioClaudeCode, tier), legacyUUID)
	}

	if unified {
		model := tierModel("cc", serverconfig.RuleUUIDBuiltinCC, "tingly/cc")
		env["ANTHROPIC_MODEL"] = model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = model
	} else {
		env["ANTHROPIC_MODEL"] = tierModel("default", serverconfig.RuleUUIDBuiltinCCDefault, "tingly/cc-default")
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = tierModel("haiku", serverconfig.RuleUUIDBuiltinCCHaiku, "tingly/cc-haiku")
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = tierModel("opus", serverconfig.RuleUUIDBuiltinCCOpus, "tingly/cc-opus")
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = tierModel("sonnet", serverconfig.RuleUUIDBuiltinCCSonnet, "tingly/cc-sonnet")
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = tierModel("subagent", serverconfig.RuleUUIDBuiltinCCSubagent, "tingly/cc-subagent")
	}

	// Mirror the frontend quick-config: when any resolved model rule has the
	// 1M context flag, adjust the auto-compact window to match so Claude Code
	// doesn't compact prematurely.
	if context1M {
		env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"] = "1000000"
	}

	return env
}

// BuildCCProfileSettings materializes one derived Claude Code profile under
// ~/.tingly-box/claude/. The local/default profile lives at default/, while a
// named profile lives at <profileID>--<profileName>/ so both its stable identity
// and human name are visible without aliases or platform-specific links.
//
// Config remains the source of truth. The directory contains only rebuildable
// runtime artifacts: settings.json and its statusline.sh wrapper.
func BuildCCProfileSettings(profileID, scenarioPath, profileName string, env map[string]string) (string, error) {
	rootDir := filepath.Join(constant.GetTinglyConfDir(), "claude")
	artifactDir, err := ccProfileArtifactDir(rootDir, profileID, profileName)
	if err != nil {
		return "", err
	}
	destPath := filepath.Join(artifactDir, "settings.json")

	if err := ensureCCProfileArtifactDir(artifactDir); err != nil {
		return "", fmt.Errorf("failed to create profile directory: %w", err)
	}

	// Copy user's ~/.claude/settings.json as the base (if it exists).
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	srcPath := filepath.Join(homeDir, ".claude", "settings.json")
	baseSettings := []byte("{}")
	if data, readErr := os.ReadFile(srcPath); readErr == nil {
		baseSettings = data
	} else if !os.IsNotExist(readErr) {
		return "", fmt.Errorf("failed to read user settings: %w", readErr)
	}
	// Always reset the derived file to the current local source (or an empty
	// object when that source no longer exists) before applying profile values.
	// This prevents generated artifacts from becoming a second source of truth.
	if err := os.WriteFile(destPath, baseSettings, 0644); err != nil {
		return "", fmt.Errorf("failed to copy user settings: %w", err)
	}

	// Install the base status line script (shared across profiles).
	if _, _, err := serverconfig.InstallStatusLineScript(); err != nil {
		return "", fmt.Errorf("failed to install status line script: %w", err)
	}

	// Generate per-profile wrapper script that sets TINGLY_SCENARIO.
	wrapperPath, err := buildCCProfileStatusLineScript(artifactDir, profileID, scenarioPath)
	if err != nil {
		return "", fmt.Errorf("failed to create status line wrapper: %w", err)
	}

	statusLine := map[string]any{
		"type":    "command",
		"command": wrapperPath,
	}
	result, err := serverconfig.ApplyClaudeSettingsToPath(destPath, env,
		serverconfig.WithBackup(false), serverconfig.WithExtra("statusLine", statusLine))
	if err != nil {
		return "", fmt.Errorf("failed to apply settings: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("failed to apply settings: %s", result.Message)
	}

	removeLegacyCCProfileArtifacts(rootDir, profileID, profileName)

	return destPath, nil
}

func ccProfileArtifactDir(rootDir, profileID, profileName string) (string, error) {
	if profileID == "default" {
		return filepath.Join(rootDir, "default"), nil
	}
	if !typ.IsSimpleProfileAlias(profileID) {
		return "", fmt.Errorf("invalid Claude Code profile ID %q", profileID)
	}
	if err := typ.ValidateProfileName(profileName); err != nil {
		// Legacy names created before validation remain runnable from config, but
		// use the stable ID-only directory until the user gives them a safe name.
		return filepath.Join(rootDir, profileID), nil
	}
	return filepath.Join(rootDir, profileID+"--"+profileName), nil
}

func ensureCCProfileArtifactDir(artifactDir string) error {
	info, err := os.Lstat(artifactDir)
	if os.IsNotExist(err) {
		return os.MkdirAll(artifactDir, 0755)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("profile artifact path is not a managed directory: %s", artifactDir)
	}
	return nil
}

// RemoveCCProfileArtifacts removes only files generated by Tingly-Box for the
// selected profile. If users added anything else to the directory, it is left
// in place rather than recursively deleted.
func RemoveCCProfileArtifacts(profileID, profileName string) error {
	rootDir := filepath.Join(constant.GetTinglyConfDir(), "claude")
	return removeCCProfileArtifacts(rootDir, profileID, profileName)
}

// RemoveRenamedCCProfileArtifacts removes the old derived directory after the
// new one has been materialized. os.SameFile protects case-only renames on
// case-insensitive filesystems, where both spellings identify the same path.
func RemoveRenamedCCProfileArtifacts(profileID, oldName, newName string) error {
	rootDir := filepath.Join(constant.GetTinglyConfDir(), "claude")
	return removeRenamedCCProfileArtifacts(rootDir, profileID, oldName, newName)
}

func removeRenamedCCProfileArtifacts(rootDir, profileID, oldName, newName string) error {
	oldDir, oldErr := ccProfileArtifactDir(rootDir, profileID, oldName)
	newDir, newErr := ccProfileArtifactDir(rootDir, profileID, newName)
	if oldErr == nil && newErr == nil {
		oldInfo, oldStatErr := os.Stat(oldDir)
		newInfo, newStatErr := os.Stat(newDir)
		if oldStatErr == nil && newStatErr == nil && os.SameFile(oldInfo, newInfo) {
			return nil
		}
	}
	return removeCCProfileArtifacts(rootDir, profileID, oldName)
}

func removeCCProfileArtifacts(rootDir, profileID, profileName string) error {
	artifactDir, err := ccProfileArtifactDir(rootDir, profileID, profileName)
	if err == nil {
		if info, statErr := os.Lstat(artifactDir); statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return fmt.Errorf("refusing to clean unmanaged Claude Code profile path: %s", artifactDir)
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return fmt.Errorf("failed to inspect Claude Code profile directory: %w", statErr)
		}
		for _, name := range []string{"settings.json", "statusline.sh"} {
			if removeErr := os.Remove(filepath.Join(artifactDir, name)); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("failed to remove Claude Code profile artifact %s: %w", name, removeErr)
			}
		}
		if removeErr := os.Remove(artifactDir); removeErr != nil && !os.IsNotExist(removeErr) {
			// A non-empty directory contains user files; preserving it is expected.
			if entries, readErr := os.ReadDir(artifactDir); readErr != nil || len(entries) == 0 {
				return fmt.Errorf("failed to remove Claude Code profile directory: %w", removeErr)
			}
		}
	}

	removeLegacyCCProfileArtifacts(rootDir, profileID, profileName)
	return nil
}

func removeLegacyCCProfileArtifacts(rootDir, profileID, profileName string) {
	_ = os.Remove(filepath.Join(rootDir, profileID+".json"))
	_ = os.Remove(filepath.Join(rootDir, "statusline-"+profileID+".sh"))

	if profileName == "" || !typ.IsSimpleProfileAlias(profileName) {
		return
	}
	aliasPath := filepath.Join(rootDir, profileName+".json")
	info, err := os.Lstat(aliasPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return
	}
	if target, readErr := os.Readlink(aliasPath); readErr == nil && target == profileID+".json" {
		_ = os.Remove(aliasPath)
	}
}

func buildCCProfileStatusLineScript(profileDir, profileID, scenarioPath string) (string, error) {
	wrapperPath := filepath.Join(profileDir, "statusline.sh")

	content := fmt.Sprintf("#!/bin/bash\n"+
		"# Per-profile status line wrapper for Claude Code\n"+
		"# Profile: %s → %s\n"+
		"export TINGLY_SCENARIO=\"%s\"\n"+
		"exec ~/.claude/tingly-statusline.sh \"$@\"\n",
		profileID, scenarioPath, scenarioPath)

	if err := os.WriteFile(wrapperPath, []byte(content), 0755); err != nil {
		return "", fmt.Errorf("failed to write wrapper script: %w", err)
	}

	return wrapperPath, nil
}
