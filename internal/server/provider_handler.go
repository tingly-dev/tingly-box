package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tingly-box/internal/obs"
	"tingly-box/internal/typ"
)

// maskProviderForResponse masks sensitive data and returns a safe ProviderResponse
func maskProviderForResponse(provider *typ.Provider) ProviderResponse {
	resp := ProviderResponse{
		UUID:          provider.UUID,
		Name:          provider.Name,
		APIBase:       provider.APIBase,
		APIStyle:      string(provider.APIStyle),
		NoKeyRequired: provider.NoKeyRequired,
		Enabled:       provider.Enabled,
		ProxyURL:      provider.ProxyURL,
		AuthType:      string(provider.AuthType),
	}

	switch provider.AuthType {
	case typ.AuthTypeOAuth:
		// For OAuth, return masked OAuthDetail
		if provider.OAuthDetail != nil {
			resp.OAuthDetail = &typ.OAuthDetail{
				//AccessToken:  maskToken(provider.OAuthDetail.AccessToken),
				AccessToken:  provider.OAuthDetail.AccessToken,
				RefreshToken: provider.OAuthDetail.RefreshToken,
				ProviderType: provider.OAuthDetail.ProviderType,
				UserID:       provider.OAuthDetail.UserID,
				ExpiresAt:    provider.OAuthDetail.ExpiresAt,
				// Don't return refresh_token in responses
			}
		}
	case typ.AuthTypeAPIKey, "":
		// For api_key (or empty for backward compatibility), return masked Token
		//resp.Token = maskToken(provider.Token)
		resp.Token = provider.Token
	}

	return resp
}

func (s *Server) GetProviders(c *gin.Context) {
	providers := s.config.ListProviders()

	// Mask tokens for security
	maskedProviders := make([]ProviderResponse, len(providers))

	for i, provider := range providers {
		maskedProviders[i] = maskProviderForResponse(provider)
	}

	response := ProvidersResponse{
		Success: true,
		Data:    maskedProviders,
	}

	c.JSON(http.StatusOK, response)
}

// CreateProvider adds a new provider
func (s *Server) CreateProvider(c *gin.Context) {
	forceParam := c.Query("force")
	force := forceParam == "true"

	var req CreateProviderRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Custom validation: token is required unless NoKeyRequired is true
	if !req.NoKeyRequired && req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Token is required when No Key Required is false",
		})
		return
	}

	// Backend verification: Verify provider connection before saving (skip if no key required)
	// This is a safety measure in addition to frontend verification
	if !req.NoKeyRequired && req.Token != "" {
		probeReq := &ProbeProviderRequest{
			Name:     req.Name,
			APIBase:  req.APIBase,
			APIStyle: req.APIStyle,
			Token:    req.Token,
		}

		if !force {
			success, message, _, err := s.testProviderConnectivity(probeReq)
			if err != nil || !success {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   fmt.Sprintf("Provider verification failed: %s", message),
				})
				return
			}
		}
	}

	// Set default enabled status if not provided
	if !req.Enabled {
		req.Enabled = true
	}

	// Set default API style if not provided
	if req.APIStyle == "" {
		req.APIStyle = "openai"
	}

	uid, err := uuid.NewUUID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, CreateProviderResponse{
			Success: false,
			Message: "Provider UUID generate failed: " + err.Error(),
		})
		return
	}
	provider := &typ.Provider{
		UUID:          uid.String(),
		Name:          req.Name,
		APIBase:       req.APIBase,
		APIStyle:      typ.APIStyle(req.APIStyle),
		Token:         req.Token,
		NoKeyRequired: req.NoKeyRequired,
		Enabled:       req.Enabled,
		ProxyURL:      req.ProxyURL,
	}

	err = s.config.AddProvider(provider)
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

	// update models for current provider here too, try once and ignore error
	s.config.FetchAndSaveProviderModels(provider.UUID)

	if s.logger != nil {
		s.logger.LogAction(obs.ActionAddProvider, map[string]interface{}{
			"name":     req.Name,
			"api_base": req.APIBase,
		}, true, fmt.Sprintf("Provider %s added successfully", req.Name))
	}

	response := CreateProviderResponse{
		Success: true,
		Message: "Provider added successfully",
		Data:    provider,
	}

	c.JSON(http.StatusOK, response)
}

