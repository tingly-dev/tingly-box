// Package uiprefs serves the UI-only preference store (config.UIPrefs):
// small per-instance settings the frontend keeps server-side so a browser
// tab and the desktop window see the same state.
package uiprefs

import (
	"encoding/json"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/swagger"
)

// UIPrefsResponse carries every stored UI preference.
type UIPrefsResponse struct {
	Success bool                       `json:"success"`
	Prefs   map[string]json.RawMessage `json:"prefs,omitempty"`
	Error   string                     `json:"error,omitempty"`
}

// PatchUIPrefsRequest merges keys into the store; a null value deletes a key.
type PatchUIPrefsRequest struct {
	Prefs map[string]json.RawMessage `json:"prefs" binding:"required"`
}

// Handler serves the UI preference endpoints.
type Handler struct {
	config *config.Config
}

// NewHandler creates a UI preference handler.
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{config: cfg}
}

// Get returns all UI preferences.
func (h *Handler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, UIPrefsResponse{Success: true, Prefs: h.config.GetUIPrefs()})
}

// Patch merges the request's prefs into the store and returns the result.
func (h *Handler) Patch(c *gin.Context) {
	var req PatchUIPrefsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, UIPrefsResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	prefs, err := h.config.PatchUIPrefs(req.Prefs)
	if err != nil {
		c.JSON(http.StatusBadRequest, UIPrefsResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, UIPrefsResponse{Success: true, Prefs: prefs})
}

// RegisterRoutes registers the UI preference routes.
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	router.GET("/ui-prefs", h.Get,
		swagger.WithTags("ui-prefs"),
		swagger.WithDescription("Get the UI-only preferences shared by every UI surface (browser and desktop)"),
		swagger.WithResponseModel(UIPrefsResponse{}),
	)
	router.PATCH("/ui-prefs", h.Patch,
		swagger.WithTags("ui-prefs"),
		swagger.WithDescription("Merge UI preferences; a null value deletes the key"),
		swagger.WithRequestModel(PatchUIPrefsRequest{}),
		swagger.WithResponseModel(UIPrefsResponse{}),
	)
}
