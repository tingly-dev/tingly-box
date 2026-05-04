package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ============== Kong Command Structures ==============

// ProviderCmdKong is the Kong version of provider command with subcommands.
// The default behavior (no subcommand) is to list providers.
type ProviderCmdKong struct {
	List    ProviderListCmdKong    `kong:"cmd,name='list',default='1',hidden,help='List all providers (default)'"`
	Add     ProviderAddCmdKong     `kong:"cmd,help='Add a new provider'"`
	Delete  ProviderDeleteCmdKong  `kong:"cmd,help='Delete a provider (interactive)'"`
	Update  ProviderUpdateCmdKong  `kong:"cmd,help='Update a provider (interactive)'"`
	Details ProviderDetailsCmdKong `kong:"cmd,help='View provider details'"`
}

// ProviderListCmdKong lists all providers
type ProviderListCmdKong struct{}

func (p *ProviderListCmdKong) Run(appManager *AppManager) error {
	return runProviderList(appManager)
}

// ProviderAddCmdKong adds a new provider with optional positional args
type ProviderAddCmdKong struct {
	Name     string `kong:"arg,optional,help='Provider name'"`
	BaseURL  string `kong:"arg,optional,help='API base URL'"`
	Token    string `kong:"arg,optional,help='API token'"`
	APIStyle string `kong:"arg,optional,help='API style (openai, anthropic)'"`
}

func (p *ProviderAddCmdKong) Run(appManager *AppManager) error {
	args := []string{}
	if p.Name != "" {
		args = append(args, p.Name)
	}
	if p.BaseURL != "" {
		args = append(args, p.BaseURL)
	}
	if p.Token != "" {
		args = append(args, p.Token)
	}
	if p.APIStyle != "" {
		args = append(args, p.APIStyle)
	}
	return runAdd(appManager, args)
}

// ProviderDeleteCmdKong deletes a provider in interactive mode
type ProviderDeleteCmdKong struct{}

func (p *ProviderDeleteCmdKong) Run(appManager *AppManager) error {
	return runProviderDeleteInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ProviderUpdateCmdKong updates a provider in interactive mode
type ProviderUpdateCmdKong struct{}

func (p *ProviderUpdateCmdKong) Run(appManager *AppManager) error {
	return runProviderUpdateInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ProviderDetailsCmdKong displays provider details (without name drops to interactive)
type ProviderDetailsCmdKong struct {
	Name string `kong:"arg,optional,help='Provider name'"`
}

func (p *ProviderDetailsCmdKong) Run(appManager *AppManager) error {
	if p.Name == "" {
		return runProviderGetInteractive(appManager, bufio.NewReader(os.Stdin))
	}
	return runProviderGet(appManager, p.Name)
}

// ============== Business Logic Functions ==============

type APIStyle = protocol.APIStyle

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

// runProviderList lists all providers
func runProviderList(appManager *AppManager) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("No providers configured. Use 'config add' to add a provider.")
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
		fmt.Println("No providers configured. Use 'config add' to add a provider first.")
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

	confirmed, err := promptForConfirmation(reader, "Apply these changes? (Y/n): ")
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
