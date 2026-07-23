package servertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"
	server2 "github.com/tingly-dev/tingly-box/internal/server"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

const defaultMockProviderTimeoutSeconds = int64(2)

// TestServer represents a test server wrapper
type TestServer struct {
	appConfig *config.AppConfig
	server    *server2.Server
	ginEngine *gin.Engine
}

// NewTestServer creates a new test server
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("Failed to create app config: %v", err)
	}

	return createTestServer(t, appConfig)
}

// createTestServer creates a test server with the given appConfig
func createTestServer(t *testing.T, appConfig *config.AppConfig) *TestServer {
	t.Helper()

	// Create server instance but don't start it
	httpServer := server2.NewServer(appConfig.GetGlobalConfig())

	return &TestServer{
		appConfig: appConfig,
		server:    httpServer,
		ginEngine: httpServer.GetRouter(), // Use the server's router
	}
}

// AddTestProviders adds test providers to the configuration
func (ts *TestServer) AddTestProviders(t *testing.T) {
	t.Helper()

	upstream := NewMockProviderServer()
	t.Cleanup(upstream.Close)

	providers := []struct {
		uuid  string
		name  string
		token string
	}{
		{"openai", "openai", "sk-test-openai"},
		{"alibaba", "alibaba", "sk-test-alibaba"},
		{"anthropic", "anthropic", "sk-test-anthropic"},
		{"glm", "glm", "sk-test-glm"},
	}

	for _, p := range providers {
		provider := &typ.Provider{
			UUID:    p.uuid,
			Name:    p.name,
			APIBase: upstream.GetURL(),
			Token:   p.token,
			Enabled: true,
			Timeout: defaultMockProviderTimeoutSeconds,
		}
		if err := ts.appConfig.AddProvider(provider); err != nil {
			t.Fatalf("Failed to add provider %s: %v", p.name, err)
		}
	}
}

// CreateTestChatRequest creates a test chat completion request
func CreateTestChatRequest(model string, messages []map[string]string) map[string]interface{} {
	return map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}
}

// CreateJSONBody creates a JSON body for HTTP requests
func CreateJSONBody(data interface{}) *bytes.Buffer {
	jsonData, _ := json.Marshal(data)
	return bytes.NewBuffer(jsonData)
}

// EnsureLoadBalancingRule creates a multi-service, randomly load-balanced rule
// for the given request model + scenario if one does not already exist. The
// mock test setup adds providers but no rule for the "tingly" model, so requests
// would otherwise 404.
func (ts *TestServer) EnsureLoadBalancingRule(t *testing.T, requestModel, model string, scenario typ.RuleScenario, providers ...string) {
	t.Helper()

	gc := ts.appConfig.GetGlobalConfig()
	if gc.GetRuleByRequestModelAndScenario(requestModel, scenario) != nil {
		return
	}

	services := make([]*loadbalance.Service, 0, len(providers))
	for _, p := range providers {
		services = append(services, &loadbalance.Service{
			Provider:   p,
			Model:      model,
			Weight:     1,
			Active:     true,
			TimeWindow: 300,
		})
	}

	rule := typ.Rule{
		Scenario:     scenario,
		RequestModel: requestModel,
		UUID:         fmt.Sprintf("%s-%s", requestModel, scenario),
		Services:     services,
		LBTactic:     typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.NewRandomParams()},
		Active:       true,
	}

	if err := gc.AddOrUpdateRequestConfigByRequestModel(rule); err != nil {
		t.Fatalf("Failed to add load balancing rule %s: %v", requestModel, err)
	}
}

// NewTestServerFromConfig creates a new test server sharing an existing app config
func NewTestServerFromConfig(appConfig *config.AppConfig) *TestServer {
	httpServer := server2.NewServer(appConfig.GetGlobalConfig())

	return &TestServer{
		appConfig: appConfig,
		server:    httpServer,
		ginEngine: httpServer.GetRouter(), // Use the server's router
	}
}

// containsStatus checks if status code is in expected list
func containsStatus(actual int, expected []int) bool {
	for _, code := range expected {
		if actual == code {
			return true
		}
	}
	return false
}
