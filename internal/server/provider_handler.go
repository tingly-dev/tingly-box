package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// maskProviderForResponse masks sensitive data and returns a safe ProviderResponse
func maskProviderForResponse(provider *typ.Provider) ProviderResponse {
	resp := ProviderResponse{
		UUID:             provider.UUID,
		Name:             provider.Name,
		APIBase:          provider.APIBase,
		APIStyle:         string(provider.APIStyle),
		APIBaseOpenAI:    provider.APIBaseOpenAI,
		APIBaseAnthropic: provider.APIBaseAnthropic,
		NoKeyRequired:    provider.NoKeyRequired,
		Enabled:          provider.Enabled,
		ProxyURL:         provider.ProxyURL,
		UserAgent:        provider.UserAgent,
		AuthType:         string(provider.AuthType),
		Source:           string(provider.Source),
	}
	// Only surface vmodel_detail on vmodel providers so a stale blob on a
	// flipped-auth row can never leak via the masked response.
	if provider.AuthType == typ.AuthTypeVirtual {
		resp.VModelDetail = provider.VModelDetail
	}

	switch provider.AuthType {
	case typ.AuthTypeOAuth:
		// For OAuth, return masked OAuthDetail
		if provider.OAuthDetail != nil {
			resp.OAuthDetail = &typ.OAuthDetail{
				//AccessToken:  maskToken(provider.OAuthDetail.AccessToken),
				AccessToken:  provider.OAuthDetail.AccessToken,
				RefreshToken: provider.OAuthDetail.RefreshToken,
				Issuer:       provider.OAuthDetail.Issuer,
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

	// Connectivity verification is intentionally NOT enforced here: provider
	// keys can be added regardless of probe results (some providers don't
	// support every endpoint). Connection testing is available separately as
	// an optional, informational check via the lightweight probe endpoint.

	// Set default enabled status if not provided
	if !req.Enabled {
		req.Enabled = true
	}

	// Set default API style if not provided
	if req.APIStyle == "" {
		req.APIStyle = "openai"
	}

	// Set default auth type if not provided
	if req.AuthType == "" {
		req.AuthType = string(typ.AuthTypeAPIKey)
	}

	// Fusion-mode constraints: optional dual base URLs are only valid for
	// api_key auth, and Google-style providers cannot opt in. The global
	// fusion experiment must also be enabled — when off, fusion fields on
	// new records are rejected so users cannot accidentally opt in until
	// they explicitly turn on the experiment.
	if req.APIBaseOpenAI != "" || req.APIBaseAnthropic != "" {
		if !s.IsFusionEnabled() {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Fusion provider mode is disabled. Enable it under System → Experimental Features, or save the OpenAI and Anthropic providers separately.",
			})
			return
		}
		if req.AuthType != string(typ.AuthTypeAPIKey) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Fusion base URLs (api_base_openai / api_base_anthropic) are only supported for api_key auth providers",
			})
			return
		}
		if req.APIStyle == string(protocol.APIStyleGoogle) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Fusion base URLs are not supported for Google-style providers",
			})
			return
		}
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
		UUID:             uid.String(),
		Name:             req.Name,
		APIBase:          req.APIBase,
		APIStyle:         protocol.APIStyle(req.APIStyle),
		APIBaseOpenAI:    req.APIBaseOpenAI,
		APIBaseAnthropic: req.APIBaseAnthropic,
		Token:            req.Token,
		NoKeyRequired:    req.NoKeyRequired,
		Enabled:          true, // always make new provider enabled
		ProxyURL:         req.ProxyURL,
		UserAgent:        req.UserAgent,
		AuthType:         typ.AuthType(req.AuthType),
		Timeout:          constant.DefaultRequestTimeout,
	}

	err = s.config.AddProvider(provider)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"action":   obs.ActionAddProvider,
			"success":  false,
			"name":     req.Name,
			"api_base": req.APIBase,
		}).Error(err.Error())

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// update models for current provider here too, try once and ignore error
	s.config.FetchAndSaveProviderModels(provider.UUID)

	logrus.WithFields(logrus.Fields{
		"action":   obs.ActionAddProvider,
		"success":  true,
		"name":     req.Name,
		"api_base": req.APIBase,
	}).Info(fmt.Sprintf("Provider %s added successfully", req.Name))

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

	// Builtin providers (e.g. virtual-model defaults) are not deletable.
	// They are re-seeded at startup so any deletion would just race back.
	if existing, err := s.config.GetProviderByUUID(uid); err == nil && existing.IsBuiltin() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "Builtin providers cannot be deleted (you can disable them instead)",
		})
		return
	}

	err := s.config.DeleteProvider(uid)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"action":  obs.ActionDeleteProvider,
			"success": false,
			"name":    uid,
		}).Error(err.Error())

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logrus.WithFields(logrus.Fields{
		"action":  obs.ActionDeleteProvider,
		"success": true,
		"name":    uid,
	}).Info(fmt.Sprintf("Provider %s deleted successfully", uid))

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

	// Builtin providers are immutable except for Enabled (toggled via the
	// dedicated ToggleProvider endpoint). Reject mutating updates here so the
	// store always reflects the in-process registries on the next restart.
	if provider.IsBuiltin() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "Builtin providers are read-only (use the toggle endpoint to enable/disable)",
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
		provider.APIStyle = protocol.APIStyle(*req.APIStyle)
	}
	// Apply fusion-field updates only when the global fusion experiment is on.
	// When off, incoming fusion pointers are ignored — existing stored values
	// are preserved (so flipping the flag back on reactivates them) and no
	// new fusion data can be introduced via the update path.
	if s.IsFusionEnabled() {
		if req.APIBaseOpenAI != nil {
			provider.APIBaseOpenAI = *req.APIBaseOpenAI
		}
		if req.APIBaseAnthropic != nil {
			provider.APIBaseAnthropic = *req.APIBaseAnthropic
		}
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
	if req.UserAgent != nil {
		provider.UserAgent = *req.UserAgent
	}

	// Fusion-mode constraints: dual base URLs are only valid for api_key auth,
	// and Google-style providers cannot opt in. Validate post-merge so we
	// catch combinations introduced by partial PATCHes.
	if provider.APIBaseOpenAI != "" || provider.APIBaseAnthropic != "" {
		if provider.AuthType != typ.AuthTypeAPIKey && provider.AuthType != "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Fusion base URLs (api_base_openai / api_base_anthropic) are only supported for api_key auth providers",
			})
			return
		}
		if provider.APIStyle == protocol.APIStyleGoogle {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Fusion base URLs are not supported for Google-style providers",
			})
			return
		}
	}

	err = s.config.UpdateProvider(uid, provider)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"action":  obs.ActionUpdateProvider,
			"success": false,
			"name":    uid,
			"updates": req,
		}).Error(err.Error())

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logrus.WithFields(logrus.Fields{
		"action":  obs.ActionUpdateProvider,
		"success": true,
		"name":    uid,
	}).Info(fmt.Sprintf("Provider %s updated successfully", uid))

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
		logrus.WithFields(logrus.Fields{
			"action":  obs.ActionUpdateProvider,
			"success": false,
			"name":    uid,
			"enabled": provider.Enabled,
		}).Error(err.Error())

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

	logrus.WithFields(logrus.Fields{
		"action":  obs.ActionUpdateProvider,
		"success": true,
		"name":    uid,
		"enabled": provider.Enabled,
	}).Info(fmt.Sprintf("Provider %s %s successfully", uid, action))

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
		logrus.WithFields(logrus.Fields{
			"action":   obs.ActionFetchModels,
			"success":  false,
			"provider": uid,
		}).Error(err.Error())

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

	logrus.WithFields(logrus.Fields{
		"action":       obs.ActionFetchModels,
		"success":      true,
		"provider":     uid,
		"models_count": len(models),
	}).Info(fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), uid))

	providerModels := ProviderModelInfo{
		Models: models,
	}

	// Attach quota information if quota manager is available
	if s.quotaManager != nil {
		var ctx context.Context = c.Request.Context()
		quotaData, err := s.quotaManager.GetQuota(ctx, uid)
		if err == nil && quotaData != nil {
			providerModels.Quota = quotaData
		}
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

	// For vmodel providers the model list lives on the provider record, not in the
	// model manager cache (which is populated by FetchAndSaveProviderModels).
	// Fall back to the provider's VModelDetail when the cache is empty.
	if len(models) == 0 {
		if p, err := s.config.GetProviderByUUID(uid); err == nil && p.IsVirtual() {
			if p.VModelDetail != nil {
				models = p.VModelDetail.Models
			}
		}
	}

	providerModels := ProviderModelInfo{
		Models: models,
	}

	// Attach quota information if quota manager is available
	// Use GetQuotaNoCache to always get fresh data from DB (bypasses cache/expiration logic)
	if s.quotaManager != nil {
		var ctx context.Context = c.Request.Context()
		quotaData, err := s.quotaManager.GetQuotaNoCache(ctx, uid)
		if err == nil && quotaData != nil {
			providerModels.Quota = quotaData
		}
		// Silently ignore quota errors - models should work without quota
	}

	response := ProviderModelsResponse{
		Success: true,
		Data:    providerModels,
	}

	c.JSON(http.StatusOK, response)
}
