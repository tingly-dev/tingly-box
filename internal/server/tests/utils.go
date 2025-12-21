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
	"testing"

	"tingly-box/internal/config"
	"tingly-box/internal/server"

	"github.com/gin-gonic/gin"
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

	appConfig, err := config.NewAppConfigWithDir(configDir)
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
	// Create test config directory
	configDir := ".tingly-box"
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create test config directory: %v", err)
	}

	appConfig, err := config.NewAppConfig()
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
	// Create test config directory
	configDir := ".tingly-box"
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create test config directory: %v", err)
	}

	appConfig, err := config.NewAppConfig()
	if err != nil {
		t.Fatalf("Failed to create app config: %v", err)
	}

	// Create server instance with adaptor flag
	httpServer := server.NewServerWithAllOptions(appConfig.GetGlobalConfig(), true, enableAdaptor)

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
	// Create a simple rule
	rule := config.Rule{
		UUID:          requestModel,
		RequestModel:  requestModel,
		ResponseModel: model,
		Services: []config.Service{
			{
				Provider: providerName,
				Model:    model,
				Weight:   1,
				Active:   true,
			},
		},
		CurrentServiceIndex: 0,
		Tactic:              "round_robin",
		Active:              true,
	}

	if err := ts.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("Failed to add rule %s: %v", requestModel, err)
	}
}

// NewTestServerWithAdaptorFromConfig creates a new test server with adaptor flag using existing app config
func NewTestServerWithAdaptorFromConfig(t *testing.T, appConfig *config.AppConfig, enableAdaptor bool) *TestServer {
	// Create server instance with adaptor flag
	httpServer := server.NewServerWithAllOptions(appConfig.GetGlobalConfig(), true, enableAdaptor)

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
