package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/internal/db"
	desksvc "github.com/tingly-dev/tingly-box/internal/desk"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/codeximport"
	"github.com/tingly-dev/tingly-box/internal/server/module/configapply"
	debugmodule "github.com/tingly-dev/tingly-box/internal/server/module/debug"
	deskmodule "github.com/tingly-dev/tingly-box/internal/server/module/desk"
	"github.com/tingly-dev/tingly-box/internal/server/module/imbot"
	mcpmodule "github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	notifymodule "github.com/tingly-dev/tingly-box/internal/server/module/notify"
	oauthmodule "github.com/tingly-dev/tingly-box/internal/server/module/oauth"
	providerQuotaModule "github.com/tingly-dev/tingly-box/internal/server/module/providerquota"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	usagemodule "github.com/tingly-dev/tingly-box/internal/server/module/usage"
	virtualmodelmodule "github.com/tingly-dev/tingly-box/internal/server/module/virtualmodel"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/control"
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

// GetShutdownChannel returns the shutdown channel for the main process to listen on
func GetShutdownChannel() <-chan struct{} {
	return shutdownChan
}

func (s *Server) StopServer(c *gin.Context) {
	// Get the global server instance
	server := GetGlobalServer()
	if server == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "No server instance available to stop",
		})
		return
	}

	// Stop the server gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop server: %v", err),
		})
		return
	}

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action": obs.ActionStopServer,
		"source": "web_ui",
	}).Info("Server stopped via web interface")

	// Send shutdown signal to main process
	select {
	case shutdownChan <- struct{}{}:
	default:
		// Channel already has a signal
	}

	response := ServerActionResponse{
		Success: true,
		Message: "Server stopped successfully. The application will now exit.",
	}

	c.JSON(http.StatusOK, response)
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
		runtime := remotescenario.NewRouteRuntime(s.channelRegistry, sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), RuntimeAuditSink())
		notifyHandler = notifymodule.NewHandlerWithRouting(s.scenarioRegistry, s.interactionRegistry, runtime)
	} else {
		notifyHandler = notifymodule.NewHandler()
	}
	notifymodule.RegisterRoutes(s.engine, notifyHandler)

	// Create route manager
	manager := swagger.NewRouteManager(s.engine)

	// API routes (for web UI functionality)
	s.UseWebAPIEndpoints(manager)

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

	// Runtime memory diagnostics — per-instance memstats + heap profile over
	// the production HTTP surface. Consumed by the duo harness (per-instance
	// leak attribution) and by live incident diagnosis.
	debugmodule.RegisterRoutes(
		apiV1,
		s.getUserAuthMiddleware(),
		debugmodule.NewHandler(),
	)

	// Usage API routes - register from usage module
	// Note: apiV1 is already created above with auth middleware
	sm := s.config.StoreManager()
	if sm != nil {
		usageHandler := usagemodule.NewHandler(sm.Usage())
		usagemodule.RegisterRoutes(apiV1, usageHandler)
	}

	// ImBot settings API routes - register from imbotsettings module.
	// s.channelRegistry is passed in at construction (not wired later via
	// SetChannelRegistry) so every bot NewHandler brings up — including bots
	// enabled before the server started, which the background sync starts
	// immediately on its own goroutine — registers itself as a
	// remote.channel.Channel reachable from /tingly/:scenario and
	// /api/v1/bots/:bot/*. Passing it after the fact left a startup race
	// where such a bot could finish starting before the registry landed and
	// would then never expose a channel until manually restarted.
	imbotHandler, err := imbot.NewHandler(ctx, s.config, s.channelRegistry)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create imbotsettings handler, imbot settings APIs will not be available")
	} else {
		imbot.RegisterRoutes(apiV1, imbotHandler)
		// Store handler reference for shutdown
		s.imbotSettingsHandler = imbotHandler
	}

	// Bot interaction API — the general, caller-facing surface for driving a
	// running bot's channel: POST /api/v1/bots/:bot/{notify,interact} and
	// GET /api/v1/bots/:bot/interact/:id. Auth is inherited from the apiV1
	// group (getUserAuthMiddleware). Only registers when the bot middle layer
	// is wired (channelRegistry + interactionRegistry present); without IM
	// settings there is no channel to drive. See .design/bot-interaction-api.md.
	if s.channelRegistry != nil && s.interactionRegistry != nil {
		// chatManager backs the chat lifecycle endpoints (GET /chats,
		// DELETE, PUT .../disabled) and the outbound blocklist check — it
		// scopes the shared chat store to each bot's platform and chat-id
		// lock. nil when no IM handler is wired, in which case those
		// endpoints report unavailable.
		var chatManager notifymodule.BotChatManager
		if imbotHandler != nil {
			chatManager = newBotChatManager(s.channelRegistry, imbotHandler)
		}
		botAPI := notifymodule.NewBotAPIHandler(s.channelRegistry, s.interactionRegistry, chatManager, s.config.StoreManager().BotAccess())
		notifymodule.RegisterBotRoutes(apiV1, botAPI)
	}

	// Config apply API routes
	configapplyHandler := configapply.NewHandler(s.config, s.host)
	configapply.RegisterRoutes(apiV1, configapplyHandler)

	codexImportHandler := codeximport.NewHandler(nil, s.config)
	codeximport.RegisterRoutes(apiV1, codexImportHandler)

	// MCP runtime API routes
	mcpHandler := mcpmodule.NewHandler(s.config, s.mcpRuntime)
	mcpmodule.RegisterRoutes(apiV1, mcpHandler, mcpHandler.GetLocalHandler(), mcpHandler.GetTransportHandler())

	// Provider quota API routes — registered unconditionally (nil manager is
	// safe; Handler.available() answers 503 per request) so the routes and
	// their response models always appear in openapi.json regardless of
	// whether quota tracking happens to be configured on this build.
	quotaHandler := providerQuotaModule.NewHandler(s.quotaManager, logrus.StandardLogger())
	providerQuotaModule.RegisterRoutes(apiV1, quotaHandler)

	if s.desk = newDeskService(sm, s.config); s.desk != nil {
		registerDeskRoutes(apiV1, s.desk, statusHandler, s.deskEnabled)
	}

	// Static files and templates - try embedded assets first, fallback to filesystem
	UseWebStaticEndpoints(s.engine)
}

