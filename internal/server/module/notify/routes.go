package notify

import "github.com/tingly-dev/tingly-box/internal/server/module"

var _ module.Module = (*Handler)(nil)

// RegisterRoutes registers notification hook routes
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	ccGroup := rt.Engine.Group("/tingly/:scenario")
	ccGroup.POST("/notify", h.Notify)
	ccGroup.GET("/wait/:request_id", h.Wait)
}
