package protocoltest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/server"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestEnv wires a real gateway Server (with the full transform pipeline) to a
// VirtualServer (mock provider). It manages config, routing rules, and provides
// SendAs() for full round-trip testing.
//
// **Routing Architecture**:
//
//	Client Request → Gateway (/tingly/{scenario}/v1/...)
//	              → Protocol Transform
//	              → Provider Request (virtual-server-url/v1/...)
//	              → VirtualServer (mock provider response)
//
// The gateway handles /tingly/{scenario}/v1/... routes, transforms the request
// to provider format, and forwards to the virtual server which speaks provider
// native APIs (/v1/chat/completions, /v1/messages, etc.).
type TestEnv struct {
	appConfig *config.AppConfig
	ginEngine interface {
		ServeHTTP(http.ResponseWriter, *http.Request)
	}
	gatewayServer *httptest.Server // real HTTP server for streaming support
	virtual       *VirtualServer
	modelToken    string

	mu          sync.Mutex
	routeModels map[string]string // key → requestModel
	setupRoutes map[string]bool   // track which routes have been set up
	configDir   string            // config directory for cleanup (CLI mode only)
}

// TestEnvOption is a functional option for configuring TestEnv.
type TestEnvOption func(*testEnvConfig)

type testEnvConfig struct {
	recordDir  string
	mcpEnabled bool
}

// NewTestEnvOptionWithRecordDir creates an option to set the record directory.
// If empty, recording is disabled.
func NewTestEnvOptionWithRecordDir(dir string) TestEnvOption {
	return func(cfg *testEnvConfig) {
		cfg.recordDir = dir
	}
}

// NewTestEnvOptionWithMCP creates an option to enable the MCP feature flag.
func NewTestEnvOptionWithMCP() TestEnvOption {
	return func(cfg *testEnvConfig) {
		cfg.mcpEnabled = true
	}
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
		virtual:       NewVirtualServer(t),
		modelToken:    appConfig.GetGlobalConfig().GetModelToken(),
		routeModels:   make(map[string]string),
		setupRoutes:   make(map[string]bool),
	}
}

// Close cleans up resources. For testing mode, it's a no-op (resources are cleaned up via t.Cleanup).
// For CLI mode, it closes the servers and removes the config directory.
func (env *TestEnv) Close() {
	// For CLI mode, close the gateway server
	if env.gatewayServer != nil {
		env.gatewayServer.Close()
	}
	// For CLI mode, clean up virtual server
	if env.virtual != nil {
		env.virtual.Close()
	}
	// For CLI mode, remove config directory
	if env.configDir != "" {
		os.RemoveAll(env.configDir)
	}
}

// NewTestEnvForCLI creates a TestEnv for CLI use (without testing.T).
// Resources must be cleaned up via explicit Close() call.
func NewTestEnvForCLI(opts ...TestEnvOption) (*TestEnv, error) {
	// Apply options
	cfg := &testEnvConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	configDir, err := os.MkdirTemp("", "pv-cli-*")
	if err != nil {
		return nil, fmt.Errorf("create temp config dir: %w", err)
	}

	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		os.RemoveAll(configDir)
		return nil, fmt.Errorf("create app config: %w", err)
	}

	// Build server options
	serverOpts := []server.ServerOption{server.WithAdaptor(false)}
	if cfg.recordDir != "" {
		serverOpts = append(serverOpts, server.WithRecordDir(cfg.recordDir))
	}
	if cfg.mcpEnabled {
		_ = appConfig.GetGlobalConfig().SetScenarioFlag(typ.ScenarioGlobal, serverconfig.ExtensionMCP, true)
	}

	gatewayServer := server.NewServer(appConfig.GetGlobalConfig(), serverOpts...)
	router := gatewayServer.GetRouter()
	ts := httptest.NewServer(router)

	virtual := NewVirtualServerForCLI()

	return &TestEnv{
		appConfig:     appConfig,
		ginEngine:     router,
		gatewayServer: ts,
		virtual:       virtual,
		modelToken:    appConfig.GetGlobalConfig().GetModelToken(),
		routeModels:   make(map[string]string),
		setupRoutes:   make(map[string]bool),
		configDir:     configDir, // Store for cleanup
	}, nil
}

// VirtualURL returns the URL of the underlying virtual server.
func (env *TestEnv) VirtualURL() string { return env.virtual.URL() }

