package quota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
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
	current *ProviderUsage
	getErr  error
	saved   []*ProviderUsage
	deleted []string
}

func (s *recordingStore) Save(_ context.Context, usage *ProviderUsage) error {
	s.saved = append(s.saved, usage)
	s.current = usage
	return nil
}

func (s *recordingStore) Get(_ context.Context, _ string) (*ProviderUsage, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.current == nil {
		return nil, ErrUsageNotFound
	}
	return s.current, nil
}

func (s *recordingStore) Delete(_ context.Context, uuid string) error {
	s.deleted = append(s.deleted, uuid)
	return nil
}

type sequenceFetcher struct {
	errs  []error
	usage *ProviderUsage
	calls int
}

func (*sequenceFetcher) Name() string                 { return "sequence-test" }
func (*sequenceFetcher) ProviderType() ProviderType   { return ProviderTypeCodex }
func (*sequenceFetcher) Validate(*typ.Provider) error { return nil }
func (*sequenceFetcher) RequiresAuth() typ.AuthType   { return typ.AuthTypeOAuth }
func (f *sequenceFetcher) Fetch(context.Context, *typ.Provider) (*ProviderUsage, error) {
	f.calls++
	if f.calls <= len(f.errs) {
		return nil, f.errs[f.calls-1]
	}
	return f.usage, nil
}

type cancelingFetcher struct {
	cancel context.CancelFunc
}

func (*cancelingFetcher) Name() string                 { return "canceling-test" }
func (*cancelingFetcher) ProviderType() ProviderType   { return ProviderTypeCodex }
func (*cancelingFetcher) Validate(*typ.Provider) error { return nil }
func (*cancelingFetcher) RequiresAuth() typ.AuthType   { return typ.AuthTypeOAuth }
func (f *cancelingFetcher) Fetch(context.Context, *typ.Provider) (*ProviderUsage, error) {
	f.cancel()
	return nil, context.Canceled
}

type serializedFetcher struct {
	current atomic.Int32
	maximum atomic.Int32
	calls   atomic.Int32
}

func (*serializedFetcher) Name() string                 { return "serialized-test" }
func (*serializedFetcher) ProviderType() ProviderType   { return ProviderTypeCodex }
func (*serializedFetcher) Validate(*typ.Provider) error { return nil }
func (*serializedFetcher) RequiresAuth() typ.AuthType   { return typ.AuthTypeOAuth }
func (f *serializedFetcher) Fetch(_ context.Context, provider *typ.Provider) (*ProviderUsage, error) {
	f.calls.Add(1)
	current := f.current.Add(1)
	defer f.current.Add(-1)
	for {
		maximum := f.maximum.Load()
		if current <= maximum || f.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	return &ProviderUsage{
		ProviderUUID: provider.UUID,
		FetchedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(time.Minute),
	}, nil
}

func newRetryTestManager(t *testing.T, config *Config, store Store, fetcher Fetcher) (*Manager, *typ.Provider) {
	t.Helper()
	provider := &typ.Provider{
		UUID:    "codex-1",
		Name:    "Codex",
		APIBase: "https://chatgpt.com/backend-api",
		Enabled: true,
	}
	manager := NewManager(config, store, managerTestProviderManager{providers: []*typ.Provider{provider}}, logrus.New())
	manager.retryDelay = func(int) time.Duration { return 0 }
	if err := manager.RegisterFetcher(fetcher); err != nil {
		t.Fatal(err)
	}
	return manager, provider
}

func TestFetchProviderQuotaRetriesTransientErrors(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 2
	want := &ProviderUsage{
		ProviderUUID: "codex-1",
		ProviderName: "Codex",
		ProviderType: ProviderTypeCodex,
		FetchedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(time.Minute),
	}
	fetcher := &sequenceFetcher{errs: []error{io.EOF, io.ErrUnexpectedEOF}, usage: want}
	store := &recordingStore{}
	manager, provider := newRetryTestManager(t, config, store, fetcher)

	got, err := manager.fetchProviderQuota(context.Background(), provider)
	if err != nil {
		t.Fatalf("fetchProviderQuota() error: %v", err)
	}
	if got != want {
		t.Fatalf("fetchProviderQuota() = %p, want %p", got, want)
	}
	if fetcher.calls != 3 {
		t.Fatalf("Fetch() calls = %d, want 3", fetcher.calls)
	}
	if len(store.saved) != 1 || store.saved[0] != want {
		t.Fatalf("saved = %#v, want the successful usage once", store.saved)
	}
}

func TestFetchProviderQuotaDoesNotRetryPermanentErrors(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 2
	fetcher := &sequenceFetcher{errs: []error{errors.New("status 401")}}
	store := &recordingStore{}
	manager, provider := newRetryTestManager(t, config, store, fetcher)

	usage, err := manager.fetchProviderQuota(context.Background(), provider)
	if err != nil {
		t.Fatalf("fetchProviderQuota() error: %v", err)
	}
	if fetcher.calls != 1 {
		t.Fatalf("Fetch() calls = %d, want 1", fetcher.calls)
	}
	if usage.LastError != "status 401" {
		t.Fatalf("LastError = %q, want status 401", usage.LastError)
	}
}

func TestFetchProviderQuotaDoesNotRetryCanceledContext(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 2
	fetcher := &sequenceFetcher{errs: []error{context.Canceled}}
	store := &recordingStore{}
	manager, provider := newRetryTestManager(t, config, store, fetcher)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := manager.fetchProviderQuota(ctx, provider)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fetchProviderQuota() error = %v, want context.Canceled", err)
	}
	if fetcher.calls != 0 {
		t.Fatalf("Fetch() calls = %d, want 0 for an already-canceled request", fetcher.calls)
	}
}

