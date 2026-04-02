package fetcher

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Usage: OPENROUTER_API_KEY=sk-... go test -run TestOpenRouterE2E -v
func TestOpenRouterE2E(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set, skipping e2e test")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	fetcher := NewOpenRouterFetcher(logger)
	provider := &typ.Provider{
		UUID:     "openrouter-e2e",
		Name:     "OpenRouter",
		Token:    apiKey,
		AuthType: typ.AuthTypeAPIKey,
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
	if usage.Primary != nil {
		fmt.Printf("Primary: %s — used=%.2f limit=%.2f (%.1f%%)\n",
			usage.Primary.Label, usage.Primary.Used, usage.Primary.Limit, usage.Primary.UsedPercent)
	}
	if usage.Cost != nil {
		fmt.Printf("Cost: used=$%.2f limit=$%.2f currency=%s\n",
			usage.Cost.Used, usage.Cost.Limit, usage.Cost.CurrencyCode)
	}
}
