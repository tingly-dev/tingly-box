package tui

import (
	"context"
	"os"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// tuiHarnessManager satisfies TUIManager by delegating to an AppConfig. It
// mirrors the production *command.AppManager surface but lives here to keep
// the test inside the tui package (avoids a command → tui → command cycle).
type tuiHarnessManager struct {
	ac *config.AppConfig
}

func (m *tuiHarnessManager) GetGlobalConfig() *serverconfig.Config { return m.ac.GetGlobalConfig() }
func (m *tuiHarnessManager) StartServerAt(int) error               { return nil }

// newTUIHarness builds a TUIManager backed by a real AppConfig so the
// model-lookup cascade can be exercised end-to-end without a server. The
// embedded template manager is attached so the template fallback path is
// reachable.
func newTUIHarness(t *testing.T) TUIManager {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "tingly-tui-models-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ac, err := config.NewAppConfig(config.WithConfigDir(tempDir))
	if err != nil {
		t.Fatalf("NewAppConfig: %v", err)
	}

	tm := data.NewEmbeddedOnlyTemplateManager()
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatalf("template init: %v", err)
	}
	ac.GetGlobalConfig().SetTemplateManager(tm)

	return &tuiHarnessManager{ac: ac}
}

func addTestProvider(t *testing.T, mgr TUIManager, name, apiBase string, apiStyle protocol.APIStyle) *typ.Provider {
	t.Helper()
	result, err := usecase.NewProviderUseCase(mgr.GetGlobalConfig()).Add(usecase.CreateProviderRequest{
		Name: name, APIBase: apiBase, Token: "tok", APIStyle: apiStyle,
	})
	if err != nil {
		t.Fatalf("ProviderUseCase.Add: %v", err)
	}
	return result.Provider
}

// TestAvailableModels_PrefersDBOverTemplate: when the DB has fresh models
// from a /v1/models call, they win over the embedded template list. We
// must not silently return template defaults when fresh API data exists.
func TestAvailableModels_PrefersDBOverTemplate(t *testing.T) {
	mgr := newTUIHarness(t)
	p := addTestProvider(t, mgr, "openai", "https://api.openai.com", protocol.APIStyleOpenAI)

	if err := mgr.GetGlobalConfig().GetModelManager().SaveModels(p, []string{"dbm-1", "dbm-2"}, db.ModelSourceAPI); err != nil {
		t.Fatalf("SaveModels: %v", err)
	}

	got := availableModels(mgr.GetGlobalConfig(), p)
	if len(got) != 2 || got[0] != "dbm-1" || got[1] != "dbm-2" {
		t.Errorf("expected DB models, got %v", got)
	}
}

// TestAvailableModels_FallsBackToTemplate: when no models are in the DB, the
// embedded template list is returned. This is the regression guard for the
// half-done fallback in FetchAndSaveProviderModels (the bug that bit us:
// success but no DB write → caller had to read template itself).
func TestAvailableModels_FallsBackToTemplate(t *testing.T) {
	mgr := newTUIHarness(t)
	p := addTestProvider(t, mgr, "anthropic", "https://api.anthropic.com", protocol.APIStyleAnthropic)

	got := availableModels(mgr.GetGlobalConfig(), p)
	if len(got) == 0 {
		t.Fatalf("expected template fallback to provide Anthropic models, got empty list")
	}
}

// TestAvailableModels_EmptyWhenNoSource: an unknown provider — no DB cache,
// no embedded template match, and an unreachable /v1/models endpoint —
// resolves to an empty list. pickProviderModel uses this to decide between a
// Select and a free-form Input. Note AvailableModels walks the full chain, so
// this case now exercises a real (failing) /v1/models fetch against the
// bogus host before falling through to empty; the assertion still holds
// because every tier comes up empty.
func TestAvailableModels_EmptyWhenNoSource(t *testing.T) {
	mgr := newTUIHarness(t)
	p := addTestProvider(t, mgr, "custom", "https://api.totally-made-up-vendor.example/v1", protocol.APIStyleOpenAI)

	if got := availableModels(mgr.GetGlobalConfig(), p); len(got) != 0 {
		t.Errorf("expected empty list for unknown provider, got %v", got)
	}
}
