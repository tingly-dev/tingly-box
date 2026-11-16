package servertest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/appconfig"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// "Test all" on a non-default Team page probes its rules through the real
// gateway under the scope team:<id>. Before the fix the gateway refused that
// scope with {"error":"unknown profile ..."} and, separately, any upstream that
// answers {"error":"text"} turned into a Go JSON decode error. The probe must
// instead surface the upstream's own status and message.
func TestProbeTeamRule_UpstreamStringErrorIsSurfaced(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key for team upstream"}`))
	}))
	defer upstream.Close()

	appConfig, err := appconfig.NewAppConfig(appconfig.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	ts := createTestServer(t, appConfig)
	cfg := appConfig.GetGlobalConfig()

	gateway := httptest.NewServer(ts.ginEngine)
	defer gateway.Close()
	u, err := url.Parse(gateway.URL)
	require.NoError(t, err)
	cfg.ServerPort, err = strconv.Atoi(u.Port())
	require.NoError(t, err)

	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "team-up", Name: "team-up", APIBase: upstream.URL, APIStyle: protocol.APIStyleOpenAI,
		Token: "sk-test", Enabled: true, Timeout: 5,
	}))
	team, err := cfg.StoreManager().Team().Create("Research")
	require.NoError(t, err)
	scope := db.TeamScenario(team.ID)
	require.NoError(t, cfg.AddOrUpdateRequestConfigByRequestModel(typ.Rule{
		UUID: "team-rule", Scenario: scope, RequestModel: "team-model", Active: true,
		Services: []*loadbalance.Service{{Provider: "team-up", Model: "up-model", Weight: 1, Active: true, TimeWindow: 300}},
		LBTactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.NewRandomParams()},
	}))

	prober := probe.NewE2EProber(cfg, client.NewClientPool())
	_, err = prober.Probe(context.Background(), &probe.E2ERequest{
		TargetType: probe.E2ETargetRule,
		RuleUUID:   "team-rule",
		Scenario:   string(scope),
	})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "unmarshal")
	require.Equal(t, http.StatusUnauthorized, protocol.UpstreamStatus(err, 0), "err: %v", err)
	require.Contains(t, protocol.UpstreamMessage(err), "invalid api key for team upstream")
}

// The dialog renders the probe response's error message, so the endpoint must
// carry the upstream's own status and text, not the SDK's bare
// "OpenAI API error: 401 Unauthorized" summary.
func TestProbeEndpoint_TeamRuleShowsUpstreamMessage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key for team upstream"}`))
	}))
	defer upstream.Close()

	appConfig, err := appconfig.NewAppConfig(appconfig.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	ts := createTestServer(t, appConfig)
	cfg := appConfig.GetGlobalConfig()

	gateway := httptest.NewServer(ts.ginEngine)
	defer gateway.Close()
	u, err := url.Parse(gateway.URL)
	require.NoError(t, err)
	cfg.ServerPort, err = strconv.Atoi(u.Port())
	require.NoError(t, err)

	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "team-up", Name: "team-up", APIBase: upstream.URL, APIStyle: protocol.APIStyleOpenAI,
		Token: "sk-test", Enabled: true, Timeout: 5,
	}))
	team, err := cfg.StoreManager().Team().Create("Research")
	require.NoError(t, err)
	scope := db.TeamScenario(team.ID)
	require.NoError(t, cfg.AddOrUpdateRequestConfigByRequestModel(typ.Rule{
		UUID: "team-rule", Scenario: scope, RequestModel: "team-model", Active: true,
		Services: []*loadbalance.Service{{Provider: "team-up", Model: "up-model", Weight: 1, Active: true, TimeWindow: 300}},
		LBTactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.NewRandomParams()},
	}))

	body, _ := json.Marshal(map[string]any{"target_type": "rule", "rule_uuid": "team-rule", "scenario": string(scope), "stream": true})
	req, _ := http.NewRequest(http.MethodPost, "/api/v2/probe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.GetUserToken())
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.NotContains(t, resp.Error.Message, "unmarshal")
	require.Contains(t, resp.Error.Message, "401")
	require.Contains(t, resp.Error.Message, "invalid api key for team upstream")
}
