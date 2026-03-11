package imbotsettings

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control"
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// Handler handles ImBot settings HTTP requests
type Handler struct {
	config *config.Config
	store  *db.ImBotSettingsStore
}

// NewHandler creates a new ImBot settings handler
func NewHandler(cfg *config.Config) *Handler {
	sm := cfg.StoreManager()
	return &Handler{
		config: cfg,
		store:  sm.ImBotSettings(),
	}
}

// ListSettings returns all ImBot configurations
func (h *Handler) ListSettings(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	settings, err := h.store.ListSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := ListResponse{
		Success:  true,
		Settings: settings,
	}

	c.JSON(http.StatusOK, response)
}

// GetSettings returns a single ImBot configuration by UUID
func (h *Handler) GetSettings(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	settings, err := h.store.GetSettingsByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if settings were found (empty UUID means not found)
	if settings.UUID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "ImBot settings not found"})
		return
	}

	response := SettingsResponse{
		Success:  true,
		Settings: settings,
	}

	c.JSON(http.StatusOK, response)
}

// CreateSettings creates a new ImBot configuration
func (h *Handler) CreateSettings(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	// Normalize platform
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "telegram"
	}

	// Get platform config to determine auth type if not provided
	authType := strings.TrimSpace(req.AuthType)
	if authType == "" {
		if config, exists := imbot.GetPlatformConfig(platform); exists {
			authType = config.AuthType
		}
	}

	// Handle backward compatibility: if legacy token is provided, populate auth map
	authMap := req.Auth
	if authMap == nil {
		authMap = make(map[string]string)
	}
	if req.Token != "" && authType == "token" {
		authMap["token"] = strings.TrimSpace(req.Token)
	}

	settings := db.Settings{
		Name:               strings.TrimSpace(req.Name),
		Platform:           platform,
		AuthType:           authType,
		Auth:               authMap,
		ProxyURL:           strings.TrimSpace(req.ProxyURL),
		ChatIDLock:         strings.TrimSpace(req.ChatID),
		BashAllowlist:      normalizeAllowlist(req.BashAllowlist),
		Enabled:            req.Enabled,
		SmartGuideProvider: strings.TrimSpace(req.SmartGuideProvider),
		SmartGuideModel:    strings.TrimSpace(req.SmartGuideModel),
	}

	created, err := h.store.CreateSettings(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", created.UUID).WithField("platform", created.Platform).Info("ImBot settings created")

	// Start the bot if enabled
	if created.Enabled {
		if botManager := remote_control.GetBotManager(); botManager != nil {
			ctx := context.Background()
			if err := botManager.Start(ctx, created.UUID); err != nil {
				logrus.WithError(err).WithField("uuid", created.UUID).Warn("Failed to start bot after creation")
			}
		}
	}

	response := SettingsResponse{
		Success:  true,
		Settings: created,
	}

	c.JSON(http.StatusOK, response)
}

