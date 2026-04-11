package protocol_validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/server_validate"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestEnv wires a real gateway Server (with the full transform pipeline) to a
// VirtualServer. It manages config, routing rules, and provides SendAs() for
// full round-trip testing.
type TestEnv struct {
	appConfig *config.AppConfig
	ginEngine interface {
		ServeHTTP(http.ResponseWriter, *http.Request)
	}
	gatewayServer *httptest.Server // real HTTP server for streaming support
	virtual       *server_validate.VirtualServer
	modelToken    string

	mu          sync.Mutex
	routeModels map[string]string // key → requestModel
}

// NewTestEnv creates a TestEnv with a fresh gateway config and a new VirtualServer.
// All resources are cleaned up via t.Cleanup.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	configDir, err := os.MkdirTemp("", "pv-test-*")
	if err != nil {
		t.Fatalf("create temp config dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(configDir) })

	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		t.Fatalf("create app config: %v", err)
	}

	gatewayServer := server.NewServer(appConfig.GetGlobalConfig(), server.WithAdaptor(false))
	router := gatewayServer.GetRouter()
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	return &TestEnv{
		appConfig:     appConfig,
		ginEngine:     router,
		gatewayServer: ts,
		virtual:       server_validate.NewVirtualServer(t),
		modelToken:    appConfig.GetGlobalConfig().GetModelToken(),
		routeModels:   make(map[string]string),
	}
}

// Close is a no-op; resources are cleaned up via t.Cleanup registered in NewTestEnv.
func (env *TestEnv) Close() {}

// VirtualURL returns the URL of the underlying virtual server.
func (env *TestEnv) VirtualURL() string { return env.virtual.URL() }

// VirtualCallCount returns the number of requests received by the virtual server.
func (env *TestEnv) VirtualCallCount() int { return env.virtual.CallCount() }

