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
	appConfig       *config.AppConfig
	providerManager *config.ProviderManager
	config          *config.AppConfig
	server          *server.Server
	ginEngine       *gin.Engine
}

// NewTestServer creates a new test server with custom config directory
func NewTestServerWithConfigDir(t *testing.T, configDir string) *TestServer {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create config directory %s: %v", configDir, err)
	}

	appConfig, err := config.NewAppConfigWithConfigDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create app config: %v", err)
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
		config:    appConfig,
		server:    httpServer,
		ginEngine: httpServer.GetRouter(), // Use the server's router
	}
}

// AddTestProviders adds test providers to the configuration
func (ts *TestServer) AddTestProviders(t *testing.T) {
	providers := []struct {
		name    string
		apiBase string
		token   string
	}{
		{"openai", "https://api.openai.com/v1", "sk-test-openai"},
		{"alibaba", "https://dashscope.aliyuncs.com/compatible-mode/v1", "sk-test-alibaba"},
		{"anthropic", "https://api.anthropic.com", "sk-test-anthropic"},
		{"glm", "https://open.bigmodel.cn/api/paas/v4", "sk-test-glm"},
	}

	for _, p := range providers {
		provider := &config.Provider{
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
func (ts *TestServer) GetProviderToken(providerName string, isRealConfig bool) string {
	if isRealConfig {
		// Use Anthropic provider token for real config
		provider, err := ts.appConfig.GetProvider(providerName)
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
