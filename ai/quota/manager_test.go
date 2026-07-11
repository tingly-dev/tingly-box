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
func (*concurrencyTestFetcher) ProviderType() ProviderType   { return ProviderTypeOpenAI }
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

func TestRefreshBoundsConcurrency(t *testing.T) {
	providers := make([]*typ.Provider, 20)
	for i := range providers {
		providers[i] = &typ.Provider{
			UUID:    fmt.Sprintf("provider-%d", i),
			Name:    fmt.Sprintf("Provider %d", i),
			APIBase: "https://api.openai.com/v1",
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
		{"https://OPENAI.Azure.com/openai", ProviderTypeOpenAI},
		{"https://generativelanguage.GOOGLEAPIS.COM", ProviderTypeGemini},
		{"https://openrouter.ai/api/v1", ProviderTypeOpenRouter},
		{"https://api.minimaxi.com/v1", ProviderTypeMiniMaxCN},
		{"https://api.minimax.chat/v1", ProviderTypeMiniMax},
		{"https://chatgpt.com/backend-api", ProviderTypeCodex},
		{"https://example.com/v1", ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.apiBase, func(t *testing.T) {
			t.Parallel()
			got := inferProviderType(&typ.Provider{APIBase: tt.apiBase})
			if got != tt.want {
				t.Fatalf("inferProviderType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkInferProviderType(b *testing.B) {
	provider := &typ.Provider{APIBase: "https://gateway.example.com/proxy/OPENROUTER.AI/api/v1"}
	b.ReportAllocs()
	for b.Loop() {
		_ = inferProviderType(provider)
	}
}
