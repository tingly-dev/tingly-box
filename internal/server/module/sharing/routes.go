package sharing

import "github.com/gin-gonic/gin"

// RegisterRoutes wires all token management endpoints onto the given group.
func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	tokens := group.Group("/tokens")

	tokens.POST("", h.Create)
	tokens.GET("", h.List)
	tokens.GET("/:token_id", h.Get)
	tokens.DELETE("/:token_id", h.Delete)
	tokens.PUT("/:token_id/enable", h.Enable)
	tokens.PUT("/:token_id/disable", h.Disable)
	tokens.POST("/:token_id/regenerate", h.Regenerate)
}
