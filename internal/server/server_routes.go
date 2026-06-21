package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	"github.com/tingly-dev/tingly-box/internal/typ"
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
	// DEPRECATED: now we only use path with scenario for openai and anthropic
	//// OpenAI v1 API group
	//openaiV1 := s.engine.Group("/openai/v1")
	//s.SetupOpenAIEndpoints(openaiV1)
	//
	//// OpenAI API alias (without version)
	//openai := s.engine.Group("/openai")
	//s.SetupOpenAIEndpoints(openai)
	//
	//// Anthropic v1 API group
	//anthropicV1 := s.engine.Group("/anthropic/v1")
	//s.SetupAnthropicEndpoints(anthropicV1)

	// Passthrough endpoints (no request/response transformation, just model replacement)
	// Non-versioned passthrough routes
	passthroughOpenai := s.engine.Group("/passthrough/openai")
	s.SetupPassthroughOpenAIEndpoints(passthroughOpenai)

	passthroughAnthropic := s.engine.Group("/passthrough/anthropic")
	s.SetupPassthroughAnthropicEndpoints(passthroughAnthropic)

	// Versioned passthrough routes
	passthroughOpenaiV1 := s.engine.Group("/passthrough/openai/v1")
	s.SetupPassthroughOpenAIEndpoints(passthroughOpenaiV1)

	// scenario routes with middleware to inject scenario into context.
	// profileAliasMiddleware runs first so it can rewrite a profile name alias
	// (e.g. "claude_code:mine") to its canonical ID form ("claude_code:p1")
	// before contextMiddleware validates and downstream stages consume it.
	scenario := s.engine.Group("/tingly/:scenario")
	scenario.Use(s.profileAliasMiddleware)
	scenario.Use(s.contextMiddleware)
	s.SetupMixinEndpoints(scenario)
	// Claude Code v2.1+ sends HEAD <ANTHROPIC_BASE_URL> as a connectivity
	// check before making any API call. Respond 200 so CC doesn't treat the
	// missing route as a server error and spiral into api_retry storms.
	scenario.HEAD("", func(c *gin.Context) { c.Status(http.StatusOK) })

	// scenario v1 routes with middleware
	scenarioV1 := s.engine.Group("/tingly/:scenario/v1")
	scenarioV1.Use(s.profileAliasMiddleware)
	scenarioV1.Use(s.contextMiddleware)
	s.SetupMixinEndpoints(scenarioV1)
	scenarioV1.HEAD("", func(c *gin.Context) { c.Status(http.StatusOK) })
}

func (s *Server) SetupMixinEndpoints(group *gin.RouterGroup) {
	// Chat completions endpoint (OpenAI compatible)
	group.POST("/chat/completions", s.getModelAuthMiddleware(), s.HandleOpenAIChatCompletions)

	// Responses API endpoints (OpenAI compatible)
	group.POST("/responses", s.getModelAuthMiddleware(), s.HandleResponsesCreate)
	group.GET("/responses/:id", s.getModelAuthMiddleware(), s.ResponsesGet)

	// Chat completions endpoint (Anthropic compatible)
	group.POST("/messages", s.getModelAuthMiddleware(), s.HandleAnthropicMessages)
	// Count tokens endpoint (Anthropic compatible)
	group.POST("/messages/count_tokens", s.getModelAuthMiddleware(), s.AnthropicCountTokens)

	// Embeddings endpoint (OpenAI compatible)
	group.POST("/embeddings", s.getModelAuthMiddleware(), s.HandleOpenAIEmbeddings)

	// Image generation endpoint (OpenAI compatible).
	// Routed directly to upstream POST /v1/images/generations; the Responses API
	// (POST /responses with the image_generation tool) is exposed in parallel via
	// the same scenario, with the caller choosing which surface to use.
	group.POST("/images/generations", s.getModelAuthMiddleware(), s.HandleOpenAIImageGeneration)

	// Models endpoint (routed by scenario: openai -> OpenAIListModels, anthropic/claude_code -> AnthropicListModels)
	group.GET("/models", s.getModelAuthMiddleware(), s.ListModelsByScenario)
}

