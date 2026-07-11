package quota

import (
	"context"
	"sync"
	"testing"

	typ "github.com/tingly-dev/tingly-box/ai"
)

type registryTestFetcher struct{ providerType ProviderType }

func (f registryTestFetcher) Name() string                 { return string(f.providerType) }
func (f registryTestFetcher) ProviderType() ProviderType   { return f.providerType }
func (f registryTestFetcher) Validate(*typ.Provider) error { return nil }
func (f registryTestFetcher) RequiresAuth() typ.AuthType   { return "" }
func (f registryTestFetcher) Fetch(context.Context, *typ.Provider) (*ProviderUsage, error) {
	return nil, nil
}

func TestRegistryConcurrentReads(t *testing.T) {
	registry := NewRegistry()
	fetcher := registryTestFetcher{providerType: ProviderTypeOpenAI}
	if err := registry.Register(fetcher); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				registry.Get(ProviderTypeOpenAI)
				registry.List()
				registry.ProviderTypes()
			}
		}()
	}
	wg.Wait()
}
