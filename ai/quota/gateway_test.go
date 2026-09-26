package quota

import (
	"encoding/json"
	"slices"
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

func limitWindow(key string, used, limit float64, minutes int, resets time.Time) *UsageWindow {
	w := &UsageWindow{
		Key: key, Type: WindowTypeSession, Kind: WindowKindLimit, Label: key,
		Used: used, Limit: limit, Unit: UsageUnitTokens,
		WindowMinutes: minutes, ResetsAt: &resets,
	}
	applyWindowSemantics(w)
	return w
}

func TestProjectModelQuotaPicksMostHeadroom(t *testing.T) {
	t.Parallel()
	now := time.Now()
	hot := &ProviderUsage{FetchedAt: now, Windows: []*UsageWindow{limitWindow("5h", 95, 100, 300, now.Add(time.Hour))}}
	cool := &ProviderUsage{FetchedAt: now, Windows: []*UsageWindow{limitWindow("5h", 30, 100, 300, now.Add(2*time.Hour))}}
	unknown := &ProviderUsage{FetchedAt: now}

	got := ProjectModelQuota("sonnet", []ServiceQuota{
		{Model: "a", Usage: hot}, {Model: "b", Usage: unknown}, {Model: "c", Usage: cool},
	}, false)

	if got.Unreadable || len(got.Windows) != 1 {
		t.Fatalf("projection = %+v, want one window", got)
	}
	if got.Windows[0].Percent() != 30 {
		t.Errorf("used = %v, want 30 (the service with the most headroom)", got.Windows[0].Percent())
	}
	if got.RecoversAt == nil || !got.RecoversAt.Equal(*cool.Windows[0].ResetsAt) {
		t.Errorf("recovers_at = %v, want the chosen service's reset", got.RecoversAt)
	}
	// The projection is a copy: the stored usage must not change under it.
	got.Windows[0].Used = 0
	if cool.Windows[0].Used != 30 {
		t.Error("projection aliases the stored window")
	}
}

func TestProjectModelQuotaUnreadable(t *testing.T) {
	t.Parallel()
	got := ProjectModelQuota("sonnet", []ServiceQuota{{Model: "a", Usage: &ProviderUsage{LastError: "boom https://secret"}}}, true)
	if !got.Unreadable || len(got.Windows) != 0 {
		t.Fatalf("projection = %+v, want unreadable", got)
	}
	raw, _ := json.Marshal(got)
	if string(raw) != `{"model":"sonnet","unreadable":true}` {
		t.Errorf("unreadable projection leaks detail: %s", raw)
	}
}

func TestProjectModelQuotaRedactsForSharingKeys(t *testing.T) {
	t.Parallel()
	// A fixed clock: the leak check below scans the JSON for digits, which a
	// nanosecond timestamp could contain by chance.
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	balance := 12.5
	usage := &ProviderUsage{
		FetchedAt: now,
		Account:   &UsageAccount{Email: "owner@example.com"},
		Windows: []*UsageWindow{
			limitWindow("5h", 400, 1000, 300, now.Add(time.Hour)),
			{Key: "credit", Type: WindowTypeBalance, Kind: WindowKindResource, Available: &balance, Unit: UsageUnitCurrency, CurrencyCode: "USD"},
			{Key: "mcp", Type: WindowTypeCustom, Unlimited: true},
		},
		RawResponse: json.RawMessage(`{"secret":true}`),
	}

	got := ProjectModelQuota("sonnet", []ServiceQuota{{Model: "a", Usage: usage}}, true)
	if len(got.Windows) != 2 {
		t.Fatalf("windows = %d, want the countable and the unlimited one (balance dropped)", len(got.Windows))
	}
	w := got.Windows[0]
	if w.Unit != UsageUnitPercent || w.Limit != 100 || w.Used != 40 || w.Available == nil || *w.Available != 60 {
		t.Errorf("redacted window = %+v, want 40/100 percent", w)
	}
	if !got.Windows[1].Unlimited {
		t.Errorf("unlimited flag lost: %+v", got.Windows[1])
	}
	raw, _ := json.Marshal(got)
	for _, leak := range []string{"owner@example.com", "secret", "USD", "12.5", "1000"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("redacted projection leaks %q: %s", leak, raw)
		}
	}

	full := ProjectModelQuota("sonnet", []ServiceQuota{{Model: "a", Usage: usage}}, false)
	if len(full.Windows) != 3 || full.Windows[0].Limit != 1000 {
		t.Errorf("operator projection = %+v, want windows as stored", full.Windows)
	}
}

func TestGatewayUsageRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	gq := &GatewayQuota{Models: []ModelQuota{
		{Model: "sonnet", Windows: []*UsageWindow{
			limitWindow("5h", 20, 100, 300, now.Add(time.Hour)),
			limitWindow("7d", 90, 100, 7*24*60, now.Add(48*time.Hour)),
		}},
		{Model: "gpt", Windows: []*UsageWindow{limitWindow("5h", 10, 100, 300, now.Add(time.Hour))}},
		{Model: "private", Unreadable: true},
	}}

	usage := GatewayUsage(gq)
	usage.ProviderType = ProviderTypeTinglyBox
	if len(usage.Breakdowns) != 3 {
		t.Fatalf("breakdowns = %d, want one per model", len(usage.Breakdowns))
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("windows = %d, want the binding window of each readable model", len(usage.Windows))
	}
	var labels []string
	for _, w := range usage.Windows {
		labels = append(labels, w.Label)
	}
	if !containsAll(labels, "sonnet · 7d", "gpt · 5h") {
		t.Errorf("labels = %v", labels)
	}

	// Each model is judged by its own windows, not by the hottest model.
	if pct, ok := usage.ForModel("gpt").Pct(WindowKindLimit); !ok || pct != 10 {
		t.Errorf("ForModel(gpt).Pct = %v, %v; want 10", pct, ok)
	}
	if pct, ok := usage.ForModel("sonnet").Pct(WindowKindLimit); !ok || pct != 90 {
		t.Errorf("ForModel(sonnet).Pct = %v, %v; want 90", pct, ok)
	}
	if _, ok := usage.ForModel("private").Pct(WindowKindLimit); ok {
		t.Error("ForModel(private).Pct known, want unknown")
	}
	if _, ok := usage.ForModel("absent").Pct(WindowKindLimit); ok {
		t.Error("ForModel(absent).Pct known, want unknown")
	}

	// A gateway re-serving this usage projects the model's own breakdown.
	again := ProjectModelQuota("alias", []ServiceQuota{{Model: "gpt", Usage: usage}}, true)
	if len(again.Windows) != 1 || again.Windows[0].Percent() != 10 {
		t.Errorf("re-projection = %+v, want gpt's window", again.Windows)
	}
}

func TestForModelLeavesVendorsAlone(t *testing.T) {
	t.Parallel()
	now := time.Now()
	usage := &ProviderUsage{ProviderType: ProviderTypeAnthropic, Windows: []*UsageWindow{limitWindow("5h", 70, 100, 300, now)}}
	usage.AddBreakdown("opus", "opus", BreakdownGroupModel, limitWindow("opus", 5, 100, 300, now))
	got, ok := usage.ForModel("opus").Pct(WindowKindLimit)
	want, _ := usage.Pct(WindowKindLimit)
	if !ok || got != want {
		t.Errorf("ForModel().Pct = %v, want account-level %v", got, want)
	}
}

func containsAll(list []string, want ...string) bool {
	for _, w := range want {
		if !slices.Contains(list, w) {
			return false
		}
	}
	return true
}
