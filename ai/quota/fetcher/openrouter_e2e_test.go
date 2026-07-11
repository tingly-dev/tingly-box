package fetcher

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

// Usage: OPENROUTER_API_KEY=sk-... go test -run TestOpenRouterE2E -v
func TestOpenRouterE2E(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set, skipping e2e test")
	}

	fetcher := NewOpenRouterFetcher()
	provider := &ai.Provider{
		UUID:     "openrouter-e2e",
		Name:     "OpenRouter",
		Token:    apiKey,
		AuthType: ai.AuthTypeAPIKey,
		Enabled:  true,
	}

	if err := fetcher.Validate(provider); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	usage, err := fetcher.Fetch(ctx, provider)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	fmt.Printf("Provider: %s (%s)\n", usage.ProviderName, usage.ProviderType)
	for _, window := range usage.Windows {
		fmt.Printf("Window[%s]: %s — used=%.2f limit=%.2f (%.1f%%)\n",
			window.Key, window.Label, window.Used, window.Limit, window.UsedPercent)
	}
	if usage.Cost != nil {
		fmt.Printf("Cost: used=$%.2f limit=$%.2f currency=%s\n",
			usage.Cost.Used, usage.Cost.Limit, usage.Cost.CurrencyCode)
	}
}
