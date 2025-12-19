package server

import (
	"fmt"
	"net/http"
	"tingly-box/internal/config"
	"tingly-box/internal/obs"

	"github.com/gin-gonic/gin"
)

func (s *Server) GetProviders(c *gin.Context) {
	providers := s.config.ListProviders()

	// Mask tokens for security
	maskedProviders := make([]ProviderResponse, len(providers))

	for i, provider := range providers {
		maskedProviders[i] = ProviderResponse{
			Name:     provider.Name,
			APIBase:  provider.APIBase,
			APIStyle: string(provider.APIStyle),
			Token:    maskToken(provider.Token),
			Enabled:  provider.Enabled,
		}
	}

	response := ProvidersResponse{
		Success: true,
		Data:    maskedProviders,
	}

	c.JSON(http.StatusOK, response)
}

// AddProvider adds a new provider
func (s *Server) AddProvider(c *gin.Context) {
	var req AddProviderRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Set default enabled status if not provided
	if !req.Enabled {
		req.Enabled = true
	}

	// Set default API style if not provided
	if req.APIStyle == "" {
		req.APIStyle = "openai"
	}

	provider := &config.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: config.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  req.Enabled,
	}

	err := s.config.AddProvider(provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionAddProvider, map[string]interface{}{
				"name":     req.Name,
				"api_base": req.APIBase,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if s.logger != nil {
		s.logger.LogAction(obs.ActionAddProvider, map[string]interface{}{
			"name":     req.Name,
			"api_base": req.APIBase,
		}, true, fmt.Sprintf("Provider %s added successfully", req.Name))
	}

	response := AddProviderResponse{
		Success: true,
		Message: "Provider added successfully",
		Data:    provider,
	}

	c.JSON(http.StatusOK, response)
}

// DeleteProvider removes a provider
func (s *Server) DeleteProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	err := s.config.DeleteProvider(providerName)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionDeleteProvider, map[string]interface{}{
				"name": providerName,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if s.logger != nil {
		s.logger.LogAction(obs.ActionDeleteProvider, map[string]interface{}{
			"name": providerName,
		}, true, fmt.Sprintf("Provider %s deleted successfully", providerName))
	}

	response := DeleteProviderResponse{
		Success: true,
		Message: "Provider deleted successfully",
	}

	c.JSON(http.StatusOK, response)
}

// UpdateProvider updates an existing provider
func (s *Server) UpdateProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	var req UpdateProviderRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Get existing provider
	provider, err := s.config.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Update fields if provided
	if req.Name != nil {
		provider.Name = *req.Name
	}
	if req.APIBase != nil {
		provider.APIBase = *req.APIBase
	}
	if req.APIStyle != nil {
		provider.APIStyle = config.APIStyle(*req.APIStyle)
	}
	// Only update token if it's provided and not empty
	if req.Token != nil && *req.Token != "" {
		provider.Token = *req.Token
	}
	if req.Enabled != nil {
		provider.Enabled = *req.Enabled
	}

	err = s.config.UpdateProvider(providerName, provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
				"name":    providerName,
				"updates": req,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if s.logger != nil {
		s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
			"name": providerName,
		}, true, fmt.Sprintf("Provider %s updated successfully", providerName))
	}

	// Return masked provider data
	responseProvider := ProviderResponse{
		Name:     provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
		Token:    maskToken(provider.Token),
		Enabled:  provider.Enabled,
	}

	response := UpdateProviderResponse{
		Success: true,
		Message: "Provider updated successfully",
		Data:    responseProvider,
	}

	c.JSON(http.StatusOK, response)
}

// GetProvider returns details for a specific provider (with masked token)
func (s *Server) GetProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	provider, err := s.config.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Mask the token for security
	responseProvider := ProviderResponse{
		Name:     provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
		Token:    provider.Token, // Security: Token:    maskToken(provider.Token),
		Enabled:  provider.Enabled,
	}

	response := struct {
		Success bool             `json:"success"`
		Data    ProviderResponse `json:"data"`
	}{
		Success: true,
		Data:    responseProvider,
	}

	c.JSON(http.StatusOK, response)
}

// ToggleProvider enables/disables a provider
func (s *Server) ToggleProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	provider, err := s.config.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Toggle enabled status
	provider.Enabled = !provider.Enabled

	err = s.config.UpdateProvider(providerName, provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
				"name":    providerName,
				"enabled": provider.Enabled,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	action := "disabled"
	if provider.Enabled {
		action = "enabled"
	}

	if s.logger != nil {
		s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
			"name":    providerName,
			"enabled": provider.Enabled,
		}, true, fmt.Sprintf("Provider %s %s successfully", providerName, action))
	}

	response := ToggleProviderResponse{
		Success: true,
		Message: fmt.Sprintf("Provider %s %s successfully", providerName, action),
	}
	response.Data.Enabled = provider.Enabled

	c.JSON(http.StatusOK, response)
}

func (s *Server) FetchProviderModels(c *gin.Context) {
	providerName := c.Param("name")

	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	// Fetch and save models
	err := s.config.FetchAndSaveProviderModels(providerName)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
				"provider": providerName,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, FetchProviderModelsResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to fetch models from provider %s: %s", providerName, err.Error()),
			Data:    nil,
		})
		return
	}

	// Get the updated models
	modelManager := s.config.GetModelManager()
	models := modelManager.GetModels(providerName)

	if s.logger != nil {
		s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
			"provider":     providerName,
			"models_count": len(models),
		}, true, fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), providerName))
	}

	response := FetchProviderModelsResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), providerName),
		Data:    models,
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) GetProviderModels(c *gin.Context) {
	providerModelManager := s.config.GetModelManager()
	if providerModelManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Provider model manager not available",
		})
		return
	}

	providers := providerModelManager.GetAllProviders()
	providerModels := make(map[string]*ProviderModelInfo)

	for _, providerName := range providers {
		models := providerModelManager.GetModels(providerName)
		apiBase, lastUpdated, _ := providerModelManager.GetProviderInfo(providerName)

		providerModels[providerName] = &ProviderModelInfo{
			Models:      models,
			APIBase:     apiBase,
			LastUpdated: lastUpdated,
		}
	}

	response := ProviderModelsResponse{
		Success: true,
		Data:    providerModels,
	}

	c.JSON(http.StatusOK, response)
}
