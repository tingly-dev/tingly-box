package command

import (
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
		fmt.Printf("   UUID: %s\n", provider.UUID)
		fmt.Printf("   URL: %s\n", provider.APIBase)
		fmt.Printf("   Style: %s\n", provider.APIStyle)
		fmt.Printf("   Status: %s\n", status)
		fmt.Println(strings.Repeat("-", 80))
	}

	return nil
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
