package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"
	"tingly-box/internal/server/middleware"
	"tingly-box/internal/util"

	"tingly-box/internal/auth"
	"tingly-box/internal/config"
	"tingly-box/internal/obs"

	"github.com/gin-gonic/gin"
)

// Server represents the HTTP server
type Server struct {
	config     *config.Config
	jwtManager *auth.JWTManager
	engine     *gin.Engine
	httpServer *http.Server
	watcher    *config.ConfigWatcher
	logger     *obs.MemoryLogger

	// middleware
	statsMW         *middleware.StatsMiddleware
	debugMW         *middleware.DebugMiddleware
	loadBalancer    *LoadBalancer
	loadBalancerAPI *LoadBalancerAPI

	// client pool for caching
	clientPool *ClientPool

	// options
	enableUI      bool
	enableAdaptor bool
	host          string
}

// NewServerWithAllOptions creates a new HTTP server with UI and adaptor options
// Deprecated: Use NewServerWithFunctionalOptions instead
func NewServerWithAllOptions(cfg *config.Config, enableUI bool, enableAdaptor bool) *Server {
	return NewServer(cfg, WithUI(enableUI), WithAdaptor(enableAdaptor))
}

// ServerOption defines a functional option for Server configuration
type ServerOption func(*Server)

// WithUI enables or disables the UI for the server
func WithUI(enabled bool) ServerOption {
	return func(s *Server) {
		s.enableUI = enabled
	}
}

func WithHost(host string) ServerOption {
	return func(s *Server) {
		s.host = host
	}
}

// WithAdaptor enables or disables the adaptor for the server
func WithAdaptor(enabled bool) ServerOption {
	return func(s *Server) {
		s.enableAdaptor = enabled
	}
}

