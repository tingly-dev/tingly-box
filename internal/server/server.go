package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"tingly-box/internal/auth"
	"tingly-box/internal/config"
	"tingly-box/internal/memory"

	"github.com/gin-gonic/gin"
)

// Server represents the HTTP server
type Server struct {
	config          *config.Config
	providerManager *config.ProviderManager
	jwtManager      *auth.JWTManager
	router          *gin.Engine
	httpServer      *http.Server
	watcher         *config.ConfigWatcher
	webUI           *WebUI
	useWebUI        bool
	memoryLogger    *memory.MemoryLogger
}

// NewServer creates a new HTTP server instance
func NewServer(cfg *config.Config) *Server {
	return NewServerWithOptions(cfg, true)
}

// NewServerWithOptions creates a new HTTP server with UI option
func NewServerWithOptions(cfg *config.Config, enableUI bool) *Server {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Check and generate tokens if needed
	jwtManager := auth.NewJWTManager(cfg.GetJWTSecret())

	if !cfg.HasUserToken() {
		log.Println("No user token found in global config, generating new user token...")
		apiKey, err := jwtManager.GenerateAPIKey("user")
		if err != nil {
			log.Printf("Failed to generate user API key: %v", err)
		} else {
			if err := cfg.SetUserToken(apiKey); err != nil {
				log.Printf("Failed to save generated user token: %v", err)
			} else {
				log.Printf("Generated and saved new user API token: %s", apiKey)
			}
		}
	} else {
		log.Printf("Using existing user token from global config")
	}

	if !cfg.HasModelToken() {
		log.Println("No model token found in global config, generating new model token...")
		apiKey, err := jwtManager.GenerateAPIKey("model")
		if err != nil {
			log.Printf("Failed to generate model API key: %v", err)
		} else {
			if err := cfg.SetModelToken(apiKey); err != nil {
				log.Printf("Failed to save generated model token: %v", err)
			} else {
				log.Printf("Generated and saved new model API token: %s", apiKey)
			}
		}
	} else {
		log.Printf("Using existing model token from global config")
	}

	// Initialize model manager
	providerManager, err := config.NewProviderManager(config.GetModelsDir())
	if err != nil {
		log.Printf("Warning: Failed to initialize model manager: %v", err)
		providerManager = nil
	}

	// Initialize memory logger
	memoryLogger, err := memory.NewMemoryLogger()
	if err != nil {
		log.Printf("Warning: Failed to initialize memory logger: %v", err)
		memoryLogger = nil
	}

	server := &Server{
		config:          cfg,
		jwtManager:      jwtManager,
		router:          gin.New(),
		providerManager: providerManager,
		memoryLogger:    memoryLogger,
		useWebUI:        enableUI,
	}

	// Setup middleware
	server.setupMiddleware()

	// Setup routes
	server.setupRoutes()

	// Setup configuration watcher
	server.setupConfigWatcher()

	return server
}

// setupConfigWatcher initializes the configuration hot-reload watcher
func (s *Server) setupConfigWatcher() {
	watcher, err := config.NewConfigWatcher(s.config)
	if err != nil {
		log.Printf("Failed to create config watcher: %v", err)
		return
	}

	s.watcher = watcher

	// Add callback for configuration changes
	watcher.AddCallback(func(newConfig *config.Config) {
		log.Println("Configuration updated, reloading...")
		// Update JWT manager with new secret if changed
		s.jwtManager = auth.NewJWTManager(newConfig.JWTSecret)
		log.Println("JWT manager reloaded with new secret")
	})
}

// setupMiddleware configures server middleware
func (s *Server) setupMiddleware() {
	s.router.Use(RequestLoggerMiddleware())

	// Logger middleware
	s.router.Use(gin.Logger())

	// Recovery middleware
	s.router.Use(gin.Recovery())

	// CORS middleware
	s.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})
}

