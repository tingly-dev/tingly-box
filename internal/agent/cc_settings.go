package agent

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
		"ANTHROPIC_BASE_URL":   baseURL + "/tingly/" + scenarioPath,
		"ANTHROPIC_AUTH_TOKEN": apiKey,
		"TINGLY_API_URL":       baseURL,
	}
	// Named profiles inherit tunables from the main settings file. The main
	// synthetic profile keeps the historical canonical defaults.
	if !isProfile {
		env["DISABLE_TELEMETRY"] = "1"
		env["DISABLE_ERROR_REPORTING"] = "1"
		env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] = "1"
		env["API_TIMEOUT_MS"] = "3000000"
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

// ClaudeCodeSettingsSnapshot is the relevant subset of a Claude Code settings
// file. Env may contain keys outside the typed preference surface; callers that
// expose data through the API must convert it with ClaudeCodePrefsFromEnv.
type ClaudeCodeSettingsSnapshot struct {
	Exists      bool
	Env         map[string]string
	DefaultMode string
	StatusLine  bool
}

// ReadMainClaudeCodeSettings reads the user's source-of-truth settings file.
func ReadMainClaudeCodeSettings() (ClaudeCodeSettingsSnapshot, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ClaudeCodeSettingsSnapshot{}, fmt.Errorf("failed to get home directory: %w", err)
	}
	return readClaudeCodeSettings(filepath.Join(homeDir, ".claude", "settings.json"))
}

