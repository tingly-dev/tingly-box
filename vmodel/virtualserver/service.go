package virtualserver

import (
	"github.com/gin-gonic/gin"

	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
)

// Service manages the virtual model service.
//
// Each protocol has its own registry. A model registered in the Anthropic
// registry is callable only via /messages; a model in the OpenAI registry
// is callable only via /chat/completions. ID collisions across the two
// registries are intentional and legal — the registry IS the protocol context.
type Service struct {
	anthropicReg *anthropicvm.Registry
	openaiReg    *openaivm.Registry
	handler      *Handler
}

// NewService creates a fully initialized Service with default models registered
// in both protocol registries.
func NewService() *Service {
	a := anthropicvm.NewRegistry()
	anthropicvm.RegisterDefaults(a)

	o := openaivm.NewRegistry()
	openaivm.RegisterDefaults(o)

	return &Service{
		anthropicReg: a,
		openaiReg:    o,
		handler:      NewHandler(a, o),
	}
}

// GetAnthropicRegistry returns the Anthropic-protocol model registry.
func (s *Service) GetAnthropicRegistry() *anthropicvm.Registry { return s.anthropicReg }

// GetOpenAIRegistry returns the OpenAI Chat-protocol model registry.
func (s *Service) GetOpenAIRegistry() *openaivm.Registry { return s.openaiReg }

// GetHandler returns the HTTP handler.
func (s *Service) GetHandler() *Handler {
	return s.handler
}

// SetupRoutes mounts virtual-model endpoints on a single mixed-protocol
// group at <group>/{models,chat/completions,messages}.
//
// Deprecated: prefer SetupOpenAIRoutes + SetupAnthropicRoutes so OpenAI and
// Anthropic clients each see only their own model IDs. Retained for test
// fixtures.
func (s *Service) SetupRoutes(group *gin.RouterGroup) {
	group.GET("/models", s.handler.ListModels)
	group.POST("/chat/completions", s.handler.ChatCompletions)
	group.POST("/responses", s.handler.Responses)
	group.POST("/messages", s.handler.Messages)
}

// SetupOpenAIRoutes mounts the OpenAI-only entrypoints on the given group.
// Typical wiring:
//
//	openai := engine.Group("/virtual/openai")
//	svc.SetupOpenAIRoutes(openai)
//
// This produces /virtual/openai/v1/models and
// /virtual/openai/v1/chat/completions — drop-in compatible with the OpenAI
// SDK base URL convention.
func (s *Service) SetupOpenAIRoutes(group *gin.RouterGroup) {
	v1 := group.Group("/v1")
	v1.GET("/models", s.handler.ListOpenAIModels)
	v1.POST("/chat/completions", s.handler.ChatCompletions)
	v1.POST("/responses", s.handler.Responses)
}

// SetupAnthropicRoutes mounts the Anthropic-only entrypoints on the given
// group. Typical wiring:
//
//	anthropic := engine.Group("/virtual/anthropic")
//	svc.SetupAnthropicRoutes(anthropic)
//
// This produces /virtual/anthropic/v1/models and
// /virtual/anthropic/v1/messages — drop-in compatible with the Anthropic
// SDK base URL convention.
func (s *Service) SetupAnthropicRoutes(group *gin.RouterGroup) {
	v1 := group.Group("/v1")
	v1.GET("/models", s.handler.ListAnthropicModels)
	v1.POST("/messages", s.handler.Messages)
}
