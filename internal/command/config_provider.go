package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ConfigProviderCmdKong groups provider operations under `config provider`.
// The default (no subcommand) drops into the provider sub-menu so it mirrors
// the interactive top-level config menu.
type ConfigProviderCmdKong struct {
	Interactive ConfigProviderInteractiveCmdKong `kong:"cmd,name='interactive',default='1',hidden,help='Interactive provider management'"`

	Add    ConfigProviderAddCmdKong    `kong:"cmd,help='Add a new provider'"`
	List   ConfigProviderListCmdKong   `kong:"cmd,help='List all providers'"`
	Delete ConfigProviderDeleteCmdKong `kong:"cmd,help='Delete a provider (interactive)'"`
	Update ConfigProviderUpdateCmdKong `kong:"cmd,help='Update a provider (interactive)'"`
	Get    ConfigProviderGetCmdKong    `kong:"cmd,help='Get provider details by UUID'"`
}

// ConfigProviderInteractiveCmdKong runs the provider interactive sub-menu.
type ConfigProviderInteractiveCmdKong struct{}

func (c *ConfigProviderInteractiveCmdKong) Run(appManager *AppManager) error {
	return runProviderSubMenu(appManager, bufio.NewReader(os.Stdin))
}

// ConfigProviderAddCmdKong adds a new provider.
type ConfigProviderAddCmdKong struct {
	Name     string `kong:"arg,optional,help='Provider name'"`
	BaseURL  string `kong:"arg,optional,help='API base URL'"`
	Token    string `kong:"arg,optional,help='API token'"`
	APIStyle string `kong:"arg,optional,help='API style (openai, anthropic)'"`
}

func (c *ConfigProviderAddCmdKong) Run(appManager *AppManager) error {
	args := []string{}
	if c.Name != "" {
		args = append(args, c.Name)
	}
	if c.BaseURL != "" {
		args = append(args, c.BaseURL)
	}
	if c.Token != "" {
		args = append(args, c.Token)
	}
	if c.APIStyle != "" {
		args = append(args, c.APIStyle)
	}
	return runAdd(appManager, args)
}

// ConfigProviderListCmdKong lists all providers.
type ConfigProviderListCmdKong struct{}

func (c *ConfigProviderListCmdKong) Run(appManager *AppManager) error {
	return runProviderList(appManager)
}

// ConfigProviderDeleteCmdKong deletes a provider via interactive selection.
type ConfigProviderDeleteCmdKong struct{}

func (c *ConfigProviderDeleteCmdKong) Run(appManager *AppManager) error {
	return runProviderDeleteInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ConfigProviderUpdateCmdKong updates a provider via interactive selection.
type ConfigProviderUpdateCmdKong struct{}

func (c *ConfigProviderUpdateCmdKong) Run(appManager *AppManager) error {
	return runProviderUpdateInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ConfigProviderGetCmdKong displays a provider's details by UUID. Names are
// not unique (UUID is the PK), so we require UUID. Empty UUID drops to
// interactive selection.
type ConfigProviderGetCmdKong struct {
	UUID string `kong:"arg,optional,help='Provider UUID'"`
}

func (c *ConfigProviderGetCmdKong) Run(appManager *AppManager) error {
	if c.UUID == "" {
		return runProviderGetInteractive(appManager, bufio.NewReader(os.Stdin))
	}
	return runProviderGet(appManager, c.UUID)
}

// runProviderSubMenu shows the provider sub-menu (reached from `config` or
// `config provider` with no further args). Returning nil drops back to the
// caller, which is either the top-level menu loop or the Kong command.
func runProviderSubMenu(appManager *AppManager, reader *bufio.Reader) error {
	for {
		showProviderSubMenu()
		fmt.Print("Select an option (1-5, 0 to go back): ")

		input, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				return nil
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

		switch choice {
		case "1":
			if err := runAdd(appManager, []string{}); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "2":
			if err := runProviderList(appManager); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "3":
			if err := runProviderUpdateInteractive(appManager, reader); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "4":
			if err := runProviderDeleteInteractive(appManager, reader); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "5":
			if err := runProviderGetInteractive(appManager, reader); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "0":
			return nil
		default:
			fmt.Println("Invalid choice. Please select 1-5 or 0 to go back.")
		}

		fmt.Println("\nPress Enter to continue...")
		_, _ = reader.ReadString('\n')
	}
}

// showProviderSubMenu displays the provider sub-menu.
func showProviderSubMenu() {
	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("Provider Management")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("1. Add a new provider")
	fmt.Println("2. List all providers")
	fmt.Println("3. Update a provider")
	fmt.Println("4. Delete a provider")
	fmt.Println("5. View provider details")
	fmt.Println()
	fmt.Println("0. Back")
	fmt.Println(strings.Repeat("-", 60))
}
