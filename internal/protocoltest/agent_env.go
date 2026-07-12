package protocoltest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// VirtualMockAnswerMarker is a substring guaranteed to appear in the default
// virtual upstream's answer (the shared TextScenario's fixed reply, "The
// capital of France is Paris."). Agent-CLI callers use it to verify that the
// gateway round trip's content actually reached the CLI's output — a zero
// exit code alone can mask a CLI that printed an error and exited cleanly.
const VirtualMockAnswerMarker = "Paris"

// AgentType represents the type of agent Agent to test
type AgentType string

const (
	AgentTypeClaudeCode AgentType = "claude"
	AgentTypeCodex      AgentType = "codex"
	AgentTypeOpenCode   AgentType = "opencode"
)

// String returns the string representation of AgentType
func (pt AgentType) String() string {
	return string(pt)
}

// Scenario returns the corresponding RuleScenario for this Agent
func (pt AgentType) Scenario() typ.RuleScenario {
	switch pt {
	case AgentTypeClaudeCode:
		return typ.ScenarioClaudeCode
	case AgentTypeCodex:
		return typ.ScenarioCodex
	case AgentTypeOpenCode:
		return typ.ScenarioOpenCode
	default:
		return ""
	}
}

// AgentTestResult represents the result of a single Agent test
type AgentTestResult struct {
	// Name is the test name
	Name string

	// Agent is the Agent type being tested
	Agent AgentType

	// Scenario is the test scenario (e.g., "text", "streaming", "tool_use")
	Scenario string

	// Passed indicates whether the test passed
	Passed bool

	// Skipped indicates whether the test was skipped
	Skipped bool

	// SkipReason explains why the test was skipped
	SkipReason string

	// Errors contains any assertion errors
	Errors []AssertionError

	// Duration is how long the test took
	Duration int64 // milliseconds

	// HTTPStatus is the HTTP status code received
	HTTPStatus int

	// RequestHeaders contains the request headers sent to the virtual server
	RequestHeaders http.Header

	// RequestBody contains the request body sent to the virtual server
	RequestBody []byte

	// ResponseBody contains the raw response body
	ResponseBody []byte
}

// AgentTestEnv provides an isolated test environment for Agent testing
// It includes:
// - A temporary config directory
// - A gateway server with virtual provider
// - Routing rules configured for the Agent
// - A virtual server that captures requests for validation
type AgentTestEnv struct {
	// configDir is the temporary configuration directory
	configDir string

	// appConfig is the application configuration
	appConfig *config.AppConfig

	// gatewayServer is the HTTP test server for the gateway
	gatewayServer *httptest.Server

	// virtualServer is the mock provider server
	virtualServer *VirtualServer

	// baseURL is the base URL for the gateway
	baseURL string

	// modelToken is the API token for requests
	modelToken string

	// capturedRequests contains requests captured by the virtual server
	capturedRequests []*CapturedRequest

	// closed indicates whether the environment has been closed
	closed bool
}

// NewAgentTestEnv creates a new Agent test environment
// The environment is isolated with a temporary config directory
// and must be cleaned up with Close() when done
func NewAgentTestEnv(AgentType AgentType) (*AgentTestEnv, error) {
	core, err := newGatewayCore("harness-agent-*", nil)
	if err != nil {
		return nil, err
	}

	// Pre-register the shared text scenario as the virtual server's only
	// mock. Agent CLIs send their built-in request model (not a
	// scenario-encoded one), so the responder serves its fallback scenario —
	// keeping exactly one registered makes the fallback deterministic, and
	// its fixed answer (VirtualMockAnswerMarker) lets callers assert the
	// round trip's content in the CLI's output. Replay registers additional
	// scenarios on demand (SetupVirtualAgentScenario).
	core.virtual.RegisterScenario(TextScenario())

	return &AgentTestEnv{
		configDir:        core.configDir,
		appConfig:        core.appConfig,
		gatewayServer:    core.gateway,
		virtualServer:    core.virtual,
		baseURL:          core.gateway.URL,
		modelToken:       core.modelToken,
		capturedRequests: make([]*CapturedRequest, 0),
		closed:           false,
	}, nil
}

// Close cleans up the test environment
// If preserve is true, the config directory is kept for inspection
func (env *AgentTestEnv) Close(preserve bool) error {
	if env.closed {
		return nil
	}

	// Close gateway server
	if env.gatewayServer != nil {
		env.gatewayServer.Close()
	}

	// Close virtual server
	if env.virtualServer != nil {
		env.virtualServer.Close()
	}

	// Release the config's database handles (see TestEnv.Close for why)
	if env.appConfig != nil {
		_ = env.appConfig.GetGlobalConfig().CloseStores()
	}

	// Clean up config directory unless preserved
	if !preserve && env.configDir != "" {
		if err := os.RemoveAll(env.configDir); err != nil {
			return fmt.Errorf("remove config dir: %w", err)
		}
	}

	env.closed = true
	return nil
}

