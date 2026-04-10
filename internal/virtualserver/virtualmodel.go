package virtualserver

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// HandlerInterface is satisfied by virtualserver.Handler and allows virtualmodel
// to reference the handler without a circular import.
type HandlerInterface interface {
	ListModels(c *gin.Context)
	ChatCompletions(c *gin.Context)
	Messages(c *gin.Context)
}

// Service manages the virtual model service.
type Service struct {
	registry *virtualmodel.Registry
	handler  HandlerInterface
}

// NewService creates a new Service. Call SetHandler after constructing
// the virtualserver.Handler to complete wiring.
func NewService() *Service {
	registry := virtualmodel.NewRegistry()
	registry.RegisterDefaults()
	return &Service{registry: registry}
}

// SetHandler wires the HTTP handler (virtualserver.Handler) into the service.
func (s *Service) SetHandler(h HandlerInterface) {
	s.handler = h
}

// GetRegistry returns the model registry.
func (s *Service) GetRegistry() *virtualmodel.Registry {
	return s.registry
}

// GetHandler returns the HTTP handler.
func (s *Service) GetHandler() HandlerInterface {
	return s.handler
}

// RegisterModel registers a new virtual model.
func (s *Service) RegisterModel(vm *virtualmodel.VirtualModel) error {
	return s.registry.Register(vm)
}

// UnregisterModel unregisters a virtual model.
func (s *Service) UnregisterModel(id string) {
	s.registry.Unregister(id)
}

// GetModel retrieves a virtual model by ID.
func (s *Service) GetModel(id string) *virtualmodel.VirtualModel {
	return s.registry.Get(id)
}

// ListModels returns all registered models.
func (s *Service) ListModels() []virtualmodel.Model {
	return s.registry.ListModels()
}

// SetupRoutes sets up virtual model routes on a Gin router group.
// Panics if SetHandler has not been called.
func (s *Service) SetupRoutes(group *gin.RouterGroup) {
	if s.handler == nil {
		panic("virtualmodel: SetupRoutes called before SetHandler")
	}
	group.GET("/models", s.handler.ListModels)
	group.POST("/chat/completions", s.handler.ChatCompletions)
	group.POST("/messages", s.handler.Messages)
}