func TestFetchProviderQuotaStopsWithoutSavingWhenContextIsCanceled(t *testing.T) {
	config := DefaultConfig()
	fetchedAt := time.Now().Add(-time.Hour)
	cached := &ProviderUsage{
		ProviderUUID: "codex-1",
		ProviderName: "Codex",
		ProviderType: ProviderTypeCodex,
		FetchedAt:    fetchedAt,
		ExpiresAt:    time.Now().Add(-time.Minute),
		Windows:      []*UsageWindow{{Key: "current", Limit: 100, Used: 20}},
	}
	store := &recordingStore{current: cached}
	ctx, cancel := context.WithCancel(context.Background())
	fetcher := &cancelingFetcher{cancel: cancel}
	manager, provider := newRetryTestManager(t, config, store, fetcher)

	usage, err := manager.fetchProviderQuota(ctx, provider)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fetchProviderQuota() error = %v, want context.Canceled", err)
	}
	if usage != cached {
		t.Fatal("context cancellation replaced the cached quota")
	}
	if !usage.FetchedAt.Equal(fetchedAt) || len(usage.Windows) != 1 {
		t.Fatal("cached quota payload was not preserved")
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved %d records with a canceled context, want none", len(store.saved))
	}
}

func TestFetchProviderQuotaRetriesTimeout(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 1
	fetcher := &sequenceFetcher{
		errs:  []error{context.DeadlineExceeded},
		usage: &ProviderUsage{ProviderUUID: "codex-1", FetchedAt: time.Now()},
	}
	store := &recordingStore{}
	manager, provider := newRetryTestManager(t, config, store, fetcher)

	if _, err := manager.fetchProviderQuota(context.Background(), provider); err != nil {
		t.Fatalf("fetchProviderQuota() error: %v", err)
	}
	if fetcher.calls != 2 {
		t.Fatalf("Fetch() calls = %d, want 2", fetcher.calls)
	}
}

func TestFetchProviderQuotaCoalescesSameProvider(t *testing.T) {
	fetcher := &serializedFetcher{}
	store := &recordingStore{}
	manager, provider := newRetryTestManager(t, DefaultConfig(), store, fetcher)

	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := manager.fetchProviderQuota(context.Background(), provider)
			errs <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("fetchProviderQuota() error: %v", err)
		}
	}
	if got := fetcher.maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrent fetches for one provider = %d, want 1", got)
	}
	if got := fetcher.calls.Load(); got != 1 {
		t.Fatalf("Fetch() calls = %d, want 1 coalesced request", got)
	}
}

