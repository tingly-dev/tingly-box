package quota

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGatewayQuotaURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		apiBase string
		want    string
		ok      bool
	}{
		{"http://localhost:12581/tingly/claude_code", "http://localhost:12581/tingly/claude_code/quota", true},
		{"https://tb.example.com/tingly/team/v1", "https://tb.example.com/tingly/team/quota", true},
		{"https://tb.example.com/tingly/team/v1/", "https://tb.example.com/tingly/team/quota", true},
		{"https://example.com/llm/tingly/openai/v1", "https://example.com/llm/tingly/openai/quota", true},
		{"tb.internal:12581/tingly/codex", "http://tb.internal:12581/tingly/codex/quota", true},
		{"http://localhost:12581/tingly", "", false},
		{"http://localhost:12581/tingly/", "", false},
		{"https://api.anthropic.com/v1", "", false},
		{"https://example.com/tinglybox/team", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := GatewayQuotaURL(tt.apiBase)
		if got != tt.want || ok != tt.ok {
			t.Errorf("GatewayQuotaURL(%q) = %q, %v; want %q, %v", tt.apiBase, got, ok, tt.want, tt.ok)
		}
	}
}

func relayTestUpstreams() []*ProviderUsage {
	// A fixed clock: the leak check scans the JSON for digits, which a
	// nanosecond timestamp could contain by chance.
	t0 := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	resets := t0.Add(5 * time.Hour)
	balance := 42.5
	return []*ProviderUsage{
		{
			ProviderName: "Anthropic Max", FetchedAt: t0.Add(time.Minute),
			Account:     &UsageAccount{Email: "owner@example.com"},
			RawResponse: json.RawMessage(`{"secret":true}`),
			Windows: []*UsageWindow{
				{Key: "5h", Label: "5h", Type: WindowTypeSession, Kind: WindowKindLimit,
					Used: 970000, Limit: 1000000, UsedPercent: 97, Unit: UsageUnitTokens, WindowMinutes: 300, ResetsAt: &resets},
				{Key: "credit", Label: "Credit", Type: WindowTypeBalance, Kind: WindowKindResource,
					Available: &balance, Unit: UsageUnitCurrency, CurrencyCode: "USD"},
			},
		},
		{ProviderName: "Broken", LastError: "401 from https://vendor.example/secret"},
		{
			ProviderName: "Only a balance", FetchedAt: t0,
			Windows: []*UsageWindow{{Key: "credit", Type: WindowTypeBalance, Available: &balance, Unit: UsageUnitCurrency}},
		},
		{
			ProviderName: "Codex", FetchedAt: t0.Add(2 * time.Minute),
			Windows: []*UsageWindow{{Key: "primary", Label: "5h", Type: WindowTypeSession, Kind: WindowKindLimit,
				Used: 30, Limit: 100, UsedPercent: 30, Unit: UsageUnitPercent}},
		},
	}
}

func TestRelayUsagePercentOnly(t *testing.T) {
	t.Parallel()

	got := RelayUsage(relayTestUpstreams(), true)

	if len(got.Windows) != 2 {
		t.Fatalf("windows = %d, want 2 (balances dropped, broken provider skipped)", len(got.Windows))
	}
	first, second := got.Windows[0], got.Windows[1]
	if first.Label != "upstream 1 · 5h" || first.Unit != UsageUnitPercent || first.Used != 97 || first.Limit != 100 {
		t.Errorf("first window = %+v", first)
	}
	// The balance-only provider contributes nothing, so it takes no number.
	if second.Label != "upstream 2 · 5h" || second.Used != 30 {
		t.Errorf("second window = %+v", second)
	}
	raw, _ := json.Marshal(got)
	for _, leak := range []string{"Anthropic", "owner@example.com", "secret", "970000", "1000000", "42.5", "USD", "Codex", "vendor.example"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("relay leaks %q: %s", leak, raw)
		}
	}
}

func TestRelayUsageForOperator(t *testing.T) {
	t.Parallel()

	upstreams := relayTestUpstreams()
	got := RelayUsage(upstreams, false)

	// The operator sees every window as stored, still anonymised.
	if len(got.Windows) != 4 {
		t.Fatalf("windows = %d, want 4", len(got.Windows))
	}
	if w := got.Windows[0]; w.Limit != 1000000 || w.Label != "upstream 1 · 5h" {
		t.Errorf("first window = %+v", w)
	}
	if w := got.Windows[2]; w.Label != "upstream 2 · balance" || w.Available == nil {
		t.Errorf("balance-only provider = %+v", w)
	}
	if !got.FetchedAt.Equal(upstreams[2].FetchedAt) {
		t.Errorf("fetched_at = %v, want the oldest reading", got.FetchedAt)
	}
	if got.Account != nil || got.RawResponse != nil || got.ProviderName != "" {
		t.Errorf("relay carries identity: %+v", got)
	}
	// Relaying copies: the stored windows must not change.
	if upstreams[0].Windows[0].Label != "5h" {
		t.Error("relay rewrote the stored window")
	}
}
