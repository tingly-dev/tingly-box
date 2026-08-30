package command

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// APIStyle is re-exported from internal/protocol for the CLI prompts.
type APIStyle = protocol.APIStyle

// runProviderList lists all providers
func runProviderList(appManager *AppManager) error {
	providers := usecase.NewProviderUseCase(appManager.GetGlobalConfig()).List().Providers

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

// runProviderGetInteractive runs interactive get mode. Selection happens by
// menu number so we can pass the chosen provider's UUID downstream (names
// aren't unique, so picking by name is ambiguous).
func runProviderGetInteractive(appManager *AppManager, reader *bufio.Reader) error {
	providers := usecase.NewProviderUseCase(appManager.GetGlobalConfig()).List().Providers

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
	result, err := usecase.NewProviderUseCase(appManager.GetGlobalConfig()).Get(usecase.GetProviderRequest{UUID: uuid})
	if err != nil {
		return err
	}
	provider := result.Provider

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
