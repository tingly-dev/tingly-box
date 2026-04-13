package protocol_validate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server_validate"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProfileType represents the type of agent profile to test
type ProfileType string

const (
	ProfileTypeClaudeCode ProfileType = "claude"
	ProfileTypeCodex      ProfileType = "codex"
	ProfileTypeOpenCode   ProfileType = "opencode"
)

// String returns the string representation of ProfileType
func (pt ProfileType) String() string {
	return string(pt)
}

// Scenario returns the corresponding RuleScenario for this profile
func (pt ProfileType) Scenario() typ.RuleScenario {
	switch pt {
	case ProfileTypeClaudeCode:
		return typ.ScenarioClaudeCode
	case ProfileTypeCodex:
		return typ.ScenarioCodex
	case ProfileTypeOpenCode:
		return typ.ScenarioOpenCode
	default:
		return ""
	}
}

// ProfileTestResult represents the result of a single profile test
type ProfileTestResult struct {
	// Name is the test name
	Name string

	// Profile is the profile type being tested
	Profile ProfileType

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

// ProfileTestEnv provides an isolated test environment for profile testing
// It includes:
// - A temporary config directory
// - A gateway server with virtual provider
// - Routing rules configured for the profile
// - A virtual server that captures requests for validation
type ProfileTestEnv struct {
	// configDir is the temporary configuration directory
	configDir string

	// appConfig is the application configuration
	appConfig *config.AppConfig

	// gatewayServer is the HTTP test server for the gateway
	gatewayServer *httptest.Server

	// virtualServer is the mock provider server
	virtualServer *server_validate.VirtualServer

	// baseURL is the base URL for the gateway
	baseURL string

	// modelToken is the API token for requests
	modelToken string

	// capturedRequests contains requests captured by the virtual server
	capturedRequests []*CapturedRequest

	// closed indicates whether the environment has been closed
	closed bool
}

// CapturedRequest represents a request captured by the virtual server
type CapturedRequest struct {
	// Headers contains the request headers
	Headers http.Header

	// Body contains the request body
	Body []byte

	// Method is the HTTP method
	Method string

	// Path is the request path
	Path string
}

// NewProfileTestEnv creates a new profile test environment
// The environment is isolated with a temporary config directory
// and must be cleaned up with Close() when done
func NewProfileTestEnv(profileType ProfileType) (*ProfileTestEnv, error) {
	// Create temporary config directory
	configDir, err := os.MkdirTemp("", "harness-profile-*")
	if err != nil {
		return nil, fmt.Errorf("create temp config dir: %w", err)
	}

	// Create app config
	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		os.RemoveAll(configDir)
		return nil, fmt.Errorf("create app config: %w", err)
	}

	// Start virtual server (mock provider)
	virtualServer := server_validate.NewVirtualServerForCLI()

	// Create gateway server with real routing
	gatewayServer := server.NewServer(appConfig.GetGlobalConfig(), server.WithAdaptor(false))
	router := gatewayServer.GetRouter()
	ts := httptest.NewServer(router)

	return &ProfileTestEnv{
		configDir:        configDir,
		appConfig:        appConfig,
		gatewayServer:    ts,
		virtualServer:    virtualServer,
		baseURL:          ts.URL,
		modelToken:       appConfig.GetGlobalConfig().GetModelToken(),
		capturedRequests: make([]*CapturedRequest, 0),
		closed:           false,
	}, nil
}

// Close cleans up the test environment
// If preserve is true, the config directory is kept for inspection
func (env *ProfileTestEnv) Close(preserve bool) error {
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
func (env *ProfileTestEnv) ConfigDir() string {
	return env.configDir
}

// BaseURL returns the base URL of the gateway server
func (env *ProfileTestEnv) BaseURL() string {
	return env.baseURL
}

// ModelToken returns the model token for requests
func (env *ProfileTestEnv) ModelToken() string {
	return env.modelToken
}

// VirtualServerURL returns the URL of the virtual server
func (env *ProfileTestEnv) VirtualServerURL() string {
	if env.virtualServer == nil {
		return ""
	}
	return env.virtualServer.URL()
}

// SetupProfile configures the environment for a specific profile type
// This creates the necessary provider and routing rules
func (env *ProfileTestEnv) SetupProfile(profileType ProfileType, providerName string, modelName string) error {
	virtualURL := env.VirtualServerURL()
	if virtualURL == "" {
		return fmt.Errorf("virtual server not initialized")
	}

	// Create provider pointing to virtual server
	provider := &typ.Provider{
		UUID:     providerName,
		Name:     providerName,
		APIBase:  virtualURL,
		APIStyle: "openai", // Default, will be adjusted per profile
		Token:    "test-virtual-token",
		Enabled:  true,
		Timeout:  30000,
	}

	// Adjust API style based on profile type
	switch profileType {
	case ProfileTypeClaudeCode:
		provider.APIStyle = "anthropic"
	case ProfileTypeCodex:
		provider.APIStyle = "openai"
	case ProfileTypeOpenCode:
		provider.APIStyle = "anthropic"
	}

	// Add provider to config
	if err := env.appConfig.AddProvider(provider); err != nil {
		return fmt.Errorf("add provider: %w", err)
	}

	// Find the existing built-in rule and update it with our test service.
	// Built-in rules are initialized with empty services; we inject the virtual server service.
	scenario := profileType.Scenario()

	// Resolve the built-in rule UUID and its request model
	var builtinUUID string
	var requestModel string
	switch profileType {
	case ProfileTypeClaudeCode:
		builtinUUID = "built-in-cc"
		requestModel = "tingly/cc"
	case ProfileTypeCodex:
		builtinUUID = "built-in-codex"
		requestModel = "tingly-codex"
	case ProfileTypeOpenCode:
		builtinUUID = "built-in-opencode"
		requestModel = "tingly-opencode"
	default:
		return fmt.Errorf("unknown profile type: %s", profileType)
	}

	rule := typ.Rule{
		UUID:          builtinUUID,
		Scenario:      scenario,
		RequestModel:  requestModel,
		ResponseModel: modelName,
		Services: []*loadbalance.Service{
			{
				Provider: providerName,
				Model:    modelName,
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

// AppConfig returns the application configuration
func (env *ProfileTestEnv) AppConfig() *serverconfig.Config {
	return env.appConfig.GetGlobalConfig()
}
