package server

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/codeximport"
	"github.com/tingly-dev/tingly-box/internal/server/module/configapply"
	debugmodule "github.com/tingly-dev/tingly-box/internal/server/module/debug"
	"github.com/tingly-dev/tingly-box/internal/server/module/imbot"
	mcpmodule "github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	notifymodule "github.com/tingly-dev/tingly-box/internal/server/module/notify"
	oauthmodule "github.com/tingly-dev/tingly-box/internal/server/module/oauth"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	usagemodule "github.com/tingly-dev/tingly-box/internal/server/module/usage"
	"github.com/tingly-dev/tingly-box/swagger"
)

// GenerateOpenAPI creates an OpenAPI v3 schema without starting the server
func GenerateOpenAPI(cfg *config.Config) (string, error) {
	// Set gin to release mode to suppress debug output
	gin.SetMode(gin.ReleaseMode)

	// Create a fresh gin engine
	engine := gin.New()

	// Create a minimal server instance for route registration
	server := &Server{
		engine: engine,
		config: cfg,
		// webHandler/guardrailsHandler need no live logger/token-manager
		// wiring for schema generation — their handlers are only referenced
		// (never invoked) here.
		webHandler: NewWebHandler(WebDeps{Config: cfg}),
	}
	server.guardrailsHandler = NewGuardrailsHandler(GuardrailsDeps{
		Config:             cfg,
		Runtime:            server,
		GuardrailsConfigMu: &server.guardrailsConfigMu,
	})

	// Create route manager
	manager := swagger.NewRouteManager(engine)

	// Register all routes using the same logic as the running server
	registerAllAPIRoutes(engine, manager, server, cfg)

	// Generate and return OpenAPI v3 JSON
	return manager.GenerateOpenAPI(swagger.VersionV3)
}

// registerAllAPIRoutes registers all API routes for both the running server and OpenAPI generation
// This is extracted from UseUIEndpoints to allow OpenAPI generation without starting the server
func registerAllAPIRoutes(engine *gin.Engine, manager *swagger.RouteManager, s *Server, cfg *config.Config) {
	// Claude Code status line endpoints (no auth required) - register from claudecode module
	quotaMgr := statusline.NewCache()
	statusHandler := statusline.NewHandler(cfg, nil, quotaMgr, nil)
	statusline.RegisterRoutes(engine, statusHandler)

	// Claude Code notification hook endpoint (no auth required)
	notifyHandler := notifymodule.NewHandler()
	notifymodule.RegisterRoutes(engine, notifyHandler)

	// Web API endpoints (uses the same method as the running server)
	s.UseWebAPIEndpoints(manager)

	// OAuth API routes - register from oauth module
	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.getUserAuthMiddleware())
	oauthmodule.RegisterRoutes(apiV1, s.getUserAuthMiddleware(), s.oauthHandler)
	// Register callback routes (unauthenticated)
	oauthmodule.RegisterCallbackRoutes(manager, s.oauthHandler)

	// Runtime memory diagnostics routes
	debugmodule.RegisterRoutes(apiV1, s.getUserAuthMiddleware(), debugmodule.NewHandler())

	// Usage API routes - register from usage module
	sm := cfg.StoreManager()
	if sm != nil {
		usageHandler := usagemodule.NewHandler(sm.Usage())
		usagemodule.RegisterRoutes(apiV1, usageHandler)
	}

	// ImBot settings API routes - register from imbotsettings module
	ctx := context.Background()
	imbotHandler, err := imbot.NewHandler(ctx, cfg)
	if err != nil {
		fmt.Printf("Failed to create imbotsettings handler: %v\n", err)
	} else {
		imbot.RegisterRoutes(apiV1, imbotHandler)
	}

	// Config apply API routes
	configapplyHandler := configapply.NewHandler(cfg, "")
	configapply.RegisterRoutes(apiV1, configapplyHandler)

	codexImportHandler := codeximport.NewHandler(nil, cfg)
	codeximport.RegisterRoutes(apiV1, codexImportHandler)

	// MCP runtime API routes
	mcpHandler := mcpmodule.NewHandler(cfg)
	mcpmodule.RegisterRoutes(apiV1, mcpHandler, mcpHandler.GetLocalHandler(), mcpHandler.GetTransportHandler())

	// Provider quota API routes
	// Note: skipped for OpenAPI generation as quotaManager is not available
}
