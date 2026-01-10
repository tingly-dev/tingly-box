package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"tingly-box/internal/config/template"
)

// TemplateResponse represents the response for provider template endpoints
type TemplateResponse struct {
	Success bool                                  `json:"success"`
	Data    map[string]*template.ProviderTemplate `json:"data,omitempty"`
	Message string                                `json:"message,omitempty"`
	Version string                                `json:"version,omitempty"`
}

// SingleTemplateResponse represents the response for a single templatwe
type SingleTemplateResponse struct {
	Success bool                       `json:"success"`
	Data    *template.ProviderTemplate `json:"data,omitempty"`
	Message string                     `json:"message,omitempty"`
}

// GetProviderTemplates returns all provider templates
func (s *Server) GetProviderTemplates(c *gin.Context) {
	if s.templateManager == nil {
		c.JSON(http.StatusInternalServerError, TemplateResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	templates := s.templateManager.GetAllTemplates()
	version := s.templateManager.GetVersion()

	c.JSON(http.StatusOK, TemplateResponse{
		Success: true,
		Data:    templates,
		Version: version,
	})
}

// GetProviderTemplate returns a single provider template by ID
func (s *Server) GetProviderTemplate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, TemplateResponse{
			Success: false,
			Message: "Template ID is required",
		})
		return
	}

	if s.templateManager == nil {
		c.JSON(http.StatusInternalServerError, TemplateResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	template, err := s.templateManager.GetTemplate(id)
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
func (s *Server) RefreshProviderTemplates(c *gin.Context) {
	if s.templateManager == nil {
		c.JSON(http.StatusInternalServerError, TemplateResponse{
			Success: false,
			Message: "Template manager not initialized",
		})
		return
	}

	registry, err := s.templateManager.FetchFromGitHub(context.Background())
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
func (s *Server) GetProviderTemplateVersion(c *gin.Context) {
	if s.templateManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Template manager not initialized",
		})
		return
	}

	version := s.templateManager.GetVersion()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"version": version,
	})
}
