package benchmark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Default configuration constants
const (
	DefaultPort           = 8080
	DefaultChatDelayMs    = 100
	DefaultMessageDelayMs = 150
	DefaultRandomDelayMin = 10
	DefaultRandomDelayMax = 100
)

// MockServer represents the mock server
type MockServer struct {
	config *serverConfig
	server *http.Server
	engine *gin.Engine
}

// serverConfig holds server configuration
type serverConfig struct {
	port                 int
	defaultModels        []Model
	defaultChatResponses []json.RawMessage
	defaultMsgResponses  []json.RawMessage
	loopChatResponses    bool
	loopMsgResponses     bool
	chatDelayMs          int
	msgDelayMs           int
	randomDelayMinMs     int
	randomDelayMaxMs     int
	apiKey               string
}

// Option is a functional option for configuring the MockServer
type Option func(*serverConfig)

// WithPort sets the server port
func WithPort(port int) Option {
	return func(c *serverConfig) {
		c.port = port
	}
}

// WithDefaultModels sets the default model list
func WithDefaultModels(models []Model) Option {
	return func(c *serverConfig) {
		c.defaultModels = models
	}
}

// WithChatResponses sets the default chat completion responses
func WithChatResponses(responses []json.RawMessage, loop bool) Option {
	return func(c *serverConfig) {
		c.defaultChatResponses = responses
		c.loopChatResponses = loop
	}
}

// WithChatResponse adds a single chat completion response (as raw JSON)
func WithChatResponse(response json.RawMessage) Option {
	return func(c *serverConfig) {
		c.defaultChatResponses = append(c.defaultChatResponses, response)
	}
}

// WithChatResponseContent adds a chat response from content string
func WithChatResponseContent(content string) Option {
	response := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "gpt-3.5-turbo",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": len(content) / 4, // rough estimate
			"total_tokens":      10 + len(content)/4,
		},
	}
	data, _ := json.Marshal(response)
	return WithChatResponse(json.RawMessage(data))
}

// WithMessageResponses sets the default message responses
func WithMessageResponses(responses []json.RawMessage, loop bool) Option {
	return func(c *serverConfig) {
		c.defaultMsgResponses = responses
		c.loopMsgResponses = loop
	}
}

// WithMessageResponse adds a single message response (as raw JSON)
func WithMessageResponse(response json.RawMessage) Option {
	return func(c *serverConfig) {
		c.defaultMsgResponses = append(c.defaultMsgResponses, response)
	}
}

// WithMessageResponseContent adds a message response from content string
func WithMessageResponseContent(content string) Option {
	response := map[string]interface{}{
		"id":   fmt.Sprintf("msg_%d", time.Now().Unix()),
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": content,
			},
		},
		"model": "claude-3-sonnet-20240229",
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": len(content) / 4, // rough estimate
		},
	}
	data, _ := json.Marshal(response)
	return WithMessageResponse(json.RawMessage(data))
}

// WithChatDelay sets a fixed delay for chat responses (in milliseconds)
func WithChatDelay(delayMs int) Option {
	return func(c *serverConfig) {
		c.chatDelayMs = delayMs
	}
}

// WithMessageDelay sets a fixed delay for message responses (in milliseconds)
func WithMessageDelay(delayMs int) Option {
	return func(c *serverConfig) {
		c.msgDelayMs = delayMs
	}
}

// WithRandomDelay sets a random delay range (in milliseconds)
func WithRandomDelay(minMs, maxMs int) Option {
	return func(c *serverConfig) {
		c.randomDelayMinMs = minMs
		c.randomDelayMaxMs = maxMs
	}
}

// WithApiKey sets the authentication key required for requests
func WithApiKey(key string) Option {
	return func(c *serverConfig) {
		c.apiKey = key
	}
}

