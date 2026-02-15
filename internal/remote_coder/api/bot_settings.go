package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/remote_coder/bot"
)

type BotSettingsHandler struct {
	store *bot.Store
}

type BotSettingsPayload struct {
	Name          string            `json:"name,omitempty"`
	Platform      string            `json:"platform"`
	AuthType      string            `json:"auth_type"`
	Auth          map[string]string `json:"auth"`
	ProxyURL      string            `json:"proxy_url,omitempty"`
	ChatID        string            `json:"chat_id,omitempty"`
	BashAllowlist []string          `json:"bash_allowlist,omitempty"`
	// Legacy field for backward compatibility
	Token string `json:"token,omitempty"`
}

func NewBotSettingsHandler(store *bot.Store) *BotSettingsHandler {
	return &BotSettingsHandler{store: store}
}

func (h *BotSettingsHandler) GetSettings(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "bot store unavailable"})
		return
	}

	settings, err := h.store.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	response := gin.H{
		"success":        true,
		"name":           settings.Name,
		"platform":       settings.Platform,
		"auth_type":      settings.AuthType,
		"auth":           settings.Auth,
		"proxy_url":      settings.ProxyURL,
		"chat_id":        settings.ChatIDLock,
		"bash_allowlist": settings.BashAllowlist,
	}

	// Include legacy token field for backward compatibility
	if settings.Token != "" {
		response["token"] = settings.Token
	}

	c.JSON(http.StatusOK, response)
}

func (h *BotSettingsHandler) UpdateSettings(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "bot store unavailable"})
		return
	}

	var payload BotSettingsPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	platform := strings.TrimSpace(payload.Platform)
	if platform == "" {
		platform = "telegram"
	}

	// Get platform config to determine auth type if not provided
	authType := strings.TrimSpace(payload.AuthType)
	if authType == "" {
		if config, exists := bot.GetPlatformConfig(platform); exists {
			authType = config.AuthType
		}
	}

	// Handle backward compatibility: if legacy token is provided, populate auth map
	authMap := payload.Auth
	if authMap == nil {
		authMap = make(map[string]string)
	}
	if payload.Token != "" && authType == "token" {
		authMap["token"] = strings.TrimSpace(payload.Token)
	}

	if err := h.store.SaveSettings(bot.Settings{
		Name:          strings.TrimSpace(payload.Name),
		Platform:      platform,
		AuthType:      authType,
		Auth:          authMap,
		ProxyURL:      strings.TrimSpace(payload.ProxyURL),
		ChatIDLock:    strings.TrimSpace(payload.ChatID),
		BashAllowlist: normalizeAllowlist(payload.BashAllowlist),
		Token:         strings.TrimSpace(payload.Token), // Legacy field
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetPlatforms returns all supported bot platforms with their configurations
func (h *BotSettingsHandler) GetPlatforms(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "bot store unavailable"})
		return
	}

	platforms := bot.GetAllPlatforms()
	platformResponses := make([]gin.H, 0, len(platforms))

	for _, p := range platforms {
		platformResponses = append(platformResponses, gin.H{
			"platform":     p.Platform,
			"display_name": p.DisplayName,
			"auth_type":    p.AuthType,
			"category":     p.Category,
			"fields":       p.Fields,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"platforms": platformResponses,
		"categories": gin.H{
			"im":         bot.CategoryLabels["im"],
			"enterprise": bot.CategoryLabels["enterprise"],
			"business":   bot.CategoryLabels["business"],
		},
	})
}

// GetPlatformConfig returns auth configuration for a specific platform
func (h *BotSettingsHandler) GetPlatformConfig(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "bot store unavailable"})
		return
	}

	platform := c.Query("platform")
	if platform == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "platform parameter is required"})
		return
	}

	config, exists := bot.GetPlatformConfig(platform)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "unknown platform"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"platform": gin.H{
			"platform":     config.Platform,
			"display_name": config.DisplayName,
			"auth_type":    config.AuthType,
			"category":     config.Category,
			"fields":       config.Fields,
		},
	})
}

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
