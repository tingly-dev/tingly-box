package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tingly-box/internal/config"
	"tingly-box/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
)

// TestServer represents a test server wrapper
type TestServer struct {
	appConfig *config.AppConfig
	server    *server.Server
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
	for _, p := range appConfig.GetGlobalConfig().ListProviders() {
		p.UUID = p.Name
	}

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
	httpServer := server.NewServer(appConfig.GetGlobalConfig())

	return &TestServer{
		appConfig: appConfig,
		server:    httpServer,
		ginEngine: httpServer.GetRouter(), // Use the server's router
	}
}

// NewTestServerWithAdaptor creates a new test server with adaptor flag
func NewTestServerWithAdaptor(t *testing.T, enableAdaptor bool) *TestServer {
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
	httpServer := server.NewServer(appConfig.GetGlobalConfig(), server.WithAdaptor(enableAdaptor))

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
		provider := &config.Provider{
			UUID:    p.uuid,
			Name:    p.name,
			APIBase: p.apiBase,
			Token:   p.token,
			Enabled: true,
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

// CreateTestProvider creates a test provider configuration
func CreateTestProvider(name, apiBase, token string) *config.Provider {
	return &config.Provider{
		Name:    name,
		APIBase: apiBase,
		Token:   token,
		Enabled: true,
	}
}

// CaptureRequest captures HTTP request details
func CaptureRequest(handler gin.HandlerFunc) (*http.Request, map[string]interface{}, error) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create a test request
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":    "test-model",
		"messages": []map[string]string{{"role": "user", "content": "test"}},
	})

	req, _ := http.NewRequest("POST", "/test", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler(c)

	var requestData map[string]interface{}
	json.NewDecoder(c.Request.Body).Decode(&requestData)

	return req, requestData, nil
}

// AddTestProvider adds a single test provider
func (ts *TestServer) AddTestProvider(t *testing.T, name, apiBase, apiStyle string, enabled bool) {
	provider := &config.Provider{
		UUID:     name, // for test, use name as uuid for convenience
		Name:     name,
		APIBase:  apiBase,
		APIStyle: config.APIStyle(apiStyle),
		Token:    "test-token",
		Enabled:  enabled,
	}
	if err := ts.appConfig.AddProvider(provider); err != nil {
		t.Fatalf("Failed to add provider %s: %v", name, err)
	}
}

// AddTestProviderWithURL adds a provider with a specific URL
func (ts *TestServer) AddTestProviderWithURL(t *testing.T, name, url, apiStyle string, enabled bool) {
	provider := &config.Provider{
		UUID:     name, // use name as uuid for convenience
		Name:     name,
		APIBase:  url,
		APIStyle: config.APIStyle(apiStyle),
		Token:    "test-token",
		Enabled:  enabled,
	}
	if err := ts.appConfig.AddProvider(provider); err != nil {
		t.Fatalf("Failed to add provider %s: %v", name, err)
	}
}

// AddTestRule adds a test rule that routes to a specific provider
func (ts *TestServer) AddTestRule(t *testing.T, requestModel, providerName, model string) {
	// Create a simple rule with proper LBTactic
	rule := config.Rule{
		UUID:          requestModel,
		RequestModel:  requestModel,
		ResponseModel: model,
		Services: []config.Service{
			{
				Provider:   providerName,
				Model:      model,
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		LBTactic: config.Tactic{
			Type:   config.TacticRoundRobin,
			Params: config.DefaultRoundRobinParams(),
		},
		Active: true,
	}

	if err := ts.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("Failed to add rule %s: %v", requestModel, err)
	}
}

// NewTestServerWithAdaptorFromConfig creates a new test server with adaptor flag using existing app config
func NewTestServerWithAdaptorFromConfig(t *testing.T, appConfig *config.AppConfig, enableAdaptor bool) *TestServer {
	// Create server instance with adaptor flag
	httpServer := server.NewServer(appConfig.GetGlobalConfig(), server.WithAdaptor(enableAdaptor))

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
	realConfigDir := config.GetTinglyConfDir()

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