// authMiddleware creates a middleware that checks for valid authentication
func (ms *MockServer) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// If no auth key is configured, allow all requests
		if ms.config.apiKey == "" {
			c.Next()
			return
		}

		// Check Authorization header (Bearer token)
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			// Remove "Bearer " prefix if present
			token := authHeader
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				token = authHeader[7:]
			}
			if token == ms.config.apiKey {
				c.Next()
				return
			}
		}

		// Check x-api-key header (Anthropic style)
		apiKey := c.GetHeader("x-api-key")
		if apiKey == ms.config.apiKey {
			c.Next()
			return
		}

		// If we reach here, authentication failed
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"type":    "authentication_error",
				"message": "Invalid authentication key",
			},
		})
		c.Abort()
	}
}

// WithDefaultOptions applies sensible defaults
func WithDefaultOptions() Option {
	return func(c *serverConfig) {
		c.port = DefaultPort
		c.defaultModels = getDefaultModels()
		c.loopChatResponses = true
		c.loopMsgResponses = true
	}
}

// WithOpenAIDefaults sets defaults for OpenAI endpoints
func WithOpenAIDefaults() Option {
	return func(c *serverConfig) {
		WithDefaultOptions()(c)
		WithChatResponseContent("Hello! This is a default response from the OpenAI mock server.")(c)
		WithChatDelay(DefaultChatDelayMs)(c)
	}
}

// WithAnthropicDefaults sets defaults for Anthropic endpoints
func WithAnthropicDefaults() Option {
	return func(c *serverConfig) {
		WithDefaultOptions()(c)
		WithMessageResponseContent("Hello! This is a default response from the Anthropic mock server.")(c)
		WithMessageDelay(DefaultMessageDelayMs)(c)
	}
}

// WithBothDefaults sets defaults for both OpenAI and Anthropic
func WithBothDefaults() Option {
	return func(c *serverConfig) {
		WithDefaultOptions()(c)
		WithChatResponseContent("Hello! This is a default response from the OpenAI mock server.")(c)
		WithMessageResponseContent("Hello! This is a default response from the Anthropic mock server.")(c)
		WithChatDelay(DefaultChatDelayMs)(c)
		WithMessageDelay(DefaultMessageDelayMs)(c)
	}
}

// NewMockServer creates a new mock server with the given options
func NewMockServer(opts ...Option) *MockServer {
	config := &serverConfig{}

	// Apply default options first
	WithDefaultOptions()(config)

	// Apply provided options
	for _, opt := range opts {
		opt(config)
	}

	return &MockServer{
		config: config,
	}
}

// Start starts the mock server
func (ms *MockServer) Start() error {
	gin.SetMode(gin.ReleaseMode)

	// Create Gin router
	ms.engine = gin.New()

	// Add middleware
	// ms.engine.Use(gin.Logger())
	ms.engine.Use(gin.Recovery())

	// Add auth middleware if auth key is configured
	if ms.config.apiKey != "" {
		ms.engine.Use(ms.authMiddleware())
	}

	// Add CORS middleware if needed
	ms.engine.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	})

	// Create route groups
	v1 := ms.engine.Group("/v1")
	openai := ms.engine.Group("/openai/v1")
	anthropic := ms.engine.Group("/anthropic/v1")

	// Register OpenAI endpoints
	openai.GET("/models", ms.handleOpenAIModels)
	openai.POST("/chat/completions", ms.handleOpenAIChat)

	// Register Anthropic endpoints
	anthropic.GET("/models", ms.handleAnthropicModels)
	anthropic.POST("/messages", ms.handleAnthropicMessages)

	// Generic endpoints (for backward compatibility)
	v1.GET("/models", ms.handleOpenAIModels)          // Default to OpenAI
	v1.POST("/chat/completions", ms.handleOpenAIChat) // Default to OpenAI

	ms.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", ms.config.port),
		Handler: ms.engine,
	}

	return ms.server.ListenAndServe()
}

// Stop stops the mock server
func (ms *MockServer) Stop() error {
	if ms.server != nil {
		return ms.server.Close()
	}
	return nil
}

// Port returns the configured port
func (ms *MockServer) Port() int {
	return ms.config.port
}

// UseAuthMiddleware applies the auth middleware to all routes
func (ms *MockServer) UseAuthMiddleware() {
	if ms.engine != nil {
		ms.engine.Use(ms.authMiddleware())
	}
}
