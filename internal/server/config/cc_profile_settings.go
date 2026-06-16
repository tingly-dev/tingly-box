package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GenerateCCProfileEnv builds the env map for a Claude Code profile settings.json.
// scenarioPath is the profiled scenario name (e.g. "claude_code:p1").
// The returned map is ready to be written to the "env" block via ApplyClaudeSettingsToPath.
func GenerateCCProfileEnv(cfg *Config, scenarioPath string, unified bool, baseURL, apiKey string) map[string]string {
	env := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       baseURL + "/tingly/" + scenarioPath,
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"TINGLY_API_URL":                           baseURL,
	}

	ruleModel := func(fallback string, uuid string) string {
		if cfg != nil {
			if r := cfg.GetRuleByUUID(uuid); r != nil && r.Active {
				if m := strings.TrimSpace(r.RequestModel); m != "" {
					if r.Flags.Context1M && !strings.HasSuffix(m, Context1MSuffix) {
						m += Context1MSuffix
					}
					return m
				}
			}
		}
		return fallback
	}

	tierModel := func(tier string) string {
		return ruleModel(tier, BuiltinRuleUUID(typ.RuleScenario(scenarioPath), tier))
	}

	if unified {
		model := tierModel("cc")
		env["ANTHROPIC_MODEL"] = model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = model
	} else {
		env["ANTHROPIC_MODEL"] = tierModel("default")
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = tierModel("haiku")
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = tierModel("opus")
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = tierModel("sonnet")
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = tierModel("subagent")
	}

	return env
}

// BuildCCProfileSettings creates or updates the Claude Code profile settings file at
// ~/.tingly-box/claude/<profileID>.json. It copies ~/.claude/settings.json as a base
// (if it exists), installs the shared status line script, generates a per-profile
// wrapper script, and merges the tingly env vars. Returns the path to the written file.
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
	if _, _, err := InstallStatusLineScript(); err != nil {
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
	result, err := ApplyClaudeSettingsToPath(destPath, env, WithBackup(false), WithExtra("statusLine", statusLine))
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
