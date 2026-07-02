package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============== Kong Command Structures ==============

// CCmdKong launches Claude Code with tingly-box-specific flags.
// Put tingly-box flags before Claude Code args; unknown flags are passed through
// to Claude Code so users do not need to insert a literal '--'.
type CCmdKong struct {
	Profile string   `kong:"flag,name='profile',help='Claude Code profile to use'"`
	Port    int      `kong:"flag,name='port',help='Tingly-Box server port (default: from config or 12580)'"`
	Args    []string `kong:"arg,optional,passthrough='all',help='Additional arguments to pass to Claude Code (e.g., --model opus)'"`
}

func (c *CCmdKong) Run(appManager *AppManager) error {
	// Check if user wants help in Claude args (e.g., "cc -- --help")
	// This is handled by passing --help to Claude, not by showing tingly-box help
	// Use --port if provided, otherwise 0 (will fallback to config)
	port := c.Port
	return runCC(appManager, c.Profile, port, c.Args)
}

// ============== Business Logic Functions ==============

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
	baseURL := fmt.Sprintf("http://localhost:%d", port)
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
		if sc := globalConfig.GetScenarioConfig(scenario); sc != nil {
			envUnified = sc.GetDefaultFlags().Unified
		}
	}
	env := agent.GenerateCCEnv(globalConfig, baseURL, apiKey, scenarioPath, envUnified, profileID != "")

	// Build settings file. Profile mode uses the profileID; default mode uses
	// "default" as a stable, predictable name so the file is reused across runs.
	settingsID := profileID
	if settingsID == "" {
		settingsID = "default"
	}
	profileName := ""
	if profileMeta != nil {
		profileName = profileMeta.Name
	}
	settingsPath, err := agent.BuildCCProfileSettings(settingsID, scenarioPath, profileName, env)
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

	// Exec replaces current process (on Windows, which has no true exec(),
	// this instead starts the child detached and exits immediately) so
	// tingly-box does not remain resident for the claude session.
	binPath := variant.Path
	//nolint:gosec // intentional exec of user-installed CLI
	if err := execReplace(binPath, execArgs, os.Environ()); err != nil {
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
