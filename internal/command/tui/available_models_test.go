package tui

import (
	"context"
	"os"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// tuiHarnessManager satisfies TUIManager by delegating to an AppConfig. It
// mirrors the production *command.AppManager surface but lives here to keep
// the test inside the tui package (avoids a command → tui → command cycle).
type tuiHarnessManager struct {
	ac *config.AppConfig
}

func (m *tuiHarnessManager) ListProviders() []*typ.Provider { return m.ac.ListProviders() }
func (m *tuiHarnessManager) GetProvider(id string) (*typ.Provider, error) {
	return m.ac.GetProviderByUUID(id)
}
func (m *tuiHarnessManager) AddProvider(name, apiBase, token string, apiStyle protocol.APIStyle) (string, error) {
	p := &typ.Provider{
		Name: name, APIBase: apiBase, Token: token, APIStyle: apiStyle,
		AuthType: typ.AuthTypeAPIKey, Enabled: true,
	}
	if err := m.ac.AddProvider(p); err != nil {
		return "", err
	}
	return p.UUID, nil
}
func (m *tuiHarnessManager) UpdateProviderByUUID(uuid string, p *typ.Provider) error {
	return m.ac.GetGlobalConfig().UpdateProvider(uuid, p)
}
func (m *tuiHarnessManager) DeleteProviderByUUID(uuid string) error {
	return m.ac.GetGlobalConfig().DeleteProvider(uuid)
}
func (m *tuiHarnessManager) FetchAndSaveProviderModels(uuid string) error {
	return m.ac.FetchAndSaveProviderModels(uuid)
}
func (m *tuiHarnessManager) ListRules() []typ.Rule { return m.ac.GetGlobalConfig().Rules }
func (m *tuiHarnessManager) GetRuleByUUID(uuid string) *typ.Rule {
	return m.ac.GetGlobalConfig().GetRuleByUUID(uuid)
}
func (m *tuiHarnessManager) AddRule(r typ.Rule) error { return m.ac.GetGlobalConfig().AddRule(r) }
func (m *tuiHarnessManager) UpdateRule(uuid string, r typ.Rule) error {
	return m.ac.GetGlobalConfig().UpdateRule(uuid, r)
}
func (m *tuiHarnessManager) DeleteRule(uuid string) error {
	return m.ac.GetGlobalConfig().DeleteRule(uuid)
}
func (m *tuiHarnessManager) SaveConfig() error                          { return m.ac.Save() }
func (m *tuiHarnessManager) GetGlobalConfig() *serverconfig.Config       { return m.ac.GetGlobalConfig() }
func (m *tuiHarnessManager) GetServerPort() int                          { return 0 }
func (m *tuiHarnessManager) SetupServerWithPort(int) error               { return nil }
func (m *tuiHarnessManager) StartServer() error                          { return nil }

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

// TestAvailableModels_PrefersDBOverTemplate: when the DB has fresh models
// from a /v1/models call, they win over the embedded template list. We
// must not silently return template defaults when fresh API data exists.
func TestAvailableModels_PrefersDBOverTemplate(t *testing.T) {
	mgr := newTUIHarness(t)
	uuid, err := mgr.AddProvider("openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	p, err := mgr.GetProvider(uuid)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}

	if err := mgr.GetGlobalConfig().GetModelManager().SaveModels(p, []string{"dbm-1", "dbm-2"}, db.ModelSourceAPI); err != nil {
		t.Fatalf("SaveModels: %v", err)
	}

	got := availableModels(mgr, p)
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
	uuid, err := mgr.AddProvider("anthropic", "https://api.anthropic.com", "tok", protocol.APIStyleAnthropic)
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	p, err := mgr.GetProvider(uuid)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}

	got := availableModels(mgr, p)
	if len(got) == 0 {
		t.Fatalf("expected template fallback to provide Anthropic models, got empty list")
	}
}

// TestAvailableModels_EmptyWhenNoSource: an unknown provider with no DB
// cache and no template match returns nil. pickProviderModel uses this to
// decide between a Select and a free-form Input.
func TestAvailableModels_EmptyWhenNoSource(t *testing.T) {
	mgr := newTUIHarness(t)
	uuid, err := mgr.AddProvider("custom", "https://api.totally-made-up-vendor.example/v1", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	p, err := mgr.GetProvider(uuid)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}

	if got := availableModels(mgr, p); len(got) != 0 {
		t.Errorf("expected empty list for unknown provider, got %v", got)
	}
}
