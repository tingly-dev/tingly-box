package providertemplate

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/data"
)

// Handler handles provider template HTTP requests
type Handler struct {
	templateManager *data.TemplateManager
}

// NewHandler creates a new provider template handler
func NewHandler(templateManager *data.TemplateManager) *Handler {
	return &Handler{
		templateManager: templateManager,
	}
}

// GetProviderTemplates returns all provider templates
func (h *Handler) GetProviderTemplates(c *gin.Context) {
	if h.templateManager == nil {
		c.JSON(http.StatusInternalServerError, TemplateResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	templates := h.templateManager.GetAllTemplates()
	version := h.templateManager.GetVersion()

	c.JSON(http.StatusOK, TemplateResponse{
		Success: true,
		Data:    templates,
		Version: version,
	})
}

// GetProviderTemplate returns a single provider template by ID
func (h *Handler) GetProviderTemplate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, TemplateResponse{
			Success: false,
			Message: "Template ID is required",
		})
		return
	}

	if h.templateManager == nil {
		c.JSON(http.StatusInternalServerError, TemplateResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	template, err := h.templateManager.GetTemplate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, TemplateResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SingleTemplateResponse{
		Success: true,
		Data:    template,
	})
}

// RefreshProviderTemplates fetches the latest templates from GitHub
func (h *Handler) RefreshProviderTemplates(c *gin.Context) {
	if h.templateManager == nil {
		c.JSON(http.StatusInternalServerError, TemplateResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	registry, err := h.templateManager.FetchFromGitHub(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, TemplateResponse{
			Success: false,
			Message: "Failed to refresh templates from GitHub: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, TemplateResponse{
		Success: true,
		Data:    registry.Providers,
		Version: registry.Version,
		Message: "Templates refreshed successfully",
	})
}

// GetProviderTemplateVersion returns the current template registry version
func (h *Handler) GetProviderTemplateVersion(c *gin.Context) {
	if h.templateManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Template manager not initialized",
		})
		return
	}

	version := h.templateManager.GetVersion()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"version": version,
	})
}
