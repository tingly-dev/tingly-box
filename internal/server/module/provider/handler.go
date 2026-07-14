// Package provider handles CRUD and model-management HTTP endpoints for AI
// provider configurations. Business logic (token masking, dual-mode endpoint
// resolution, model cache fallback) lives here; routing metadata is in
// routes.go.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/dataio"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	providerquota "github.com/tingly-dev/tingly-box/internal/server/module/providerquota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles provider HTTP requests.
type Handler struct {
	config       *config.Config
	quotaManager providerquota.Manager // may be nil
}

// NewHandler creates a Handler. quotaManager may be nil when quota support is
// not configured.
func NewHandler(cfg *config.Config, qm providerquota.Manager) *Handler {
	return &Handler{config: cfg, quotaManager: qm}
}

// maskForResponse masks sensitive data and returns a safe ProviderResponse.
func maskForResponse(p *typ.Provider) ProviderResponse {
	resp := ProviderResponse{
		UUID:             p.UUID,
		Name:             p.Name,
		APIBase:          p.APIBase,
		APIStyle:         string(p.APIStyle),
		APIBaseOpenAI:    p.APIBaseOpenAI,
		APIBaseAnthropic: p.APIBaseAnthropic,
		NoKeyRequired:    p.NoKeyRequired,
		Enabled:          p.Enabled,
		ProxyURL:         p.ProxyURL,
		UserAgent:        p.UserAgent,
		AuthType:         string(p.AuthType),
		Source:           string(p.Source),
	}
	// Only surface vmodel_detail on vmodel providers so a stale blob on a
	// flipped-auth row can never leak via the masked response.
	if p.AuthType == typ.AuthTypeVirtual {
		resp.VModelDetail = p.VModelDetail
	}

	switch p.AuthType {
	case typ.AuthTypeOAuth:
		if p.OAuthDetail != nil {
			resp.OAuthDetail = &typ.OAuthDetail{
				AccessToken:  p.OAuthDetail.AccessToken,
				RefreshToken: p.OAuthDetail.RefreshToken,
				Issuer:       p.OAuthDetail.Issuer,
				UserID:       p.OAuthDetail.UserID,
				ExpiresAt:    p.OAuthDetail.ExpiresAt,
			}
		}
	case typ.AuthTypeAPIKey, "":
		resp.Token = p.Token
	}

	return resp
}

// GetProviders lists all configured providers with masked credentials.
func (h *Handler) GetProviders(c *gin.Context) {
	providers := h.config.ListProviders()

	maskedProviders := make([]ProviderResponse, len(providers))
	for i, p := range providers {
		maskedProviders[i] = maskForResponse(p)
	}

	c.JSON(http.StatusOK, ProvidersResponse{Success: true, Data: maskedProviders})
}

// GetProvider returns details for a specific provider by UUID.
func (h *Handler) GetProvider(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Provider name is required"})
		return
	}

	p, err := h.config.GetProviderByUUID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Provider not found"})
		return
	}

	c.JSON(http.StatusOK, struct {
		Success bool             `json:"success"`
		Data    ProviderResponse `json:"data"`
	}{Success: true, Data: maskForResponse(p)})
}

