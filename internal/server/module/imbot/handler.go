package imbot

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/remote/channel"
)

// LifecycleController is the narrow surface the server uses to drive the
// imbot module's lifecycle. Replacing the previous untyped interface{} +
// inline type assertions makes the contract explicit and is the single
// seam at which an out-of-process implementation could later be swapped in.
type LifecycleController interface {
	StartAllEnabled(ctx context.Context) error
	StopAll()
	RestartBotByUUID(ctx context.Context, uuid string) error
	Sync(ctx context.Context) error
	Shutdown()
	SetChannelRegistry(reg *channel.Registry)
}

// Handler handles ImBot settings HTTP requests
type Handler struct {
	config           *config.Config
	store            *db.ImBotSettingsStore
	botMgr           *BotManager           // Local bot manager, not global
	qrLoginHandler   *WeChatQRLoginHandler // WeChat QR login handler
	feishuRegHandler *FeishuRegHandler     // Feishu/Lark one-click registration handler
}

// NewHandler creates a new ImBot settings handler
func NewHandler(ctx context.Context, cfg *config.Config) (*Handler, error) {
	sm := cfg.StoreManager()
	botMgr, err := NewBotManager(ctx, cfg)
	if err != nil {
		return nil, err
	}
	h := &Handler{
		config: cfg,
		store:  sm.ImBotSettings(),
		botMgr: botMgr,
	}
	// Initialize QR login handler
	h.qrLoginHandler = NewWeChatQRLoginHandler(h.store)
	// Initialize Feishu/Lark one-click registration handler
	h.feishuRegHandler = NewFeishuRegHandler(h.store)
	return h, nil
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
		DefaultCwd:         strings.TrimSpace(req.DefaultCwd),
		DefaultAgent:       strings.TrimSpace(req.DefaultAgent),
		Enabled:            req.Enabled,
		SmartGuideProvider: strings.TrimSpace(req.SmartGuideProvider),
		SmartGuideModel:    strings.TrimSpace(req.SmartGuideModel),
		RequirePairing:     req.RequirePairing,
	}

	created, err := h.store.CreateSettings(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", created.UUID).WithField("platform", created.Platform).Info("ImBot settings created")

	// Start the bot if enabled
	if created.Enabled {
		if h.botMgr != nil {
			ctx := context.Background()
			if err := h.botMgr.StartBot(ctx, created.UUID); err != nil {
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
		platform = currentSettings.Platform // Keep existing if not provided
	}

	// Get platform config to determine auth type if not provided
	authType := strings.TrimSpace(req.AuthType)
	if authType == "" {
		if config, exists := imbot.GetPlatformConfig(platform); exists {
			authType = config.AuthType
		} else {
			authType = currentSettings.AuthType // Keep existing if not found
		}
	}

	// Build auth map - only update if provided
	authMap := currentSettings.Auth // Start with existing
	if req.Auth != nil && len(req.Auth) > 0 {
		authMap = req.Auth
	} else {
		// Ensure we have a map
		if authMap == nil {
			authMap = make(map[string]string)
		}
	}

	// Handle backward compatibility: if legacy token is provided, populate auth map
	if req.Token != "" && (authType == "token" || authType == "") {
		authMap["token"] = strings.TrimSpace(req.Token)
	}

	// Build settings struct with partial update support
	settings := db.Settings{
		Enabled: currentSettings.Enabled, // Default to current, may be overridden below
	}

	// Only set fields if they are provided in the request
	if req.Name != "" {
		settings.Name = strings.TrimSpace(req.Name)
	}

	settings.Platform = platform
	settings.AuthType = authType
	settings.Auth = authMap

	if req.ProxyURL != "" {
		settings.ProxyURL = strings.TrimSpace(req.ProxyURL)
	}

	if req.ChatID != "" {
		settings.ChatIDLock = strings.TrimSpace(req.ChatID)
	}

	if req.BashAllowlist != nil {
		settings.BashAllowlist = normalizeAllowlist(req.BashAllowlist)
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

	// Handle default_cwd config (partial update)
	if req.DefaultCwd != nil {
		settings.DefaultCwd = strings.TrimSpace(*req.DefaultCwd)
	} else {
		settings.DefaultCwd = currentSettings.DefaultCwd
	}

	// Handle default_agent config (partial update)
	if req.DefaultAgent != nil {
		settings.DefaultAgent = strings.TrimSpace(*req.DefaultAgent)
	} else {
		settings.DefaultAgent = currentSettings.DefaultAgent
	}

	// Handle enabled status
	if req.Enabled != nil {
		settings.Enabled = *req.Enabled
	}

	// Handle require_pairing (partial update); nil → leave unchanged in DB.
	if req.RequirePairing != nil {
		settings.RequirePairing = req.RequirePairing
	}

	if err := h.store.UpdateSettings(uuid, settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", uuid).Info("ImBot settings updated")

	// Update SmartGuide routing rule if provider/model configured
	// This ensures the routing rule stays in sync with the configuration
	if settings.SmartGuideProvider != "" && settings.SmartGuideModel != "" {
		if err := h.config.EnsureSmartGuideRuleForBot(uuid, settings.Name, settings.SmartGuideProvider, settings.SmartGuideModel); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"uuid":     uuid,
				"provider": settings.SmartGuideProvider,
				"model":    settings.SmartGuideModel,
			}).Error("Failed to update SmartGuide routing rule")
		} else {
			logrus.WithFields(logrus.Fields{
				"uuid":     uuid,
				"provider": settings.SmartGuideProvider,
				"model":    settings.SmartGuideModel,
			}).Info("SmartGuide routing rule updated")
		}
	}

	// Handle enabled status changes only
	// Config changes (provider, model, etc.) take effect automatically via dynamic lookup
	if currentSettings.Enabled != settings.Enabled {
		if h.botMgr != nil {
			ctx := context.Background()
			if settings.Enabled {
				// Disabled -> Enabled: start the bot
				go func() {
					if err := h.botMgr.StartBot(ctx, uuid); err != nil {
						logrus.WithError(err).WithField("uuid", uuid).Error("Failed to start bot after enabling")
					}
				}()
			} else {
				// Enabled -> Disabled: stop the bot
				go func() {
					if err := h.botMgr.StopBot(uuid); err != nil {
						logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to stop bot after disabling")
					}
				}()
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

	// Stop the bot if it's running (async for fast delete)
	// The bot will be stopped in the background while we delete from database
	if h.botMgr != nil {
		go func() {
			if err := h.botMgr.StopBot(uuid); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to stop bot during delete (continuing anyway)")
			}
		}()
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
	if h.botMgr != nil {
		ctx := context.Background()
		if newStatus {
			// Start the bot
			if err := h.botMgr.StartBot(ctx, uuid); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to start bot after toggle")
			}
		} else {
			// Stop the bot
			if err := h.botMgr.StopBot(uuid); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to stop bot after toggle")
			}
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

// SetChannelRegistry wires the remote channel registry through to the
// underlying bot manager. Used by the server during route registration so
// each running bot exposes itself as a remote.channel.Channel.
func (h *Handler) SetChannelRegistry(reg *channel.Registry) {
	if h.botMgr == nil {
		return
	}
	h.botMgr.SetChannelRegistry(reg)
}

// StartAllEnabled starts all enabled bots (delegates to BotManager)
func (h *Handler) StartAllEnabled(ctx context.Context) error {
	if h.botMgr == nil {
		return fmt.Errorf("bot manager is nil")
	}
	return h.botMgr.StartAllEnabled(ctx)
}

// RestartBotByUUID stops then starts a single bot. Used by both the admin
// HTTP endpoint and the LifecycleController interface.
func (h *Handler) RestartBotByUUID(ctx context.Context, uuid string) error {
	if h.botMgr == nil {
		return fmt.Errorf("bot manager is nil")
	}
	return h.botMgr.RestartBot(ctx, uuid)
}

// RestartBot is the HTTP handler for POST /imbot-admin/restart/:uuid.
// Restarts a single bot without affecting the rest of the server.
func (h *Handler) RestartBot(c *gin.Context) {
	if h.botMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Bot manager not available"})
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID is required"})
		return
	}

	if err := h.botMgr.RestartBot(c.Request.Context(), uuid); err != nil {
		logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to restart bot")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("uuid", uuid).Info("Bot restarted via admin API")
	c.JSON(http.StatusOK, gin.H{"success": true, "uuid": uuid, "running": h.botMgr.IsRunning(uuid)})
}

// Reload is the HTTP handler for POST /imbot-admin/reload. Re-reads bot
// settings and starts/stops bots to match the current enabled flags. Does
// not restart bots whose enabled state has not changed.
func (h *Handler) Reload(c *gin.Context) {
	if h.botMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Bot manager not available"})
		return
	}

	if err := h.botMgr.Sync(c.Request.Context()); err != nil {
		logrus.WithError(err).Warn("Failed to reload bots")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logrus.Info("Bot configuration reloaded via admin API")
	// Include per-bot status so callers (e.g. the CLI poke after `remote add`)
	// can report what actually happened: Sync swallows individual start
	// failures by design, so success alone doesn't mean every bot is up.
	c.JSON(http.StatusOK, gin.H{"success": true, "bots": h.botMgr.GetStatus()})
}

// StopAll stops all running bots (delegates to BotManager)
func (h *Handler) StopAll() {
	if h.botMgr != nil {
		h.botMgr.StopAll()
	}
}

// Sync ensures running bots match enabled settings (delegates to BotManager)
func (h *Handler) Sync(ctx context.Context) error {
	if h.botMgr == nil {
		return fmt.Errorf("bot manager is nil")
	}
	return h.botMgr.Sync(ctx)
}

// Shutdown stops all running bots and cleans up resources
func (h *Handler) Shutdown() {
	if h.botMgr != nil {
		h.botMgr.Shutdown()
	}
}