func TestFetchProviderQuotaStopsAtRetryLimit(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 2
	fetcher := &sequenceFetcher{errs: []error{io.EOF, io.EOF, io.EOF, io.EOF}}
	store := &recordingStore{}
	manager, provider := newRetryTestManager(t, config, store, fetcher)

	usage, err := manager.fetchProviderQuota(context.Background(), provider)
	if err != nil {
		t.Fatalf("fetchProviderQuota() error: %v", err)
	}
	if fetcher.calls != 3 {
		t.Fatalf("Fetch() calls = %d, want 3 (initial request plus 2 retries)", fetcher.calls)
	}
	if usage.LastError != io.EOF.Error() {
		t.Fatalf("LastError = %q, want EOF", usage.LastError)
	}
}

func TestFetchProviderQuotaPreservesLastSuccessfulSnapshot(t *testing.T) {
	config := DefaultConfig()
	config.RetryOnFailure = false
	fetchedAt := time.Now().Add(-time.Hour)
	cached := &ProviderUsage{
		ProviderUUID: "codex-1",
		ProviderName: "Codex",
		ProviderType: ProviderTypeCodex,
		FetchedAt:    fetchedAt,
		ExpiresAt:    time.Now().Add(-time.Minute),
		Windows: []*UsageWindow{{
			Key:         "current",
			Type:        WindowTypeSession,
			Used:        25,
			Limit:       100,
			UsedPercent: 25,
		}},
		RawResponse: []byte(`{"plan_type":"plus"}`),
	}
	store := &recordingStore{current: cached}
	fetcher := &sequenceFetcher{errs: []error{io.EOF}}
	manager, provider := newRetryTestManager(t, config, store, fetcher)
	before := time.Now()

	usage, err := manager.fetchProviderQuota(context.Background(), provider)
	if err != nil {
		t.Fatalf("fetchProviderQuota() error: %v", err)
	}
	if usage != cached {
		t.Fatal("fetch failure replaced the last successful snapshot")
	}
	if !usage.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("FetchedAt = %v, want last successful time %v", usage.FetchedAt, fetchedAt)
	}
	if len(usage.Windows) != 1 || string(usage.RawResponse) != `{"plan_type":"plus"}` {
		t.Fatal("cached quota data was not preserved")
	}
	if usage.LastError != io.EOF.Error() || usage.LastErrorAt == nil {
		t.Fatalf("refresh error not recorded: %#v", usage)
	}
	wantRetryAt := before.Add(min(config.CacheTTL, failedRefreshTTL))
	if usage.ExpiresAt.Before(wantRetryAt.Add(-time.Second)) || usage.ExpiresAt.After(wantRetryAt.Add(time.Second)) {
		t.Fatalf("ExpiresAt = %v, want a bounded retry delay", usage.ExpiresAt)
	}
	if len(store.saved) != 1 || store.saved[0] != cached {
		t.Fatalf("saved = %#v, want updated cached snapshot", store.saved)
	}
}

func TestFetchProviderQuotaDoesNotOverwriteWhenCacheReadFails(t *testing.T) {
	config := DefaultConfig()
	config.RetryOnFailure = false
	store := &recordingStore{getErr: errors.New("database is busy")}
	fetcher := &sequenceFetcher{errs: []error{io.EOF}}
	manager, provider := newRetryTestManager(t, config, store, fetcher)

	usage, err := manager.fetchProviderQuota(context.Background(), provider)
	if err == nil || !strings.Contains(err.Error(), "load cached quota after fetch failure") {
		t.Fatalf("fetchProviderQuota() error = %v, want cache read error", err)
	}
	if usage != nil {
		t.Fatalf("usage = %#v, want nil", usage)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved %d records after uncertain cache read, want none", len(store.saved))
	}
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
