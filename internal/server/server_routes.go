package server

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	sharing "github.com/tingly-dev/tingly-box/internal/server/module/sharing"
	team "github.com/tingly-dev/tingly-box/internal/server/module/team"
	"github.com/tingly-dev/tingly-box/swagger"
)

// setupMiddleware configures server middleware
func (s *Server) setupMiddleware() {
	// Recovery middleware
	s.engine.Use(gin.Recovery())

	// Memory log middleware for HTTP request logging
	if s.memoryLogMW != nil {
		s.engine.Use(s.memoryLogMW.Middleware())
	}

	// CORS middleware
	s.engine.Use(middleware.CORS())
}

// setupRoutes configures server routes
func (s *Server) setupRoutes(ctx context.Context) {

	s.UseAIEndpoints()

	s.UseLoadBalanceEndpoints()

	// Multi-tenant token management API
	s.UseTokenManagementEndpoints()

	// Virtual model endpoints for testing
	s.UseVirtualModelEndpoints()

	// Integrate Web UI routes if enabled
	if s.enableUI {
		s.UseUIEndpoints(ctx)
	}
}

func (s *Server) UseAIEndpoints() {
	// The gateway route shape (/tingly/:scenario[/v1]/...) is owned by
	// protocolserver; the host only supplies the engine and model auth.
	s.aiHandler.RegisterRoutes(s.engine, s.getModelAuthMiddleware())
}

// UseVirtualModelEndpoints sets up the direct virtual-model entrypoints,
// split per protocol:
//
//	/virtual/openai/v1/{models,chat/completions,responses}
//	/virtual/anthropic/v1/{models,messages}
//
// These bypass the provider/rule/scenario pipeline and call the in-process
// handler directly — useful when a client wants a fixed URL pointed at the
// vmodel registry without configuring a provider. The protocol split
// ensures /models returns only the model IDs the chosen protocol can
// actually dispatch.
//
// The canonical path for virtual models in normal use is still
// /v1/messages and /v1/chat/completions, where the dispatcher
// short-circuits to the same handler when it resolves to a vmodel provider
// (see HandleAnthropicMessages and HandleOpenAIChatCompletions).
func (s *Server) UseVirtualModelEndpoints() {
	mw := s.getModelAuthMiddleware()

	openai := s.engine.Group("/virtual/openai")
	openai.Use(mw)
	s.virtualModelService.SetupOpenAIRoutes(openai)

	anthropic := s.engine.Group("/virtual/anthropic")
	anthropic.Use(mw)
	s.virtualModelService.SetupAnthropicRoutes(anthropic)
}

func (s *Server) UseLoadBalanceEndpoints() {
	// API routes for load balancer management
	api := s.engine.Group("/api/v1/load-balancer")
	api.Use(s.getUserAuthMiddleware()) // Require user authentication for management APIs

	// Load balancer API routes
	s.loadBalancerAPI.RegisterRoutes(api)
}

// UseTokenManagementEndpoints registers the token management API endpoints.
func (s *Server) UseTokenManagementEndpoints() {
	if s.config == nil {
		return
	}
	sm := s.config.StoreManager()
	if sm == nil {
		return
	}
	store := sm.APIToken()
	if store == nil {
		return
	}

	manager := swagger.NewRouteManager(s.engine)
	api := manager.NewGroup("api", "v1", "")
	api.Router.Use(s.getUserAuthMiddleware())
	sharing.RegisterRoutes(api, sharing.NewHandler(store))
	team.RegisterRoutes(api, team.NewHandler(sm.Team()))
}
