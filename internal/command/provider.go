package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ProviderCommand represents the unified provider management command
// It provides both interactive mode (no args) and subcommands for specific operations
func ProviderCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider [command] [args]",
		Short: "Manage AI provider configurations",
		Long: `Manage AI provider configurations.

When run without arguments, enters interactive mode for provider management.
Available operations in interactive mode:
  - Add new providers
  - List all providers
  - Update existing providers
  - Delete providers
  - View provider details

Subcommands can be used for direct operations:
  add    Add a new provider
  list   List all providers
  delete Delete a provider by name
  update Update an existing provider
  get    Get provider details by name`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// No subcommand provided, run interactive mode
			return runProviderInteractiveMode(appManager)
		},
	}

	// Add subcommands
	cmd.AddCommand(providerAddCommand(appManager))
	cmd.AddCommand(providerListCommand(appManager))
	cmd.AddCommand(providerDeleteCommand(appManager))
	cmd.AddCommand(providerUpdateCommand(appManager))
	cmd.AddCommand(providerGetCommand(appManager))

	return cmd
}

// runProviderInteractiveMode runs the interactive provider management interface
func runProviderInteractiveMode(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		showProviderMenu()
		fmt.Print("Select an option (1-5, 0 to exit): ")

		input, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("\n👋 Exiting provider management...")
				return nil
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

		switch choice {
		case "1":
			if err := runProviderAddInteractive(appManager, reader); err != nil {
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
			fmt.Println("Exiting provider management...")
			return nil
		default:
			fmt.Println("Invalid choice. Please select 1-5 or 0 to exit.")
		}

		fmt.Println("\nPress Enter to continue...")
		_, _ = reader.ReadString('\n')
	}
}

// showProviderMenu displays the provider management menu
func showProviderMenu() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Provider Management")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("1. Add a new provider")
	fmt.Println("2. List all providers")
	fmt.Println("3. Update a provider")
	fmt.Println("4. Delete a provider")
	fmt.Println("5. View provider details")
	fmt.Println()
	fmt.Println("0. Exit")
	fmt.Println(strings.Repeat("=", 60))
}

// providerAddCommand creates the add subcommand
func providerAddCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [name] [baseurl] [token] [api_style]",
		Short: "Add a new provider",
		Long: `Add a new AI provider with name, API base URL, token, and optional API style.

Examples:
  provider add openai https://api.openai.com/v1 your-token-here openai
  provider add anthropic https://api.anthropic.com your-token-here anthropic

Or run without arguments for interactive mode.`,
		Args: cobra.MaximumNArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(appManager, args)
		},
	}
	return cmd
}

// providerListCommand creates the list subcommand
func providerListCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all providers",
		Long:  "Display all configured AI providers with their details.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProviderList(appManager)
		},
	}
	return cmd
}

// providerDeleteCommand creates the delete subcommand
func providerDeleteCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a provider",
		Long: `Delete an AI provider in interactive mode.

You will be prompted to select which provider to delete.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProviderDeleteInteractive(appManager, bufio.NewReader(os.Stdin))
		},
	}
	return cmd
}

// providerUpdateCommand creates the update subcommand
func providerUpdateCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a provider",
		Long: `Update an AI provider in interactive mode.

You will be prompted to select which provider to update and what to change.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProviderUpdateInteractive(appManager, bufio.NewReader(os.Stdin))
		},
	}
	return cmd
}

// providerGetCommand creates the get subcommand
func providerGetCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [name]",
		Short: "Get provider details",
		Long: `Display detailed information about a specific provider.

If no name is provided, enters interactive mode to select a provider.

Example: provider get openai`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runProviderGetInteractive(appManager, bufio.NewReader(os.Stdin))
			}
			return runProviderGet(appManager, args[0])
		},
	}
	return cmd
}

// runProviderList lists all providers
func runProviderList(appManager *AppManager) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("No providers configured. Use 'provider add' to add a provider.")
		return nil
	}

	fmt.Println("\nAll Configured Providers")
	fmt.Println(strings.Repeat("-", 80))

	for i, provider := range providers {
		status := "❌ Disabled"
		if provider.Enabled {
			status = "✅ Enabled"
		}
		fmt.Printf("%d. %s\n", i+1, provider.Name)
		fmt.Printf("   URL: %s\n", provider.APIBase)
		fmt.Printf("   Style: %s\n", provider.APIStyle)
		fmt.Printf("   Status: %s\n", status)
		fmt.Println(strings.Repeat("-", 80))
	}

	return nil
}

// runProviderAddInteractive runs interactive add mode
func runProviderAddInteractive(appManager *AppManager, reader *bufio.Reader) error {
	fmt.Println("\nAdd New Provider")

	return runAdd(appManager, []string{})
}

