package quota

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	typ "github.com/tingly-dev/tingly-box/ai"
)

type managerTestProviderManager struct{ providers []*typ.Provider }

func (m managerTestProviderManager) ListProviders() []*typ.Provider { return m.providers }
func (m managerTestProviderManager) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	for _, provider := range m.providers {
		if provider.UUID == uuid {
			return provider, nil
		}
	}
	return nil, errors.New("not found")
}

type managerTestStore struct{}

func (managerTestStore) Save(context.Context, *ProviderUsage) error { return nil }
func (managerTestStore) Get(context.Context, string) (*ProviderUsage, error) {
	return nil, ErrUsageNotFound
}
func (managerTestStore) List(context.Context) ([]*ProviderUsage, error) { return nil, nil }
func (managerTestStore) Delete(context.Context, string) error           { return nil }
func (managerTestStore) CleanupExpired(context.Context) (int64, error)  { return 0, nil }
func (managerTestStore) Close() error                                   { return nil }

type concurrencyTestFetcher struct {
	current atomic.Int32
	maximum atomic.Int32
}

func (*concurrencyTestFetcher) Name() string                 { return "concurrency-test" }
func (*concurrencyTestFetcher) ProviderType() ProviderType   { return ProviderTypeAnthropic }
func (*concurrencyTestFetcher) Validate(*typ.Provider) error { return nil }
func (*concurrencyTestFetcher) RequiresAuth() typ.AuthType   { return "" }
func (f *concurrencyTestFetcher) Fetch(_ context.Context, provider *typ.Provider) (*ProviderUsage, error) {
	current := f.current.Add(1)
	defer f.current.Add(-1)
	for {
		maximum := f.maximum.Load()
		if current <= maximum || f.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	time.Sleep(time.Millisecond)
	return &ProviderUsage{ProviderUUID: provider.UUID}, nil
}

// TestGetQuota_NotFoundIsUnwrapped locks in that GetQuota
// returns ErrUsageNotFound identically to what the store
// returned — not a re-wrapped error. Callers such as the provider-quota
// batch handler compare with == against this sentinel to skip providers
// with no quota data instead of failing the whole request; a wrapped error
// silently breaks that comparison (this was a real bug: BatchGetQuota 500'd
// for any provider with no quota data, e.g. a vmodel/local provider, instead
// of just omitting it from the result).
func TestGetQuota_NotFoundIsUnwrapped(t *testing.T) {
	manager := NewManager(DefaultConfig(), managerTestStore{}, managerTestProviderManager{}, logrus.New())

	if _, err := manager.GetQuota(context.Background(), "missing"); err != ErrUsageNotFound {
		t.Fatalf("GetQuota() error = %v, want ErrUsageNotFound (identity, via ==)", err)
	}
}

func TestRefreshBoundsConcurrency(t *testing.T) {
	providers := make([]*typ.Provider, 20)
	for i := range providers {
		providers[i] = &typ.Provider{
			UUID:    fmt.Sprintf("provider-%d", i),
			Name:    fmt.Sprintf("Provider %d", i),
			APIBase: "https://api.anthropic.com/v1",
			Enabled: true,
		}
	}

	fetcher := &concurrencyTestFetcher{}
	manager := NewManager(DefaultConfig(), managerTestStore{}, managerTestProviderManager{providers}, logrus.New())
	if err := manager.RegisterFetcher(fetcher); err != nil {
		t.Fatal(err)
	}

	results, err := manager.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(providers) {
		t.Fatalf("Refresh() returned %d results, want %d", len(results), len(providers))
	}
	if got := fetcher.maximum.Load(); got != maxConcurrentRefreshes {
		t.Fatalf("maximum concurrent fetches = %d, want %d", got, maxConcurrentRefreshes)
	}
}

func TestInferProviderTypeAPIBaseCaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		apiBase string
		want    ProviderType
	}{
		{"HTTPS://API.ANTHROPIC.COM/V1", ProviderTypeAnthropic},
		{"HTTPS://API.DEEPSEEK.COM/V1", ProviderTypeDeepSeek},
		// OpenAI is intentionally unclassified (see the OpenAI-disabling
		// commit): its legacy usage API's current requirements are unverified,
		// so the manager skips it rather than reading it wrong.
		{"https://OPENAI.Azure.com/openai", ""},
		{"https://generativelanguage.GOOGLEAPIS.COM", ProviderTypeGemini},
		{"https://openrouter.ai/api/v1", ProviderTypeOpenRouter},
		{"https://api.minimaxi.com/v1", ProviderTypeMiniMaxCN},
		{"https://api.minimax.chat/v1", ProviderTypeMiniMax},
		{"https://chatgpt.com/backend-api", ProviderTypeCodex},
		{"https://api.kimi.com/coding/v1", ProviderTypeKimiCode},
		{"https://example.com/v1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.apiBase, func(t *testing.T) {
			t.Parallel()
			got := inferProviderType(&typ.Provider{APIBase: tt.apiBase})
			if got != tt.want {
				t.Fatalf("inferProviderType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInferProviderTypeKimiCodeOAuth(t *testing.T) {
	t.Parallel()

	provider := &typ.Provider{
		APIBase:  "https://gateway.example.com/v1",
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			Issuer: typ.IssuerKimiCode,
		},
	}
	if got := inferProviderType(provider); got != ProviderTypeKimiCode {
		t.Fatalf("inferProviderType() = %q, want %q", got, ProviderTypeKimiCode)
	}
}

func BenchmarkInferProviderType(b *testing.B) {
	provider := &typ.Provider{APIBase: "https://gateway.example.com/proxy/OPENROUTER.AI/api/v1"}
	b.ReportAllocs()
	for b.Loop() {
		_ = inferProviderType(provider)
	}
}

func TestUnreadableProvidersReportWhyAndNothingElse(t *testing.T) {
	// No placeholder window; see Unreadable.
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	usage := Unreadable("u", "n", ProviderTypeCursor, "quota API not available", now, time.Hour)

	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown", pct, ok)
	}
	if len(usage.Windows) != 0 {
		t.Errorf("Windows = %d; want none", len(usage.Windows))
	}
	if usage.LastError == "" || usage.LastErrorAt == nil {
		t.Error("the reason should be recorded")
	}
	if !usage.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Errorf("ExpiresAt = %v; want the ttl applied to now", usage.ExpiresAt)
	}
}