// ConfigDir returns the temporary config directory path
func (env *AgentTestEnv) ConfigDir() string {
	return env.configDir
}

// BaseURL returns the base URL of the gateway server
func (env *AgentTestEnv) BaseURL() string {
	return env.baseURL
}

// ModelToken returns the model token for requests
func (env *AgentTestEnv) ModelToken() string {
	return env.modelToken
}

// VirtualServerURL returns the URL of the virtual server
func (env *AgentTestEnv) VirtualServerURL() string {
	if env.virtualServer == nil {
		return ""
	}
	return env.virtualServer.URL()
}

// SetupAgent configures the environment for a specific Agent type
// This creates the necessary provider and routing rules
func (env *AgentTestEnv) SetupAgent(AgentType AgentType, providerName string, modelName string) error {
	virtualURL := env.VirtualServerURL()
	if virtualURL == "" {
		return fmt.Errorf("virtual server not initialized")
	}

	// Create provider pointing to virtual server
	provider := &typ.Provider{
		UUID:     providerName,
		Name:     providerName,
		APIBase:  virtualURL,
		APIStyle: "openai", // Default, will be adjusted per Agent
		Token:    "test-virtual-token",
		Enabled:  true,
		Timeout:  30000,
	}

	// Adjust API style based on Agent type
	switch AgentType {
	case AgentTypeClaudeCode:
		provider.APIStyle = "anthropic"
	case AgentTypeCodex:
		provider.APIStyle = "openai"
	case AgentTypeOpenCode:
		provider.APIStyle = "anthropic"
	}

	// Add provider to config
	if err := env.appConfig.AddProvider(provider); err != nil {
		return fmt.Errorf("add provider: %w", err)
	}

	// Built-in rules are seeded with empty services; repoint injects the
	// virtual-server service.
	return env.repointBuiltinRule(AgentType, providerName, modelName)
}

// SetupRealAgent configures the environment to route through a real upstream provider.
// Unlike SetupAgent, it does not use the virtual server — the provider points at the
// real apiBase with the real apiKey. apiStyle must be "openai" or "anthropic".
// apiType is optional and specifies the target API type (e.g., "anthropic_v1", "openai_chat").
// If empty, a default is chosen based on apiStyle.
func (env *AgentTestEnv) SetupRealAgent(AgentType AgentType, providerName string, modelName string, apiBase string, apiKey string, apiStyle string) error {
	provider := &typ.Provider{
		UUID:     providerName,
		Name:     providerName,
		APIBase:  apiBase,
		APIStyle: protocol.APIStyle(apiStyle),
		Token:    apiKey,
		Enabled:  true,
		Timeout:  60000,
	}

	if err := env.appConfig.AddProvider(provider); err != nil {
		return fmt.Errorf("add provider: %w", err)
	}

	return env.repointBuiltinRule(AgentType, providerName, modelName)
}

// SetupVModelAgent configures the environment so the agent's built-in rule
// routes to a seeded builtin virtual-model provider.
//
// Unlike SetupAgent (external VirtualServer mock) and SetupRealAgent (real
// upstream), this exercises the in-process vmodel dispatch path:
//
//	gateway → built-in-<agent> rule → vmodel builtin provider
//	        → provider.IsVirtual() short-circuit → in-process vmodel handler
//
// The builtin vmodel providers are seeded into the provider store by
// server.NewServer, so no provider is added here — only the rule is repointed.
//
// vmodelID must be a model registered in the vmodel registry for the agent's
// protocol (e.g. "virtual-claude-3", "echo-model" for Anthropic-style agents).
func (env *AgentTestEnv) SetupVModelAgent(AgentType AgentType, vmodelID string) error {
	var providerUUID string
	switch AgentType {
	case AgentTypeClaudeCode, AgentTypeOpenCode:
		providerUUID = virtualserver.BuiltinAnthropicUUID
	case AgentTypeCodex:
		providerUUID = virtualserver.BuiltinOpenAIUUID
	default:
		return fmt.Errorf("unknown Agent type: %s", AgentType)
	}

	if _, err := env.appConfig.GetGlobalConfig().GetProviderByUUID(providerUUID); err != nil {
		return fmt.Errorf("builtin vmodel provider %q not seeded: %w", providerUUID, err)
	}

	return env.repointBuiltinRule(AgentType, providerUUID, vmodelID)
}

// AppConfig returns the application configuration
func (env *AgentTestEnv) AppConfig() *serverconfig.Config {
	return env.appConfig.GetGlobalConfig()
}