// VirtualCallCount returns the number of requests received by the virtual server.
func (env *TestEnv) VirtualCallCount() int { return env.virtual.CallCount() }

// SetupRoute configures a gateway rule that routes source protocol requests
// to the virtual server acting as a target protocol provider.
//
// The virtual server is pre-registered with the scenario's mock responses.
// If the route has already been set up, this is a no-op (idempotent).
//
// **Routing Flow**:
// 1. Client sends request to gateway: POST /tingly/{scenario}/v1/chat/completions
// 2. Gateway transforms request to provider format based on source protocol
// 3. Gateway forwards to provider: POST {virtualURL}/v1/chat/completions
// 4. VirtualServer (provider mock) returns pre-configured scenario response
//
// The provider's APIBase includes the /v1 suffix for OpenAI-style providers
// to match actual provider API structure.
func (env *TestEnv) SetupRoute(source, target protocol.APIType, s Scenario) {
	env.setupRouteCore(source, target, s, nil)
}

// setupRouteCore wires the provider + rule for a (source, target, scenario)
// route. When flags is non-nil it is stamped onto the rule, so requests routed
// through it traverse the real flag-resolution + transform path. flags==nil
// preserves the original flag-free behavior used by the protocol matrix.
func (env *TestEnv) setupRouteCore(source, target protocol.APIType, s Scenario, flags *typ.RuleFlags) {
	key := routeKey(source, target, s.Name)

	env.mu.Lock()
	if env.setupRoutes[key] {
		env.mu.Unlock()
		return
	}
	env.setupRoutes[key] = true
	env.mu.Unlock()

	env.virtual.RegisterScenario(s)

	virtualURL := env.virtual.URL()

	// Make provider UUID unique per source+target+scenario to avoid conflicts
	providerUUID := fmt.Sprintf("virtual-%s-%s-%s", source, target, s.Name)
	providerName := providerUUID // Use UUID as name for uniqueness

	providerModel := fmt.Sprintf("virtual-model-%s", s.Name)
	requestModel := fmt.Sprintf("pv-%s-to-%s-%s", source, target, s.Name)

	apiStyle := targetToAPIStyle(target)

	providerAPIBase := virtualURL
	if apiStyle == protocol.APIStyleOpenAI {
		providerAPIBase = virtualURL + "/v1"
	}

	provider := &typ.Provider{
		UUID:               providerName,
		Name:               providerName,
		APIBase:            providerAPIBase,
		APIStyle:           apiStyle,
		OpenAIEndpointMode: targetToOpenAIEndpointMode(target),
		Token:              "virtual-token",
		Enabled:            true,
		Timeout:            int64(constant.DefaultRequestTimeout),
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
	if flags != nil {
		rule.Flags = *flags
	}
	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

	env.mu.Lock()
	env.routeModels[key] = requestModel
	env.mu.Unlock()
}

// SetupRouteWithFlags wires a route exactly like SetupRoute but stamps rule.Flags
// onto the gateway rule, so a request routed through it exercises the real
// flag-resolution and transform pipeline. Returns the request model to send to.
func (env *TestEnv) SetupRouteWithFlags(source, target protocol.APIType, s Scenario, flags typ.RuleFlags) string {
	env.setupRouteCore(source, target, s, &flags)
	return env.findRouteModel(source, target, s.Name)
}

// SendAs sends a request to the gateway as the given source protocol,
// using the request model configured by SetupRoute, and returns the parsed result.
//
// Streaming requests use the real httptest.Server (env.gatewayServer) because
// httptest.ResponseRecorder does not support Gin's streaming/SSE machinery.
// Non-streaming requests use the recorder for simplicity.
func (env *TestEnv) SendAs(t *testing.T, source, target protocol.APIType, s Scenario, streaming bool) *RoundTripResult {
	t.Helper()

	requestModel := env.findRouteModel(source, target, s.Name)
	if requestModel == "" {
		t.Fatalf("no route configured for source=%s target=%s scenario=%s — call SetupRoute first", source, target, s.Name)
	}

	result, err := env.sendModel(source, target, s.Name, requestModel, streaming)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	return result
}

// SendAsCLI sends a request to the gateway as the given source protocol,
// using the request model configured by SetupRoute, and returns the parsed result.
// This version is for CLI use and returns errors instead of calling t.Fatalf.
func (env *TestEnv) SendAsCLI(source, target protocol.APIType, s Scenario, streaming bool) (*RoundTripResult, error) {
	requestModel := env.findRouteModel(source, target, s.Name)
	if requestModel == "" {
		return nil, fmt.Errorf("no route configured for source=%s target=%s scenario=%s — call SetupRoute first", source, target, s.Name)
	}

	return env.sendModel(source, target, s.Name, requestModel, streaming)
}

// sendModel sends a request to the gateway as the given source protocol using
// an explicit request model, and parses the response into a RoundTripResult.
// It is the shared core behind SendAs / SendAsCLI and the idempotency harness,
// which needs to drive different models (baseline vs. round-trip) through the
// same gateway. The target/scenarioName arguments are response metadata only.
//
// Streaming requests use the real httptest.Server (env.gatewayServer) because
// httptest.ResponseRecorder does not support Gin's streaming/SSE machinery.
// Non-streaming requests use the recorder for simplicity.
func (env *TestEnv) sendModel(source, target protocol.APIType, scenarioName, requestModel string, streaming bool) (*RoundTripResult, error) {
	path, body := buildRequest(source, requestModel, streaming)
	return env.dispatch(source, target, scenarioName, path, body, nil, streaming)
}

// dispatch drives a single request through the gateway and parses the result.
// It is the shared core behind sendModel and the flag-behavior sender, so both
// exercise the identical gateway path; extraHeaders are applied on top of the
// default Content-Type / Authorization headers (nil for none).
func (env *TestEnv) dispatch(source, target protocol.APIType, scenarioName, path string, body []byte, extraHeaders map[string]string, streaming bool) (*RoundTripResult, error) {
	result := &RoundTripResult{
		SourceProtocol: source,
		TargetProtocol: target,
		ScenarioName:   scenarioName,
		IsStreaming:    streaming,
	}

	setHeaders := func(h http.Header) {
		h.Set("Content-Type", "application/json")
		h.Set("Authorization", "Bearer "+env.modelToken)
		for k, v := range extraHeaders {
			h.Set(k, v)
		}
	}

	if streaming {
		url := env.gatewayServer.URL + path
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		setHeaders(req.Header)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("do streaming request: %w", err)
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
			return nil, fmt.Errorf("new request: %w", err)
		}
		setHeaders(req.Header)

		w := httptest.NewRecorder()
		env.ginEngine.ServeHTTP(w, req)

		result.HTTPStatus = w.Code
		result.RawBody = w.Body.Bytes()
		parsed := parseFromJSON(result.RawBody, sourceToStyle(source))
		fillFromParsedResult(result, parsed, sourceToStyle(source), false)
	}

	return result, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func routeKey(source, target protocol.APIType, scenario string) string {
	return fmt.Sprintf("%s|%s|%s", source, target, scenario)
}

func (env *TestEnv) findRouteModel(source, target protocol.APIType, scenarioName string) string {
	env.mu.Lock()
	defer env.mu.Unlock()
	key := routeKey(source, target, scenarioName)
	return env.routeModels[key]
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
	case protocol.TypeOpenAIResponses:
		return protocol.APIStyleOpenAI // Responses API uses OpenAI style
	default:
		return protocol.APIStyleOpenAI
	}
}

// targetToOpenAIEndpointMode tells the gateway which OpenAI endpoint a provider
// exposes. Without this, ResolveOpenAIEndpoint falls back to chat for every
// OpenAI-style provider, so a target=openai_responses route would silently
// forward to /chat/completions instead of /responses. chat and responses are
// two distinct protocols; this makes the harness route to the right one.
// Non-OpenAI targets return the zero value (ignored for Anthropic/Google).
func targetToOpenAIEndpointMode(target protocol.APIType) ai.OpenAIEndpointMode {
	switch target {
	case protocol.TypeOpenAIResponses:
		return ai.EndpointModeResponses
	case protocol.TypeOpenAIChat:
		return ai.EndpointModeChat
	default:
		return ai.EndpointModeUnknown
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
	case protocol.TypeOpenAIResponses:
		return protocol.APIStyleOpenAI // Responses API uses OpenAI style
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
		// Check if this is a Responses API response by looking for "output" field
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
		// Try to assemble as Responses API first
		r = sse.AssembleOpenAIResponsesStream(events)
		// If that failed, try Chat Completions
		if r == nil || len(r.Content) == 0 {
			r = sse.AssembleOpenAIStream(events)
		}
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