func readClaudeCodeSettings(path string) (ClaudeCodeSettingsSnapshot, error) {
	snapshot := ClaudeCodeSettingsSnapshot{Env: map[string]string{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	snapshot.Exists = true
	var raw struct {
		Env         map[string]any  `json:"env"`
		DefaultMode string          `json:"defaultMode"`
		StatusLine  json.RawMessage `json:"statusLine"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return snapshot, fmt.Errorf("failed to parse Claude Code settings: %w", err)
	}
	for key, value := range raw.Env {
		if text, ok := value.(string); ok {
			snapshot.Env[key] = text
		}
	}
	snapshot.DefaultMode = raw.DefaultMode
	snapshot.StatusLine = len(raw.StatusLine) > 0 && string(raw.StatusLine) != "null"
	return snapshot, nil
}

// CCProfileSettingsResolution contains both the inherited profile base and the
// effective result after persistent overrides. Env includes server-owned keys
// and is ready to materialize; the preference fields are safe for API output.
type CCProfileSettingsResolution struct {
	BasePreferences      ClaudeCodePrefs
	EffectivePreferences ClaudeCodePrefs
	InheritedDefaultMode string
	EffectiveDefaultMode string
	Env                  map[string]string
	HasOverrides         bool
}

// ResolveCCProfileSettings composes main settings, rule-derived profile model
// slots, stored overrides, and protected connection values in that order.
func ResolveCCProfileSettings(cfg *serverconfig.Config, baseURL, apiKey, scenarioPath string, profile typ.ProfileMeta) (CCProfileSettingsResolution, error) {
	snapshot, err := ReadMainClaudeCodeSettings()
	if err != nil {
		return CCProfileSettingsResolution{}, err
	}

	baseEnv := map[string]string{}
	if snapshot.Exists {
		baseEnv = maps.Clone(snapshot.Env)
	} else {
		defaults := DefaultClaudeCodePrefs(profile.Unified)
		defaultValues, valuesErr := defaults.Values()
		if valuesErr != nil {
			return CCProfileSettingsResolution{}, valuesErr
		}
		for key, value := range defaultValues {
			baseEnv[key] = value
		}
	}

	generated := GenerateCCEnv(cfg, baseURL, apiKey, scenarioPath, profile.Unified, true)
	for key, value := range generated {
		baseEnv[key] = value
	}
	basePreferences, err := ClaudeCodePrefsFromEnv(baseEnv)
	if err != nil {
		return CCProfileSettingsResolution{}, err
	}

	effectiveEnv := maps.Clone(baseEnv)
	if profile.ClaudeCode != nil {
		for key, value := range profile.ClaudeCode.Env {
			effectiveEnv[key] = value
		}
		for _, key := range profile.ClaudeCode.UnsetEnv {
			delete(effectiveEnv, key)
		}
	}
	effectivePreferences, err := ClaudeCodePrefsFromEnv(effectiveEnv)
	if err != nil {
		return CCProfileSettingsResolution{}, err
	}

	// Connection values are owned by the current server context and always win,
	// including for configs written by older versions or edited by hand.
	for _, key := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "TINGLY_API_URL"} {
		effectiveEnv[key] = generated[key]
	}
	effectiveEnv["NO_PROXY"] = appendNoProxy(effectiveEnv["NO_PROXY"], "localhost", "127.0.0.1", "::1")
	inheritedDefaultMode, ok := NormalizeClaudeCodeDefaultMode(snapshot.DefaultMode)
	if !ok {
		inheritedDefaultMode = DefaultClaudeCodeDefaultMode
	}
	effectiveDefaultMode := inheritedDefaultMode
	if profile.ClaudeCode != nil && profile.ClaudeCode.DefaultMode != "" {
		if normalized, valid := NormalizeClaudeCodeDefaultMode(profile.ClaudeCode.DefaultMode); valid {
			effectiveDefaultMode = normalized
		}
	}

	return CCProfileSettingsResolution{
		BasePreferences:      basePreferences,
		EffectivePreferences: effectivePreferences,
		InheritedDefaultMode: inheritedDefaultMode,
		EffectiveDefaultMode: effectiveDefaultMode,
		Env:                  effectiveEnv,
		HasOverrides: profile.ClaudeCode != nil && (len(profile.ClaudeCode.Env) > 0 ||
			len(profile.ClaudeCode.UnsetEnv) > 0 || profile.ClaudeCode.DefaultMode != ""),
	}, nil
}

// DiffCCProfileConfig stores a minimal, stable delta between the profile base
// and the full desired typed configuration submitted by the editor.
func DiffCCProfileConfig(base ClaudeCodePrefs, inheritedDefaultMode string, desired ClaudeCodePrefs, desiredDefaultMode string) (*typ.ClaudeCodeProfileConfig, error) {
	baseValues, err := base.Values()
	if err != nil {
		return nil, err
	}
	desiredValues, err := desired.Values()
	if err != nil {
		return nil, err
	}
	overrides := &typ.ClaudeCodeProfileConfig{Env: map[string]string{}}
	for key, value := range desiredValues {
		if baseValues[key] != value {
			overrides.Env[key] = value
		}
	}
	for key := range baseValues {
		if _, present := desiredValues[key]; !present {
			overrides.UnsetEnv = append(overrides.UnsetEnv, key)
		}
	}
	slices.Sort(overrides.UnsetEnv)
	if desiredDefaultMode != inheritedDefaultMode {
		overrides.DefaultMode = desiredDefaultMode
	}
	if len(overrides.Env) == 0 && len(overrides.UnsetEnv) == 0 && overrides.DefaultMode == "" {
		return nil, nil
	}
	return overrides, nil
}

// MaterializeCCProfileSettings resolves and writes one named profile using its
// persisted override delta. It is the shared path for HTTP mutations and CLI
// launch, preventing either surface from generating a different artifact.
func MaterializeCCProfileSettings(cfg *serverconfig.Config, baseURL, apiKey, scenarioPath string, profile typ.ProfileMeta) (string, error) {
	resolved, err := ResolveCCProfileSettings(cfg, baseURL, apiKey, scenarioPath, profile)
	if err != nil {
		return "", err
	}
	return BuildCCProfileSettings(profile.ID, scenarioPath, profile.Name, resolved.Env,
		serverconfig.WithDefaultMode(resolved.EffectiveDefaultMode))
}

// BuildCCProfileSettings materializes one derived Claude Code profile under
// ~/.tingly-box/claude/. The local/default profile lives at default/, while a
// named profile lives at <profileID>--<profileName>/ so both its stable identity
// and human name are visible without aliases or platform-specific links.
//
// Config remains the source of truth. The directory contains only rebuildable
// runtime artifacts: settings.json and its statusline.sh wrapper.
func BuildCCProfileSettings(profileID, scenarioPath, profileName string, env map[string]string, opts ...serverconfig.ApplyOption) (string, error) {
	destPath, err := CCProfileSettingsPath(profileID, profileName)
	if err != nil {
		return "", err
	}
	artifactDir := filepath.Dir(destPath)
	rootDir := filepath.Dir(artifactDir)

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
	applyOpts := []serverconfig.ApplyOption{serverconfig.WithBackup(false), serverconfig.WithExtra("statusLine", statusLine)}
	applyOpts = append(applyOpts, opts...)
	result, err := serverconfig.ApplyClaudeSettingsToPath(destPath, env, applyOpts...)
	if err != nil {
		return "", fmt.Errorf("failed to apply settings: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("failed to apply settings: %s", result.Message)
	}

	removeLegacyCCProfileArtifacts(rootDir, profileID, profileName)

	return destPath, nil
}

// CCProfileSettingsPath derives the canonical generated settings path from
// current profile metadata. The path is never persisted; callers recompute it
// so profile renames and legacy-name fallbacks cannot leave stale locations.
func CCProfileSettingsPath(profileID, profileName string) (string, error) {
	rootDir := filepath.Join(constant.GetTinglyConfDir(), "claude")
	artifactDir, err := ccProfileArtifactDir(rootDir, profileID, profileName)
	if err != nil {
		return "", err
	}
	return filepath.Join(artifactDir, "settings.json"), nil
}

// InspectCCProfileSettings derives the canonical path and reports whether the
// generated settings file currently exists as a regular file.
func InspectCCProfileSettings(profileID, profileName string) (string, bool, error) {
	settingsPath, err := CCProfileSettingsPath(profileID, profileName)
	if err != nil {
		return "", false, err
	}
	info, err := os.Lstat(settingsPath)
	if os.IsNotExist(err) {
		return settingsPath, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to inspect Claude Code profile settings: %w", err)
	}
	return settingsPath, info.Mode().IsRegular(), nil
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
