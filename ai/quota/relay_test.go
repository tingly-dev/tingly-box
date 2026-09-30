package quota

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGatewayQuotaURL(t *testing.T) {
	for base, want := range map[string]string{
		"http://localhost:12581/tingly/claude_code": "http://localhost:12581/tingly/claude_code/quota",
		"https://tb.example.com/tingly/team/v1/":    "https://tb.example.com/tingly/team/quota",
		"https://example.com/llm/tingly/openai/v1":  "https://example.com/llm/tingly/openai/quota",
		"tb.internal:12581/tingly/codex":            "http://tb.internal:12581/tingly/codex/quota",
		"http://localhost:12581/tingly":             "",
		"https://api.anthropic.com/v1":              "",
		"https://example.com/tinglybox/team":        "",
	} {
		got, ok := GatewayQuotaURL(base)
		if got != want || ok != (want != "") {
			t.Errorf("GatewayQuotaURL(%q) = %q, %v; want %q", base, got, ok, want)
		}
	}
}

func TestRelayUsage(t *testing.T) {
	t0 := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC) // fixed: the leak check scans for digits
	balance := 42.5
	got := RelayUsage([]*ProviderUsage{
		{ProviderName: "Anthropic Max", FetchedAt: t0.Add(time.Minute), Account: &UsageAccount{Email: "owner@example.com"},
			Windows: []*UsageWindow{{Key: "5h", Label: "5h", Type: WindowTypeSession, Kind: WindowKindLimit, Used: 970000, Limit: 1000000, Unit: UsageUnitTokens}}},
		{ProviderName: "Balance only", FetchedAt: t0, // nothing countable: skipped, takes no number
			Windows: []*UsageWindow{{Type: WindowTypeBalance, Available: &balance, Unit: UsageUnitCurrency, CurrencyCode: "USD"}}},
		{ProviderName: "Codex", FetchedAt: t0.Add(2 * time.Minute),
			Windows: []*UsageWindow{{Type: WindowTypeWeekly, Used: 30, Limit: 100, Unit: UsageUnitPercent}}},
	})

	if len(got.Windows) != 2 || got.Windows[0].Label != "Anthropic Max · 5h" || got.Windows[1].Label != "Codex · weekly" {
		t.Fatalf("windows = %+v", got.Windows)
	}
	if w := got.Windows[0]; w.Used != 97 || w.Limit != 100 || w.Unit != UsageUnitPercent {
		t.Errorf("first window = %+v, want 97/100 percent", w)
	}
	if !got.FetchedAt.Equal(t0.Add(time.Minute)) {
		t.Errorf("fetched_at = %v, want the oldest relayed reading", got.FetchedAt)
	}
	raw, _ := json.Marshal(got)
	for _, leak := range []string{"owner@", "970000", "42.5", "USD"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("relay leaks %q: %s", leak, raw)
		}
	}
}
