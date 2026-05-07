package imbot

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
)

// resolveRequirePairing applies the same tri-state logic as
// bot.BotSetting.IsRequirePairing on a db.Settings row: explicit value wins,
// nil falls back to the platform default.
func resolveRequirePairing(s db.Settings) bool {
	if s.RequirePairing != nil {
		return *s.RequirePairing
	}
	return bot.PlatformDefaultsRequirePairing(s.Platform)
}

// GetPairingCode reveals the bot's current TOFU pairing code so the operator
// can /bind from their DM. The cleartext code is included in the response;
// every reveal is recorded in the audit log.
func (h *Handler) GetPairingCode(c *gin.Context) {
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
	if settings.UUID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "ImBot settings not found"})
		return
	}

	if !resolveRequirePairing(settings) {
		c.JSON(http.StatusOK, PairingCodeResponse{
			Success: true,
			Active:  false,
			Message: "TOFU pairing is not enabled for this bot.",
		})
		return
	}

	if h.botMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Bot manager unavailable"})
		return
	}
	pm := h.botMgr.PairingManager()
	if pm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Pairing manager unavailable"})
		return
	}

	code, expiresAt, ok := pm.Current(uuid)
	if !ok || code == "" {
		c.JSON(http.StatusOK, PairingCodeResponse{
			Success: true,
			Active:  false,
			Message: "No active pairing code. The bot may be stopped, or the code was already consumed. Click Rotate to mint a new one.",
		})
		return
	}

	if audit := h.botMgr.AuditLogger(); audit != nil {
		audit.Info("imbot.pair.reveal", c.GetString("user_id"), c.ClientIP(),
			"pairing code revealed via web UI",
			map[string]interface{}{
				"bot_uuid": uuid,
				"by":       "web",
			})
	}
	logrus.WithField("uuid", uuid).Info("ImBot pairing code revealed")

	c.JSON(http.StatusOK, PairingCodeResponse{
		Success:   true,
		Active:    true,
		Code:      code,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	})
}

// RotatePairingCode mints a fresh pairing code, replacing any existing one.
// The previous code is invalidated immediately. Every rotation is audited.
func (h *Handler) RotatePairingCode(c *gin.Context) {
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
	if settings.UUID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "ImBot settings not found"})
		return
	}

	if !resolveRequirePairing(settings) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "TOFU pairing is not enabled for this bot. Enable Require Pairing in the bot settings first.",
		})
		return
	}

	if h.botMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Bot manager unavailable"})
		return
	}
	pm := h.botMgr.PairingManager()
	if pm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Pairing manager unavailable"})
		return
	}

	code, expiresAt := pm.Mint(uuid)
	if code == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mint pairing code"})
		return
	}

	if audit := h.botMgr.AuditLogger(); audit != nil {
		audit.Info("imbot.pair.rotate", c.GetString("user_id"), c.ClientIP(),
			"pairing code rotated via web UI",
			map[string]interface{}{
				"bot_uuid": uuid,
				"by":       "web",
			})
	}
	logrus.WithField("uuid", uuid).Info("ImBot pairing code rotated")

	c.JSON(http.StatusOK, PairingCodeResponse{
		Success:   true,
		Active:    true,
		Code:      code,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	})
}
