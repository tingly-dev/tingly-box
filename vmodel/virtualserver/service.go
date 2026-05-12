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

// SetupRoutes sets up virtual model routes on a Gin router group.
func (s *Service) SetupRoutes(group *gin.RouterGroup) {
	group.GET("/models", s.handler.ListModels)
	group.POST("/chat/completions", s.handler.ChatCompletions)
	group.POST("/messages", s.handler.Messages)
}
