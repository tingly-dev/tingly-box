package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============== Kong Command Structures ==============

// CCmdKong launches Claude Code with passthrough mode
// Kong's passthrough mode requires at least one positional arg
type CCmdKong struct {
	Args []string `kong:"arg,optional,passthrough"`
}

func (c *CCmdKong) Run(appManager *AppManager) error {
	profile, port, claudeArgs, err := parseCCFlags(c.Args)
	if err != nil {
		return err
	}
	return runCC(appManager, profile, port, claudeArgs)
}

// ============== Business Logic Functions ==============

// parseCCFlags consumes tingly-box-specific flags from the beginning of args
// and returns the remaining args verbatim for claude.
//
// Recognized flags: -p/--profile, --tingly-port. Scanning stops at the first
// token that is not a recognized tingly-box flag, so everything from that
// point on is passed to claude unchanged — no "--" separator required.
func parseCCFlags(args []string) (profile string, port int, claudeArgs []string, err error) {
	i := 0
	for i < len(args) {
		switch {
		case args[i] == "--profile" || args[i] == "-p":
			if i+1 >= len(args) {
				return "", 0, nil, fmt.Errorf("flag %s requires a value", args[i])
			}
			profile = args[i+1]
			i += 2

		case args[i] == "--tingly-port":
			if i+1 >= len(args) {
				return "", 0, nil, fmt.Errorf("flag %s requires a value", args[i])
			}
			p, parseErr := strconv.Atoi(args[i+1])
			if parseErr != nil || p <= 0 || p > 65535 {
				return "", 0, nil, fmt.Errorf("flag --tingly-port requires a valid port number, got %q", args[i+1])
			}
			port = p
			i += 2

		default:
			// First unrecognized token — everything from here is claude's
			return profile, port, args[i:], nil
		}
	}
	return profile, port, nil, nil
}

// runCC orchestrates: ensure server → resolve profile → write settings → exec claude.
// If portOverride > 0, it takes precedence over the server's configured port.
func runCC(appManager *AppManager, profile string, portOverride int, claudeArgs []string) error {
	globalConfig := appManager.GetGlobalConfig()
	scenario := typ.ScenarioClaudeCode

	// Resolve profile if specified
	var profileID string
	var profileMeta *typ.ProfileMeta
	if profile != "" {
		resolved, err := globalConfig.ResolveProfileNameOrID(scenario, profile)
		if err != nil {
			// Profile not found — show interactive list so user can pick one
			profiles := globalConfig.GetProfiles(scenario)
			selected, selErr := selectProfileInteractive(profiles, profile)
			if selErr != nil {
				return fmt.Errorf("profile error: %w", err)
			}
			resolved = selected
		}
		profileID = resolved

		// Get profile metadata
		profiles := globalConfig.GetProfiles(scenario)
		for i := range profiles {
			if profiles[i].ID == profileID {
				profileMeta = &profiles[i]
				break
			}
		}
	}

	// Build the scenario path (with or without profile)
	scenarioPath := string(scenario)
	if profileID != "" {
		scenarioPath = string(typ.ProfiledScenarioName(scenario, profileID))
	}

	// Build base URL and token
	port := portOverride
	if port == 0 {
		port = appManager.GetServerPort()
	}
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	apiKey := globalConfig.GetModelToken()

	// Unified mode determination:
	// 1. If profile is used, use profile's unified setting
	// 2. Otherwise, use scenario flag (defaults to false/separate mode)
	var envUnified bool
	if profileMeta != nil {
		// Profile mode: use profile's unified setting
		envUnified = profileMeta.Unified
	} else {
		// Default mode: use scenario flag
		envUnified = globalConfig.GetScenarioFlag(scenario, "unified")
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
		// Default mode: same as profile mode but with a predictable file name
		// and no profile suffix in the scenario path.
		settingsPath, err = buildTempSettings(env, scenarioPath)
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

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to run claude CLI: %w", err)
	}
	return nil
}

