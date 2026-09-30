package protocolserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type fakeQuotaReader map[string]*quota.ProviderUsage

func (f fakeQuotaReader) GetQuota(_ context.Context, uuid string) (*quota.ProviderUsage, error) {
	if u, ok := f[uuid]; ok {
		return u, nil
	}
	return nil, quota.ErrProviderUnsupported
}

func TestHandleScenarioQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	add := func(model string, scenario typ.RuleScenario, provider string) {
		if err := cfg.AddRequestConfig(typ.Rule{UUID: model, Scenario: scenario, RequestModel: model, Active: true,
			Services: []*loadbalance.Service{{Provider: provider, Model: model, Active: true}}}); err != nil {
			t.Fatal(err)
		}
	}
	add("opus", typ.ScenarioTeam, "anthropic") // two team models on one account
	add("sonnet", typ.ScenarioTeam, "anthropic")
	add("other", typ.ScenarioClaudeCode, "openrouter") // another scenario: out of scope

	window := &quota.UsageWindow{Key: "5h", Label: "5h", Kind: quota.WindowKindLimit, Used: 300, Limit: 1000}
	h := NewHandler(ProtocolHandlerDeps{Config: cfg, QuotaReader: fakeQuotaReader{
		"anthropic":  {ProviderName: "Anthropic Max", Windows: []*quota.UsageWindow{window}},
		"openrouter": {Windows: []*quota.UsageWindow{window}},
	}})
	call := func(authKind string) *httptest.ResponseRecorder {
		router := gin.New()
		router.GET("/tingly/:scenario/quota", func(c *gin.Context) {
			c.Set(constant.CtxKeyAuthKind, authKind)
		}, h.HandleScenarioQuota)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tingly/team/quota", nil))
		return rec
	}

	if rec := call(constant.AuthKindSharingKey); rec.Code != http.StatusForbidden {
		t.Fatalf("sharing key: status = %d, want 403", rec.Code)
	}
	rec := call(constant.AuthKindGlobalModelToken)
	var u quota.ProviderUsage
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &u) != nil {
		t.Fatalf("owner: status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(u.Windows) != 1 || u.Windows[0].Label != "Anthropic Max · 5h" || u.Windows[0].Used != 30 {
		t.Errorf("windows = %+v, want the shared account once, as a percentage", u.Windows)
	}
}