// CreateProvider adds a new provider.
func (h *Handler) CreateProvider(c *gin.Context) {
	var req CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Custom validation: token is required unless NoKeyRequired is true.
	if !req.NoKeyRequired && req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Token is required when No Key Required is false"})
		return
	}

	// Connectivity verification is intentionally NOT enforced here: provider
	// keys can be added regardless of probe results (some providers don't
	// support every endpoint). Connection testing is available separately as
	// an optional, informational check via the lightweight probe endpoint.

	if !req.Enabled {
		req.Enabled = true
	}
	if req.APIStyle == "" {
		req.APIStyle = "openai"
	}
	if req.AuthType == "" {
		req.AuthType = string(typ.AuthTypeAPIKey)
	}

	// Dual-mode constraints: optional dual base URLs are only valid for
	// api_key auth, and Google-style providers cannot opt in.
	if req.APIBaseOpenAI != "" || req.APIBaseAnthropic != "" {
		if req.AuthType != string(typ.AuthTypeAPIKey) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Dual base URLs (api_base_openai / api_base_anthropic) are only supported for api_key auth providers",
			})
			return
		}
		if req.APIStyle == string(protocol.APIStyleGoogle) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Dual base URLs are not supported for Google-style providers",
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

	p := &typ.Provider{
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

	if err = h.config.AddProvider(p); err != nil {
		logrus.WithFields(logrus.Fields{
			"action":   obs.ActionAddProvider,
			"success":  false,
			"name":     req.Name,
			"api_base": req.APIBase,
		}).Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Update models for the new provider; try once and ignore errors.
	h.config.FetchAndSaveProviderModels(p.UUID)

	logrus.WithFields(logrus.Fields{
		"action":   obs.ActionAddProvider,
		"success":  true,
		"name":     req.Name,
		"api_base": req.APIBase,
	}).Info(fmt.Sprintf("Provider %s added successfully", req.Name))

	c.JSON(http.StatusOK, CreateProviderResponse{
		Success: true,
		Message: "Provider added successfully",
		Data:    p,
	})
}

// DeleteProvider removes a provider by UUID.
func (h *Handler) DeleteProvider(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Provider name is required"})
		return
	}

	// Builtin providers (e.g. virtual-model defaults) are not deletable.
	if existing, err := h.config.GetProviderByUUID(uid); err == nil && existing.IsBuiltin() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "Builtin providers cannot be deleted (you can disable them instead)",
		})
		return
	}

	if err := h.config.DeleteProvider(uid); err != nil {
		logrus.WithFields(logrus.Fields{
			"action":  obs.ActionDeleteProvider,
			"success": false,
			"name":    uid,
		}).Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	logrus.WithFields(logrus.Fields{
		"action":  obs.ActionDeleteProvider,
		"success": true,
		"name":    uid,
	}).Info(fmt.Sprintf("Provider %s deleted successfully", uid))

	c.JSON(http.StatusOK, DeleteProviderResponse{Success: true, Message: "Provider deleted successfully"})
}

// UpdateProvider updates an existing provider by UUID.
func (h *Handler) UpdateProvider(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Provider name is required"})
		return
	}

	var req UpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	p, err := h.config.GetProviderByUUID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Provider not found"})
		return
	}

	// Builtin providers are immutable except for Enabled (toggled via the
	// dedicated ToggleProvider endpoint).
	if p.IsBuiltin() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "Builtin providers are read-only (use the toggle endpoint to enable/disable)",
		})
		return
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.APIBase != nil {
		p.APIBase = *req.APIBase
	}
	if req.APIStyle != nil {
		p.APIStyle = protocol.APIStyle(*req.APIStyle)
	}
	if req.APIBaseOpenAI != nil {
		p.APIBaseOpenAI = *req.APIBaseOpenAI
	}
	if req.APIBaseAnthropic != nil {
		p.APIBaseAnthropic = *req.APIBaseAnthropic
	}
	if req.Token != nil && *req.Token != "" {
		p.Token = *req.Token
	}
	if req.NoKeyRequired != nil {
		p.NoKeyRequired = *req.NoKeyRequired
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if req.ProxyURL != nil {
		p.ProxyURL = *req.ProxyURL
	}
	if req.UserAgent != nil {
		p.UserAgent = *req.UserAgent
	}

	// Dual-mode constraints: validate post-merge so we catch combinations
	// introduced by partial PATCHes.
	if p.APIBaseOpenAI != "" || p.APIBaseAnthropic != "" {
		if p.AuthType != typ.AuthTypeAPIKey && p.AuthType != "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Dual base URLs (api_base_openai / api_base_anthropic) are only supported for api_key auth providers",
			})
			return
		}
		if p.APIStyle == protocol.APIStyleGoogle {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Dual base URLs are not supported for Google-style providers",
			})
			return
		}
	}

	if err = h.config.UpdateProvider(uid, p); err != nil {
		logrus.WithFields(logrus.Fields{
			"action":  obs.ActionUpdateProvider,
			"success": false,
			"name":    uid,
			"updates": req,
		}).Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	logrus.WithFields(logrus.Fields{
		"action":  obs.ActionUpdateProvider,
		"success": true,
		"name":    uid,
	}).Info(fmt.Sprintf("Provider %s updated successfully", uid))

	c.JSON(http.StatusOK, UpdateProviderResponse{
		Success: true,
		Message: "Provider updated successfully",
		Data:    maskForResponse(p),
	})
}

