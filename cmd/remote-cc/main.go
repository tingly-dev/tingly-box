package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/api"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/audit"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/config"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/launcher"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/middleware"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/session"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/summarizer"
)

// CORSMiddleware adds CORS headers to allow cross-origin requests
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length")
		c.Writer.Header().Set("Access-Control-Max-Age", "43200")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logrus.Fatalf("Failed to load configuration: %v", err)
	}

	// Setup logging
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
	})

	logrus.Infof("Starting remote-cc on port %d", cfg.Port)

	// Initialize components
	store, err := session.NewMessageStore(cfg.DBPath)
	if err != nil {
		logrus.Fatalf("Failed to initialize remote-cc message store: %v", err)
	}

	sessionMgr := session.NewManager(session.Config{
		Timeout:          cfg.SessionTimeout,
		MessageRetention: cfg.MessageRetention,
	}, store)

	claudeLauncher := launcher.NewClaudeCodeLauncher()
	if path := strings.TrimSpace(os.Getenv("RCC_CLAUDE_PATH")); path != "" {
		claudeLauncher.SetCLIPath(path)
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("RCC_SKIP_PERMISSIONS"))); v == "1" || v == "true" || v == "yes" {
		claudeLauncher.SetSkipPermissions(true)
	}
	summaryEngine := summarizer.NewEngine()

	// Initialize audit logger
	auditLogger := audit.NewLogger(audit.Config{
		Console:    true,
		MaxEntries: 10000,
	})

	// Initialize rate limiter
	rateLimiter := cfg.NewRateLimiter()

	// Start rate limiter cleanup goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rateLimiter.Cleanup()
		}
	}()

	// Setup Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.Use(CORSMiddleware())

	// Health check endpoint (no auth required, no rate limit)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Availability check endpoint (no auth required, for frontend to detect if service is running)
	router.GET("/remote-cc/available", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"available": true,
			"service":   "remote-cc",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Rate limit middleware for auth endpoints (before auth middleware)
	authRateLimit := middleware.RateLimitMiddleware(rateLimiter, "/remotecc/handshake", "/remotecc/execute")

	// RemoteCC legacy-compatible API routes
	remoteCCLegacyAPI := router.Group("/remotecc")
	remoteCCLegacyAPI.Use(authRateLimit)
	remoteCCLegacyAPI.Use(config.AuthMiddleware(cfg))

	apiHandler := api.NewHandler(sessionMgr, claudeLauncher, summaryEngine, auditLogger)
	remoteCCLegacyAPI.POST("/handshake", apiHandler.Handshake)
	remoteCCLegacyAPI.POST("/execute", apiHandler.Execute)
	remoteCCLegacyAPI.GET("/status/:session_id", apiHandler.Status)
	remoteCCLegacyAPI.POST("/close", apiHandler.Close)

	// Admin endpoints (separate auth for admin token)
	adminAPI := router.Group("/admin")
	adminAPI.Use(config.AuthMiddleware(cfg))

	adminHandler := api.NewAdminHandler(sessionMgr, auditLogger, rateLimiter, cfg)
	adminAPI.GET("/logs", adminHandler.GetAuditLogs)
	adminAPI.GET("/stats", adminHandler.GetStats)
	adminAPI.GET("/ratelimit/stats", adminHandler.GetRateLimitStats)
	adminAPI.POST("/ratelimit/reset", adminHandler.ResetRateLimit)
	adminAPI.POST("/tokens/generate", adminHandler.GenerateToken)
	adminAPI.POST("/tokens/validate", adminHandler.ValidateToken)
	adminAPI.POST("/tokens/revoke", adminHandler.RevokeToken)

	// Remote-CC endpoints (always enabled)
	remoteCCAPI := router.Group("/remote-cc")
	remoteCCAPI.Use(config.AuthMiddleware(cfg))

	remoteCCHandler := api.NewRemoteCCHandler(sessionMgr, claudeLauncher, summaryEngine, auditLogger, cfg)
	remoteCCAPI.GET("/sessions", remoteCCHandler.GetSessions)
	remoteCCAPI.GET("/sessions/:id", remoteCCHandler.GetSession)
	remoteCCAPI.GET("/sessions/:id/messages", remoteCCHandler.GetSessionMessages)
	remoteCCAPI.POST("/chat", remoteCCHandler.Chat)
	remoteCCAPI.POST("/sessions/clear", remoteCCHandler.ClearSessions)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Long timeout for Claude Code execution
	}

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("Failed to start server: %v", err)
		}
	}()

	logrus.Infof("remote-cc started successfully")
	logrus.Infof("Admin endpoints available at http://localhost:%d/admin/*", cfg.Port)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logrus.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logrus.Errorf("Server forced to shutdown: %v", err)
	}

	logrus.Info("Server stopped")
}
