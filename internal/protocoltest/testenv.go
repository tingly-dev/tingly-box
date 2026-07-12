package protocoltest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
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
	appConfig     *config.AppConfig
	gatewayServer *httptest.Server // real HTTP server; every request traverses it
	virtual       *VirtualServer
	modelToken    string
	client        Client // driver used by sendModel (default: raw HTTP)

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
	client     Client
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

// NewTestEnvOptionWithClient creates an option to set the client driver used
// for sending requests through the gateway. Defaults to the raw HTTP client.
func NewTestEnvOptionWithClient(c Client) TestEnvOption {
	return func(cfg *testEnvConfig) {
		cfg.client = c
	}
}

// gatewayCore is the shared skeleton every single-process harness env builds
// on: a temp config dir, an app config, a real gateway httptest.Server, and a
// VirtualServer mock provider. TestEnv (matrix/flags) and AgentTestEnv
// (replay/agent) both assemble from it, so the boot sequence exists once.
type gatewayCore struct {
	configDir  string
	appConfig  *config.AppConfig
	gateway    *httptest.Server
	virtual    *VirtualServer
	modelToken string
}

// newGatewayCore boots the skeleton. configure (optional) runs after the app
// config exists but before the server is created, for settings the server
// reads at construction time (e.g. the MCP extension flag).
func newGatewayCore(dirPattern string, configure func(*config.AppConfig), serverOpts ...server.ServerOption) (*gatewayCore, error) {
	configDir, err := os.MkdirTemp("", dirPattern)
	if err != nil {
		return nil, fmt.Errorf("create temp config dir: %w", err)
	}

	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		os.RemoveAll(configDir)
		return nil, fmt.Errorf("create app config: %w", err)
	}
	if configure != nil {
		configure(appConfig)
	}

	gatewayServer := server.NewServer(appConfig.GetGlobalConfig(), serverOpts...)
	ts := httptest.NewServer(gatewayServer.GetRouter())

	return &gatewayCore{
		configDir:  configDir,
		appConfig:  appConfig,
		gateway:    ts,
		virtual:    NewVirtualServerForCLI(),
		modelToken: appConfig.GetGlobalConfig().GetModelToken(),
	}, nil
}

// NewTestEnv creates a TestEnv with a fresh gateway config and a new
// VirtualServer, cleaned up via t.Cleanup. It is the testing.T wrapper over
// NewTestEnvForCLI — one construction path for both entry points.
func NewTestEnv(t *testing.T, opts ...TestEnvOption) *TestEnv {
	t.Helper()

	env, err := NewTestEnvForCLI(opts...)
	if err != nil {
		t.Fatalf("create test env: %v", err)
	}
	t.Cleanup(env.Close)
	return env
}

// clientOrDefault returns the configured client or the raw HTTP default.
func clientOrDefault(c Client) Client {
	if c != nil {
		return c
	}
	return NewHTTPClient()
}

// Close shuts down the gateway and virtual servers, releases the config's
// database handles, and removes the config directory. Closing the stores
// matters: e2e suites create hundreds of envs in one process, and an
// unclosed SQLite handle per env exhausts the fd limit. Safe to call more
// than once (t.Cleanup plus an explicit defer).
func (env *TestEnv) Close() {
	if env.gatewayServer != nil {
		env.gatewayServer.Close()
	}
	if env.virtual != nil {
		env.virtual.Close()
	}
	if env.appConfig != nil {
		_ = env.appConfig.GetGlobalConfig().CloseStores()
	}
	if env.configDir != "" {
		os.RemoveAll(env.configDir)
	}
}

// NewTestEnvForCLI creates a TestEnv without a testing.T.
// Resources must be cleaned up via an explicit Close() call.
func NewTestEnvForCLI(opts ...TestEnvOption) (*TestEnv, error) {
	cfg := &testEnvConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var serverOpts []server.ServerOption
	if cfg.recordDir != "" {
		serverOpts = append(serverOpts, server.WithRecordDir(cfg.recordDir))
	}

	core, err := newGatewayCore("pv-env-*", func(ac *config.AppConfig) {
		if cfg.mcpEnabled {
			_ = ac.GetGlobalConfig().SetScenarioFlag(typ.ScenarioGlobal, serverconfig.ExtensionMCP, true)
		}
	}, serverOpts...)
	if err != nil {
		return nil, err
	}

	return &TestEnv{
		appConfig:     core.appConfig,
		gatewayServer: core.gateway,
		virtual:       core.virtual,
		modelToken:    core.modelToken,
		client:        clientOrDefault(cfg.client),
		routeModels:   make(map[string]string),
		setupRoutes:   make(map[string]bool),
		configDir:     core.configDir,
	}, nil
}

// GatewayURL returns the base URL of the real gateway HTTP server, for client
// drivers that speak real HTTP (SDKs, subprocess drivers).
func (env *TestEnv) GatewayURL() string { return env.gatewayServer.URL }

// ModelToken returns the gateway model token client drivers authenticate with.
func (env *TestEnv) ModelToken() string { return env.modelToken }

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

	rule := newHarnessRule(requestModel, sourceToRuleScenario(source), requestModel, providerModel,
		harnessService(providerName, providerModel))
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
	return env.client.Send(env, SendSpec{
		Source:       source,
		Target:       target,
		ScenarioName: scenarioName,
		RequestModel: requestModel,
		Streaming:    streaming,
		GatewayURL:   env.gatewayServer.URL,
		APIKey:       env.modelToken,
	})
}

// dispatch drives a single request through the gateway and parses the result.
// It is the shared core behind sendModel and the flag-behavior sender, so both
// exercise the identical gateway path; extraHeaders are applied on top of the
// default Content-Type / Authorization headers (nil for none).
//
// Both modes go over the real httptest.Server: an in-memory recorder would
// skip the HTTP transport (and its request contexts, write paths, timeouts),
// which has hidden real bugs — the same "diagnostics must traverse the real
// path" rule the debug module follows.
func (env *TestEnv) dispatch(source, target protocol.APIType, scenarioName, path string, body []byte, extraHeaders map[string]string, streaming bool) (*RoundTripResult, error) {
	result := &RoundTripResult{
		SourceProtocol: source,
		TargetProtocol: target,
		ScenarioName:   scenarioName,
		IsStreaming:    streaming,
	}

	req, err := http.NewRequest("POST", env.gatewayServer.URL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.modelToken)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode
	var parsed sse.ParsedResult
	if streaming {
		result.StreamEvents, result.RawBody = sse.ReadSSELines(resp.Body)
		parsed = assembleFromEvents(result.StreamEvents, sourceToStyle(source))
	} else {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
		result.RawBody = raw
		parsed = parseFromJSON(raw, sourceToStyle(source))
	}
	fillFromParsedResult(result, parsed)

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

func fillFromParsedResult(result *RoundTripResult, parsed sse.ParsedResult) {
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
