package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_coder"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// ImBotSettingsAPI provides REST endpoints for ImBot settings management
type ImBotSettingsAPI struct {
	config *config.Config
	store  *db.ImBotSettingsStore
}

// NewImBotSettingsAPI creates a new ImBot settings API
func NewImBotSettingsAPI(cfg *config.Config) *ImBotSettingsAPI {
	return &ImBotSettingsAPI{
		config: cfg,
		store:  cfg.GetImBotSettingsStore(),
	}
}

// RegisterImBotSettingsRoutes registers the ImBot settings API routes with swagger documentation
func (s *Server) RegisterImBotSettingsRoutes(manager *swagger.RouteManager) {
	imbotAPI := NewImBotSettingsAPI(s.config)

	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.authMW.UserAuthMiddleware())

	// GET /api/v1/imbot-settings - List all ImBot configurations
	apiV1.GET("/imbot-settings", imbotAPI.ListSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns all ImBot configurations"),
		swagger.WithResponseModel(ImBotSettingsListResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "ImBot settings store not available"},
		),
	)

	// GET /api/v1/imbot-settings/:uuid - Get a single ImBot configuration
	apiV1.GET("/imbot-settings/:uuid", imbotAPI.GetSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns a single ImBot configuration by UUID"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(ImBotSettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// POST /api/v1/imbot-settings - Create a new ImBot configuration
	apiV1.POST("/imbot-settings", imbotAPI.CreateSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Creates a new ImBot configuration"),
		swagger.WithRequestModel(ImBotSettingsCreateRequest{}),
		swagger.WithResponseModel(ImBotSettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request"},
		),
	)

	// PUT /api/v1/imbot-settings/:uuid - Update an existing ImBot configuration
	apiV1.PUT("/imbot-settings/:uuid", imbotAPI.UpdateSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Updates an existing ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithRequestModel(ImBotSettingsUpdateRequest{}),
		swagger.WithResponseModel(ImBotSettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// DELETE /api/v1/imbot-settings/:uuid - Delete an ImBot configuration
	apiV1.DELETE("/imbot-settings/:uuid", imbotAPI.DeleteSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Deletes an ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(DeleteResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// POST /api/v1/imbot-settings/:uuid/toggle - Toggle enabled status
	apiV1.POST("/imbot-settings/:uuid/toggle", imbotAPI.ToggleSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Toggles the enabled status of an ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(ImBotSettingsToggleResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// GET /api/v1/imbot-platforms - Get all supported platforms
	apiV1.GET("/imbot-platforms", imbotAPI.GetPlatforms,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns all supported ImBot platforms with their configurations"),
		swagger.WithResponseModel(ImBotPlatformsResponse{}),
	)

	// GET /api/v1/imbot-platform-config - Get platform auth configuration
	apiV1.GET("/imbot-platform-config", imbotAPI.GetPlatformConfig,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns auth configuration for a specific platform"),
		swagger.WithQueryConfig("platform", swagger.QueryParamConfig{
			Name:        "platform",
			Type:        "string",
			Required:    true,
			Description: "Platform identifier (telegram, discord, slack, feishu, dingtalk, whatsapp)",
		}),
		swagger.WithResponseModel(ImBotPlatformConfigResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Platform parameter is required"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Unknown platform"},
		),
	)
}

// ListSettings returns all ImBot configurations
func (api *ImBotSettingsAPI) ListSettings(c *gin.Context) {
	if api.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	settings, err := api.store.ListSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := ImBotSettingsListResponse{
		Success:  true,
		Settings: settings,
	}

	c.JSON(http.StatusOK, response)
}

// GetSettings returns a single ImBot configuration by UUID
func (api *ImBotSettingsAPI) GetSettings(c *gin.Context) {
	if api.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	settings, err := api.store.GetSettingsByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if settings were found (empty UUID means not found)
	if settings.UUID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "ImBot settings not found"})
		return
	}

	response := ImBotSettingsResponse{
		Success:  true,
		Settings: settings,
	}

	c.JSON(http.StatusOK, response)
}

// CreateSettings creates a new ImBot configuration
func (api *ImBotSettingsAPI) CreateSettings(c *gin.Context) {
	if api.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	var req ImBotSettingsCreateRequest
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
		Enabled:       req.Enabled,
	}

	created, err := api.store.CreateSettings(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", created.UUID).WithField("platform", created.Platform).Info("ImBot settings created")

	// Start the bot if enabled
	if created.Enabled {
		if botManager := remote_coder.GetBotManager(); botManager != nil {
			ctx := context.Background()
			if err := botManager.Start(ctx, created.UUID); err != nil {
				logrus.WithError(err).WithField("uuid", created.UUID).Warn("Failed to start bot after creation")
			}
		}
	}

	response := ImBotSettingsResponse{
		Success:  true,
		Settings: created,
	}

	c.JSON(http.StatusOK, response)
}

// UpdateSettings updates an existing ImBot configuration
func (api *ImBotSettingsAPI) UpdateSettings(c *gin.Context) {
	if api.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	// Get current settings to check if enabled status is changing
	currentSettings, err := api.store.GetSettingsByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if settings exist
	if currentSettings.UUID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "ImBot settings not found"})
		return
	}

	var req ImBotSettingsUpdateRequest
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

	if err := api.store.UpdateSettings(uuid, settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", uuid).Info("ImBot settings updated")

	// Handle bot lifecycle if enabled status changed
	if currentSettings.Enabled != newEnabled {
		if botManager := remote_coder.GetBotManager(); botManager != nil {
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
	updated, err := api.store.GetSettingsByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := ImBotSettingsResponse{
		Success:  true,
		Settings: updated,
	}

	c.JSON(http.StatusOK, response)
}

// DeleteSettings deletes an ImBot configuration
func (api *ImBotSettingsAPI) DeleteSettings(c *gin.Context) {
	if api.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	// Stop the bot if it's running
	if botManager := remote_coder.GetBotManager(); botManager != nil {
		botManager.Stop(uuid)
	}

	if err := api.store.DeleteSettings(uuid); err != nil {
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
func (api *ImBotSettingsAPI) ToggleSettings(c *gin.Context) {
	if api.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ImBot settings store not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	newStatus, err := api.store.ToggleSettings(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", uuid).WithField("enabled", newStatus).Info("ImBot settings toggled")

	// Notify bot manager to start or stop the bot
	if botManager := remote_coder.GetBotManager(); botManager != nil {
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

	response := ImBotSettingsToggleResponse{
		Success: true,
		Enabled: newStatus,
	}

	c.JSON(http.StatusOK, response)
}

// GetPlatforms returns all supported ImBot platforms with their configurations
func (api *ImBotSettingsAPI) GetPlatforms(c *gin.Context) {
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

	response := ImBotPlatformsResponse{
		Success:    true,
		Platforms:  platformResponses,
		Categories: categories,
	}

	c.JSON(http.StatusOK, response)
}

// GetPlatformConfig returns auth configuration for a specific platform
func (api *ImBotSettingsAPI) GetPlatformConfig(c *gin.Context) {
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

	response := ImBotPlatformConfigResponse{
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

// Request/Response types

type ImBotSettingsListResponse struct {
	Success  bool          `json:"success"`
	Settings []db.Settings `json:"settings"`
}

type ImBotSettingsResponse struct {
	Success  bool        `json:"success"`
	Settings db.Settings `json:"settings"`
}

type ImBotSettingsCreateRequest struct {
	UUID          string            `json:"uuid,omitempty"`
	Name          string            `json:"name,omitempty"`
	Platform      string            `json:"platform"`
	AuthType      string            `json:"auth_type"`
	Auth          map[string]string `json:"auth"`
	ProxyURL      string            `json:"proxy_url,omitempty"`
	ChatID        string            `json:"chat_id,omitempty"`
	BashAllowlist []string          `json:"bash_allowlist,omitempty"`
	Enabled       bool              `json:"enabled"`
	Token         string            `json:"token,omitempty"` // Legacy field
}

type ImBotSettingsUpdateRequest struct {
	Name          string            `json:"name,omitempty"`
	Platform      string            `json:"platform,omitempty"`
	AuthType      string            `json:"auth_type,omitempty"`
	Auth          map[string]string `json:"auth,omitempty"`
	ProxyURL      string            `json:"proxy_url,omitempty"`
	ChatID        string            `json:"chat_id,omitempty"`
	BashAllowlist []string          `json:"bash_allowlist,omitempty"`
	Enabled       *bool             `json:"enabled,omitempty"` // Pointer to allow partial update
	Token         string            `json:"token,omitempty"`   // Legacy field
}

type ImBotSettingsToggleResponse struct {
	Success bool `json:"success"`
	Enabled bool `json:"enabled"`
}

type ImBotPlatformsResponse struct {
	Success    bool             `json:"success"`
	Platforms  []PlatformConfig `json:"platforms"`
	Categories gin.H            `json:"categories"`
}

type ImBotPlatformConfigResponse struct {
	Success  bool           `json:"success"`
	Platform PlatformConfig `json:"platform"`
}

type PlatformConfig struct {
	Platform    string            `json:"platform"`
	DisplayName string            `json:"display_name"`
	AuthType    string            `json:"auth_type"`
	Category    string            `json:"category"`
	Fields      []imbot.FieldSpec `json:"fields"`
}

type DeleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