// ToggleProvider enables or disables a provider.
func (h *Handler) ToggleProvider(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Provider name is required"})
		return
	}

	p, err := h.config.GetProviderByUUID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Provider not found"})
		return
	}

	p.Enabled = !p.Enabled

	if err = h.config.UpdateProvider(uid, p); err != nil {
		logrus.WithFields(logrus.Fields{
			"action":  obs.ActionUpdateProvider,
			"success": false,
			"name":    uid,
			"enabled": p.Enabled,
		}).Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	action := "disabled"
	if p.Enabled {
		action = "enabled"
	}
	logrus.WithFields(logrus.Fields{
		"action":  obs.ActionUpdateProvider,
		"success": true,
		"name":    uid,
		"enabled": p.Enabled,
	}).Info(fmt.Sprintf("Provider %s %s successfully", uid, action))

	resp := ToggleProviderResponse{
		Success: true,
		Message: fmt.Sprintf("Provider %s %s successfully", uid, action),
	}
	resp.Data.Enabled = p.Enabled
	c.JSON(http.StatusOK, resp)
}

// UpdateProviderModelsByUUID fetches and caches models for the given provider.
func (h *Handler) UpdateProviderModelsByUUID(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Provider name is required"})
		return
	}

	if err := h.config.FetchAndSaveProviderModels(uid); err != nil {
		logrus.WithFields(logrus.Fields{
			"action":   obs.ActionFetchModels,
			"success":  false,
			"provider": uid,
		}).Error(err.Error())
		c.JSON(http.StatusInternalServerError, FetchProviderModelsResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to fetch models from provider %s: %s", uid, err.Error()),
		})
		return
	}

	modelManager := h.config.GetModelManager()
	models := modelManager.GetModels(uid)

	// Apply canonical ordering at the serving boundary (see GetProviderModelsByUUID).
	if p, err := h.config.GetProviderByUUID(uid); err == nil {
		config.SortProviderModels(p, models)
	}

	logrus.WithFields(logrus.Fields{
		"action":       obs.ActionFetchModels,
		"success":      true,
		"provider":     uid,
		"models_count": len(models),
	}).Info(fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), uid))

	providerModels := ProviderModelInfo{Models: models}

	if h.quotaManager != nil {
		ctx := c.Request.Context()
		if quotaData, err := h.quotaManager.GetQuota(ctx, uid); err == nil && quotaData != nil {
			providerModels.Quota = quotaData
		}
	}

	c.JSON(http.StatusOK, ProviderModelsResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), uid),
		Data:    providerModels,
	})
}

// GetProviderModelsByUUID returns the model list for a provider, falling back
// through DB cache → VModel static list → provider API → template. The template
// is a pure last-resort fallback (used only when every earlier source is
// empty); it is never merged into a non-empty list, since the embedded
// snapshot can list models the upstream has since retired.
func (h *Handler) GetProviderModelsByUUID(c *gin.Context) {
	uid := c.Param("uuid")

	providerModelManager := h.config.GetModelManager()
	if providerModelManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Provider model manager not available",
		})
		return
	}

	// Step 1: Try DB cache (API-sourced, 1h TTL).
	models := providerModelManager.GetModels(uid)
	source := ModelCacheSourceDB
	// Default to far-future for sources that never expire (VModel); other
	// sources override this below with their own TTL. Avoids serialising a
	// zero time.Time ("0001-01-01T00:00:00Z") in the response.
	const neverExpires = 20 * 365 * 24 * time.Hour
	expiresAt := time.Now().Add(neverExpires)
	lastUpdated := ""

	p, provErr := h.config.GetProviderByUUID(uid)
	isVirtual := provErr == nil && p.IsVirtual()

	if len(models) > 0 {
		if _, updated, exists := providerModelManager.GetProviderInfo(uid); exists {
			lastUpdated = updated
		}
		expiresAt = time.Now().Add(1 * time.Hour)
	} else if provErr == nil {
		if isVirtual {
			// Step 2a: VModel fallback (static, no cache).
			if p.VModelDetail != nil {
				models = p.VModelDetail.Models
				source = ModelCacheSourceVModel
			}
			// VModel lists are static — never expire.
		} else {
			// Step 2b: Try Provider API.
			if apiErr := h.config.FetchAndSaveProviderModels(uid); apiErr == nil {
				models = providerModelManager.GetModels(uid)
				if len(models) > 0 {
					source = ModelCacheSourceAPI
					expiresAt = time.Now().Add(1 * time.Hour)
				}
			}
		}
	}

	// Step 3: Template fallback — used only when we still have no models at
	// all (API failed, unsupported, or returned nothing). The embedded
	// template is a compile-time snapshot that can still list models the
	// upstream has since retired, so it must never be merged into a non-empty
	// real list — doing so would resurrect deprecated models. It is a pure
	// last-resort fallback. Read live (never persisted) so an improved
	// embedded list takes effect immediately instead of waiting out a cached
	// entry's TTL.
	if provErr == nil && !isVirtual && len(models) == 0 {
		if tm := h.config.GetTemplateManager(); tm != nil {
			if templateModels, err := tm.GetEmbeddedModelsForProvider(p); err == nil && len(templateModels) > 0 {
				models = templateModels
				source = ModelCacheSourceTemplate
				expiresAt = time.Now().Add(1 * time.Hour)
			}
		}
	}

	// Apply canonical ordering at the serving boundary so the response order
	// is authoritative regardless of cache source. The frontend relies on this
	// order and no longer sorts client-side.
	if provErr == nil {
		config.SortProviderModels(p, models)
	}

	providerModels := ProviderModelInfo{
		Models:      models,
		Source:      source,
		ExpiresAt:   expiresAt,
		LastUpdated: lastUpdated,
	}

	// Attach quota only when the provider type is supported; skip silently
	// otherwise to avoid surfacing a misleading error.
	if h.quotaManager != nil && h.quotaManager.IsProviderSupported(uid) {
		ctx := context.Background()
		if quotaData, err := h.quotaManager.GetQuotaNoCache(ctx, uid); err == nil && quotaData != nil {
			providerModels.Quota = quotaData
		}
	}

	c.JSON(http.StatusOK, ProviderModelsResponse{Success: true, Data: providerModels})
}

