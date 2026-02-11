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
	Token     string   `json:"token"`
	Allowlist []string `json:"allowlist"`
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

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"token":     settings.Token,
		"allowlist": settings.Allowlist,
	})
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

	cleanAllowlist := make([]string, 0, len(payload.Allowlist))
	for _, entry := range payload.Allowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		cleanAllowlist = append(cleanAllowlist, entry)
	}

	if err := h.store.SaveSettings(bot.Settings{
		Token:     strings.TrimSpace(payload.Token),
		Allowlist: cleanAllowlist,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
