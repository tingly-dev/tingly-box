package protocoltest

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// agentGatewayPath returns the gateway endpoint an agent's CLI posts to.
func agentGatewayPath(at AgentType) string {
	switch at {
	case AgentTypeClaudeCode:
		return "/tingly/claude_code/v1/messages"
	case AgentTypeCodex:
		return "/tingly/codex/v1/responses"
	case AgentTypeOpenCode:
		return "/tingly/opencode/v1/messages"
	default:
		return ""
	}
}

// agentAPIStyle returns the wire-format API style an agent speaks.
func agentAPIStyle(at AgentType) protocol.APIStyle {
	switch at {
	case AgentTypeCodex:
		return protocol.APIStyleOpenAI
	default:
		return protocol.APIStyleAnthropic
	}
}

// repointBuiltinRule updates the agent's built-in rule so its fixed
// RequestModel routes to a single service{providerUUID, upstreamModel}.
// It is the shared core of the SetupVirtualAgentScenario / SetupVModelAgent
// replay wiring.
func (env *AgentTestEnv) repointBuiltinRule(agentType AgentType, providerUUID, upstreamModel string) error {
	var builtinUUID, requestModel string
	switch agentType {
	case AgentTypeClaudeCode:
		builtinUUID, requestModel = "built-in-cc", "tingly/cc"
	case AgentTypeCodex:
		builtinUUID, requestModel = "built-in-codex", "tingly-codex"
	case AgentTypeOpenCode:
		builtinUUID, requestModel = "built-in-opencode", "tingly-opencode"
	default:
		return fmt.Errorf("unknown Agent type: %s", agentType)
	}

	rule := typ.Rule{
		UUID:          builtinUUID,
		Scenario:      agentType.Scenario(),
		RequestModel:  requestModel,
		ResponseModel: upstreamModel,
		Services: []*loadbalance.Service{
			{
				Provider: providerUUID,
				Model:    upstreamModel,
				Weight:   1,
				Active:   true,
			},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.DefaultRandomParams(),
		},
		Active: true,
	}

	if err := env.appConfig.GetGlobalConfig().UpdateRequestConfigByUUID(builtinUUID, rule); err != nil {
		return fmt.Errorf("update rule: %w", err)
	}
	return nil
}

// SetupVirtualAgentScenario wires the agent's built-in rule to the in-process
// VirtualServer and registers `scenario`'s mock responses so the VirtualServer
// serves them deterministically.
//
// This is the "virtual" replay upstream: because the response is fully
// controlled by the scenario's MockResponses, the caller can run the
// scenario's content-level Assertions against the round-trip result.
//
// The rule's upstream model encodes the scenario name as
// "virtual-model-<scenario>" so the VirtualServer's scenario
// detection resolves the right mock.
func (env *AgentTestEnv) SetupVirtualAgentScenario(agentType AgentType, scenario Scenario) error {
	virtualURL := env.VirtualServerURL()
	if virtualURL == "" {
		return fmt.Errorf("virtual server not initialized")
	}

	env.virtualServer.RegisterScenario(scenario)

	style := agentAPIStyle(agentType)
	apiBase := virtualURL
	if style == protocol.APIStyleOpenAI {
		// OpenAI-style providers expect the /v1 prefix on the base URL so the
		// gateway forwards to {base}/responses etc. — mirrors TestEnv.SetupRoute.
		apiBase = virtualURL + "/v1"
	}

	providerName := fmt.Sprintf("virtual-replay-%s-%s", agentType, scenario.Name)
	provider := &typ.Provider{
		UUID:     providerName,
		Name:     providerName,
		APIBase:  apiBase,
		APIStyle: style,
		Token:    "test-virtual-token",
		Enabled:  true,
		Timeout:  30000,
	}
	if err := env.appConfig.AddProvider(provider); err != nil {
		return fmt.Errorf("add provider: %w", err)
	}

	upstreamModel := fmt.Sprintf("virtual-model-%s", scenario.Name)
	return env.repointBuiltinRule(agentType, providerName, upstreamModel)
}

// ReplayFixture sends a raw request body to the agent's gateway endpoint and
// parses the response into a RoundTripResult ready for Scenario assertions.
//
// The endpoint path, auth header, and response API style are all derived from
// agentType. `streaming` selects SSE vs JSON response parsing — it must match
// the fixture's own "stream" flag.
func (env *AgentTestEnv) ReplayFixture(agentType AgentType, body []byte, streaming bool) (*RoundTripResult, error) {
	path := agentGatewayPath(agentType)
	if path == "" {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}
	style := agentAPIStyle(agentType)

	req, err := http.NewRequest(http.MethodPost, env.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if style == protocol.APIStyleAnthropic {
		req.Header.Set("x-api-key", env.modelToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+env.modelToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	result := &RoundTripResult{IsStreaming: streaming, HTTPStatus: resp.StatusCode}
	if streaming {
		events, raw := sse.ReadSSELines(resp.Body)
		result.StreamEvents = events
		result.RawBody = raw
		parsed := assembleFromEvents(events, style)
		fillFromParsedResult(result, parsed, style, true)
	} else {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
		result.RawBody = raw
		parsed := parseFromJSON(raw, style)
		fillFromParsedResult(result, parsed, style, false)
	}
	return result, nil
}
