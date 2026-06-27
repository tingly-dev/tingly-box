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

// BuildCCProfileSettings creates or updates the Claude Code profile settings
// file at ~/.tingly-box/claude/<profileID>.json. It copies ~/.claude/settings.json
// as a base (if it exists), installs the shared status line script, generates a
// per-profile wrapper script, and merges the tingly env vars.
// Returns the path to the written file.
func BuildCCProfileSettings(profileID, scenarioPath string, env map[string]string) (string, error) {
	profileDir := filepath.Join(constant.GetTinglyConfDir(), "claude")
	destPath := filepath.Join(profileDir, profileID+".json")

	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create profile directory: %w", err)
	}

	// Copy user's ~/.claude/settings.json as the base (if it exists).
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	srcPath := filepath.Join(homeDir, ".claude", "settings.json")
	if data, err := os.ReadFile(srcPath); err == nil {
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return "", fmt.Errorf("failed to copy user settings: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read user settings: %w", err)
	}

	// Install the base status line script (shared across profiles).
	if _, _, err := serverconfig.InstallStatusLineScript(); err != nil {
		return "", fmt.Errorf("failed to install status line script: %w", err)
	}

	// Generate per-profile wrapper script that sets TINGLY_SCENARIO.
	wrapperPath, err := buildCCProfileStatusLineScript(profileDir, profileID, scenarioPath)
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

	return destPath, nil
}

func buildCCProfileStatusLineScript(profileDir, profileID, scenarioPath string) (string, error) {
	wrapperPath := filepath.Join(profileDir, fmt.Sprintf("statusline-%s.sh", profileID))

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
