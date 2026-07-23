package servertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	server2 "github.com/tingly-dev/tingly-box/internal/server"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	realConfigTestEnv                       = "TINGLY_BOX_TEST_REAL_CONFIG"
	realConfigProviderTimeoutEnv            = "TINGLY_BOX_TEST_REAL_TIMEOUT_SECONDS"
	defaultRealConfigProviderTimeoutSeconds = int64(10)
	defaultMockProviderTimeoutSeconds       = int64(2)
)

// TestServer represents a test server wrapper
type TestServer struct {
	appConfig *config.AppConfig
	server    *server2.Server
	ginEngine *gin.Engine
}

// NewTestServerWithConfigDir creates a test server with a custom config directory.
func NewTestServerWithConfigDir(t *testing.T, configDir string) *TestServer {
	t.Helper()

	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create config directory %s: %v", configDir, err)
	}

	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		t.Fatalf("Failed to create app config: %v", err)
	}

	// use name to set provider uuid for testing
	for idx, p := range appConfig.GetGlobalConfig().ListProviders() {
		p.UUID = fmt.Sprintf("%d", idx)
	}

	if err := appConfig.Save(); err != nil {
		t.Fatalf("Failed to save app config: %v", err)
	}

	return createTestServer(t, appConfig)
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
// would otherwise 404. The real-config variants already have the rule, so the
// existence check leaves them untouched.
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

// TestConfigDir represents a temporary config directory for testing
type TestConfigDir struct {
	path string
}

// NewTestConfigDirCopy creates a bounded snapshot of the real configuration.
// Real-config tests are opt-in because they may contact external providers.
func NewTestConfigDirCopy(t *testing.T) *TestConfigDir {
	t.Helper()

	if os.Getenv(realConfigTestEnv) != "1" {
		t.Skipf("real-config test disabled; set %s=1 to enable", realConfigTestEnv)
	}

	realConfigDir := constant.GetTinglyConfDir()
	if _, err := os.Stat(realConfigDir); os.IsNotExist(err) {
		t.Skipf("Real config directory not found at %s, skipping test", realConfigDir)
	}

	timeoutSeconds, err := parseRealConfigProviderTimeout(os.Getenv(realConfigProviderTimeoutEnv))
	if err != nil {
		t.Fatalf("invalid %s: %v", realConfigProviderTimeoutEnv, err)
	}

	tempDir := t.TempDir()
	if err := snapshotRealConfig(realConfigDir, tempDir, timeoutSeconds); err != nil {
		t.Fatalf("Failed to snapshot real config: %v", err)
	}

	return &TestConfigDir{path: tempDir}
}

func parseRealConfigProviderTimeout(value string) (int64, error) {
	if value == "" {
		return defaultRealConfigProviderTimeoutSeconds, nil
	}

	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("must be a positive integer number of seconds")
	}
	return seconds, nil
}

func snapshotRealConfig(src, dst string, providerTimeoutSeconds int64) error {
	configData, err := os.ReadFile(filepath.Join(src, "config.json"))
	if err != nil {
		return fmt.Errorf("read config.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "config.json"), configData, 0600); err != nil {
		return fmt.Errorf("write config.json: %w", err)
	}

	sourceProviders, err := db.NewProviderStore(src)
	if err != nil {
		return fmt.Errorf("open source providers: %w", err)
	}
	defer sourceProviders.Close()

	providers, err := sourceProviders.List()
	if err != nil {
		return fmt.Errorf("list source providers: %w", err)
	}

	destinationProviders, err := db.NewProviderStore(dst)
	if err != nil {
		return fmt.Errorf("open destination providers: %w", err)
	}
	defer destinationProviders.Close()

	for _, provider := range providers {
		provider.Timeout = providerTimeoutSeconds
		if err := destinationProviders.Save(provider); err != nil {
			return fmt.Errorf("save provider %q: %w", provider.Name, err)
		}
	}
	return nil
}

// Path returns the path to the temporary config directory
func (td *TestConfigDir) Path() string {
	return td.path
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
