package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ConfigCmdKong is the unified configuration management command.
// Two second-level groups are exposed:
//
//	config provider — provider CRUD
//	config rule     — rule CRUD + import/export
//
// Finer-grained operations live under those groups and only appear in
// `config provider --help` / `config rule --help` (not in `config --help`).
type ConfigCmdKong struct {
	// Interactive mode (default when no subcommand is given)
	Interactive ConfigInteractiveCmdKong `kong:"cmd,name='interactive',default='1',hidden,help='Interactive configuration management'"`

	Provider ConfigProviderCmdKong `kong:"cmd,help='Manage providers (add/list/update/delete/get)'"`
	Rule     ConfigRuleCmdKong     `kong:"cmd,help='Manage routing rules (add/list/update/delete/export/import)'"`
}

// ConfigInteractiveCmdKong runs the interactive config menu.
type ConfigInteractiveCmdKong struct{}

func (c *ConfigInteractiveCmdKong) Run(appManager *AppManager) error {
	return runConfigInteractiveMode(appManager)
}

// runConfigInteractiveMode runs the top-level interactive configuration menu.
// It dispatches into either the provider or rule sub-menu.
func runConfigInteractiveMode(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		showConfigMenu()
		fmt.Print("Select an option (1-2, 0 to exit): ")

		input, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("\n👋 Exiting configuration management...")
				return nil
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

		switch choice {
		case "1":
			if err := runProviderSubMenu(appManager, reader); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "2":
			if err := runRuleSubMenu(appManager, reader); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "0":
			fmt.Println("Exiting configuration management...")
			return nil
		default:
			fmt.Println("Invalid choice. Please select 1-2 or 0 to exit.")
		}
	}
}

// showConfigMenu displays the top-level configuration management menu.
func showConfigMenu() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Configuration Management")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("1. Manage providers")
	fmt.Println("2. Manage rules")
	fmt.Println()
	fmt.Println("0. Exit")
	fmt.Println(strings.Repeat("=", 60))
}