// DeleteProvider removes a provider
func (s *Server) DeleteProvider(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	err := s.config.DeleteProvider(uid)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionDeleteProvider, map[string]interface{}{
				"name": uid,
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
			"name": uid,
		}, true, fmt.Sprintf("Provider %s deleted successfully", uid))
	}

	response := DeleteProviderResponse{
		Success: true,
		Message: "Provider deleted successfully",
	}

	c.JSON(http.StatusOK, response)
}

// UpdateProvider updates an existing provider
func (s *Server) UpdateProvider(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
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

	// check existing
	if req.Name != nil {
		name := *req.Name
		existed, err := s.config.GetProviderByName(name)
		if err == nil && uid != existed.UUID {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   fmt.Sprintf("provider with name '%s' already exists", name),
			})
			return
		}
	}

	// Get existing provider
	provider, err := s.config.GetProviderByUUID(uid)
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
		provider.APIStyle = typ.APIStyle(*req.APIStyle)
	}
	// Only update token if it's provided and not empty
	if req.Token != nil && *req.Token != "" {
		provider.Token = *req.Token
	}
	if req.NoKeyRequired != nil {
		provider.NoKeyRequired = *req.NoKeyRequired
	}
	if req.Enabled != nil {
		provider.Enabled = *req.Enabled
	}
	if req.ProxyURL != nil {
		provider.ProxyURL = *req.ProxyURL
	}

	err = s.config.UpdateProvider(uid, provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
				"name":    uid,
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
			"name": uid,
		}, true, fmt.Sprintf("Provider %s updated successfully", uid))
	}

	// Return masked provider data
	responseProvider := maskProviderForResponse(provider)

	response := UpdateProviderResponse{
		Success: true,
		Message: "Provider updated successfully",
		Data:    responseProvider,
	}

	c.JSON(http.StatusOK, response)
}

// GetProvider returns details for a specific provider (with masked token)
func (s *Server) GetProvider(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	provider, err := s.config.GetProviderByUUID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Mask the token for security
	responseProvider := maskProviderForResponse(provider)

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
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	provider, err := s.config.GetProviderByUUID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Toggle enabled status
	provider.Enabled = !provider.Enabled

	err = s.config.UpdateProvider(uid, provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
				"name":    uid,
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
			"name":    uid,
			"enabled": provider.Enabled,
		}, true, fmt.Sprintf("Provider %s %s successfully", uid, action))
	}

	response := ToggleProviderResponse{
		Success: true,
		Message: fmt.Sprintf("Provider %s %s successfully", uid, action),
	}
	response.Data.Enabled = provider.Enabled

	c.JSON(http.StatusOK, response)
}

func (s *Server) UpdateProviderModelsByUUID(c *gin.Context) {
	uid := c.Param("uuid")

	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	// Fetch and save models
	err := s.config.FetchAndSaveProviderModels(uid)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
				"provider": uid,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, FetchProviderModelsResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to fetch models from provider %s: %s", uid, err.Error()),
			Data:    nil,
		})
		return
	}

	// Get the updated models
	modelManager := s.config.GetModelManager()
	models := modelManager.GetModels(uid)

	if s.logger != nil {
		s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
			"provider":     uid,
			"models_count": len(models),
		}, true, fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), uid))
	}

	providerModels := ProviderModelInfo{
		Models: models,
	}

	response := ProviderModelsResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), uid),
		Data:    providerModels,
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) GetProviderModelsByUUID(c *gin.Context) {
	uid := c.Param("uuid")

	providerModelManager := s.config.GetModelManager()
	if providerModelManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Provider model manager not available",
		})
		return
	}

	models := providerModelManager.GetModels(uid)
	providerModels := ProviderModelInfo{
		Models: models,
	}

	response := ProviderModelsResponse{
		Success: true,
		Data:    providerModels,
	}

	c.JSON(http.StatusOK, response)
}