func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		var buf bytes.Buffer
		var body []byte

		// Only read body if it exists (for POST/PUT requests)
		if c.Request.Body != nil {
			tee := io.TeeReader(c.Request.Body, &buf)
			body, _ = ioutil.ReadAll(tee)
			c.Request.Body = ioutil.NopCloser(&buf)
		}

		// Log details after the request is processed
		duration := time.Since(start)
		fmt.Printf("Method: %s | Path: %s | Status: %d | Headers: %v | Body: %s | Duration: %v\n",
			c.Request.Method,
			c.Request.URL.Path,
			c.Request.Header,
			c.Writer.Status(),
			string(body),
			duration,
		)

		// Process the request
		c.Next()
	}
}

// setupRoutes configures server routes
func (s *Server) setupRoutes() {
	// Health check endpoint
	s.router.GET("/health", s.HealthCheck)

	// Models endpoint
	s.router.GET("/v1/models", s.ListModels)

	// OpenAI v1 API group
	openaiV1 := s.router.Group("/openai/v1")
	{
		// Chat completions endpoint (OpenAI compatible)
		openaiV1.POST("/chat/completions", s.ModelAuth(), s.OpenAIChatCompletions)
		// Models endpoint (OpenAI compatible)
		openaiV1.GET("/models", s.ModelAuth(), s.ListModels)
	}

	// Anthropic v1 API group
	anthropicV1 := s.router.Group("/anthropic/v1")
	{
		// Chat completions endpoint (Anthropic compatible)
		anthropicV1.POST("/messages", s.ModelAuth(), s.AnthropicMessages)
		// Models endpoint (Anthropic compatible)
		anthropicV1.GET("/models", s.ModelAuth(), s.AnthropicModels)
	}

	// Legacy API v1 group for backward compatibility
	v1 := s.router.Group("/v1")
	{
		// Chat completions endpoint (OpenAI compatible)
		v1.POST("/chat/completions", s.ModelAuth(), s.ChatCompletions)
	}

	// Integrate Web UI routes if enabled
	if s.useWebUI {
		useWebUI(s)

		// Token generation endpoint (for UI and management)
		s.router.POST("/api/token", s.UserAuth(), s.GenerateToken)
		s.router.GET("/api/token", s.UserAuth(), s.GetToken)
	}
}

// Start starts the HTTP server
func (s *Server) Start(port int) error {
	// Start configuration watcher
	if s.watcher != nil {
		if err := s.watcher.Start(); err != nil {
			log.Printf("Failed to start config watcher: %v", err)
		} else {
			log.Println("Configuration hot-reload enabled")
		}
	}

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	fmt.Printf("Starting server on port %d\n", port)
	fmt.Printf("OpenAI v1 Chat API endpoint: http://localhost:%d/openai/v1/chat/completions\n", port)
	fmt.Printf("Anthropic v1 Message API endpoint: http://localhost:%d/anthropic/v1/messages\n", port)

	// Get user token for Web UI URL
	webUIURL := fmt.Sprintf("http://localhost:%d/dashboard", port)
	if s.config.HasUserToken() {
		userToken := s.config.GetUserToken()
		webUIURL = fmt.Sprintf("http://localhost:%d/dashboard?user_auth_token=%s", port, userToken)
	}
	fmt.Printf("Web UI: %s\n", webUIURL)

	return s.httpServer.ListenAndServe()
}

// GetRouter returns the Gin router for testing purposes
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

// GetProviderManager returns the provider manager for testing purposes
func (s *Server) GetProviderManager() *config.ProviderManager {
	return s.providerManager
}

// SetProviderManager sets the provider manager for testing purposes
func (s *Server) SetProviderManager(providerManager *config.ProviderManager) {
	s.providerManager = providerManager
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	// Stop configuration watcher
	if s.watcher != nil {
		s.watcher.Stop()
		log.Println("Configuration watcher stopped")
	}

	fmt.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}
