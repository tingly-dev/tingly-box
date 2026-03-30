package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CCCommand creates the `cc` subcommand that configures and launches Claude Code.
func CCCommand(appManager *AppManager) *cobra.Command {
	var profile string
	var unified bool

	cmd := &cobra.Command{
		Use:   "cc",
		Short: "Launch Claude Code with tingly-box routing",
		Long: `Launch Claude Code with tingly-box as the API proxy.

A temporary settings file is created and passed to Claude Code via --settings,
so the existing Claude Code configuration is not modified.

Profiles can be used to switch between different rule sets for the same scenario.`,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Re-parse flags: everything before "--" belongs to tingly-box,
			// everything after is passthrough to claude.
			var tinglyArgs, claudeArgs []string
			seenDashDash := false
			for _, arg := range args {
				if arg == "--" {
					seenDashDash = true
					continue
				}
				if seenDashDash {
					claudeArgs = append(claudeArgs, arg)
				} else {
					tinglyArgs = append(tinglyArgs, arg)
				}
			}

			// Parse tingly-box flags from tinglyArgs
			parsedProfile, parsedUnified, remaining, err := parseCCFlags(tinglyArgs)
			if err != nil {
				return err
			}
			// Explicit flags override defaults
			if profile == "" && parsedProfile != "" {
				profile = parsedProfile
			}
			if !cmd.Flags().Changed("unified") && parsedUnified != nil {
				unified = *parsedUnified
			}
			// Remaining tingly args that aren't our flags → treat as claude args
			claudeArgs = append(remaining, claudeArgs...)

			return runCC(appManager, profile, unified, claudeArgs)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile ID or name (e.g., p1, Premium)")
	cmd.Flags().BoolVar(&unified, "unified", true, "Unified mode (all models point to same rule)")

	return cmd
}

// parseCCFlags extracts known tingly-box flags from args.
// Returns parsed profile, unified, and remaining unknown args.
func parseCCFlags(args []string) (profile string, unified *bool, remaining []string, err error) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--profile" || args[i] == "-p":
			if i+1 >= len(args) {
				return "", nil, nil, fmt.Errorf("flag %s requires a value", args[i])
			}
			profile = args[i+1]
			i++ // skip next

		case args[i] == "--unified":
			v := true
			unified = &v

		case args[i] == "--no-unified":
			v := false
			unified = &v

		default:
			// Unknown flag → passthrough to claude
			remaining = append(remaining, args[i])
		}
	}
	return profile, unified, remaining, nil
}

