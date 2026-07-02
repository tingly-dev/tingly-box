package server

import (
	"context"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	assets "github.com/tingly-dev/tingly-box/internal"
	"github.com/tingly-dev/tingly-box/internal/server/module/codeximport"
	"github.com/tingly-dev/tingly-box/internal/server/module/configapply"
	"github.com/tingly-dev/tingly-box/internal/server/module/imbot"
	mcpmodule "github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	notifymodule "github.com/tingly-dev/tingly-box/internal/server/module/notify"
	oauthmodule "github.com/tingly-dev/tingly-box/internal/server/module/oauth"
	providerQuotaModule "github.com/tingly-dev/tingly-box/internal/server/module/providerquota"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	usagemodule "github.com/tingly-dev/tingly-box/internal/server/module/usage"
	virtualmodelmodule "github.com/tingly-dev/tingly-box/internal/server/module/virtualmodel"
	"github.com/tingly-dev/tingly-box/remote/audit"
	"github.com/tingly-dev/tingly-box/remote/binding"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	remotescenario "github.com/tingly-dev/tingly-box/remote/scenario"
	"github.com/tingly-dev/tingly-box/remote/scenario/builtin/claudecode"
	"github.com/tingly-dev/tingly-box/swagger"
)

// GlobalServerManager manages the global server instance for web UI control
var (
	globalServer     *Server
	globalServerLock sync.RWMutex
	shutdownChan     = make(chan struct{}, 1)
)

// SetGlobalServer sets the global server instance for web UI control
func SetGlobalServer(server *Server) {
	globalServerLock.Lock()
	defer globalServerLock.Unlock()
	globalServer = server
}

// GetGlobalServer gets the global server instance
func GetGlobalServer() *Server {
	globalServerLock.RLock()
	defer globalServerLock.RUnlock()
	return globalServer
}

// Init sets up Server routes and templates on the main server engine
func (s *Server) UseUIEndpoints(ctx context.Context) {

	// API endpoints are handled separately and won't match this pattern
	// Admin/backend routes that need their own pages:
	// - /provider, /api-keys, /oauth, /routing, /system, /history etc.
	// All serve the same index.html, letting React Router handle the navigation

	// Exclude API routes from SPA catch-all by registering them first
	// The routes registered below (manager APIs, OAuth, usage, etc.) will take precedence

	// Claude Code status line endpoints (no auth required) - register from claudecode module
	// These must be registered before the /tingly/:scenario routes
	var quotaMgr statusline.QuotaManager
	if s.quotaManager != nil {
		quotaMgr = s.quotaManager
	}
	statusHandler := statusline.NewHandler(s.config, s.loadBalancer, statusline.NewCache(), quotaMgr)
	statusline.RegisterRoutes(s.engine, statusHandler)

	// Remote middle-layer wiring for /tingly/:scenario hook events.
	// When the bot settings store is reachable we set up:
	//   - channelRegistry: running bots register imbot-backed Channels
	//     (see internal/remote/channel/imchannel)
	//   - interactionRegistry: shared long-poll registry the wait
	//     endpoint reads from
	//   - scenarioRegistry: name → plugin (Phase 1 ships claudecode)
	// Everything stays nil for setups without imbot settings; in that
	// case the notify HTTP module falls back to desktop notifications.
	var notifyHandler *notifymodule.Handler
	if sm := s.config.StoreManager(); sm != nil && sm.ImBotSettings() != nil {
		s.channelRegistry = channel.NewRegistry()
		s.interactionRegistry = interaction.New[interaction.Result](30 * time.Second)
		s.scenarioRegistry = remotescenario.NewRegistry()
		s.scenarioRegistry.Register(claudecode.New(s.interactionRegistry))
		resolver := binding.NewResolver(sm.ImBotSettings())
		auditLog := audit.NewLogger(audit.Config{Console: true, MaxEntries: 1000})
		runtime := remotescenario.NewDefaultRuntime(s.channelRegistry, resolver, runtimeAuditSink(auditLog))
		notifyHandler = notifymodule.NewHandlerWithRouting(s.scenarioRegistry, s.interactionRegistry, runtime)
	} else {
		notifyHandler = notifymodule.NewHandler()
	}
	notifymodule.RegisterRoutes(s.engine, notifyHandler)

	// Create route manager
	manager := swagger.NewRouteManager(s.engine)

	// API routes (for web UI functionality)
	s.useWebAPIEndpoints(manager)

	// OAuth API routes - register from oauth module
	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.getUserAuthMiddleware())
	oauthmodule.RegisterRoutes(apiV1, s.getUserAuthMiddleware(), s.oauthHandler)
	// Register callback routes (unauthenticated)
	oauthmodule.RegisterCallbackRoutes(manager, s.oauthHandler)

	// Virtual-model management routes — expose registry contents for the
	// Credentials > Virtual Models sub-tab. The providers themselves are
	// served via the standard provider CRUD endpoints (Source=builtin).
	virtualmodelmodule.RegisterRoutes(
		apiV1,
		s.getUserAuthMiddleware(),
		virtualmodelmodule.NewHandler(s.virtualModelService),
	)

	// Usage API routes - register from usage module
	// Note: apiV1 is already created above with auth middleware
	sm := s.config.StoreManager()
	if sm != nil {
		usageHandler := usagemodule.NewHandler(sm.Usage())
		usagemodule.RegisterRoutes(apiV1, usageHandler)
	}

	// ImBot settings API routes - register from imbotsettings module
	imbotHandler, err := imbot.NewHandler(ctx, s.config)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create imbotsettings handler, imbot settings APIs will not be available")
	} else {
		imbot.RegisterRoutes(apiV1, imbotHandler)
		// Store handler reference for shutdown
		s.imbotSettingsHandler = imbotHandler
		// Wire the channel registry so each running bot exposes itself
		// as a remote.channel.Channel reachable from /tingly/:scenario
		// scenario plugins.
		if s.channelRegistry != nil {
			imbotHandler.SetChannelRegistry(s.channelRegistry)
		}
	}

	// Config apply API routes
	configapplyHandler := configapply.NewHandler(s.config, s.host)
	configapply.RegisterRoutes(apiV1, configapplyHandler)

	codexImportHandler := codeximport.NewHandler(nil, s.config)
	codeximport.RegisterRoutes(apiV1, codexImportHandler)

	// MCP runtime API routes
	mcpHandler := mcpmodule.NewHandler(s.config, s.mcpRuntime)
	mcpmodule.RegisterRoutes(apiV1, mcpHandler, mcpHandler.GetLocalHandler(), mcpHandler.GetTransportHandler())

	// Provider quota API routes
	if s.quotaManager != nil {
		quotaHandler := providerQuotaModule.NewHandler(s.quotaManager, logrus.StandardLogger())
		quotaHandler.RegisterRoutes(apiV1.Router)
		logrus.Info("Provider quota API routes registered")
	}

	// Static files and templates - try embedded assets first, fallback to filesystem
	s.useWebStaticEndpoints(s.engine)
}

