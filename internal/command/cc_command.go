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
	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// ============== Kong Command Structures ==============

// CCmdKong launches Claude Code with tingly-box-specific flags.
// Put tingly-box flags before Claude Code args; unknown flags are passed through
// to Claude Code so users do not need to insert a literal '--'.
//
// --profile duplicates `tingly-box profile <id>` (both resolve through
// runCC below) and is kept only for existing scripts/muscle memory; prefer
// `profile` for anything profile-related; see profile_command.go.
type CCmdKong struct {
	Profile string   `kong:"flag,name='profile',help='Claude Code profile to use (deprecated: prefer \"tingly-box profile <id>\")'"`
	Port    int      `kong:"flag,name='port',help='Tingly-Box server port (default: detected from running server, else config or 12580)'"`
	Args    []string `kong:"arg,optional,passthrough='all',help='Additional arguments to pass to Claude Code (e.g., --model opus)'"`
}

func (c *CCmdKong) Run(appManager *app.AppManager) error {
	// Check if user wants help in Claude args (e.g., "cc -- --help")
	// This is handled by passing --help to Claude, not by showing tingly-box help
	// Use --port if provided, otherwise 0 (will fallback to config)
	port := c.Port
	if c.Profile != "" {
		fmt.Fprintln(os.Stderr, "Note: 'tingly-box cc --profile' is deprecated; use 'tingly-box profile "+c.Profile+"' instead.")
	}
	return runCC(appManager, c.Profile, port, c.Args)
}

// ============== Business Logic Functions ==============

// runCC orchestrates: ensure server → resolve profile → write settings → exec claude.
// If portOverride > 0, it takes precedence over the server's configured port.
func runCC(appManager *app.AppManager, profile string, portOverride int, claudeArgs []string) error {
	globalConfig := appManager.GetGlobalConfig()
	scenario := typ.ScenarioClaudeCode

	// Resolve profile if specified
	var profileID string
	var profileMeta *typ.ProfileMeta
	if profile != "" {
		profileUC := usecase.NewProfileUseCase(globalConfig)
		resolved, err := profileUC.Resolve(usecase.GetProfileRequest{
			Scenario:   scenario,
			Identifier: profile,
		})
		if err != nil {
			// Profile not found — show interactive list so user can pick one
			profiles := profileUC.List(usecase.ListProfilesRequest{Scenario: scenario}).Profiles
			selected, selErr := selectProfileInteractive(profiles, profile)
			if selErr != nil {
				return fmt.Errorf("profile error: %w", selErr)
			}
			if selected != "" {
				resolved, err = profileUC.Resolve(usecase.GetProfileRequest{
					Scenario:   scenario,
					Identifier: selected,
				})
				if err != nil {
					return err
				}
			}
		}
		if resolved.Profile.ID != "" {
			profileID = resolved.Profile.ID
			meta := resolved.Profile
			profileMeta = &meta
		}
	}

	// Build the scenario path (with or without profile)
	scenarioPath := string(scenario)
	if profileID != "" {
		scenarioPath = string(typ.ProfiledScenarioName(scenario, profileID))
	}

	// Build base URL and token. Without an explicit --port, prefer the port
	// the running server actually listens on (runtime port file) over the
	// configured default, so `--port` at server start doesn't need repeating.
	port := portOverride
	if port == 0 {
		port = appManager.GetRuntimeServerPort()
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
	// Build settings file. Profile mode uses the profileID; default mode uses
	// "default" as a stable, predictable name so the file is reused across runs.
	var settingsPath string
	var err error
	if profileMeta != nil {
		settingsPath, err = agent.MaterializeCCProfileSettings(globalConfig, baseURL, apiKey, scenarioPath, *profileMeta)
	} else {
		env := agent.GenerateCCEnv(globalConfig, baseURL, apiKey, scenarioPath, envUnified, false)
		settingsPath, err = agent.BuildCCProfileSettings("default", scenarioPath, "", env)
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

// selectProfileInteractive lists the profiles and prompts the user to pick one
// (see matchProfileInput for accepted input; "0"/empty skips, re-prompts up to
// 3 times on bad input). notFoundName is the profile name/ID the user
// originally requested, used only in messaging.
// Returns the selected profile ID, "" to skip, or an error.
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
	for _, p := range profiles {
		fmt.Fprintf(os.Stderr, "  [%s] %s (%s)\n", p.ID, p.Name, profileModeLabel(p))
	}
	fmt.Fprintf(os.Stderr, "  [0] Continue without profile\n")

	scanner := bufio.NewScanner(os.Stdin)
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		fmt.Fprintf(os.Stderr, "Select profile (ID, number or name; 0 to skip): ")
		if !scanner.Scan() {
			return "", fmt.Errorf("no input")
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "0" {
			return "", nil
		}
		id, err := matchProfileInput(profiles, line)
		if err == nil {
			return id, nil
		}
		lastErr = err
		fmt.Fprintf(os.Stderr, "  %v\n", err)
	}
	return "", lastErr
}

// matchProfileInput resolves free-form input to a profile ID, in order:
//  1. exact ID or name (case-insensitive);
//  2. pure digits N -> ID "pN", strictly (not a list position, so it stays
//     stable when profiles are deleted, and never falls back to fuzzy);
//  3. a unique ID/name prefix, then a unique substring.
func matchProfileInput(profiles []typ.ProfileMeta, input string) (string, error) {
	for _, p := range profiles {
		if strings.EqualFold(p.ID, input) || strings.EqualFold(p.Name, input) {
			return p.ID, nil
		}
	}
	if n, err := strconv.Atoi(input); err == nil {
		id := fmt.Sprintf("p%d", n)
		for _, p := range profiles {
			if p.ID == id {
				return p.ID, nil
			}
		}
		return "", fmt.Errorf("no profile with ID %s", id)
	}

	lower := strings.ToLower(input)
	for _, match := range []func(s string) bool{
		func(s string) bool { return strings.HasPrefix(s, lower) },
		func(s string) bool { return strings.Contains(s, lower) },
	} {
		var hits []typ.ProfileMeta
		for _, p := range profiles {
			if match(strings.ToLower(p.ID)) || match(strings.ToLower(p.Name)) {
				hits = append(hits, p)
			}
		}
		switch len(hits) {
		case 0:
			continue
		case 1:
			return hits[0].ID, nil
		default:
			names := make([]string, len(hits))
			for i, h := range hits {
				names[i] = h.Name
			}
			return "", fmt.Errorf("'%s' is ambiguous: matches %s", input, strings.Join(names, ", "))
		}
	}
	return "", fmt.Errorf("no profile matches '%s'", input)
}
