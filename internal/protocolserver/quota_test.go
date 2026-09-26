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
	add := func(model string, scenario typ.RuleScenario, provider string) {
		if err := cfg.AddRequestConfig(typ.Rule{
			UUID: model + "-" + string(scenario), Scenario: scenario, RequestModel: model, Active: true,
			Services: []*loadbalance.Service{{Provider: provider, Model: model, Active: true}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Two team rules on one account, one on another team's scope.
	add("opus", typ.ScenarioTeam, "anthropic")
	add("sonnet", typ.ScenarioTeam, "anthropic")
	add("other", typ.ProfiledScenarioName(typ.ScenarioTeam, "other"), "openrouter")

	fiveHour := func(used, limit float64) *quota.UsageWindow {
		return &quota.UsageWindow{Key: "5h", Label: "5h", Type: quota.WindowTypeSession, Kind: quota.WindowKindLimit,
			Used: used, Limit: limit, UsedPercent: used * 100 / limit, Unit: quota.UsageUnitTokens, WindowMinutes: 300}
	}
	reader := fakeQuotaReader{
		"anthropic":  {ProviderName: "Anthropic Max", FetchedAt: time.Now(), Windows: []*quota.UsageWindow{fiveHour(300, 1000)}},
		"openrouter": {ProviderName: "OpenRouter", FetchedAt: time.Now(), Windows: []*quota.UsageWindow{fiveHour(1, 2)}},
	}

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
		}, h.HandleScenarioQuota)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tingly/team/quota", nil))
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) quota.ProviderUsage {
		var u quota.ProviderUsage
		if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
			t.Fatal(err)
		}
		return u
	}

	if rec := call(constant.AuthKindSharingKey); rec.Code != http.StatusForbidden {
		t.Fatalf("private team: status = %d, want 403", rec.Code)
	}

	visible = true
	rec := call(constant.AuthKindSharingKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("shared team: status = %d: %s", rec.Code, rec.Body.String())
	}
	for _, leak := range []string{"Anthropic", "OpenRouter", "anthropic", "1000"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("sharing-key reply leaks %q: %s", leak, rec.Body.String())
		}
	}
	// The account behind both team models is relayed once; the other team's
	// provider is out of scope.
	u := decode(rec)
	if len(u.Windows) != 1 || u.Windows[0].Label != "upstream 1 · 5h" || u.Windows[0].Used != 30 || u.Windows[0].Limit != 100 {
		t.Fatalf("sharing-key windows = %+v", u.Windows)
	}

	u = decode(call(constant.AuthKindGlobalModelToken))
	if len(u.Windows) != 1 || u.Windows[0].Limit != 1000 {
		t.Errorf("operator windows = %+v, want stored figures", u.Windows)
	}
}
