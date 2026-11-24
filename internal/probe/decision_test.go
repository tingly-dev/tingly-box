package probe

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestDecisionProbeLoopbackAndCurl(t *testing.T) {
	var gotPath string
	var gotBody []byte
	responseBody := `{"model":"model-a","answers":{"q":{"type":"noul","noul":0.8}},"usage":{"input_tokens":7,"output_tokens":1}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tingly-Matched-Rule", "decision-rule")
		w.Header().Set("X-Tingly-Selected-Provider", "test-provider")
		w.Header().Set("X-Tingly-Selected-Model", "upstream-model")
		_, _ = w.Write([]byte(responseBody))
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	cfg := newTestConfig(t)
	cfg.ServerPort, err = strconv.Atoi(port)
	require.NoError(t, err)
	cfg.ModelToken = "test-token"
	require.NoError(t, cfg.AddRule(typ.Rule{UUID: "decision-rule", Scenario: typ.ScenarioDecisions, RequestModel: "model-a", Active: true}))
	svc := NewE2EProber(cfg, client.NewClientPool())
	req := &E2ERequest{TargetType: E2ETargetRule, Scenario: "decisions", RuleUUID: "decision-rule", Operation: "decisions", DecisionRequest: json.RawMessage(`{"state":"x","model":"ignored","questions":{"q":{"type":"noul","instructions":"urgent?"}}}`)}
	require.NoError(t, ValidateE2ERequest(req))
	result, err := svc.Probe(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "/tingly/decisions/decisions", gotPath)
	require.Equal(t, "model-a", gjson.GetBytes(gotBody, "model").String())
	require.Equal(t, 0.8, gjson.GetBytes(result.DecisionResponse, "answers.q.noul").Float())
	require.Equal(t, 7, int(result.Usage.InputTokens))
	require.Equal(t, "decision-rule", result.MatchedRule)
	require.Equal(t, "test-provider", result.SelectedProvider)
	require.Equal(t, "upstream-model", result.SelectedModel)
	require.False(t, result.Stream)
	curl, err := svc.BuildCurl(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:"+port+"/tingly/decisions/decisions", curl.URL)
	require.JSONEq(t, string(gotBody), curl.Body)
	responseBody = `{"model":"model-a","answers":{"q":{"type":"noul","noul":0.8}},"usage":{"input_tokens":7}}`
	result, err = svc.Probe(context.Background(), req)
	require.NoError(t, err)
	require.Nil(t, result.Usage, "do not report an invented output-token count")
}
func TestDecisionProbeValidation(t *testing.T) {
	valid := &E2ERequest{TargetType: E2ETargetRule, Scenario: "decisions", RuleUUID: "r", Operation: "decisions"}
	require.NoError(t, ValidateE2ERequest(valid))
	for _, req := range []*E2ERequest{
		{TargetType: E2ETargetRule, Scenario: "decisions", RuleUUID: "r", Operation: "decisions", Stream: boolPtr(true)},
		{TargetType: E2ETargetRule, Scenario: "decisions", RuleUUID: "r", Operation: "decisions", Thinking: ThinkingMedium},
		{TargetType: E2ETargetRule, Scenario: "decisions", RuleUUID: "r", Operation: "decisions", DecisionRequest: json.RawMessage(`{"state":"x","questions":{}}`)},
		{TargetType: E2ETargetProvider, ProviderUUID: "p", Model: "m", Operation: "decisions"},
	} {
		require.Error(t, ValidateE2ERequest(req))
	}
}

func TestDecisionProbeWildcardNeedsConcreteModel(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.ServerPort, cfg.ModelToken = 18080, "test-token"
	require.NoError(t, cfg.AddRule(typ.Rule{UUID: "wildcard", Scenario: typ.ScenarioDecisions, RequestModel: "*", Active: true}))
	svc := NewE2EProber(cfg, client.NewClientPool())
	req := &E2ERequest{TargetType: E2ETargetRule, Scenario: "decisions", RuleUUID: "wildcard", Operation: "decisions"}
	_, _, _, err := svc.resolveRuleTarget(context.Background(), req)
	require.ErrorContains(t, err, "concrete model")
	req.Model = "my-model"
	_, model, _, err := svc.resolveRuleTarget(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "my-model", model)
}

func TestDecisionProbeResponseValidation(t *testing.T) {
	request := []byte(`{"state":"x","questions":{"q":{"type":"noul","instructions":"x"}}}`)
	for _, response := range []string{`{"answers":{}}`, `{"answers":{"other":{"type":"noul","noul":0.8}}}`, `{"answers":{"q":{"type":"noul"}}}`, `{"answers":{"q":{"type":"noul","noul":1.2}}}`, `{"answers":[{"type":"noul","noul":0.8}]}`} {
		require.Error(t, validateDecisionResponse(request, []byte(response)), response)
	}
	require.NoError(t, validateDecisionResponse(request, []byte(`{"answers":{"q":{"type":"noul","noul":0.8}}}`)))
}
