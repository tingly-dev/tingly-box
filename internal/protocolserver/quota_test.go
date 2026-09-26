package protocolserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	add := func(model string, scenario typ.RuleScenario, services ...*loadbalance.Service) {
		if err := cfg.AddRequestConfig(typ.Rule{
			UUID: model + "-" + string(scenario), Scenario: scenario, RequestModel: model,
			Active: true, Services: services,
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("sonnet", typ.ScenarioTeam, &loadbalance.Service{Provider: "anthropic", Model: "claude-sonnet", Active: true})
	add("other-team-model", typ.ProfiledScenarioName(typ.ScenarioTeam, "other"),
		&loadbalance.Service{Provider: "anthropic", Model: "claude-opus", Active: true})

	resets := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	reader := fakeQuotaReader{"anthropic": {
		ProviderName: "Anthropic Max", FetchedAt: resets.Add(-time.Hour),
		Account: &quota.UsageAccount{Email: "owner@example.com"},
		Windows: []*quota.UsageWindow{{
			Key: "5h", Type: quota.WindowTypeSession, Kind: quota.WindowKindLimit, Label: "5h",
			Used: 300, Limit: 1000, UsedPercent: 30, Unit: quota.UsageUnitTokens,
			WindowMinutes: 300, ResetsAt: &resets,
		}},
	}}

	visible := false
	h := NewHandler(ProtocolHandlerDeps{
		Config:           cfg,
		QuotaReader:      reader,
		TeamQuotaVisible: func(teamID string) bool { return teamID == "t-default" && visible },
	})

	call := func(authKind string) *httptest.ResponseRecorder {
		router := gin.New()
		router.GET("/tingly/:scenario/quota", func(c *gin.Context) {
			c.Set(constant.CtxKeyAuthKind, authKind)
			if authKind == constant.AuthKindSharingKey {
				c.Set(constant.CtxKeyTeamID, "t-default")
			}
			c.Next()
		}, h.HandleScenarioQuota)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tingly/team/quota", nil))
		return rec
	}

	// A team that keeps quota private refuses its sharing keys.
	if rec := call(constant.AuthKindSharingKey); rec.Code != http.StatusForbidden {
		t.Fatalf("private team: status = %d, want 403", rec.Code)
	}

	visible = true
	rec := call(constant.AuthKindSharingKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("shared team: status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, leak := range []string{"Anthropic", "owner@example.com", "claude-sonnet", "anthropic", "1000", "other-team-model"} {
		if strings.Contains(body, leak) {
			t.Errorf("sharing-key reply leaks %q: %s", leak, body)
		}
	}
	var gq quota.GatewayQuota
	if err := json.Unmarshal(rec.Body.Bytes(), &gq); err != nil {
		t.Fatal(err)
	}
	if len(gq.Models) != 1 || gq.Models[0].Model != "sonnet" || len(gq.Models[0].Windows) != 1 {
		t.Fatalf("models = %+v, want sonnet with one window", gq.Models)
	}
	if w := gq.Models[0].Windows[0]; w.Unit != quota.UsageUnitPercent || w.Used != 30 || w.Limit != 100 {
		t.Errorf("sharing-key window = %+v, want 30/100 percent", w)
	}

	// The operator's own model token reads the windows as stored.
	rec = call(constant.AuthKindGlobalModelToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("global token: status = %d", rec.Code)
	}
	gq = quota.GatewayQuota{}
	if err := json.Unmarshal(rec.Body.Bytes(), &gq); err != nil {
		t.Fatal(err)
	}
	if w := gq.Models[0].Windows[0]; w.Limit != 1000 || w.Unit != quota.UsageUnitTokens {
		t.Errorf("global-token window = %+v, want stored figures", w)
	}
}