func (s *Server) UseIndexHTML(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	f, err := assets.WebDistAssets.Open("web/dist/index.html")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func (s *Server) useWebStaticEndpoints(engine *gin.Engine) {
	// Load templates and static files on the main engine - try embedded first
	log.Printf("Using embedded assets on main server")

	// Serve static assets from embedded filesystem
	st, _ := fs.Sub(assets.WebDistAssets, "web/dist/assets")
	engine.StaticFS("/assets", http.FS(st))

	// SPA catch-all - must be registered LAST
	// Serves index.html for all non-API frontend routes, letting React Router handle navigation
	// NoRoute handles unmatched paths including nested routes like /provider/settings/detail/123
	engine.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes - let them return 404s
		path := c.Request.URL.Path
		// Check if this looks like an API route
		if path == "" || strings.HasPrefix(path, "/api/v") || strings.HasPrefix(path, "/v") || strings.HasPrefix(path, "/openai") || strings.HasPrefix(path, "/anthropic") || strings.HasPrefix(path, "/tingly") {
			// This looks like an API route, return 404
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "API endpoint not found",
					"type":    "invalid_request_error",
					"code":    "not_found",
				},
			})
			c.Abort()
			return
		}

		s.UseIndexHTML(c)
	})
}

// runtimeAuditSink adapts an audit.Logger into the AuditFunc the
// scenario runtime hands to plugins. Plugin actions (e.g.
// claude_code.interactive.start / .done / .error) land here as audit
// entries with structured details.
func runtimeAuditSink(log *audit.Logger) remotescenario.AuditFunc {
	if log == nil {
		return nil
	}
	return func(action string, fields map[string]any) {
		details := map[string]interface{}{}
		for k, v := range fields {
			details[k] = v
		}
		log.Log(audit.Entry{
			Timestamp: time.Now(),
			Level:     audit.LevelInfo,
			Action:    action,
			Success:   true,
			Details:   details,
		})
	}
}

// GetShutdownChannel returns the shutdown channel for the main process to listen on
func GetShutdownChannel() <-chan struct{} {
	return shutdownChan
}

func init() {
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".png", "image/png")
}
