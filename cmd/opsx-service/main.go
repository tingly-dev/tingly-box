package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/cmd/opsx-service/internal/api"
	"github.com/tingly-dev/tingly-box/cmd/opsx-service/internal/audit"
	"github.com/tingly-dev/tingly-box/cmd/opsx-service/internal/config"
	"github.com/tingly-dev/tingly-box/cmd/opsx-service/internal/launcher"
	"github.com/tingly-dev/tingly-box/cmd/opsx-service/internal/middleware"
	"github.com/tingly-dev/tingly-box/cmd/opsx-service/internal/session"
	"github.com/tingly-dev/tingly-box/cmd/opsx-service/internal/summarizer"
)

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

	logrus.Infof("Starting opsx-service on port %d", cfg.Port)

	// Initialize components
	sessionMgr := session.NewManager(session.Config{
		Timeout: cfg.SessionTimeout,
	})

	claudeLauncher := launcher.NewClaudeCodeLauncher()
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

	// Health check endpoint (no auth required, no rate limit)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Rate limit middleware for auth endpoints (before auth middleware)
	authRateLimit := middleware.RateLimitMiddleware(rateLimiter, "/opsx/handshake", "/opsx/execute")

	// API endpoints with authentication and rate limiting
	opsxAPI := router.Group("/opsx")
	opsxAPI.Use(authRateLimit)
	opsxAPI.Use(config.AuthMiddleware(cfg))

	apiHandler := api.NewHandler(sessionMgr, claudeLauncher, summaryEngine, auditLogger)
	opsxAPI.POST("/handshake", apiHandler.Handshake)
	opsxAPI.POST("/execute", apiHandler.Execute)
	opsxAPI.GET("/status/:session_id", apiHandler.Status)
	opsxAPI.POST("/close", apiHandler.Close)

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

	// Remote-CC endpoints (conditionally enabled)
	if cfg.EnableRemoteCC {
		logrus.Infof("Remote-CC feature enabled")

		remoteCCAPI := router.Group("/remote-cc")
		remoteCCAPI.Use(config.AuthMiddleware(cfg))

		remoteCCHandler := api.NewRemoteCCHandler(sessionMgr, claudeLauncher, summaryEngine, auditLogger, cfg)
		remoteCCAPI.GET("/sessions", remoteCCHandler.GetSessions)
		remoteCCAPI.GET("/sessions/:id", remoteCCHandler.GetSession)
		remoteCCAPI.POST("/chat", remoteCCHandler.Chat)
	}

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

	logrus.Infof("opsx-service started successfully")
	logrus.Infof("API endpoints available at http://localhost:%d/opsx/*", cfg.Port)
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
