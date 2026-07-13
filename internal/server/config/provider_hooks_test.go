package config

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/server/hooks"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type providerUpdateHookFunc func(*typ.Provider)

func (f providerUpdateHookFunc) OnProviderUpdate(provider *typ.Provider) {
	f(provider)
}

func TestAddProviderNotifiesUpdateHooks(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()), WithDisableMigration(), WithDisableBuiltIn())
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	provider := &typ.Provider{
		UUID:    "provider-add-hook",
		Name:    "provider add hook",
		APIBase: "https://example.com/v1",
		Token:   "sk-test",
	}

	notified := make(chan *typ.Provider, 1)
	cfg.RegisterProviderUpdateHook(providerUpdateHookFunc(func(p *typ.Provider) {
		notified <- p
	}))

	if err := cfg.AddProvider(provider); err != nil {
		t.Fatalf("AddProvider error: %v", err)
	}

	select {
	case got := <-notified:
		if got.UUID != provider.UUID {
			t.Fatalf("notified provider UUID = %q, want %q", got.UUID, provider.UUID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider add update hook")
	}
}

func TestAddProviderInvalidatesRegisteredClientPool(t *testing.T) {
	transportPool := client.GetGlobalTransportPool()
	transportPool.Clear()
	t.Cleanup(transportPool.Clear)

	cfg, err := NewConfig(WithConfigDir(t.TempDir()), WithDisableMigration(), WithDisableBuiltIn())
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	providerUUID := "provider-add-client-pool-hook"
	transportPool.GetTransport(providerUUID, "", "", ai.IssuerMock, typ.SessionID{})
	if got := transportPool.Stats()["transport_count"]; got != 1 {
		t.Fatalf("transport_count before AddProvider = %v, want 1", got)
	}

	cfg.RegisterProviderUpdateHook(hooks.NewClientPoolInvalidationHook(client.NewClientPool()))

	if err := cfg.AddProvider(&typ.Provider{
		UUID:    providerUUID,
		Name:    "provider add client pool hook",
		APIBase: "https://example.com/v1",
		Token:   "sk-test",
	}); err != nil {
		t.Fatalf("AddProvider error: %v", err)
	}

	deadline := time.After(time.Second)
	for {
		if got := transportPool.Stats()["transport_count"]; got == 0 {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("transport_count after AddProvider = %v, want 0", transportPool.Stats()["transport_count"])
		case <-time.After(10 * time.Millisecond):
		}
	}
}
