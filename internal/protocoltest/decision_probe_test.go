package protocoltest

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestDecisionProbeThroughGatewayToVirtualModel(t *testing.T) {
	env := NewTestEnv(t)
	model := env.SetupDecisionRoute(t, "decision-first")
	cfg := env.appConfig.GetGlobalConfig()
	_, port, err := net.SplitHostPort(env.gatewayServer.Listener.Addr().String())
	require.NoError(t, err)
	cfg.ServerPort, err = strconv.Atoi(port)
	require.NoError(t, err)
	rule := cfg.MatchRuleByModelAndScenario(model, typ.ScenarioOpenAI)
	require.NotNil(t, rule)
	request := &probe.E2ERequest{TargetType: probe.E2ETargetRule, Scenario: "openai", RuleUUID: rule.UUID, Operation: "decisions", DecisionRequest: json.RawMessage(`{"state":"invoice problem","questions":{"q":{"type":"choice","instructions":"Which team?","criteria":{"billing":"Invoices","technical":"Errors"}}}}`)}
	require.NoError(t, probe.ValidateE2ERequest(request))
	result, err := probe.NewE2EProber(cfg, client.NewClientPool()).Probe(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "billing", gjson.GetBytes(result.DecisionResponse, "answers.q.choice").String())
	require.Equal(t, rule.UUID, result.MatchedRule)
	require.NotEmpty(t, result.SelectedProviderUUID)
	require.Equal(t, "decision-first", result.SelectedModel)
	require.NotNil(t, result.Usage)
}
