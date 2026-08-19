package protocolserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/middleware"
)

// RegisterRoutes mounts the LLM gateway surface (/tingly/:scenario[/v1]/...)
// onto engine. modelAuth gates every model endpoint (model API-token trust
// domain — distinct from the host's user/admin auth).
//
// The host server owns the gin engine and the auth middleware; this package
// owns the route shape and the handlers behind it.
func (ph *ProtocolHandler) RegisterRoutes(engine *gin.Engine, modelAuth gin.HandlerFunc) {
	// scenario routes with middleware to inject scenario into context.
	// legacyScenarioAliasMiddleware runs first so a deprecated scenario id
	// (e.g. "agent" -> "custom") is rewritten before profileAliasMiddleware
	// parses any profile suffix on it. profileAliasMiddleware then rewrites a
	// profile name alias (e.g. "claude_code:mine") to its canonical ID form
	// ("claude_code:p1")
	// before contextMiddleware validates and downstream stages consume it.
	scenario := engine.Group("/tingly/:scenario")
	scenario.Use(middleware.ClearServerIOTimeouts())
	scenario.Use(ph.legacyScenarioAliasMiddleware)
	scenario.Use(ph.profileAliasMiddleware)
	scenario.Use(ph.contextMiddleware)
	// tracingMiddleware runs after profileAliasMiddleware so the span's
	// scenario reflects the canonical "base:pN" form, matching usage records.
	scenario.Use(ph.tracingMiddleware)
	ph.SetupMixinEndpoints(scenario, modelAuth)
	// Claude Code v2.1+ sends HEAD <ANTHROPIC_BASE_URL> as a connectivity
	// check before making any API call. Respond 200 so CC doesn't treat the
	// missing route as a server error and spiral into api_retry storms.
	scenario.HEAD("", func(c *gin.Context) { c.Status(http.StatusOK) })

	// scenario v1 routes with middleware
	scenarioV1 := engine.Group("/tingly/:scenario/v1")
	scenarioV1.Use(middleware.ClearServerIOTimeouts())
	scenarioV1.Use(ph.legacyScenarioAliasMiddleware)
	scenarioV1.Use(ph.profileAliasMiddleware)
	scenarioV1.Use(ph.contextMiddleware)
	scenarioV1.Use(ph.tracingMiddleware)
	ph.SetupMixinEndpoints(scenarioV1, modelAuth)
	scenarioV1.HEAD("", func(c *gin.Context) { c.Status(http.StatusOK) })
}

// SetupMixinEndpoints registers the protocol-mixed endpoint set (OpenAI +
// Anthropic surfaces on one group), each gated by modelAuth.
func (ph *ProtocolHandler) SetupMixinEndpoints(group *gin.RouterGroup, modelAuth gin.HandlerFunc) {
	// Chat completions endpoint (OpenAI compatible)
	group.POST("/chat/completions", modelAuth, ph.teamScopeMiddleware, ph.HandleOpenAIChatCompletions)

	// Responses API endpoints (OpenAI compatible)
	group.POST("/responses", modelAuth, ph.teamScopeMiddleware, ph.HandleResponsesCreate)
	group.GET("/responses/:id", modelAuth, ph.teamScopeMiddleware, ph.HandleResponsesGet)

	// Chat completions endpoint (Anthropic compatible)
	group.POST("/messages", modelAuth, ph.teamScopeMiddleware, ph.HandleAnthropicMessages)
	// Count tokens endpoint (Anthropic compatible)
	group.POST("/messages/count_tokens", modelAuth, ph.teamScopeMiddleware, ph.AnthropicCountTokens)

	// Embeddings endpoint (OpenAI compatible)
	group.POST("/embeddings", modelAuth, ph.teamScopeMiddleware, DeclareOperation("embeddings"), ph.HandleOpenAIEmbeddings)

	// Image generation endpoint (OpenAI compatible).
	// Routed directly to upstream POST /v1/images/generations; the Responses API
	// (POST /responses with the image_generation tool) is exposed in parallel via
	// the same scenario, with the caller choosing which surface to use.
	group.POST("/images/generations", modelAuth, ph.teamScopeMiddleware, DeclareOperation("image_generation"), ph.HandleOpenAIImageGeneration)

	// Models endpoint (routed by scenario: openai -> OpenAIListModels, anthropic/claude_code -> AnthropicListModels)
	group.GET("/models", modelAuth, ph.teamScopeMiddleware, ph.ListModelsByScenario)
}

// SetupOpenAIEndpoints registers the OpenAI-only endpoint set on group.
func (ph *ProtocolHandler) SetupOpenAIEndpoints(group *gin.RouterGroup, modelAuth gin.HandlerFunc) {
	// Chat completions endpoint (OpenAI compatible)
	group.POST("/chat/completions", modelAuth, ph.HandleOpenAIChatCompletions)
	// Models endpoint (OpenAI compatible)
	group.GET("/models", modelAuth, ph.HandleOpenAIListModels)

	// Responses API endpoints (OpenAI compatible)
	group.POST("/responses", modelAuth, ph.HandleResponsesCreate)
	group.GET("/responses/:id", modelAuth, ph.HandleResponsesGet)
}

// SetupAnthropicEndpoints registers the Anthropic-only endpoint set on group.
func (ph *ProtocolHandler) SetupAnthropicEndpoints(group *gin.RouterGroup, modelAuth gin.HandlerFunc) {
	// Chat completions endpoint (Anthropic compatible)
	group.POST("/messages", modelAuth, ph.HandleAnthropicMessages)
	// Count tokens endpoint (Anthropic compatible)
	group.POST("/messages/count_tokens", modelAuth, ph.AnthropicCountTokens)
	// Models endpoint (Anthropic compatible)
	group.GET("/models", modelAuth, ph.HandleAnthropicListModels)
}
