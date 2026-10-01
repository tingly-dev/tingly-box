package imbot

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/remote/control/bot"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
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

// logPairAudit records a web-UI pairing event through the regular application
// log, keeping the imbot.pair.* action family consistent whether the event
// came from a chat command or the web UI.
func logPairAudit(c *gin.Context, action, uuid, message string) {
	logrus.WithFields(logrus.Fields{
		"action":    action,
		"user_id":   c.GetString(constant.CtxKeyUserID),
		"client_ip": c.ClientIP(),
		"bot_uuid":  uuid,
		"by":        "web",
	}).Info(message)
}

// GetPairingCode reveals the bot's current TOFU pairing code so the operator
// can /bind from their DM. The cleartext code is included in the response;
// every reveal is recorded in the audit log.
func (h *Handler) GetPairingCode(c *gin.Context) {
	if h.store == nil {
		apierr.Message(c, http.StatusServiceUnavailable, "ImBot settings store not available")
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		apierr.Message(c, http.StatusBadRequest, "UUID is required")
		return
	}

	settings, err := h.store.GetSettingsByUUID(uuid)
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	if settings.UUID == "" {
		apierr.Message(c, http.StatusNotFound, "ImBot settings not found")
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
		apierr.Message(c, http.StatusServiceUnavailable, "Bot manager unavailable")
		return
	}
	pm := h.botMgr.PairingManager()
	if pm == nil {
		apierr.Message(c, http.StatusServiceUnavailable, "Pairing manager unavailable")
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

	logPairAudit(c, "imbot.pair.reveal", uuid, "pairing code revealed via web UI")

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
		apierr.Message(c, http.StatusServiceUnavailable, "ImBot settings store not available")
		return
	}

	uuid := c.Param("uuid")
	if uuid == "" {
		apierr.Message(c, http.StatusBadRequest, "UUID is required")
		return
	}

	settings, err := h.store.GetSettingsByUUID(uuid)
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	if settings.UUID == "" {
		apierr.Message(c, http.StatusNotFound, "ImBot settings not found")
		return
	}

	if !resolveRequirePairing(settings) {
		apierr.Message(c, http.StatusBadRequest, "TOFU pairing is not enabled for this bot. Enable Require Pairing in the bot settings first.")
		return
	}

	if h.botMgr == nil {
		apierr.Message(c, http.StatusServiceUnavailable, "Bot manager unavailable")
		return
	}
	pm := h.botMgr.PairingManager()
	if pm == nil {
		apierr.Message(c, http.StatusServiceUnavailable, "Pairing manager unavailable")
		return
	}

	code, expiresAt := pm.Mint(uuid)
	if code == "" {
		apierr.Message(c, http.StatusInternalServerError, "Failed to mint pairing code")
		return
	}

	logPairAudit(c, "imbot.pair.rotate", uuid, "pairing code rotated via web UI")

	c.JSON(http.StatusOK, PairingCodeResponse{
		Success:   true,
		Active:    true,
		Code:      code,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	})
}
