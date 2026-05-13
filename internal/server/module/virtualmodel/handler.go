// Package virtualmodel exposes management endpoints for the in-process
// virtual-model providers. The providers themselves are persisted in the
// shared ProviderStore (Source=builtin); this module only surfaces the
// registry contents so the frontend can render the Credentials sub-tab and
// any future configuration UI without re-implementing registry traversal.
package virtualmodel

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// Handler serves the virtual-model management endpoints.
type Handler struct {
	service *virtualserver.Service
}

// NewHandler returns a Handler backed by the given vmodel service.
func NewHandler(service *virtualserver.Service) *Handler {
	return &Handler{service: service}
}

// ListAvailableModels returns the union of models registered across the
// anthropic and openai protocol registries, tagged by protocol. The frontend
// uses this to populate the Credentials > Virtual Models sub-tab.
func (h *Handler) ListAvailableModels(c *gin.Context) {
	if h.service == nil {
		c.JSON(http.StatusOK, AvailableModelsResponse{Success: true, Data: nil})
		return
	}

	entries := make([]AvailableModelEntry, 0)
	for _, m := range h.service.GetAnthropicRegistry().ListModels() {
		entries = append(entries, AvailableModelEntry{ID: m.ID, Protocol: "anthropic"})
	}
	for _, m := range h.service.GetOpenAIRegistry().ListModels() {
		entries = append(entries, AvailableModelEntry{ID: m.ID, Protocol: "openai"})
	}

	c.JSON(http.StatusOK, AvailableModelsResponse{Success: true, Data: entries})
}