func (s *Server) SetupOpenAIEndpoints(group *gin.RouterGroup) {
	// Chat completions endpoint (OpenAI compatible)
	group.POST("/chat/completions", s.getModelAuthMiddleware(), s.HandleOpenAIChatCompletions)
	// Models endpoint (OpenAI compatible)
	group.GET("/models", s.getModelAuthMiddleware(), s.HandleOpenAIListModels)

	// Responses API endpoints (OpenAI compatible)
	group.POST("/responses", s.getModelAuthMiddleware(), s.HandleResponsesCreate)
	group.GET("/responses/:id", s.getModelAuthMiddleware(), s.ResponsesGet)
}

func (s *Server) SetupAnthropicEndpoints(group *gin.RouterGroup) {
	// Chat completions endpoint (Anthropic compatible)
	group.POST("/messages", s.getModelAuthMiddleware(), s.HandleAnthropicMessages)
	// Count tokens endpoint (Anthropic compatible)
	group.POST("/messages/count_tokens", s.getModelAuthMiddleware(), s.AnthropicCountTokens)
	// Models endpoint (Anthropic compatible)
	group.GET("/models", s.getModelAuthMiddleware(), s.HandleAnthropicListModels)
}

// SetupPassthroughOpenAIEndpoints sets up pass-through endpoints for OpenAI-style requests
// These endpoints bypass request/response transformations and only replace the model name
func (s *Server) SetupPassthroughOpenAIEndpoints(group *gin.RouterGroup) {
	// POST endpoints that use passthrough (proxy with model replacement)
	group.POST("/chat/completions", s.getModelAuthMiddleware(), s.PassthroughOpenAI)
	group.POST("/responses", s.getModelAuthMiddleware(), s.PassthroughOpenAI)
	// GET responses/:id also uses passthrough
	group.GET("/responses/*path", s.getModelAuthMiddleware(), s.PassthroughOpenAI)
	// Models endpoint returns tingly-box's model list (not passthrough)
	group.GET("/models", s.getModelAuthMiddleware(), s.HandleOpenAIListModels)
}

// contextMiddleware is a middleware that extracts the scenario parameter from the URL path
// and injects it into the request context for use by downstream components (e.g., RecordRoundTripper).
// It also validates profile suffixes (e.g., "claude_code:p1") if present.
func (s *Server) contextMiddleware(c *gin.Context) {
	rawScenario := c.Param("scenario")
	ctx := context.WithValue(c.Request.Context(), client.ScenarioContextKey, rawScenario)
	c.Request = c.Request.WithContext(ctx)

	// Validate profile if present (e.g., "claude_code:p1")
	if typ.IsProfiledScenario(typ.RuleScenario(rawScenario)) {
		base, profileID := typ.ParseScenarioProfile(typ.RuleScenario(rawScenario))
		if base == "" || profileID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("invalid scenario format: '%s'", rawScenario),
			})
			c.Abort()
			return
		}

		// Check base scenario exists in registry
		if _, ok := typ.GetScenarioDescriptor(typ.RuleScenario(rawScenario)); !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("unknown scenario '%s'", base),
			})
			c.Abort()
			return
		}

		// Check profile exists in config
		if s.config != nil {
			if _, ok := s.config.GetProfile(typ.RuleScenario(base), profileID); !ok {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   fmt.Sprintf("unknown profile '%s' for scenario '%s'", profileID, base),
				})
				c.Abort()
				return
			}
		}
	}

	c.Next()
}

// SetupPassthroughAnthropicEndpoints sets up pass-through endpoints for Anthropic-style requests
// These endpoints bypass request/response transformations and only replace the model name
func (s *Server) SetupPassthroughAnthropicEndpoints(group *gin.RouterGroup) {
	// POST endpoints that use passthrough (proxy with model replacement)
	group.POST("/messages", s.getModelAuthMiddleware(), s.PassthroughAnthropic)
	group.POST("/messages/count_tokens", s.getModelAuthMiddleware(), s.PassthroughAnthropic)
	// Models endpoint returns tingly-box's model list (not passthrough)
	group.GET("/models", s.getModelAuthMiddleware(), s.HandleAnthropicListModels)
}

// UseVirtualModelEndpoints sets up the direct virtual-model entrypoints,
// split per protocol:
//
//	/virtual/openai/v1/{models,chat/completions}
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

// UseTokenManagementEndpoints registers the token management API endpoints
func (s *Server) UseTokenManagementEndpoints() {
	// API routes for token management
	api := s.engine.Group("/api/v1")
	api.Use(s.getUserAuthMiddleware()) // Require user authentication for management APIs

	// Token management API routes
	s.registerTokenManagementAPI(api)
}
