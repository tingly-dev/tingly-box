package virtualmodel

import (
	"github.com/gin-gonic/gin"
)

// Service manages the virtual model service
type Service struct {
	registry *Registry
	handler  *Handler
}

// NewService creates a new virtual model service
func NewService() *Service {
	registry := NewRegistry()
	registry.RegisterDefaults()

	handler := NewHandler(registry)

	return &Service{
		registry: registry,
		handler:  handler,
	}
}

// GetRegistry returns the model registry
func (s *Service) GetRegistry() *Registry {
	return s.registry
}

// GetHandler returns the HTTP handler
func (s *Service) GetHandler() *Handler {
	return s.handler
}

// RegisterModel registers a new virtual model
func (s *Service) RegisterModel(vm *VirtualModel) error {
	return s.registry.Register(vm)
}

// UnregisterModel unregisters a virtual model
func (s *Service) UnregisterModel(id string) {
	s.registry.Unregister(id)
}

// GetModel retrieves a virtual model by ID
func (s *Service) GetModel(id string) *VirtualModel {
	return s.registry.Get(id)
}

// ListModels returns all registered models
func (s *Service) ListModels() []Model {
	return s.registry.ListModels()
}

// SetupRoutes sets up the virtual model routes on a Gin router group
func (s *Service) SetupRoutes(group *gin.RouterGroup) {
	group.GET("/models", s.handler.ListModels)
	group.POST("/chat/completions", s.handler.ChatCompletions)
}