// runCC orchestrates: ensure server → resolve profile → write settings → exec claude.
func runCC(appManager *AppManager, profile string, unified bool, claudeArgs []string) error {
	globalConfig := appManager.GetGlobalConfig()
	scenario := typ.ScenarioClaudeCode

	// Resolve profile if specified
	var profileID string
	if profile != "" {
		resolved, err := globalConfig.ResolveProfileNameOrID(scenario, profile)
		if err != nil {
			return fmt.Errorf("profile error: %w", err)
		}
		profileID = resolved
	}

	// Build the scenario path (with or without profile)
	scenarioPath := string(scenario)
	if profileID != "" {
		scenarioPath = string(typ.ProfiledScenarioName(scenario, profileID))
	}

	// Build base URL and token
	port := appManager.GetServerPort()
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	apiKey := globalConfig.GetModelToken()

	// Generate env map (profile always uses separate mode)
	envUnified := unified
	if profileID != "" {
		envUnified = false
	}
	env := generateCCEnv(baseURL, apiKey, scenarioPath, envUnified, profileID != "")

	// Build settings file
	var settingsPath string
	var err error
	if profileID != "" {
		// Profile mode: copy user's settings.json to ~/.tingly-box/claude/<profileID>.json
		// then merge the env section with tingly-box routing vars.
		settingsPath, err = buildProfileSettings(profileID, env, scenarioPath)
	} else {
		// Default mode: create a temp settings file with only env vars.
		settingsPath, err = buildTempSettings(env)
	}
	if err != nil {
		return err
	}

	// Discover claude binary
	variant, err := claude.FindClaudeCLI(context.Background())
	if err != nil {
		return fmt.Errorf("claude CLI not found: %w", err)
	}

	// Build claude args: --settings <file> + passthrough
	execArgs := []string{"--settings", settingsPath}
	execArgs = append(execArgs, claudeArgs...)

	// Exec replaces current process
	binPath := variant.Path
	//nolint:gosec // intentional exec of user-installed CLI
	execCmd := exec.Command(binPath, execArgs...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = os.Environ()

	return execCmd.Run()
}

// buildProfileSettings copies the user's ~/.claude/settings.json to
// ~/.tingly-box/claude/<profileID>.json, then applies (merges) the tingly-box
// env vars and status line config into it.
func buildProfileSettings(profileID string, env map[string]string, scenarioPath string) (string, error) {
	profileDir := filepath.Join(constant.GetTinglyConfDir(), "claude")
	destPath := filepath.Join(profileDir, profileID+".json")

	// Ensure the profile directory exists
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create profile directory: %w", err)
	}

	// Copy user's ~/.claude/settings.json as the base (if it exists)
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
	// If file doesn't exist, destPath may not exist yet — ApplyClaudeSettingsToPath will create it

	// Install the base status line script (shared across profiles)
	if _, _, err := config.InstallStatusLineScript(); err != nil {
		return "", fmt.Errorf("failed to install status line script: %w", err)
	}

	// Generate a per-profile wrapper script that sets TINGLY_SCENARIO
	wrapperPath, err := buildProfileStatusLineScript(profileDir, profileID, scenarioPath)
	if err != nil {
		return "", fmt.Errorf("failed to create status line wrapper: %w", err)
	}

	// Apply tingly-box env vars + statusLine config
	statusLine := map[string]any{
		"type":    "command",
		"command": wrapperPath,
	}
	result, err := config.ApplyClaudeSettingsToPath(destPath, env, config.KV{Key: "statusLine", Value: statusLine})
	if err != nil {
		return "", fmt.Errorf("failed to apply settings: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("failed to apply settings: %s", result.Message)
	}

	return destPath, nil
}

// buildProfileStatusLineScript creates a per-profile wrapper script that sets
// TINGLY_SCENARIO before invoking the base tingly-statusline.sh.
func buildProfileStatusLineScript(profileDir, profileID, scenarioPath string) (string, error) {
	wrapperPath := filepath.Join(profileDir, fmt.Sprintf("statusline-%s.sh", profileID))

	wrapper := fmt.Sprintf(`#!/bin/bash
# Per-profile status line wrapper for Claude Code
# Profile: %s → %s
export TINGLY_SCENARIO="%s"
exec ~/.claude/tingly-statusline.sh "$@"
`, profileID, scenarioPath, scenarioPath)

	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0755); err != nil {
		return "", fmt.Errorf("failed to write wrapper script: %w", err)
	}

	return wrapperPath, nil
}

// buildTempSettings creates a temporary settings file containing only the env vars.
func buildTempSettings(env map[string]string) (string, error) {
	settingsContent, err := json.MarshalIndent(map[string]interface{}{
		"env": env,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to generate settings: %w", err)
	}

	tmpDir := filepath.Join(os.TempDir(), "tingly-box-cc")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(tmpDir, "settings-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp settings file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer tmpFile.Close()

	if _, err := tmpFile.Write(settingsContent); err != nil {
		return "", fmt.Errorf("failed to write settings file: %w", err)
	}

	return tmpPath, nil
}

// generateCCEnv builds the env vars map for Claude Code settings.
// When isProfile is true, model names use short names (e.g. "default") instead of "tingly/cc-default".
func generateCCEnv(baseURL, apiKey, scenarioPath string, unified bool, isProfile bool) map[string]string {
	env := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       baseURL + "/tingly/" + scenarioPath,
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
	}

	if unified {
		env["ANTHROPIC_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc"
	} else if isProfile {
		env["ANTHROPIC_MODEL"] = "default"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "haiku"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "opus"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "sonnet"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "subagent"
	} else {
		env["ANTHROPIC_MODEL"] = "tingly/cc-default"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc-haiku"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc-opus"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc-sonnet"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc-subagent"
	}

	return env
}
