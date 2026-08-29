package command

import (
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// ============== Kong Command Structures ==============

// ProfileCmdKong is the Kong command for managing and using Claude Code profiles.
//
// Management mode:
//
//	tingly-box profile --list             → list all profiles (non-interactive)
//	tingly-box profile --show p1          → show profile details
//
// Launch mode:
//
//	tingly-box profile                    → list profiles (interactive on TTY, select to launch)
//	tingly-box profile p1                 → launch Claude Code with profile p1
//	tingly-box profile --port 12580 p1    → launch Claude Code with profile p1 on port 12580
//	tingly-box profile p1 --model opus    → launch with profile p1, passing --model opus to Claude
//	tingly-box profile p1 -p hi           → launch with profile p1, using Claude's print mode
//
// Put tingly-box flags before the profile name; after the profile name, flags are
// treated as Claude Code arguments. A literal '--' still works for compatibility.
type ProfileCmdKong struct {
	List      bool     `kong:"flag,name='list',help='List all profiles (non-interactive)'"`
	Show      bool     `kong:"flag,name='show',help='Show profile details instead of launching'"`
	Port      int      `kong:"flag,name='port',help='Connect to tingly-box on the specified port (default: detected from running server, else config)'"`
	ProfileID string   `kong:"arg,optional,help='Profile name or ID to launch Claude Code with'"`
	Args      []string `kong:"arg,optional,passthrough='all',help='Additional arguments to pass to Claude Code (e.g., --model opus)'"`
}

func (p *ProfileCmdKong) Run(appManager *AppManager) error {
	// Validate: --list and --show are mutually exclusive
	if p.List && p.Show {
		return fmt.Errorf("--list and --show are mutually exclusive")
	}

	if p.Show {
		if p.ProfileID == "" {
			// Show all profiles then let user pick one
			return profileShowInteractive(appManager)
		}
		return profileShow(appManager, p.ProfileID)
	}

	if p.List {
		return profileList(appManager)
	}

	// No flags: either launch with the provided profile, or show interactive list
	if p.ProfileID != "" {
		return profileUse(appManager, p.ProfileID, p.Port, p.Args)
	}

	// No positional arg: interactive mode (lists profiles + prompt)
	// Note: In interactive mode, we don't support additional Claude args
	if len(p.Args) > 0 {
		return fmt.Errorf("additional arguments are only supported when a profile is specified")
	}
	return profileLaunchInteractive(appManager, p.Port)
}

// ============== Business Logic ==============

const profileScenario = typ.ScenarioClaudeCode

// profileList prints all profiles for the claude_code scenario.
func profileList(appManager *AppManager) error {
	profiles := usecase.NewProfileUseCase(appManager.GetGlobalConfig()).List(usecase.ListProfilesRequest{
		Scenario: profileScenario,
	}).Profiles

	if len(profiles) == 0 {
		fmt.Println("No profiles configured for Claude Code.")
		fmt.Println("Use 'tingly-box tui' / 'tb tui' to create profiles via the web UI.")
		return nil
	}

	fmt.Println("Claude Code profiles:")
	for _, p := range profiles {
		mode := "separate"
		if p.Unified {
			mode = "unified"
		}
		fmt.Printf("  %s  %-20s %s\n", p.ID, p.Name, mode)
	}
	return nil
}

// profileShow prints detailed information about a specific profile.
func profileShow(appManager *AppManager, nameOrID string) error {
	profileUC := usecase.NewProfileUseCase(appManager.GetGlobalConfig())
	result, err := profileUC.Get(usecase.GetProfileRequest{
		Scenario:   profileScenario,
		Identifier: nameOrID,
	})
	if err != nil {
		// Try showing all profiles and let user pick
		profiles := profileUC.List(usecase.ListProfilesRequest{Scenario: profileScenario}).Profiles
		if len(profiles) == 0 {
			return fmt.Errorf("no profiles configured for Claude Code")
		}
		fmt.Fprintf(os.Stderr, "Profile '%s' not found.\n", nameOrID)
		selected, selErr := selectProfileInteractive(profiles, nameOrID)
		if selErr != nil {
			return fmt.Errorf("profile error: %w", selErr)
		}
		if selected == "" {
			return nil
		}
		result, err = profileUC.Get(usecase.GetProfileRequest{
			Scenario:   profileScenario,
			Identifier: selected,
		})
		if err != nil {
			return err
		}
	}

	mode := "separate"
	if result.Profile.Unified {
		mode = "unified"
	}

	fmt.Printf("Profile: %s (%s)\n", result.Profile.ID, result.Profile.Name)
	fmt.Printf("  Scenario: %s\n", result.Scenario)
	fmt.Printf("  Mode:     %s\n", mode)
	fmt.Println("  Rules:")

	if len(result.Rules) == 0 {
		fmt.Println("    (no routing rules configured)")
	} else {
		for _, rule := range result.Rules {
			svc := "(not configured)"
			if rule.Configured {
				svc = fmt.Sprintf("%s / %s", rule.ProviderName, rule.Model)
			}
			status := "active"
			if !rule.Active {
				status = "inactive"
			}
			fmt.Printf("    %-10s %s [%s]\n", rule.RequestModel, svc, status)
		}
	}

	return nil
}

// profileShowInteractive lists profiles and prompts the user to pick one to inspect.
func profileShowInteractive(appManager *AppManager) error {
	profiles := usecase.NewProfileUseCase(appManager.GetGlobalConfig()).List(usecase.ListProfilesRequest{
		Scenario: profileScenario,
	}).Profiles

	if len(profiles) == 0 {
		fmt.Println("No profiles configured for Claude Code.")
		fmt.Println("Use 'tingly-box tui' / 'tb tui' to create profiles via the web UI.")
		return nil
	}

	selected, err := selectProfileInteractive(profiles, "")
	if err != nil {
		return err
	}
	if selected == "" {
		return nil // user chose to skip
	}
	return profileShow(appManager, selected)
}

// profileUse launches Claude Code with the specified profile via runCC —
// the same underlying call `tingly-box cc --profile <name>` makes; this is
// now the preferred, documented form (see CCmdKong's deprecation note).
// If port > 0, it overrides the configured server port.
// Additional args are passed to Claude Code.
func profileUse(appManager *AppManager, nameOrID string, port int, extraArgs []string) error {
	profileUC := usecase.NewProfileUseCase(appManager.GetGlobalConfig())

	// Resolve profile name → ID (handles both name and ID lookup).
	// If not found, show interactive list so the user can pick one.
	resolved, err := profileUC.Resolve(usecase.GetProfileRequest{
		Scenario:   profileScenario,
		Identifier: nameOrID,
	})
	if err != nil {
		profiles := profileUC.List(usecase.ListProfilesRequest{Scenario: profileScenario}).Profiles
		if len(profiles) == 0 {
			return fmt.Errorf("profile '%s' not found and no profiles are configured", nameOrID)
		}
		fmt.Fprintf(os.Stderr, "Profile '%s' not found.\n", nameOrID)
		selected, selErr := selectProfileInteractive(profiles, nameOrID)
		if selErr != nil {
			return fmt.Errorf("profile error: %w", selErr)
		}
		if selected == "" {
			return fmt.Errorf("no profile selected")
		}
		resolved, err = profileUC.Resolve(usecase.GetProfileRequest{
			Scenario:   profileScenario,
			Identifier: selected,
		})
		if err != nil {
			return err
		}
	}

	// Delegate to runCC with the resolved profile ID and additional args.
	return runCC(appManager, resolved.Profile.ID, port, extraArgs)
}

// profileLaunchInteractive lists profiles and prompts the user to pick one to launch.
// port > 0 overrides the configured server port.
func profileLaunchInteractive(appManager *AppManager, port int) error {
	profiles := usecase.NewProfileUseCase(appManager.GetGlobalConfig()).List(usecase.ListProfilesRequest{
		Scenario: profileScenario,
	}).Profiles

	if len(profiles) == 0 {
		fmt.Println("No profiles configured for Claude Code.")
		fmt.Println("Use 'tingly-box tui' / 'tb tui' to create profiles via the web UI.")
		return nil
	}

	selected, err := selectProfileInteractive(profiles, "")
	if err != nil {
		return err
	}
	if selected == "" {
		fmt.Println("Cancelled.")
		return nil // user chose to skip
	}
	return runCC(appManager, selected, port, nil)
}
