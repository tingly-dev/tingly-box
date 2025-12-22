package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"
	"tingly-box/internal/server/middleware"
	"tingly-box/internal/util"

	"tingly-box/internal/auth"
	"tingly-box/internal/config"
	"tingly-box/internal/obs"

	"github.com/gin-gonic/gin"
	"github.com/pkg/browser"
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
	errorMW         *middleware.ErrorLogMiddleware
	authMW          *middleware.AuthMiddleware
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
			apiKey = "tingly-box-" + apiKey
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
	var errorMW *middleware.ErrorLogMiddleware
	errorLogPath := filepath.Join(cfg.ConfigDir, config.LogDirName, config.DebugLogFileName)
	errorMW = middleware.NewErrorLogMiddleware(errorLogPath, 10)
	log.Printf("Error middleware initialized (debug=true in config), logging to: %s", errorLogPath)

	// Create server struct first with applied options
	server.jwtManager = jwtManager
	server.engine = gin.New()
	server.logger = memoryLogger
	server.clientPool = NewClientPool() // Initialize client pool
	server.errorMW = errorMW

	// Initialize statistics middleware with server reference
	statsMW := middleware.NewStatsMiddleware(cfg)

	// Initialize auth middleware
	authMW := middleware.NewAuthMiddleware(cfg, jwtManager)

	// Initialize load balancer
	loadBalancer := NewLoadBalancer(statsMW, cfg)

	// Initialize load balancer API
	loadBalancerAPI := NewLoadBalancerAPI(loadBalancer, cfg)

	// Update server with dependencies
	server.statsMW = statsMW
	server.authMW = authMW
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
	if s.errorMW != nil {
		s.engine.Use(s.errorMW.Middleware())
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

	// Models endpoint
	//s.engine.GET("/v1/models", s.ListModels)

	// OpenAI v1 API group
	openaiV1 := s.engine.Group("/openai/v1")
	{
		// Chat completions endpoint (OpenAI compatible)
		openaiV1.POST("/chat/completions", s.authMW.ModelAuthMiddleware(), s.OpenAIChatCompletions)
		// Models endpoint (OpenAI compatible)
		//openaiV1.GET("/models", s.ModelAuthMiddleware(), s.ListModels)
	}

	// OpenAI API alias (without version)
	openai := s.engine.Group("/openai")
	{
		// Chat completions endpoint (OpenAI compatible)
		openai.POST("/chat/completions", s.authMW.ModelAuthMiddleware(), s.OpenAIChatCompletions)
		// Models endpoint (OpenAI compatible)
		//openai.GET("/models", s.ModelAuthMiddleware(), s.ListModels)
	}

	// Anthropic v1 API group
	anthropicV1 := s.engine.Group("/anthropic/v1")
	{
		// Chat completions endpoint (Anthropic compatible)
		anthropicV1.POST("/messages", s.authMW.ModelAuthMiddleware(), s.AnthropicMessages)
		// Count tokens endpoint (Anthropic compatible)
		anthropicV1.POST("/messages/count_tokens", s.authMW.ModelAuthMiddleware(), s.AnthropicCountTokens)
		// Models endpoint (Anthropic compatible)
		//anthropicV1.GET("/models", s.ModelAuthMiddleware(), s.AnthropicModels)
	}

	// API routes for load balancer management
	api := s.engine.Group("/api")
	api.Use(s.authMW.UserAuthMiddleware()) // Require user authentication for management APIs
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
	addr := fmt.Sprintf("%s:%d", s.host, port)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.engine,
	}

	resolvedHost := util.ResolveHost(s.host)
	if !s.enableUI {
		fmt.Printf("OpenAI v1 Chat API endpoint: http://%s:%d/openai/v1/chat/completions\n", resolvedHost, port)
		fmt.Printf("Anthropic v1 Message API endpoint: http://%s:%d/anthropic/v1/messages\n", resolvedHost, port)
		//Fixme:: we should not hardcode it here
		fmt.Printf("Mode name: %s\n", "tingly")
		fmt.Printf("Model API key: %s\n", s.config.GetModelToken())
		return s.httpServer.ListenAndServe()
	}

	// CASE 2: Web UI Mode ---
	webUIURL := fmt.Sprintf("http://%s:%d", resolvedHost, port)
	if s.config.HasUserToken() {
		webUIURL = fmt.Sprintf("%s/?user_auth_token=%s", webUIURL, s.config.GetUserToken())
	}

	fmt.Printf("Web UI: %s\n", webUIURL)
	fmt.Printf("Starting server and opening browser...\n")

	// Use a channel to capture the immediate error if ListenAndServe fails
	serverError := make(chan error, 1)
	go func() {
		serverError <- s.httpServer.ListenAndServe()
	}()

	// Instead of a fixed 100ms sleep, we poll the port
	if err := waitForPort(addr, 2*time.Second); err != nil {
		// Check if the server goroutine already caught a "port in use" error
		select {
		case e := <-serverError:
			return e
		default:
			return fmt.Errorf("timeout: server did not start on %s: %v", addr, err)
		}
	}

	// Server is up, now open browser
	browser.OpenURL(webUIURL)

	// Block until server shuts down or errors out
	return <-serverError
}

// Helper: Polls the port to ensure it's open before browser opens
func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("port %s not reachable", addr)
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
	if s.errorMW != nil {
		s.errorMW.Stop()
	}

	// Stop configuration watcher
	if s.watcher != nil {
		s.watcher.Stop()
		log.Println("Configuration watcher stopped")
	}

	fmt.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) GetInfoConfig(c *gin.Context) {
	// Return configuration information
	configInfo := ConfigInfo{
		ConfigPath: s.config.ConfigFile,
		ConfigDir:  s.config.ConfigDir,
	}

	c.JSON(http.StatusOK, ConfigInfoResponse{
		Success: true,
		Data:    configInfo,
	})
}
