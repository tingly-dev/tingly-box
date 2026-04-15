package notify

import "github.com/gin-gonic/gin"

// RegisterRoutes registers notification hook routes
func RegisterRoutes(engine *gin.Engine, handler *Handler) {
	ccGroup := engine.Group("/tingly/:scenario")
	ccGroup.POST("/notify", handler.Notify)
}