// ImportProviders imports providers from base64/JSONL encoded export data.
// Registered at /provider-import (see routes.go); only providers are
// imported — dataio export/import no longer carries rule data.
func (h *Handler) ImportProviders(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	var req ImportProvidersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Set default conflict handling
	// OnProviderConflict: Only matters when the same provider UUID already exists
	//   - "use": use the existing provider with the same UUID
	//   - "skip": skip importing this provider
	// Note: Provider names can be duplicated; if name exists, a suffix is added automatically
	if req.OnProviderConflict == "" {
		req.OnProviderConflict = "use" // Use existing if same UUID found
	}

	opts := dataio.ImportOptions{
		OnProviderConflict: req.OnProviderConflict,
		Quiet:              true,
	}

	result, err := dataio.Import(req.Data, cfg, dataio.FormatAuto, opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Failed to import providers: " + err.Error(),
		})
		return
	}

	response := ImportProvidersResponse{
		Success: true,
		Message: "Providers imported successfully",
	}
	response.Data.ProvidersCreated = result.ProvidersCreated
	response.Data.ProvidersUsed = result.ProvidersUsed

	// Convert provider import info to response format
	for _, providerInfo := range result.Providers {
		response.Data.Providers = append(response.Data.Providers, ProviderImportInfo{
			UUID:   providerInfo.UUID,
			Name:   providerInfo.Name,
			Action: providerInfo.Action,
		})
	}

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action":            obs.ActionUpdateProvider,
		"providers_created": result.ProvidersCreated,
	}).Info(
		fmt.Sprintf(
			"Provider import completed: created=%d, used=%d",
			result.ProvidersCreated, result.ProvidersUsed,
		),
	)

	c.JSON(http.StatusOK, response)
}

// ExportProvider exports a single provider (identified by the required
// "uuid" query parameter) as base64/JSONL encoded data (see internal/dataio).
// Format defaults to base64 and is selected via the "format" query
// parameter ("base64" or "jsonl").
func (h *Handler) ExportProvider(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	uid := c.Query("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "uuid is required",
		})
		return
	}

	formatStr := c.DefaultQuery("format", string(dataio.FormatBase64))
	format := dataio.Format(formatStr)
	if format != dataio.FormatBase64 && format != dataio.FormatJSONL {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("invalid format %q: supported formats are base64 and jsonl", formatStr),
		})
		return
	}

	p, err := cfg.GetProviderByUUID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Provider not found"})
		return
	}

	result, err := dataio.Export(&dataio.ExportRequest{Providers: []*typ.Provider{p}}, format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to export provider: " + err.Error(),
		})
		return
	}

	response := ExportProviderResponse{
		Success: true,
		Message: "Provider exported successfully",
	}
	response.Data.Format = string(format)
	response.Data.Data = result.Content

	logrus.WithFields(logrus.Fields{
		"action":   obs.ActionUpdateProvider,
		"provider": uid,
		"format":   format,
	}).Info(fmt.Sprintf("Provider export completed: uuid=%s, format=%s", uid, format))

	c.JSON(http.StatusOK, response)
}
