package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/agent"
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
	settingsPath, err := agent.BuildCCProfileSettings(settingsID, scenarioPath, env)
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
