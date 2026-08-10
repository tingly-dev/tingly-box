package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newTestProviderConfig returns a config with an embedded template manager
// wired up, so ResolveProviderModels can fall through to the template level
// without a real network call.
func newTestProviderConfig(t *testing.T) *serverconfig.Config {
	t.Helper()
	cfg, err := serverconfig.NewConfig(
		serverconfig.WithConfigDir(t.TempDir()),
		serverconfig.WithDisableBuiltIn(),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	tm := data.NewEmbeddedOnlyTemplateManager()
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatalf("template manager Initialize: %v", err)
	}
	cfg.SetTemplateManager(tm)
	return cfg
}

// codexTestProvider mirrors internal/server/config's own test fixture: its
// /models endpoint is unsupported, so ResolveProviderModels always falls
// through to the embedded template (source=template) without a network call.
func codexTestProvider() *typ.Provider {
	return &typ.Provider{
		Name:     "Codex OAuth",
		APIBase:  protocol.CodexAPIBase,
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeOAuth,
		Enabled:  true,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken: "test-codex-token",
			Issuer:      ai.IssuerCodex,
		},
	}
}

func TestErrProviderNotFound_Error(t *testing.T) {
	err := ErrProviderNotFound{UUID: "provider-1"}
	if got := err.Error(); !strings.Contains(got, "provider-1") {
		t.Errorf("Error() = %q, expected it to contain %q", got, "provider-1")
	}
}

func TestProviderUseCase_Add(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	tests := []struct {
		name    string
		req     CreateProviderRequest
		wantErr bool
	}{
		{
			name: "openai style, no proxy",
			req: CreateProviderRequest{
				Name:     "my-openai",
				APIBase:  "https://api.openai.com/v1",
				Token:    "sk-test",
				APIStyle: protocol.APIStyleOpenAI,
			},
		},
		{
			name: "anthropic style with proxy",
			req: CreateProviderRequest{
				Name:     "my-anthropic",
				APIBase:  "https://api.anthropic.com",
				Token:    "sk-test",
				APIStyle: protocol.APIStyleAnthropic,
				ProxyURL: "http://localhost:7890",
			},
		},
		{
			name: "duplicate name is allowed — only UUID is unique",
			req: CreateProviderRequest{
				Name:     "my-openai",
				APIBase:  "https://api.openai.com/v1",
				Token:    "sk-test-2",
				APIStyle: protocol.APIStyleOpenAI,
			},
		},
		{
			name: "empty name is rejected by the underlying config",
			req: CreateProviderRequest{
				Name:     "",
				APIBase:  "https://api.openai.com/v1",
				Token:    "sk-test",
				APIStyle: protocol.APIStyleOpenAI,
			},
			wantErr: true,
		},
		{
			name: "empty API base is rejected by the underlying config",
			req: CreateProviderRequest{
				Name:     "no-api-base",
				APIBase:  "",
				Token:    "sk-test",
				APIStyle: protocol.APIStyleOpenAI,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := uc.Add(tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Add: %v", err)
			}
			if res.Provider.UUID == "" {
				t.Fatal("expected a generated UUID")
			}
			if res.Provider.Name != tt.req.Name {
				t.Errorf("Name = %q, want %q", res.Provider.Name, tt.req.Name)
			}
			if res.Provider.ProxyURL != tt.req.ProxyURL {
				t.Errorf("ProxyURL = %q, want %q", res.Provider.ProxyURL, tt.req.ProxyURL)
			}
			if !res.Provider.Enabled {
				t.Error("expected new provider to be Enabled")
			}
		})
	}
}