// SetupRoute configures a gateway rule that routes source protocol requests
// to the virtual server acting as a target protocol provider.
//
// The virtual server is pre-registered with the scenario's mock responses.
func (env *TestEnv) SetupRoute(source, target protocol.APIType, s Scenario) {
	env.virtual.RegisterScenario(s.toVirtualServerScenario())

	virtualURL := env.virtual.URL()
	providerName := fmt.Sprintf("virtual-%s-%s", target, s.Name)
	providerModel := fmt.Sprintf("virtual-model-%s", s.Name)
	requestModel := fmt.Sprintf("pv-%s-to-%s-%s", source, target, s.Name)

	apiStyle := targetToAPIStyle(target)

	providerAPIBase := virtualURL
	if apiStyle == protocol.APIStyleOpenAI {
		providerAPIBase = virtualURL + "/v1"
	}

	provider := &typ.Provider{
		UUID:     providerName,
		Name:     providerName,
		APIBase:  providerAPIBase,
		APIStyle: apiStyle,
		Token:    "virtual-token",
		Enabled:  true,
		Timeout:  int64(constant.DefaultRequestTimeout),
	}
	_ = env.appConfig.AddProvider(provider)

	ruleScenario := sourceToRuleScenario(source)

	rule := typ.Rule{
		UUID:          requestModel,
		Scenario:      ruleScenario,
		RequestModel:  requestModel,
		ResponseModel: providerModel,
		Services: []*loadbalance.Service{
			{
				Provider:   providerName,
				Model:      providerModel,
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Active: true,
	}
	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

	key := routeKey(source, target, s.Name)
	env.mu.Lock()
	env.routeModels[key] = requestModel
	env.mu.Unlock()
}

// SendAs sends a request to the gateway as the given source protocol,
// using the request model configured by SetupRoute, and returns the parsed result.
//
// Streaming requests use the real httptest.Server (env.gatewayServer) because
// httptest.ResponseRecorder does not support Gin's streaming/SSE machinery.
// Non-streaming requests use the recorder for simplicity.
func (env *TestEnv) SendAs(t *testing.T, source protocol.APIType, s Scenario, streaming bool) *RoundTripResult {
	t.Helper()

	requestModel := env.findRouteModel(source, s.Name)
	if requestModel == "" {
		t.Fatalf("no route configured for source=%s scenario=%s — call SetupRoute first", source, s.Name)
	}

	path, body := buildRequest(source, requestModel, streaming)

	result := &RoundTripResult{
		SourceProtocol: source,
		ScenarioName:   s.Name,
		IsStreaming:    streaming,
	}

	if streaming {
		url := env.gatewayServer.URL + path
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+env.modelToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do streaming request: %v", err)
		}
		defer resp.Body.Close()

		result.HTTPStatus = resp.StatusCode
		result.StreamEvents, result.RawBody = sse.ReadSSELines(resp.Body)
		fillFromParsedResult(result, sse.ParsedResult{}, sourceToStyle(source), true)
		parsed := assembleFromEvents(result.StreamEvents, sourceToStyle(source))
		fillFromParsedResult(result, parsed, sourceToStyle(source), true)
	} else {
		req, err := http.NewRequest("POST", path, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+env.modelToken)

		w := httptest.NewRecorder()
		env.ginEngine.ServeHTTP(w, req)

		result.HTTPStatus = w.Code
		result.RawBody = w.Body.Bytes()
		parsed := parseFromJSON(result.RawBody, sourceToStyle(source))
		fillFromParsedResult(result, parsed, sourceToStyle(source), false)
	}

	return result
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func routeKey(source, target protocol.APIType, scenario string) string {
	return fmt.Sprintf("%s|%s|%s", source, target, scenario)
}

func (env *TestEnv) findRouteModel(source protocol.APIType, scenarioName string) string {
	env.mu.Lock()
	defer env.mu.Unlock()
	prefix := fmt.Sprintf("%s|", source)
	suffix := fmt.Sprintf("|%s", scenarioName)
	for k, v := range env.routeModels {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix &&
			len(k) >= len(suffix) && k[len(k)-len(suffix):] == suffix {
			return v
		}
	}
	return ""
}

func buildRequest(source protocol.APIType, model string, streaming bool) (path string, body []byte) {
	switch source {
	case protocol.TypeAnthropicV1:
		return "/tingly/anthropic/v1/messages", mustMarshal(map[string]interface{}{
			"model":      model,
			"max_tokens": 1024,
			"messages": []map[string]interface{}{
				{"role": "user", "content": "What is the capital of France?"},
			},
			"stream": streaming,
		})
	case protocol.TypeAnthropicBeta:
		return "/tingly/anthropic/v1/messages?beta=true", mustMarshal(map[string]interface{}{
			"model":      model,
			"max_tokens": 1024,
			"messages": []map[string]interface{}{
				{"role": "user", "content": []map[string]interface{}{
					{"type": "text", "text": "What is the capital of France?"},
				}},
			},
			"stream": streaming,
		})
	case protocol.TypeOpenAIChat:
		return "/tingly/openai/v1/chat/completions", mustMarshal(map[string]interface{}{
			"model": model,
			"messages": []map[string]interface{}{
				{"role": "user", "content": "What is the capital of France?"},
			},
			"stream": streaming,
		})
	case protocol.TypeOpenAIResponses:
		return "/tingly/openai/v1/responses", mustMarshal(map[string]interface{}{
			"model": model,
			"input": []map[string]interface{}{
				{"type": "message", "role": "user", "content": []map[string]interface{}{
					{"type": "input_text", "text": "What is the capital of France?"},
				}},
			},
			"stream": streaming,
		})
	default:
		return "/tingly/openai/v1/chat/completions", mustMarshal(map[string]interface{}{
			"model":    model,
			"messages": []map[string]interface{}{{"role": "user", "content": "What is the capital of France?"}},
			"stream":   streaming,
		})
	}
}

func targetToAPIStyle(target protocol.APIType) protocol.APIStyle {
	switch target {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return protocol.APIStyleAnthropic
	case protocol.TypeGoogle:
		return protocol.APIStyleGoogle
	default:
		return protocol.APIStyleOpenAI
	}
}

func sourceToRuleScenario(source protocol.APIType) typ.RuleScenario {
	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return typ.ScenarioAnthropic
	default:
		return typ.ScenarioOpenAI
	}
}

func sourceToStyle(source protocol.APIType) protocol.APIStyle {
	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return protocol.APIStyleAnthropic
	default:
		return protocol.APIStyleOpenAI
	}
}

func parseFromJSON(raw []byte, style protocol.APIStyle) sse.ParsedResult {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return sse.ParsedResult{}
	}
	var r *sse.ParsedResult
	switch style {
	case protocol.APIStyleOpenAI:
		if _, hasOutput := m["output"]; hasOutput {
			r = sse.ParseOpenAIResponsesResult(m)
		} else {
			r = sse.ParseOpenAIChatResult(m)
		}
	case protocol.APIStyleAnthropic:
		r = sse.ParseAnthropicResult(m)
	case protocol.APIStyleGoogle:
		r = sse.ParseGoogleResult(m)
	}
	if r == nil {
		return sse.ParsedResult{}
	}
	return *r
}

func assembleFromEvents(events []string, style protocol.APIStyle) sse.ParsedResult {
	var r *sse.ParsedResult
	switch style {
	case protocol.APIStyleOpenAI:
		r = sse.AssembleOpenAIStream(events)
	case protocol.APIStyleAnthropic:
		r = sse.AssembleAnthropicStream(events)
	case protocol.APIStyleGoogle:
		r = sse.AssembleGoogleStream(events)
	}
	if r == nil {
		return sse.ParsedResult{}
	}
	return *r
}

func fillFromParsedResult(result *RoundTripResult, parsed sse.ParsedResult, _ protocol.APIStyle, _ bool) {
	result.Role = parsed.Role
	result.Content = parsed.Content
	result.Model = parsed.Model
	result.FinishReason = parsed.FinishReason
	result.ThinkingContent = parsed.ThinkingContent
	for _, tc := range parsed.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCallResult{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		})
	}
	if parsed.Usage != nil {
		result.Usage = &TokenUsage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		}
	}
}