// deskSessionPoolConfig mirrors imbot's sessionPoolConfig
// (internal/server/module/imbot/manager.go) — same capacity/idle-timeout
// reasoning applies to a browser-driven Claude Code session as to a bot one.
var deskSessionPoolConfig = pool.Config{
	MaxSessions: 10,
	IdleTimeout: 10 * time.Minute,
}

// newDeskService builds the Desk service — a web front
// door onto the same remote/session + agentboot machinery @cc already
// drives, not a separate domain model (.design/desk.md). It builds
// its own control.Core, an independent in-memory session.Manager cache over
// the SAME session store @cc uses (sm.RemoteSessions()) — the sanctioned
// pattern every remote-host entry point follows (see remote/control.Core).
//
// Only the running server may call this: the service reconciles stored
// sessions on construction, so building one from schema generation would
// rewrite a live server's sessions. Returns nil if it cannot be built.
func newDeskService(sm *db.StoreManager, cfg *config.Config) *desksvc.Service {
	if sm == nil {
		return nil
	}
	maCore, err := control.NewCore(sm.RemoteSessions())
	if err != nil {
		logrus.WithError(err).Warn("Failed to create desk control core, desk APIs will not be available")
		return nil
	}
	return desksvc.NewService(desksvc.Config{
		Sessions: maCore.Session,
		Agent:    maCore.Agent,
		Routing:  tbclient.NewTBClient(cfg),
		Pool:     pool.New(deskSessionPoolConfig),
	})
}

// registerDeskRoutes is shared by UseUIEndpoints (with the live
// service) and registerAllAPIRoutes (schema generation, with nil) so the
// route set and its enabled gate cannot drift between the two paths.
func registerDeskRoutes(apiV1 *swagger.RouteGroup, svc *desksvc.Service, routes deskmodule.RouteResolver, enabled func() bool) {
	deskmodule.RegisterRoutes(apiV1, deskmodule.NewHandler(svc, routes), enabled)
}
