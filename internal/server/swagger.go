package server

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	guardrailsmodule "github.com/tingly-dev/tingly-box/internal/server/module/guardrails"
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
	server.guardrailsHandler = guardrailsmodule.NewHandler(guardrailsmodule.Deps{
		Config:             cfg,
		Runtime:            server,
		GuardrailsConfigMu: &server.guardrailsConfigMu,
	})

	// Create route manager
	manager := swagger.NewRouteManager(engine)

	// Register all routes using the same logic as the running server
	registerAllAPIRoutes(manager, server)

	// Generate and return OpenAPI v3 JSON
	return manager.GenerateOpenAPI(swagger.VersionV3)
}

// registerAllAPIRoutes registers all API routes for OpenAPI generation without
// starting the server. It mounts the same module set as the running server
// (see engineModules / apiModules).
func registerAllAPIRoutes(manager *swagger.RouteManager, s *Server) {
	engineMods, statusHandler := s.engineModules(true)
	module.Mount(&module.Routes{Engine: s.engine}, engineMods...)

	rt := s.UseWebAPIEndpoints(manager)
	module.Mount(rt, s.apiModules(context.Background(), true, statusHandler)...)
}
