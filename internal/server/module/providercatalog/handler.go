package providercatalog

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/catalog"
)

// Handler handles provider catalog HTTP requests
type Handler struct {
	catalogManager *catalog.ProviderCatalogManager
}

// NewHandler creates a new provider catalog handler
func NewHandler(catalogManager *catalog.ProviderCatalogManager) *Handler {
	return &Handler{
		catalogManager: catalogManager,
	}
}

// ListProviderCatalogs returns all provider templates
func (h *Handler) ListProviderCatalogs(c *gin.Context) {
	if h.catalogManager == nil {
		c.JSON(http.StatusInternalServerError, ProviderCatalogResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	templates := h.catalogManager.GetAllTemplates()
	version := h.catalogManager.GetVersion()

	c.JSON(http.StatusOK, ProviderCatalogResponse{
		Success: true,
		Data:    templates,
		Version: version,
	})
}

// GetProviderCatalog returns a single provider template by ID
func (h *Handler) GetProviderCatalog(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, ProviderCatalogResponse{
			Success: false,
			Message: "Template ID is required",
		})
		return
	}

	if h.catalogManager == nil {
		c.JSON(http.StatusInternalServerError, ProviderCatalogResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	template, err := h.catalogManager.GetTemplate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ProviderCatalogResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SingleProviderCatalogResponse{
		Success: true,
		Data:    template,
	})
}

// RefreshProviderCatalogs fetches the latest templates from GitHub
func (h *Handler) RefreshProviderCatalogs(c *gin.Context) {
	if h.catalogManager == nil {
		c.JSON(http.StatusInternalServerError, ProviderCatalogResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	registry, err := h.catalogManager.FetchTemplates(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProviderCatalogResponse{
			Success: false,
			Message: "Failed to refresh templates from GitHub: " + err.Error(),
		})
		return
	}

	// Serve the manager's merged view, not registry.Providers: the fetched
	// registry is pure remote content and may lack embedded-only templates.
	c.JSON(http.StatusOK, ProviderCatalogResponse{
		Success: true,
		Data:    h.catalogManager.GetAllTemplates(),
		Version: registry.Version,
		Message: "Templates refreshed successfully",
	})
}

// GetProviderCatalogVersion returns the current template registry version
func (h *Handler) GetProviderCatalogVersion(c *gin.Context) {
	if h.catalogManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Template manager not initialized",
		})
		return
	}

	version := h.catalogManager.GetVersion()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"version": version,
	})
}
