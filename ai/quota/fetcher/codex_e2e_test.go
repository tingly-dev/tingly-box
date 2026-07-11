package fetcher

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

// Usage: CODEX_ACCESS_TOKEN=eyJ... go test -run TestCodexE2E -v
func TestCodexE2E(t *testing.T) {
	accessToken := os.Getenv("CODEX_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CODEX_ACCESS_TOKEN not set, skipping e2e test")
	}

	fetcher := NewCodexFetcher()
	provider := &ai.Provider{
		UUID:     "codex-e2e",
		Name:     "Codex",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: accessToken,
			ExtraFields: map[string]interface{}{
				"account_id": os.Getenv("CODEX_ACCOUNT_ID"),
			},
		},
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
	if usage.Account != nil {
		fmt.Printf("Account: tier=%s\n", usage.Account.Tier)
	}
	for _, window := range usage.Windows {
		fmt.Printf("Window[%s]: %s — %.1f%% (resets at %v)\n",
			window.Key, window.Label, window.UsedPercent, window.ResetsAt)
	}
	if usage.Cost != nil {
		fmt.Printf("Credits: balance=$%.2f currency=%s\n",
			usage.Cost.Limit, usage.Cost.CurrencyCode)
	}
}
