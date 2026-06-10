package command

import (
	"fmt"
	"os"
	"sort"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============== Kong Command Structures ==============

// ProfileCmdKong is the Kong command for managing and using Claude Code profiles.
//
//	tingly-box profile              → list profiles (interactive on TTY, select to launch)
//	tingly-box profile p1           → launch Claude Code with profile p1
//	tingly-box profile p1 --port 12580 → launch Claude Code with profile p1 on a remote tingly-box
//	tingly-box profile --list       → list all profiles (non-interactive)
//	tingly-box profile --show p1    → show profile details
type ProfileCmdKong struct {
	ProfileID string `kong:"arg,optional,help='Profile name or ID to launch Claude Code with'"`
	List      bool   `kong:"flag,name='list',help='List all profiles'"`
	Show      bool   `kong:"flag,name='show',help='Show profile details instead of launching'"`
	Port      int    `kong:"flag,name='port',help='Connect to tingly-box on the specified port (default: from config)'"`
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
		return profileUse(appManager, p.ProfileID, p.Port)
	}

	// No positional arg: interactive mode (lists profiles + prompt)
	return profileLaunchInteractive(appManager, p.Port)
}

// ============== Business Logic ==============

const profileScenario = typ.ScenarioClaudeCode

// profileList prints all profiles for the claude_code scenario.
func profileList(appManager *AppManager) error {
	globalConfig := appManager.GetGlobalConfig()
	profiles := globalConfig.GetProfiles(profileScenario)

	if len(profiles) == 0 {
		fmt.Println("No profiles configured for Claude Code.")
		fmt.Println("Use 'tingly-box tui' to create profiles via the web UI.")
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
	globalConfig := appManager.GetGlobalConfig()

	// First try to resolve as a name or ID
	resolvedID, err := globalConfig.ResolveProfileNameOrID(profileScenario, nameOrID)
	if err != nil {
		// Try showing all profiles and let user pick
		profiles := globalConfig.GetProfiles(profileScenario)
		if len(profiles) == 0 {
			return fmt.Errorf("no profiles configured for Claude Code")
		}
		fmt.Fprintf(os.Stderr, "Profile '%s' not found.\n", nameOrID)
		selected, selErr := selectProfileInteractive(profiles, nameOrID)
		if selErr != nil {
			return fmt.Errorf("profile error: %w", err)
		}
		resolvedID = selected
	}

	meta, found := globalConfig.GetProfile(profileScenario, resolvedID)
	if !found {
		return fmt.Errorf("profile '%s' not found", resolvedID)
	}

	scenarioPath := string(typ.ProfiledScenarioName(profileScenario, resolvedID))

	// Filter rules by profiled scenario
	var rules []typ.Rule
	for _, r := range globalConfig.Rules {
		if string(r.Scenario) == scenarioPath {
			rules = append(rules, r)
		}
	}

	mode := "separate"
	if meta.Unified {
		mode = "unified"
	}

	fmt.Printf("Profile: %s (%s)\n", resolvedID, meta.Name)
	fmt.Printf("  Scenario: %s\n", scenarioPath)
	fmt.Printf("  Mode:     %s\n", mode)
	fmt.Println("  Rules:")

	if len(rules) == 0 {
		fmt.Println("    (no routing rules configured)")
	} else {
		// Sort rules by name for consistent display
		sort.Slice(rules, func(i, j int) bool {
			return rules[i].RequestModel < rules[j].RequestModel
		})

		for _, r := range rules {
			svc := ""
			if len(r.Services) > 0 && r.Services[0].Provider != "" {
				providerName := r.Services[0].Provider
				if provider, err := globalConfig.GetProviderByUUID(r.Services[0].Provider); err == nil && provider != nil {
					providerName = provider.Name
				}
				svc = fmt.Sprintf("%s / %s", providerName, r.Services[0].Model)
			} else {
				svc = "(not configured)"
			}
			status := "active"
			if !r.Active {
				status = "inactive"
			}
			fmt.Printf("    %-10s %s [%s]\n", r.RequestModel, svc, status)
		}
	}

	return nil
}

// profileShowInteractive lists profiles and prompts the user to pick one to inspect.
func profileShowInteractive(appManager *AppManager) error {
	globalConfig := appManager.GetGlobalConfig()
	profiles := globalConfig.GetProfiles(profileScenario)

	if len(profiles) == 0 {
		fmt.Println("No profiles configured for Claude Code.")
		fmt.Println("Use 'tingly-box tui' to create profiles via the web UI.")
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

// profileUse launches Claude Code with the specified profile.
// Equivalent to `tingly-box cc --profile <name>` but without passthrough args.
// If port > 0, it overrides the configured server port.
func profileUse(appManager *AppManager, nameOrID string, port int) error {
	globalConfig := appManager.GetGlobalConfig()

	// Resolve profile name → ID (handles both name and ID lookup).
	// If not found, show interactive list so the user can pick one.
	resolvedID, err := globalConfig.ResolveProfileNameOrID(profileScenario, nameOrID)
	if err != nil {
		profiles := globalConfig.GetProfiles(profileScenario)
		if len(profiles) == 0 {
			return fmt.Errorf("profile '%s' not found and no profiles are configured", nameOrID)
		}
		fmt.Fprintf(os.Stderr, "Profile '%s' not found.\n", nameOrID)
		selected, selErr := selectProfileInteractive(profiles, nameOrID)
		if selErr != nil {
			return fmt.Errorf("profile error: %w", err)
		}
		if selected == "" {
			return fmt.Errorf("no profile selected")
		}
		resolvedID = selected
	}

	// Delegate to runCC with the resolved profile ID and no passthrough args.
	return runCC(appManager, resolvedID, port, nil)
}

// profileLaunchInteractive lists profiles and prompts the user to pick one to launch.
// port > 0 overrides the configured server port.
func profileLaunchInteractive(appManager *AppManager, port int) error {
	globalConfig := appManager.GetGlobalConfig()
	profiles := globalConfig.GetProfiles(profileScenario)

	if len(profiles) == 0 {
		fmt.Println("No profiles configured for Claude Code.")
		fmt.Println("Use 'tingly-box tui' to create profiles via the web UI.")
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