func TestProviderUseCase_Get(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	created, err := uc.Add(CreateProviderRequest{
		Name: "my-openai", APIBase: "https://api.openai.com/v1",
		Token: "sk-test", APIStyle: protocol.APIStyleOpenAI,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		res, err := uc.Get(GetProviderRequest{UUID: created.Provider.UUID})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if res.Provider.UUID != created.Provider.UUID {
			t.Errorf("UUID = %q, want %q", res.Provider.UUID, created.Provider.UUID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := uc.Get(GetProviderRequest{UUID: "does-not-exist"})
		var target ErrProviderNotFound
		if !errors.As(err, &target) {
			t.Fatalf("expected ErrProviderNotFound, got %v", err)
		}
	})
}

func TestProviderUseCase_List(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	if got := uc.List(); len(got.Providers) != 0 {
		t.Fatalf("expected 0 providers initially, got %d", len(got.Providers))
	}

	if _, err := uc.Add(CreateProviderRequest{
		Name: "my-openai", APIBase: "https://api.openai.com/v1",
		Token: "sk-test", APIStyle: protocol.APIStyleOpenAI,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if got := uc.List(); len(got.Providers) != 1 {
		t.Fatalf("expected 1 provider after Add, got %d", len(got.Providers))
	}
}

func TestProviderUseCase_Update(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	created, err := uc.Add(CreateProviderRequest{
		Name: "my-openai", APIBase: "https://api.openai.com/v1",
		Token: "sk-original", APIStyle: protocol.APIStyleOpenAI,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	t.Run("replaces fields, keeps token when blank", func(t *testing.T) {
		res, err := uc.Update(UpdateProviderRequest{
			UUID:     created.Provider.UUID,
			Name:     "renamed",
			APIBase:  "https://api.openai.com/v2",
			APIStyle: protocol.APIStyleOpenAI,
			ProxyURL: "http://localhost:1080",
			// Token intentionally blank: "leave blank to keep" semantics.
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if res.Provider.Name != "renamed" {
			t.Errorf("Name = %q, want %q", res.Provider.Name, "renamed")
		}
		if res.Provider.Token != "sk-original" {
			t.Errorf("Token changed despite blank request Token: got %q", res.Provider.Token)
		}
		if res.Provider.ProxyURL != "http://localhost:1080" {
			t.Errorf("ProxyURL = %q, want %q", res.Provider.ProxyURL, "http://localhost:1080")
		}
	})

	t.Run("blank token overwritten when non-blank supplied", func(t *testing.T) {
		res, err := uc.Update(UpdateProviderRequest{
			UUID:     created.Provider.UUID,
			Name:     "renamed",
			APIBase:  "https://api.openai.com/v2",
			APIStyle: protocol.APIStyleOpenAI,
			Token:    "sk-new",
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if res.Provider.Token != "sk-new" {
			t.Errorf("Token = %q, want %q", res.Provider.Token, "sk-new")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := uc.Update(UpdateProviderRequest{UUID: "does-not-exist"})
		var target ErrProviderNotFound
		if !errors.As(err, &target) {
			t.Fatalf("expected ErrProviderNotFound, got %v", err)
		}
	})
}

func TestProviderUseCase_Delete(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	created, err := uc.Add(CreateProviderRequest{
		Name: "my-openai", APIBase: "https://api.openai.com/v1",
		Token: "sk-test", APIStyle: protocol.APIStyleOpenAI,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := uc.Delete(DeleteProviderRequest{UUID: created.Provider.UUID}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := uc.Get(GetProviderRequest{UUID: created.Provider.UUID}); err == nil {
		t.Fatal("expected provider to be gone after Delete")
	} else {
		var nf ErrProviderNotFound
		if !errors.As(err, &nf) {
			t.Fatalf("expected ErrProviderNotFound after Delete, got %T: %v", err, err)
		}
	}
}

// AvailableModels must use the full cache→vmodel→API→template chain via
// Config.ResolveProviderModels, not a hand-rolled shortcut — see
// .design/usecase-layer.md, "Known behavioral differences not yet resolved".
func TestProviderUseCase_AvailableModels_FallsThroughToTemplate(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	p := codexTestProvider()
	if err := cfg.AddProvider(p); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	res, err := uc.AvailableModels(AvailableModelsRequest{UUID: p.UUID})
	if err != nil {
		t.Fatalf("AvailableModels: %v", err)
	}
	if res.Source != string(serverconfig.ModelListSourceTemplate) {
		t.Errorf("Source = %q, want %q", res.Source, serverconfig.ModelListSourceTemplate)
	}
	if len(res.Models) == 0 {
		t.Error("expected a non-empty model list from the embedded template")
	}
}

func TestProviderUseCase_AvailableModels_UnknownProvider(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	if _, err := uc.AvailableModels(AvailableModelsRequest{UUID: "does-not-exist"}); err == nil {
		t.Fatal("expected an error for an unknown provider UUID")
	}
}

func TestProviderUseCase_RefreshModels_UnknownProvider(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	if _, err := uc.RefreshModels(RefreshModelsRequest{UUID: "does-not-exist"}); err == nil {
		t.Fatal("expected an error for an unknown provider UUID")
	}
}

func TestProviderUseCase_RefreshModels_BypassesCache(t *testing.T) {
	cfg := newTestProviderConfig(t)
	uc := NewProviderUseCase(cfg)

	p := codexTestProvider()
	if err := cfg.AddProvider(p); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	// Poison the DB cache with a sentinel. AvailableModels (forceRefresh=false)
	// must serve this cached value; RefreshModels (forceRefresh=true) must NOT —
	// only the latter proves the force-refresh path actually bypasses cache and
	// re-resolves through the template fallback.
	const poison = "POISONED-CACHE-ONLY"
	if err := cfg.GetModelManager().SaveModels(p, []string{poison}, db.ModelSourceAPI); err != nil {
		t.Fatalf("SaveModels: %v", err)
	}

	avail, err := uc.AvailableModels(AvailableModelsRequest{UUID: p.UUID})
	if err != nil {
		t.Fatalf("AvailableModels: %v", err)
	}
	if len(avail.Models) != 1 || avail.Models[0] != poison {
		t.Fatalf("AvailableModels (no force) = %v, want [%q] from cache", avail.Models, poison)
	}

	res, err := uc.RefreshModels(RefreshModelsRequest{UUID: p.UUID})
	if err != nil {
		t.Fatalf("RefreshModels: %v", err)
	}
	if len(res.Models) == 0 {
		t.Fatal("expected a non-empty model list after forcing a refresh")
	}
	for _, m := range res.Models {
		if m == poison {
			t.Fatalf("RefreshModels returned the stale cache entry %q; force-refresh did not bypass cache", poison)
		}
	}
}
