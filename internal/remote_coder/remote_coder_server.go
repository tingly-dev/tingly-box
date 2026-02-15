package remote_coder

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

	"github.com/tingly-dev/tingly-box/internal/remote_coder/api"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/audit"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/config"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/launcher"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/middleware"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/summarizer"
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

// Run starts the remote-coder service and blocks until shutdown.
func Run(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("remote-coder config is nil")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	logrus.Infof("Starting remote-coder on port %d", cfg.Port)

	store, err := session.NewMessageStore(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize remote-coder message store: %w", err)
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

	auditLogger := audit.NewLogger(audit.Config{
		Console:    true,
		MaxEntries: 10000,
	})

	rateLimiter := cfg.NewRateLimiter()
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rateLimiter.Cleanup()
		}
	}()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.Use(CORSMiddleware())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	router.GET("/remote-coder/available", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"available": true,
			"service":   "remote-coder",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	authRateLimit := middleware.RateLimitMiddleware(rateLimiter, "/remote-coder/handshake", "/remote-coder/execute")

	remoteCCLegacyAPI := router.Group("/remote-coder")
	remoteCCLegacyAPI.Use(authRateLimit)
	remoteCCLegacyAPI.Use(config.AuthMiddleware(cfg))

	apiHandler := api.NewHandler(sessionMgr, claudeLauncher, summaryEngine, auditLogger)
	remoteCCLegacyAPI.POST("/handshake", apiHandler.Handshake)
	remoteCCLegacyAPI.POST("/execute", apiHandler.Execute)
	remoteCCLegacyAPI.GET("/status/:session_id", apiHandler.Status)
	remoteCCLegacyAPI.POST("/close", apiHandler.Close)

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

	remoteCCAPI := router.Group("/remote-coder")
	remoteCCAPI.Use(config.AuthMiddleware(cfg))

	remoteCCHandler := api.NewRemoteCCHandler(sessionMgr, claudeLauncher, summaryEngine, auditLogger, cfg)
	botStore, err := bot.NewStore(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize bot store: %w", err)
	}
	defer func() {
		_ = botStore.Close()
	}()
	botSettingsHandler := api.NewBotSettingsHandler(botStore)
	remoteCCAPI.GET("/sessions", remoteCCHandler.GetSessions)
	remoteCCAPI.GET("/sessions/:id", remoteCCHandler.GetSession)
	remoteCCAPI.GET("/sessions/:id/state", remoteCCHandler.GetSessionState)
	remoteCCAPI.PUT("/sessions/:id/state", remoteCCHandler.UpdateSessionState)
	remoteCCAPI.GET("/sessions/:id/messages", remoteCCHandler.GetSessionMessages)
	remoteCCAPI.POST("/chat", remoteCCHandler.Chat)
	remoteCCAPI.POST("/sessions/clear", remoteCCHandler.ClearSessions)

	// Bot settings API - V2 multi-bot endpoints
	remoteCCAPI.GET("/bot/settings", botSettingsHandler.GetSettings)       // Legacy: returns single, with ?list=true returns array
	remoteCCAPI.GET("/bot/settings/list", botSettingsHandler.ListSettings) // V2: returns all bots
	remoteCCAPI.GET("/bot/settings/:uuid", botSettingsHandler.GetSettingsByUUID)
	remoteCCAPI.POST("/bot/settings", botSettingsHandler.CreateSettings)
	remoteCCAPI.PUT("/bot/settings/:uuid", botSettingsHandler.UpdateSettings)
	remoteCCAPI.DELETE("/bot/settings/:uuid", botSettingsHandler.DeleteSettings)
	remoteCCAPI.POST("/bot/settings/:uuid/toggle", botSettingsHandler.ToggleSettings)

	// Legacy endpoint for backward compatibility
	remoteCCAPI.PUT("/bot/settings", botSettingsHandler.UpdateSettingsLegacy)

	remoteCCAPI.GET("/bot/platforms", botSettingsHandler.GetPlatforms)
	remoteCCAPI.GET("/bot/platform-config", botSettingsHandler.GetPlatformConfig)

	// Start enabled bots
	enabledSettings, err := botStore.ListEnabledSettings()
	if err != nil {
		logrus.WithError(err).Warn("Remote-coder bot not started: failed to load settings")
	} else if len(enabledSettings) > 0 {
		for _, settings := range enabledSettings {
			// For now, only support Telegram bots
			if settings.Platform == "telegram" || settings.Platform == "" {
				token := settings.Auth["token"]
				if token == "" {
					token = settings.Token // Legacy field
				}
				if token != "" {
					go func(s bot.Settings) {
						// Create a new store instance for each goroutine to avoid race conditions
						// Each bot instance will share the same database connection
						if err := bot.RunTelegramBot(ctx, botStore, sessionMgr); err != nil {
							logrus.WithError(err).Warn("Remote-coder Telegram bot stopped")
						}
					}(settings)
					logrus.Infof("Remote-coder Telegram bot started (name: %s)", settings.Name)
				}
			}
		}
	} else {
		// Try legacy single settings for backward compatibility
		if settings, err := botStore.GetSettings(); err == nil {
			if strings.TrimSpace(settings.Token) != "" {
				go func() {
					if err := bot.RunTelegramBot(ctx, botStore, sessionMgr); err != nil {
						logrus.WithError(err).Warn("Remote-coder Telegram bot stopped")
					}
				}()
				logrus.Info("Remote-coder Telegram bot started (legacy mode)")
			} else {
				logrus.Info("Remote-coder Telegram bot not started: missing token")
			}
		} else {
			logrus.WithError(err).Warn("Remote-coder Telegram bot not started: failed to load settings")
		}
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		logrus.Info("Remote-coder shutting down (context canceled)...")
	case sig := <-sigCh:
		logrus.Infof("Remote-coder shutting down (%s)...", sig.String())
	case err := <-errCh:
		return fmt.Errorf("remote-coder server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("remote-coder shutdown failed: %w", err)
	}

	logrus.Info("Remote-coder stopped")
	return nil
}