// UpdateSettings updates an existing ImBot configuration
func (h *Handler) UpdateSettings(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	// Get current settings to check if enabled status is changing
	currentSettings, err := h.store.GetSettingsByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if settings exist
	if currentSettings.UUID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "ImBot settings not found"})
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	// Normalize platform
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "telegram"
	}

	// Get platform config to determine auth type if not provided
	authType := strings.TrimSpace(req.AuthType)
	if authType == "" {
		if config, exists := imbot.GetPlatformConfig(platform); exists {
			authType = config.AuthType
		}
	}

	// Handle backward compatibility: if legacy token is provided, populate auth map
	authMap := req.Auth
	if authMap == nil {
		authMap = make(map[string]string)
	}
	if req.Token != "" && authType == "token" {
		authMap["token"] = strings.TrimSpace(req.Token)
	}

	settings := db.Settings{
		Name:          strings.TrimSpace(req.Name),
		Platform:      platform,
		AuthType:      authType,
		Auth:          authMap,
		ProxyURL:      strings.TrimSpace(req.ProxyURL),
		ChatIDLock:    strings.TrimSpace(req.ChatID),
		BashAllowlist: normalizeAllowlist(req.BashAllowlist),
	}

	newEnabled := currentSettings.Enabled
	if req.Enabled != nil {
		newEnabled = *req.Enabled
		settings.Enabled = newEnabled
	}

	// Handle SmartGuide config (partial update)
	if req.SmartGuideProvider != nil {
		settings.SmartGuideProvider = strings.TrimSpace(*req.SmartGuideProvider)
	} else {
		settings.SmartGuideProvider = currentSettings.SmartGuideProvider
	}
	if req.SmartGuideModel != nil {
		settings.SmartGuideModel = strings.TrimSpace(*req.SmartGuideModel)
	} else {
		settings.SmartGuideModel = currentSettings.SmartGuideModel
	}

	if err := h.store.UpdateSettings(uuid, settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", uuid).Info("ImBot settings updated")

	// Handle bot lifecycle if enabled status changed
	if currentSettings.Enabled != newEnabled {
		if botManager := remote_control.GetBotManager(); botManager != nil {
			ctx := context.Background()
			if newEnabled {
				// Enable -> start the bot
				if err := botManager.Start(ctx, uuid); err != nil {
					logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to start bot after update")
				}
			} else {
				// Disable -> stop the bot
				botManager.Stop(uuid)
			}
		}
	}

	// Fetch updated settings
	updated, err := h.store.GetSettingsByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := SettingsResponse{
		Success:  true,
		Settings: updated,
	}

	c.JSON(http.StatusOK, response)
}

// DeleteSettings deletes an ImBot configuration
func (h *Handler) DeleteSettings(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	// Stop the bot if it's running
	if botManager := remote_control.GetBotManager(); botManager != nil {
		botManager.Stop(uuid)
	}

	if err := h.store.DeleteSettings(uuid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", uuid).Info("ImBot settings deleted")

	response := DeleteResponse{
		Success: true,
		Message: "ImBot settings deleted successfully",
	}

	c.JSON(http.StatusOK, response)
}

// ToggleSettings toggles the enabled status of an ImBot configuration
func (h *Handler) ToggleSettings(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	newStatus, err := h.store.ToggleSettings(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", uuid).WithField("enabled", newStatus).Info("ImBot settings toggled")

	// Notify bot manager to start or stop the bot
	if botManager := remote_control.GetBotManager(); botManager != nil {
		ctx := context.Background()
		if newStatus {
			// Start the bot
			if err := botManager.Start(ctx, uuid); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to start bot after toggle")
			}
		} else {
			// Stop the bot
			botManager.Stop(uuid)
		}
	}

	response := ToggleResponse{
		Success: true,
		Enabled: newStatus,
	}

	c.JSON(http.StatusOK, response)
}

// GetPlatforms returns all supported ImBot platforms with their configurations
func (h *Handler) GetPlatforms(c *gin.Context) {
	platforms := imbot.GetAllPlatforms()
	platformResponses := make([]PlatformConfig, 0, len(platforms))

	for _, p := range platforms {
		platformResponses = append(platformResponses, PlatformConfig{
			Platform:    p.Platform,
			DisplayName: p.DisplayName,
			AuthType:    p.AuthType,
			Category:    p.Category,
			Fields:      p.Fields,
		})
	}

	categories := gin.H{
		"im":         imbot.CategoryLabels["im"],
		"enterprise": imbot.CategoryLabels["enterprise"],
		"business":   imbot.CategoryLabels["business"],
	}

	response := PlatformsResponse{
		Success:    true,
		Platforms:  platformResponses,
		Categories: categories,
	}

	c.JSON(http.StatusOK, response)
}

// GetPlatformConfig returns auth configuration for a specific platform
func (h *Handler) GetPlatformConfig(c *gin.Context) {
	platform := c.Query("platform")
	if platform == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Platform parameter is required"})
		return
	}

	config, exists := imbot.GetPlatformConfig(platform)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown platform"})
		return
	}

	response := PlatformConfigResponse{
		Success: true,
		Platform: PlatformConfig{
			Platform:    config.Platform,
			DisplayName: config.DisplayName,
			AuthType:    config.AuthType,
			Category:    config.Category,
			Fields:      config.Fields,
		},
	}

	c.JSON(http.StatusOK, response)
}

// Helper function to normalize allowlist
func normalizeAllowlist(values []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, entry := range values {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}
