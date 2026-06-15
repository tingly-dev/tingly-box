package servertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	server2 "github.com/tingly-dev/tingly-box/internal/server"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// TestServer represents a test server wrapper
type TestServer struct {
	appConfig *config.AppConfig
	server    *server2.Server
	ginEngine *gin.Engine
}

// NewTestServer creates a new test server with custom config directory
func NewTestServerWithConfigDir(t *testing.T, configDir string) *TestServer {
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

	appConfig.Save()

	return createTestServer(t, appConfig)
}

// NewTestServer creates a new test server
func NewTestServer(t *testing.T) *TestServer {
	// Create temp config directory
	configDir, err := os.MkdirTemp("", "tingly-box-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp config directory: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		os.RemoveAll(configDir)
	})

	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		t.Fatalf("Failed to create app config: %v", err)
	}

	return createTestServer(t, appConfig)
}

// createTestServer creates a test server with the given appConfig
func createTestServer(t *testing.T, appConfig *config.AppConfig) *TestServer {
	// Create server instance but don't start it
	// Note: adapter is disabled by default in tests to test the fallback behavior
	httpServer := server2.NewServer(appConfig.GetGlobalConfig(), server2.WithAdaptor(false))

	return &TestServer{
		appConfig: appConfig,
		server:    httpServer,
		ginEngine: httpServer.GetRouter(), // Use the server's router
	}
}

// NewTestServerWithAdaptor creates a new test server with adaptor flag
func NewTestServerWithAdaptor(t *testing.T) *TestServer {
	// Create temp config directory
	configDir, err := os.MkdirTemp("", "tingly-box-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp config directory: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		os.RemoveAll(configDir)
	})

	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		t.Fatalf("Failed to create app config: %v", err)
	}

	// Create server instance with adaptor flag
	httpServer := server2.NewServer(appConfig.GetGlobalConfig())

	return &TestServer{
		appConfig: appConfig,
		server:    httpServer,
		ginEngine: httpServer.GetRouter(), // Use the server's router
	}
}

// AddTestProviders adds test providers to the configuration
func (ts *TestServer) AddTestProviders(t *testing.T) {
	providers := []struct {
		uuid    string
		name    string
		apiBase string
		token   string
	}{
		{"openai", "openai", "https://api.openai.com/v1", "sk-test-openai"},
		{"alibaba", "alibaba", "https://dashscope.aliyuncs.com/compatible-mode/v1", "sk-test-alibaba"},
		{"anthropic", "anthropic", "https://api.anthropic.com", "sk-test-anthropic"},
		{"glm", "glm", "https://open.bigmodel.cn/api/paas/v4", "sk-test-glm"},
	}

	for _, p := range providers {
		provider := &typ.Provider{
			UUID:    p.uuid,
			Name:    p.name,
			APIBase: p.apiBase,
			Token:   p.token,
			Enabled: true,
			Timeout: int64(constant.DefaultRequestTimeout),
		}
		if err := ts.appConfig.AddProvider(provider); err != nil {
			t.Fatalf("Failed to add provider %s: %v", p.name, err)
		}
	}
}

// GetProviderToken returns the appropriate token for Anthropic API requests
func (ts *TestServer) GetProviderToken(uid string, isRealConfig bool) string {
	if isRealConfig {
		// Use Anthropic provider token for real config
		provider, err := ts.appConfig.GetProviderByUUID(uid)
		if err == nil {
			return provider.Token
		}
	}
	// Use global model token for mock config
	globalConfig := ts.appConfig.GetGlobalConfig()
	return globalConfig.GetModelToken()
}

// CreateTestChatRequest creates a test chat completion request
func CreateTestChatRequest(model string, messages []map[string]string) map[string]interface{} {
	return map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}
}

// CreateTestMessage creates a test message
func CreateTestMessage(role, content string) map[string]string {
	return map[string]string{
		"role":    role,
		"content": content,
	}
}

// CreateJSONBody creates a JSON body for HTTP requests
func CreateJSONBody(data interface{}) *bytes.Buffer {
	jsonData, _ := json.Marshal(data)
	return bytes.NewBuffer(jsonData)
}

// AssertJSONResponse asserts that the response is valid JSON and checks specific fields
func AssertJSONResponse(t *testing.T, resp *http.Response, expectedStatus int, checkFields func(map[string]interface{})) {
	assert.Equal(t, expectedStatus, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	assert.NoError(t, err)

	if checkFields != nil {
		checkFields(data)
	}
}

// EnsureLoadBalancingRule creates a multi-service, randomly load-balanced rule
// for the given request model + scenario if one does not already exist. The
// mock test setup adds providers but no rule for the "tingly" model, so requests
// would otherwise 404. The real-config variants already have the rule, so the
// existence check leaves them untouched.
func (ts *TestServer) EnsureLoadBalancingRule(t *testing.T, requestModel, model string, scenario typ.RuleScenario, providers ...string) {
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

// NewTestServerWithAdaptorFromConfig creates a new test server with adaptor flag using existing app config
func NewTestServerWithAdaptorFromConfig(appConfig *config.AppConfig) *TestServer {
	// Create server instance with adaptor flag
	httpServer := server2.NewServer(appConfig.GetGlobalConfig())

	return &TestServer{
		appConfig: appConfig,
		server:    httpServer,
		ginEngine: httpServer.GetRouter(), // Use the server's router
	}
}

// Cleanup removes test files
func Cleanup() {
	os.RemoveAll("tests/.tingly-box")
}

func FindGoModRoot() (string, error) {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

// copyConfigDir copies a config directory, skipping test config subdirectories
func copyConfigDir(src, dst string) error {
	// Use the otiai10/copy library with options to skip test directories
	opts := copy.Options{
		Skip: func(srcinfo os.FileInfo, src string, dest string) (bool, error) {
			// Skip directories containing "-test-" in the path
			if srcinfo.IsDir() && strings.Contains(src, "-test-") {
				return true, nil
			}
			return false, nil
		},
	}
	return copy.Copy(src, dst, opts)
}

// TestConfigDir represents a temporary config directory for testing
type TestConfigDir struct {
	path string
}

// NewTestConfigDirCopy creates a temporary copy of the real config directory
// for testing purposes. It automatically cleans up the temporary directory
// when the test finishes.
func NewTestConfigDirCopy(t *testing.T) *TestConfigDir {
	// Get the real config directory path
	realConfigDir := constant.GetTinglyConfDir()

	// Check if real config directory exists
	if _, err := os.Stat(realConfigDir); os.IsNotExist(err) {
		t.Skipf("Real config directory not found at %s, skipping test", realConfigDir)
	}

	// Create a temporary directory for the test config
	tempDir, err := os.MkdirTemp("", "tingly-box-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Copy the real config to the temp directory
	if err := copyConfigDir(realConfigDir, tempDir); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to copy config directory: %v", err)
	}

	// Register cleanup function to remove the temp directory when test finishes
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return &TestConfigDir{path: tempDir}
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
