package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ============== Kong Command Structures (Config) ==============

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
	return runExport(appManager, c.RequestModel, c.Scenario, c.Format, c.Output)
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

// ============== Business Logic Functions ==============

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

// runAdd handles the provider addition process with both positional arguments and interactive mode
func runAdd(appManager *AppManager, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	var name, apiBase, token string
	var apiStyle APIStyle = protocol.APIStyleOpenAI // default to openai

	// Extract values from positional arguments if provided
	if len(args) > 0 {
		name = args[0]
	}
	if len(args) > 1 {
		apiBase = args[1]
	}
	if len(args) > 2 {
		token = args[2]
	}
	if len(args) > 3 {
		// Validate and set API style
		switch strings.ToLower(args[3]) {
		case "openai":
			apiStyle = protocol.APIStyleOpenAI
		case "anthropic":
			apiStyle = protocol.APIStyleAnthropic
		default:
			return fmt.Errorf("invalid API style '%s'. Supported values: openai, anthropic", args[3])
		}
	}

	// If we have all required arguments, skip interactive prompts
	if len(args) >= 3 {
		return addProviderWithConfirmation(appManager, reader, name, apiBase, token, apiStyle)
	}

	// Interactive mode for missing values
	fmt.Println("Let's add a new AI provider configuration.")
	if len(args) > 0 {
		fmt.Printf("Using provided name: %s\n", name)
	}
	if len(args) > 1 {
		fmt.Printf("Using provided API base URL: %s\n", apiBase)
	}
	if len(args) > 2 {
		fmt.Printf("Using provided token: %s\n", maskToken(token))
	}
	if len(args) > 3 {
		fmt.Printf("Using provided API style: %s\n", apiStyle)
	}
	fmt.Println()

	// Get provider name (if not provided)
	if name == "" {
		var err error
		name, err = promptForInput(reader, "Enter provider name (e.g., openai, anthropic): ", true)
		if err != nil {
			return err
		}
	}

	// Check if provider already exists
	if existingProvider, err := appManager.GetProvider(name); err == nil && existingProvider != nil {
		fmt.Printf("Provider '%s' already exists. Please use a different name or update the existing provider.\n", name)
		return fmt.Errorf("provider already exists")
	}

	// Get API base URL (if not provided)
	if apiBase == "" {
		var err error
		apiBase, err = promptForInput(reader, "Enter API base URL (e.g., https://api.openai.com/v1): ", true)
		if err != nil {
			return err
		}
	}

	// Get API token (if not provided)
	if token == "" {
		var err error
		token, err = promptForInput(reader, "Enter API token: ", true)
		if err != nil {
			return err
		}
	}

	// Get API style (if not provided)
	if len(args) < 4 {
		var err error
		apiStyle, err = promptForAPIStyle(reader, name, apiBase)
		if err != nil {
			return err
		}
	}

	return addProviderWithConfirmation(appManager, reader, name, apiBase, token, apiStyle)
}

// addProviderWithConfirmation displays summary and adds the provider
func addProviderWithConfirmation(appManager *AppManager, reader *bufio.Reader, name, apiBase, token string, apiStyle APIStyle) error {
	// Display summary and get confirmation
	fmt.Println("\n--- Configuration Summary ---")
	fmt.Printf("Provider Name: %s\n", name)
	fmt.Printf("API Base URL: %s\n", apiBase)
	fmt.Printf("API Style: %s\n", apiStyle)
	fmt.Printf("Token: %s\n", maskToken(token))
	fmt.Println("---------------------------")

	confirmed, err := promptForConfirmation(reader, "Do you want to save this configuration? (Y/n): ")
	if err != nil {
		return err
	}

	if !confirmed {
		fmt.Println("Operation cancelled.")
		return nil
	}

	// Add the provider using AppManager
	if err := appManager.AddProvider(name, apiBase, token, apiStyle); err != nil {
		return fmt.Errorf("failed to add provider: %w", err)
	}

	fmt.Printf("Successfully added provider '%s' with API style '%s'\n", name, apiStyle)
	return nil
}

// promptForInput prompts the user for input and returns the trimmed response
func promptForInput(reader *bufio.Reader, prompt string, required bool) (string, error) {
	for {
		fmt.Print(prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		if required && input == "" {
			fmt.Println("This field is required. Please enter a value.")
			continue
		}

		return input, nil
	}
}

// promptForConfirmation prompts the user for a yes/no confirmation
func promptForConfirmation(reader *bufio.Reader, prompt string) (bool, error) {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.ToLower(strings.TrimSpace(input))
	// Default to Yes if user just presses Enter
	return input == "" || input == "y" || input == "yes", nil
}

// maskToken masks the API token for display purposes
func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

// promptForAPIStyle prompts the user to select an API style with intelligent defaults
func promptForAPIStyle(reader *bufio.Reader, name, apiBase string) (APIStyle, error) {
	// Auto-detect API style based on name or URL
	var suggestedStyle APIStyle = protocol.APIStyleOpenAI
	var suggestion string

	lowerName := strings.ToLower(name)
	lowerURL := strings.ToLower(apiBase)

	if strings.Contains(lowerName, "anthropic") || strings.Contains(lowerName, "claude") ||
		strings.Contains(lowerURL, "anthropic") || strings.Contains(lowerURL, "claude") {
		suggestedStyle = protocol.APIStyleAnthropic
		suggestion = "anthropic"
	} else if strings.Contains(lowerName, "openai") || strings.Contains(lowerName, "gpt") ||
		strings.Contains(lowerURL, "openai") {
		suggestedStyle = protocol.APIStyleOpenAI
		suggestion = "openai"
	}

	fmt.Printf("\nSelect API style (default: %s):\n", suggestion)
	fmt.Println("1. openai - For OpenAI-compatible APIs")
	fmt.Println("2. anthropic - For Anthropic Claude API")
	fmt.Print("Enter choice (1-2) or press Enter for default: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return suggestedStyle, nil
	}

	switch input {
	case "1", "openai":
		return protocol.APIStyleOpenAI, nil
	case "2", "anthropic":
		return protocol.APIStyleAnthropic, nil
	default:
		fmt.Printf("Invalid choice '%s', using default: %s\n", input, suggestion)
		return suggestedStyle, nil
	}
}
