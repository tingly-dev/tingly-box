package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/dataio"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ConfigCmdKong is the unified configuration management command.
// It replaces provider/export/import commands with a single cohesive interface.
type ConfigCmdKong struct {
	// Interactive mode (default when no flags)
	Interactive ConfigInteractiveCmdKong `kong:"cmd,name='interactive',default='1',hidden,help='Interactive configuration management'"`

	// Provider management (via positional args for add, flags for other operations)
	Add    ConfigAddCmdKong    `kong:"cmd,help='Add a new provider'"`
	List   ConfigListCmdKong   `kong:"cmd,help='List all providers'"`
	Delete ConfigDeleteCmdKong `kong:"cmd,help='Delete a provider (interactive)'"`
	Update ConfigUpdateCmdKong `kong:"cmd,help='Update a provider (interactive)'"`
	Get    ConfigGetCmdKong    `kong:"cmd,help='Get provider details by name'"`

	// Import/Export
	Export ConfigExportCmdKong `kong:"cmd,help='Export configuration'"`
	Import ConfigImportCmdKong `kong:"cmd,help='Import configuration'"`
}

// ConfigInteractiveCmdKong runs the interactive config menu.
type ConfigInteractiveCmdKong struct{}

func (c *ConfigInteractiveCmdKong) Run(appManager *AppManager) error {
	return runConfigInteractiveMode(appManager)
}

// ConfigAddCmdKong adds a new provider
type ConfigAddCmdKong struct {
	Name     string `kong:"arg,optional,help='Provider name'"`
	BaseURL  string `kong:"arg,optional,help='API base URL'"`
	Token    string `kong:"arg,optional,help='API token'"`
	APIStyle string `kong:"arg,optional,help='API style (openai, anthropic)'"`
}

