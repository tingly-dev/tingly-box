package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"tingly-box/internal/auth"
	"tingly-box/internal/config"
)

// Server represents the HTTP server
type Server struct {
	config        *config.AppConfig
	jwtManager    *auth.JWTManager
	router        *gin.Engine
	httpServer    *http.Server
	watcher       *config.ConfigWatcher
	modelManager  *config.ModelManager
}

// NewServer creates a new HTTP server instance
func NewServer(appConfig *config.AppConfig) *Server {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Initialize model manager
	modelManager, err := config.NewModelManager()
	if err != nil {
		log.Printf("Warning: Failed to initialize model manager: %v", err)
		modelManager = nil
	}

	server := &Server{
		config:       appConfig,
		jwtManager:   auth.NewJWTManager(appConfig.GetJWTSecret()),
		router:       gin.New(),
		modelManager: modelManager,
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

// setupRoutes configures server routes
func (s *Server) setupRoutes() {
	// Health check endpoint
	s.router.GET("/health", s.HealthCheck)

	// Models endpoint
	s.router.GET("/v1/models", s.ListModels)

	// API v1 group
	v1 := s.router.Group("/v1")
	{
		// Chat completions endpoint (OpenAI compatible)
		v1.POST("/chat/completions", s.AuthenticateMiddleware(), s.ChatCompletions)
	}

	// Token generation endpoint
	s.router.POST("/token", s.GenerateToken)
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
	fmt.Printf("API endpoint: http://localhost:%d/v1/chat/completions\n", port)

	return s.httpServer.ListenAndServe()
}

// GetRouter returns the Gin router for testing purposes
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

// GetModelManager returns the model manager for testing purposes
func (s *Server) GetModelManager() *config.ModelManager {
	return s.modelManager
}

// SetModelManager sets the model manager for testing purposes
func (s *Server) SetModelManager(modelManager *config.ModelManager) {
	s.modelManager = modelManager
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