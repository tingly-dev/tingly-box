package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"tingly-box/internal/config"
	"tingly-box/internal/server"
)

// TestServer represents a test server wrapper
type TestServer struct {
	appConfig    *config.AppConfig
	modelManager *config.ModelManager
	config       *config.AppConfig
	server       *server.Server
	ginEngine    *gin.Engine
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

	modelManager, err := config.NewModelManager()
	if err != nil {
		t.Fatalf("Failed to create model manager: %v", err)
	}

	// Create server instance but don't start it
	httpServer := server.NewServer(appConfig)

	return &TestServer{
		appConfig:    appConfig,
		modelManager: modelManager,
		config:       appConfig,
		server:       httpServer,
		ginEngine:    httpServer.GetRouter(), // Use the server's router
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

// MockModelManager creates a mock model manager for testing
func MockModelManager(t *testing.T) *config.ModelManager {
	// Create temporary config file for testing
	configFile := "config/test_models.yaml"
	defaultConfig := config.ModelConfig{
		Models: []config.ModelDefinition{
			{
				Name:        "test-model",
				Provider:    "test-provider",
				APIBase:     "https://api.test.com/v1",
				Model:       "test-model-actual",
				Aliases:     []string{"test", "mock"},
				Description: "Test model for unit tests",
				Category:    "chat",
			},
			{
				Name:        "gpt-3.5-turbo",
				Provider:    "openai",
				APIBase:     "https://api.openai.com/v1",
				Model:       "gpt-3.5-turbo",
				Aliases:     []string{"chatgpt"},
				Description: "OpenAI GPT-3.5 Turbo",
				Category:    "chat",
			},
		},
	}

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Create model manager with test config
	modelManager, err := config.NewModelManager()
	if err != nil {
		t.Fatalf("Failed to create model manager: %v", err)
	}

	return modelManager
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
	os.RemoveAll("config/test_models.yaml")
}