func (c *ConfigAddCmdKong) Run(appManager *AppManager) error {
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

// ConfigListCmdKong lists all providers
type ConfigListCmdKong struct{}

func (c *ConfigListCmdKong) Run(appManager *AppManager) error {
	return runProviderList(appManager)
}

// ConfigDeleteCmdKong deletes a provider in interactive mode.
type ConfigDeleteCmdKong struct{}

func (c *ConfigDeleteCmdKong) Run(appManager *AppManager) error {
	return runProviderDeleteInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ConfigUpdateCmdKong updates a provider in interactive mode.
type ConfigUpdateCmdKong struct{}

func (c *ConfigUpdateCmdKong) Run(appManager *AppManager) error {
	return runProviderUpdateInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ConfigGetCmdKong displays a provider's details. Without a name it drops
// into interactive selection.
type ConfigGetCmdKong struct {
	Name string `kong:"arg,optional,help='Provider name'"`
}

func (c *ConfigGetCmdKong) Run(appManager *AppManager) error {
	if c.Name == "" {
		return runProviderGetInteractive(appManager, bufio.NewReader(os.Stdin))
	}
	return runProviderGet(appManager, c.Name)
}

// ConfigExportCmdKong exports configuration to file or stdout
type ConfigExportCmdKong struct {
	RequestModel string `kong:"flag,name='request-model',required,help='Request model name'"`
	Scenario     string `kong:"flag,name='scenario',required,help='Rule scenario'"`
	Format       string `kong:"flag,name='format',default='jsonl',help='Export format: jsonl or base64'"`
	Output       string `kong:"flag,name='output',help='Output file path (default: stdout)'"`
}

func (c *ConfigExportCmdKong) Run(appManager *AppManager) error {
	var format dataio.Format
	switch strings.ToLower(c.Format) {
	case "jsonl":
		format = dataio.FormatJSONL
	case "base64":
		format = dataio.FormatBase64
	default:
		return fmt.Errorf("invalid format '%s': supported formats are jsonl and base64", c.Format)
	}

	// Get the rule
	globalConfig := appManager.AppConfig().GetGlobalConfig()
	rule := globalConfig.GetRuleByRequestModelAndScenario(c.RequestModel, typ.RuleScenario(c.Scenario))
	if rule == nil {
		return fmt.Errorf("rule not found for request-model '%s' and scenario '%s'", c.RequestModel, c.Scenario)
	}

	// Collect providers from the rule
	providers, err := appManager.CollectProvidersFromRule(rule)
	if err != nil {
		return fmt.Errorf("failed to collect providers: %w", err)
	}

	// Export the rule with its providers
	content, err := appManager.ExportRule(rule, providers, format)
	if err != nil {
		return fmt.Errorf("failed to export rule: %w", err)
	}

	// Write to file or stdout
	if c.Output != "" {
		err := os.WriteFile(c.Output, []byte(content), 0644)
		if err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
		fmt.Printf("✓ Rule exported to %s\n", c.Output)
	} else {
		fmt.Print(content)
	}

	return nil
}

// ConfigImportCmdKong imports configuration from file or stdin
type ConfigImportCmdKong struct {
	File   string `kong:"arg,optional,help='Import file path (reads from stdin if omitted)'"`
	Format string `kong:"flag,name='format',default='auto',help='Import format: auto, jsonl, or base64'"`
}

func (c *ConfigImportCmdKong) Run(appManager *AppManager) error {
	// Build args slice - only include file if non-empty
	var args []string
	if c.File != "" {
		args = []string{c.File}
	}
	// Reuse existing import logic
	return runImport(appManager, c.Format, args)
}

// runConfigInteractiveMode runs the interactive configuration management interface
func runConfigInteractiveMode(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		showConfigMenu()
		fmt.Print("Select an option (1-6, 0 to exit): ")

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
		case "6":
			// Import/Export submenu
			runImportExportMenu(appManager, reader)
		case "0":
			fmt.Println("Exiting configuration management...")
			return nil
		default:
			fmt.Println("Invalid choice. Please select 1-6 or 0 to exit.")
		}

		fmt.Println("\nPress Enter to continue...")
		_, _ = reader.ReadString('\n')
	}
}

// showConfigMenu displays the configuration management menu
func showConfigMenu() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Configuration Management")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("1. Add a new provider")
	fmt.Println("2. List all providers")
	fmt.Println("3. Update a provider")
	fmt.Println("4. Delete a provider")
	fmt.Println("5. View provider details")
	fmt.Println("6. Import/Export configuration")
	fmt.Println()
	fmt.Println("0. Exit")
	fmt.Println(strings.Repeat("=", 60))
}

// runImportExportMenu shows the import/export submenu
func runImportExportMenu(appManager *AppManager, reader *bufio.Reader) {
	for {
		fmt.Println("\n" + strings.Repeat("-", 40))
		fmt.Println("Import/Export")
		fmt.Println(strings.Repeat("-", 40))
		fmt.Println("1. Export configuration")
		fmt.Println("2. Import configuration")
		fmt.Println("0. Back to main menu")
		fmt.Print("\nSelect an option: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			return
		}

		choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

		switch choice {
		case "1":
			// Export
			fmt.Print("Request model: ")
			requestModel, _ := reader.ReadString('\n')
			requestModel = strings.TrimSpace(requestModel)

			fmt.Print("Scenario: ")
			scenario, _ := reader.ReadString('\n')
			scenario = strings.TrimSpace(scenario)

			fmt.Print("Output file (press Enter for stdout): ")
			outputFile, _ := reader.ReadString('\n')
			outputFile = strings.TrimSpace(outputFile)

			if err := runExport(appManager, requestModel, scenario, "jsonl", outputFile); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "2":
			// Import
			fmt.Print("Input file (press Enter for stdin): ")
			inputFile, _ := reader.ReadString('\n')
			inputFile = strings.TrimSpace(inputFile)

			args := []string{}
			if inputFile != "" {
				args = append(args, inputFile)
			}
			if err := runImport(appManager, "auto", args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "0":
			return
		default:
			fmt.Println("Invalid choice.")
		}
		if choice != "0" {
			fmt.Println("\nPress Enter to continue...")
			_, _ = reader.ReadString('\n')
		}
	}
}