// recordingStore reports what a fetch attempt left behind.
type recordingStore struct {
	managerTestStore
	saved   []*ProviderUsage
	deleted []string
}

func (s *recordingStore) Save(_ context.Context, usage *ProviderUsage) error {
	s.saved = append(s.saved, usage)
	return nil
}

func (s *recordingStore) Delete(_ context.Context, uuid string) error {
	s.deleted = append(s.deleted, uuid)
	return nil
}

// Most providers have no quota fetcher, which is ordinary rather than broken.
// Recording it as a usage record put "unsupported provider type" in LastError
// and the UI showed that as a provider failure.
func TestRefreshProviderUnsupportedIsASkipNotAnError(t *testing.T) {
	provider := &typ.Provider{UUID: "u1", Name: "Some Gateway", AuthType: typ.AuthTypeAPIKey}
	store := &recordingStore{}
	m := NewManager(nil, store, managerTestProviderManager{providers: []*typ.Provider{provider}}, logrus.New())

	usage, err := m.RefreshProvider(context.Background(), "u1")
	if !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("RefreshProvider() error = %v, want ErrProviderUnsupported", err)
	}
	if usage != nil {
		t.Errorf("usage = %+v, want nil", usage)
	}
	if len(store.saved) != 0 {
		t.Errorf("saved %d records, want none", len(store.saved))
	}
	// Any record an earlier version wrote is cleared out on the way past.
	if len(store.deleted) != 1 || store.deleted[0] != "u1" {
		t.Errorf("deleted = %v, want [u1]", store.deleted)
	}
}

// Refresh walks every enabled provider, most of which have no fetcher; those
// must drop out quietly rather than land in the results as failures.
func TestRefreshSkipsUnsupportedProviders(t *testing.T) {
	store := &recordingStore{}
	m := NewManager(nil, store, managerTestProviderManager{providers: []*typ.Provider{
		{UUID: "u1", Name: "Some Gateway", AuthType: typ.AuthTypeAPIKey},
	}}, logrus.New())

	usages, err := m.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() error: %v", err)
	}
	if len(usages) != 0 {
		t.Errorf("Refresh() returned %d usages, want none", len(usages))
	}
	if len(store.saved) != 0 {
		t.Errorf("saved %d records, want none", len(store.saved))
	}
}

// A path segment is a user-chosen name and must never speak for a vendor.
// Matching the whole URL read a local gateway route named "codex1" as Codex,
// which sent that provider's token to chatgpt.com on the next refresh.
func TestInferProviderTypeIgnoresPathAndLocalHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		apiBase string
		want    ProviderType
	}{
		{"local gateway route named codex", "http://localhost:12581/tingly/codex1", ""},
		{"loopback ip", "http://127.0.0.1:12581/tingly/gemini", ""},
		{"private lan", "http://192.168.1.10:8080/v1/cursor", ""},
		{"container hostname", "http://tingly-box:12581/tingly/codex1", ""},
		{"mdns", "http://mac.local:12581/copilot/v1", ""},
		{"vendor name only in the path", "https://gateway.example.com/proxy/openrouter.ai/api/v1", ""},
		{"lookalike domain", "https://api.openai.com.evil.test/v1", ""},
		{"scheme-less base", "api.anthropic.com/v1", ProviderTypeAnthropic},
		{"scheme-less deepseek base", "api.deepseek.com/v1", ProviderTypeDeepSeek},
		{"deepseek subdomain", "https://eu.api.deepseek.com/v1", ProviderTypeDeepSeek},
		{"deepseek lookalike domain", "https://api.deepseek.com.evil.test/v1", ""},
		{"subdomain of a vendor", "https://eu.api.anthropic.com/v1", ProviderTypeAnthropic},
		{"kimi coding needs its path", "https://api.kimi.com/coding/v1", ProviderTypeKimiCode},
		{"kimi without the coding path", "https://api.kimi.com/v1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := inferProviderType(&typ.Provider{APIBase: tt.apiBase}); got != tt.want {
				t.Fatalf("inferProviderType(%q) = %q, want %q", tt.apiBase, got, tt.want)
			}
		})
	}
}
