package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// APIStyle is re-exported from internal/protocol for the CLI prompts.
type APIStyle = protocol.APIStyle

// runProviderList lists all providers
func runProviderList(appManager *AppManager) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("No providers configured. Use 'config provider add' to add a provider.")
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
		fmt.Printf("   UUID: %s\n", provider.UUID)
		fmt.Printf("   URL: %s\n", provider.APIBase)
		fmt.Printf("   Style: %s\n", provider.APIStyle)
		fmt.Printf("   Status: %s\n", status)
		fmt.Println(strings.Repeat("-", 80))
	}

	return nil
}

// runProviderUpdateInteractive runs interactive update mode
func runProviderUpdateInteractive(appManager *AppManager, reader *bufio.Reader) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("No providers configured. Use 'config provider add' to add a provider first.")
		return nil
	}

	fmt.Println("\nUpdate Provider")
	fmt.Println("\nSelect a provider to update:")

	for i, provider := range providers {
		status := "[Enabled]"
		if !provider.Enabled {
			status = "[Disabled]"
		}
		fmt.Printf("%d. %s %s (%s)\n", i+1, status, provider.Name, provider.UUID)
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
		fmt.Printf("%d. %s %s (%s)\n", i+1, status, provider.Name, provider.UUID)
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

// runProviderGetInteractive runs interactive get mode. Selection happens by
// menu number so we can pass the chosen provider's UUID downstream (names
// aren't unique, so picking by name is ambiguous).
func runProviderGetInteractive(appManager *AppManager, reader *bufio.Reader) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("❌ No providers configured.")
		return nil
	}

	fmt.Println("\nView Provider Details")
	fmt.Println("\nSelect a provider:")

	for i, provider := range providers {
		fmt.Printf("%d. %s (%s)\n", i+1, provider.Name, provider.UUID)
	}

	fmt.Print("\nEnter provider number or UUID: ")
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

	var uuid string
	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err == nil && num > 0 && num <= len(providers) {
		uuid = providers[num-1].UUID
	} else {
		uuid = choice
	}

	return runProviderGet(appManager, uuid)
}

// runProviderGet displays provider details for the given UUID. Providers are
// keyed by UUID; names are not unique and must not be used as lookup keys.
func runProviderGet(appManager *AppManager, uuid string) error {
	provider, err := appManager.GetProvider(uuid)
	if err != nil || provider == nil {
		return fmt.Errorf("provider not found: %s", uuid)
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