// selectProfileInteractive shows a numbered list of profiles and prompts the
// user to select one. notFoundName is the profile name/ID the user originally
// requested (used in the error message when profiles is empty).
// Returns the selected profile ID, or an error if no selection can be made.
func selectProfileInteractive(profiles []typ.ProfileMeta, notFoundName string) (string, error) {
	if len(profiles) == 0 {
		if notFoundName != "" {
			return "", fmt.Errorf("profile '%s' not found and no profiles are configured", notFoundName)
		}
		return "", fmt.Errorf("no profiles configured")
	}

	if notFoundName != "" {
		fmt.Fprintf(os.Stderr, "Profile '%s' not found. Available profiles:\n", notFoundName)
	} else {
		fmt.Fprintln(os.Stderr, "Available profiles:")
	}
	for i, p := range profiles {
		mode := "separate"
		if p.Unified {
			mode = "unified"
		}
		fmt.Fprintf(os.Stderr, "  [%d] %s (%s, %s)\n", i+1, p.Name, p.ID, mode)
	}
	fmt.Fprintf(os.Stderr, "  [0] Continue without profile\n")
	fmt.Fprintf(os.Stderr, "Select profile [1-%d, 0 to skip]: ", len(profiles))

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("no input")
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" || line == "0" {
		return "", nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(profiles) {
		return "", fmt.Errorf("invalid selection '%s'", line)
	}
	return profiles[n-1].ID, nil
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
	result, err := config.ApplyClaudeSettingsToPath(destPath, env, config.WithBackup(false), config.WithExtra("statusLine", statusLine))
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

// buildTempSettings creates a temporary settings file that is equivalent to
// profile mode: copies user's ~/.claude/settings.json as base, then applies
// tingly-box env vars and status line config. The file is created in a temp
// directory with a predictable name so it can be reused across launches.
func buildTempSettings(env map[string]string, scenarioPath string) (string, error) {
	tmpDir := filepath.Join(constant.GetTinglyConfDir(), "claude")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	destPath := filepath.Join(tmpDir, "default.json")

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

	// Install the base status line script
	if _, _, err := config.InstallStatusLineScript(); err != nil {
		return "", fmt.Errorf("failed to install status line script: %w", err)
	}

	// Generate a wrapper script that sets TINGLY_SCENARIO
	wrapperPath, err := buildProfileStatusLineScript(tmpDir, "default", scenarioPath)
	if err != nil {
		return "", fmt.Errorf("failed to create status line wrapper: %w", err)
	}

	// Apply tingly-box env vars + statusLine config
	statusLine := map[string]any{
		"type":    "command",
		"command": wrapperPath,
	}
	result, err := config.ApplyClaudeSettingsToPath(destPath, env, config.WithBackup(false), config.WithExtra("statusLine", statusLine))
	if err != nil {
		return "", fmt.Errorf("failed to apply settings: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("failed to apply settings: %s", result.Message)
	}

	return destPath, nil
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
		if isProfile {
			// Profile unified mode: use "cc" to match the profile rule
			env["ANTHROPIC_MODEL"] = "cc"
			env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "cc"
			env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "cc"
			env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "cc"
			env["CLAUDE_CODE_SUBAGENT_MODEL"] = "cc"
		} else {
			// Non-profile unified mode: use full model name
			env["ANTHROPIC_MODEL"] = "tingly/cc"
			env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc"
			env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc"
			env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc"
			env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc"
		}
	} else if isProfile {
		// Profile separate mode: use short names (rules have stripped prefix)
		env["ANTHROPIC_MODEL"] = "default"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "haiku"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "opus"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "sonnet"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "subagent"
	} else {
		// Non-profile separate mode: use full model names
		env["ANTHROPIC_MODEL"] = "tingly/cc-default"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc-haiku"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc-opus"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc-sonnet"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc-subagent"
	}

	return env
}