// runProviderUpdateInteractive runs interactive update mode
func runProviderUpdateInteractive(appManager *AppManager, reader *bufio.Reader) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("No providers configured. Use 'provider add' to add a provider first.")
		return nil
	}

	fmt.Println("\nUpdate Provider")
	fmt.Println("\nSelect a provider to update:")

	for i, provider := range providers {
		status := "[Enabled]"
		if !provider.Enabled {
			status = "[Disabled]"
		}
		fmt.Printf("%d. %s %s\n", i+1, status, provider.Name)
	}

	fmt.Print("\nEnter provider number: ")
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err != nil || num < 1 || num > len(providers) {
		return fmt.Errorf("invalid selection")
	}

	provider := providers[num-1]
	providerUUID := provider.UUID // Save UUID for update

	fmt.Printf("\nUpdating provider: %s\n", provider.Name)
	fmt.Printf("Current values:\n")
	fmt.Printf("  API Base: %s\n", provider.APIBase)
	fmt.Printf("  API Style: %s\n", provider.APIStyle)
	fmt.Printf("  Enabled: %v\n", provider.Enabled)

	// Prompt for new values
	fmt.Print("\nEnter new API base (press Enter to keep current): ")
	apiBase, _ := reader.ReadString('\n')
	apiBase = strings.TrimSpace(strings.TrimSuffix(apiBase, "\n"))
	if apiBase == "" {
		apiBase = provider.APIBase
	}

	fmt.Print("Enter new API token (press Enter to keep current): ")
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(strings.TrimSuffix(token, "\n"))
	if token == "" {
		// Keep existing token - we'll need to fetch it from the provider
		token = provider.Token
	}

	// API Style selection
	fmt.Printf("\nSelect API style (current: %s):\n", provider.APIStyle)
	fmt.Println("1. openai - For OpenAI-compatible APIs")
	fmt.Println("2. anthropic - For Anthropic Claude API")
	fmt.Print("Enter choice (1-2) or press Enter to keep current: ")

	styleInput, _ := reader.ReadString('\n')
	styleInput = strings.TrimSpace(strings.TrimSuffix(styleInput, "\n"))

	var apiStyle protocol.APIStyle = provider.APIStyle
	switch styleInput {
	case "1", "openai":
		apiStyle = protocol.APIStyleOpenAI
	case "2", "anthropic":
		apiStyle = protocol.APIStyleAnthropic
	case "":
		// Keep current
	default:
		return fmt.Errorf("invalid choice")
	}

	// Confirm
	fmt.Println("\n--- Update Summary ---")
	fmt.Printf("Provider: %s\n", provider.Name)
	fmt.Printf("API Base: %s\n", apiBase)
	fmt.Printf("API Style: %s\n", apiStyle)
	fmt.Println("---------------------")

	confirmed, err := promptForConfirmation(reader, "Apply these changes? (Y/n): ", true)
	if err != nil {
		return err
	}

	if !confirmed {
		fmt.Println("Update cancelled.")
		return nil
	}

	// Update the provider fields
	provider.APIBase = apiBase
	provider.Token = token
	provider.APIStyle = apiStyle

	// Save to database using UUID
	if err := appManager.UpdateProviderByUUID(providerUUID, provider); err != nil {
		return fmt.Errorf("failed to save updated provider: %w", err)
	}

	fmt.Printf("Provider '%s' updated successfully!\n", provider.Name)
	return nil
}

// runProviderDeleteInteractive runs interactive delete mode
func runProviderDeleteInteractive(appManager *AppManager, reader *bufio.Reader) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("No providers configured.")
		return nil
	}

	fmt.Println("\nDelete Provider")
	fmt.Println("\nSelect a provider to delete:")

	for i, provider := range providers {
		status := "[Enabled]"
		if !provider.Enabled {
			status = "[Disabled]"
		}
		fmt.Printf("%d. %s %s\n", i+1, status, provider.Name)
	}

	fmt.Print("\nEnter provider number: ")
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err != nil || num < 1 || num > len(providers) {
		return fmt.Errorf("invalid selection")
	}

	provider := providers[num-1]
	providerUUID := provider.UUID // Use UUID for deletion
	providerName := provider.Name

	return runProviderDeleteByUUID(appManager, providerUUID, providerName)
}

// runProviderDeleteByUUID deletes a provider by UUID (with confirmation)
func runProviderDeleteByUUID(appManager *AppManager, uuid, name string) error {
	// Confirm deletion
	fmt.Printf("Are you sure you want to delete provider '%s'? (y/N): ", name)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.TrimSuffix(input, "\n"))

	if strings.ToLower(input) != "y" && strings.ToLower(input) != "yes" {
		fmt.Println("Deletion cancelled.")
		return nil
	}

	if err := appManager.DeleteProviderByUUID(uuid); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	fmt.Printf("Provider '%s' deleted successfully!\n", name)
	return nil
}

// runProviderGetInteractive runs interactive get mode
func runProviderGetInteractive(appManager *AppManager, reader *bufio.Reader) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("❌ No providers configured.")
		return nil
	}

	fmt.Println("\nView Provider Details")
	fmt.Println("\nSelect a provider:")

	for i, provider := range providers {
		fmt.Printf("%d. %s\n", i+1, provider.Name)
	}

	fmt.Print("\nEnter provider number or name: ")
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

	var name string
	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err == nil && num > 0 && num <= len(providers) {
		name = providers[num-1].Name
	} else {
		name = choice
	}

	return runProviderGet(appManager, name)
}

// runProviderGet displays provider details
func runProviderGet(appManager *AppManager, name string) error {
	provider, err := appManager.GetProvider(name)
	if err != nil {
		return fmt.Errorf("provider not found: %s", name)
	}

	fmt.Println("\n🔍 Provider Details")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Name:      %s\n", provider.Name)
	fmt.Printf("UUID:      %s\n", provider.UUID)
	fmt.Printf("API Base:  %s\n", provider.APIBase)
	fmt.Printf("API Style: %s\n", provider.APIStyle)
	fmt.Printf("Enabled:   %v\n", provider.Enabled)
	fmt.Printf("Proxy URL: %s\n", provider.ProxyURL)
	fmt.Printf("Timeout:   %d seconds\n", provider.Timeout)

	if provider.Tags != nil && len(provider.Tags) > 0 {
		fmt.Printf("Tags:      %v\n", provider.Tags)
	}

	status := "❌ Disabled"
	if provider.Enabled {
		status = "✅ Enabled"
	}
	fmt.Printf("Status:    %s\n", status)
	fmt.Println(strings.Repeat("=", 60))

	return nil
}
