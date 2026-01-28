package server

import (
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterConfigApplyRoutes registers the config apply API routes
func (s *Server) RegisterConfigApplyRoutes(manager *swagger.RouteManager) {
	// Create a group for config apply endpoints under /api/v1
	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.authMW.UserAuthMiddleware())

	// Config apply group
	configGroup := apiV1.Router.Group("/config/apply")

	// Safe handlers that generate config from system state
	configGroup.POST("/claude", s.ApplyClaudeConfig)
	configGroup.POST("/opencode", s.ApplyOpenCodeConfigFromState)

	// Config preview group - returns config for display without applying
	previewGroup := apiV1.Router.Group("/config/preview")
	previewGroup.GET("/opencode", s.GetOpenCodeConfigPreview)
}