// NewServer creates a new HTTP server instance with functional options
func NewServer(cfg *config.Config, opts ...ServerOption) *Server {
	// Default options
	server := &Server{
		config:        cfg,
		enableUI:      true,  // Default: UI enabled
		enableAdaptor: false, // Default: adaptor disabled
	}

	// Apply provided options
	for _, opt := range opts {
		opt(server)
	}

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

	// Initialize memory logger
	memoryLogger, err := obs.NewMemoryLogger()
	if err != nil {
		log.Printf("Warning: Failed to initialize memory logger: %v", err)
		memoryLogger = nil
	}

	// Initialize debug middleware (only if debug mode is enabled)
	var debugMW *middleware.DebugMiddleware
	if cfg.GetDebug() {
		debugLogPath := filepath.Join(cfg.ConfigDir, config.LogDirName, config.DebugLogFileName)
		debugMW = middleware.NewDebugMiddleware(debugLogPath, 10)
		log.Printf("Debug middleware initialized (debug=true in config), logging to: %s", debugLogPath)
	}

	// Create server struct first with applied options
	server.jwtManager = jwtManager
	server.engine = gin.New()
	server.logger = memoryLogger
	server.clientPool = NewClientPool() // Initialize client pool
	server.debugMW = debugMW

	// Initialize statistics middleware with server reference
	statsMW := middleware.NewStatsMiddleware(cfg)

	// Initialize load balancer
	loadBalancer := NewLoadBalancer(statsMW, cfg)

	// Initialize load balancer API
	loadBalancerAPI := NewLoadBalancerAPI(loadBalancer, cfg)

	// Update server with dependencies
	server.statsMW = statsMW
	server.loadBalancer = loadBalancer
	server.loadBalancerAPI = loadBalancerAPI

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
	s.engine.Use(RequestLoggerMiddleware())

	// Logger middleware
	s.engine.Use(gin.Logger())

	// Recovery middleware
	s.engine.Use(gin.Recovery())

	// Debug middleware for logging requests/responses (only if enabled)
	if s.debugMW != nil {
		s.engine.Use(s.debugMW.Middleware())
	}

	// Statistics middleware for load balancing
	s.engine.Use(s.statsMW.Middleware())

	// CORS middleware
	s.engine.Use(func(c *gin.Context) {
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

		// Get content length from headers instead of reading the whole body
		contentLength := c.Request.ContentLength
		if contentLength == -1 {
			contentLength = 0
		}

		// Process the request
		c.Next()

		// Log essential details after the request is processed
		duration := time.Since(start)
		fmt.Printf("Method: %s | Path: %s | Status: %d | Content-Length: %d | Duration: %v\n",
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			contentLength,
			duration,
		)
	}
}

// setupRoutes configures server routes
func (s *Server) setupRoutes() {
	// Integrate Web UI routes if enabled
	if s.enableUI {
		s.UseUIEndpoints()
	}

	// Health check endpoint
	s.engine.GET("/health", s.HealthCheck)

	// Models endpoint
	//s.engine.GET("/v1/models", s.ListModels)

	// OpenAI v1 API group
	openaiV1 := s.engine.Group("/openai/v1")
	{
		// Chat completions endpoint (OpenAI compatible)
		openaiV1.POST("/chat/completions", s.ModelAuthMiddleware(), s.OpenAIChatCompletions)
		// Models endpoint (OpenAI compatible)
		//openaiV1.GET("/models", s.ModelAuthMiddleware(), s.ListModels)
	}

	// OpenAI API alias (without version)
	openai := s.engine.Group("/openai")
	{
		// Chat completions endpoint (OpenAI compatible)
		openai.POST("/chat/completions", s.ModelAuthMiddleware(), s.OpenAIChatCompletions)
		// Models endpoint (OpenAI compatible)
		//openai.GET("/models", s.ModelAuthMiddleware(), s.ListModels)
	}

	// Anthropic v1 API group
	anthropicV1 := s.engine.Group("/anthropic/v1")
	{
		// Chat completions endpoint (Anthropic compatible)
		anthropicV1.POST("/messages", s.ModelAuthMiddleware(), s.AnthropicMessages)
		// Count tokens endpoint (Anthropic compatible)
		anthropicV1.POST("/messages/count_tokens", s.ModelAuthMiddleware(), s.AnthropicCountTokens)
		// Models endpoint (Anthropic compatible)
		//anthropicV1.GET("/models", s.ModelAuthMiddleware(), s.AnthropicModels)
	}

	// API routes for load balancer management
	api := s.engine.Group("/api")
	api.Use(s.UserAuthMiddleware()) // Require user authentication for management APIs
	{
		// Load balancer API routes
		s.loadBalancerAPI.RegisterRoutes(api.Group("/v1"))
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
		Addr:    fmt.Sprintf("%s:%d", s.host, port),
		Handler: s.engine,
	}

	resolvedHost := util.ResolveHost(s.host)

	fmt.Printf("Starting server on port %d\n", port)
	fmt.Printf("OpenAI v1 Chat API endpoint: http://%s:%d/openai/v1/chat/completions\n", resolvedHost, port)
	fmt.Printf("Anthropic v1 Message API endpoint: http://%s:%d/anthropic/v1/messages\n", resolvedHost, port)

	// Get user token for Web UI URL
	webUIURL := fmt.Sprintf("http://%s:%d/dashboard", resolvedHost, port)
	if s.config.HasUserToken() {
		userToken := s.config.GetUserToken()
		webUIURL = fmt.Sprintf("http://%s:%d/dashboard?user_auth_token=%s", resolvedHost, port, userToken)
	}
	fmt.Printf("Web UI: %s\n", webUIURL)

	return s.httpServer.ListenAndServe()
}

// GetRouter returns the Gin engine for testing purposes
func (s *Server) GetRouter() *gin.Engine {
	return s.engine
}

// GetLoadBalancer returns the load balancer instance
func (s *Server) GetLoadBalancer() *LoadBalancer {
	return s.loadBalancer
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	// Stop debug middleware
	if s.debugMW != nil {
		s.debugMW.Stop()
	}

	// Stop configuration watcher
	if s.watcher != nil {
		s.watcher.Stop()
		log.Println("Configuration watcher stopped")
	}

	fmt.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}
