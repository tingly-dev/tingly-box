package scenario

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/bind"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// defaultSwitchReady rejects everything but the main Claude Code scenario:
// only the default configuration can be switched to a profile.
func (h *Handler) defaultSwitchReady(c *gin.Context) bool {
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return false
	}
	if typ.RuleScenario(c.Param("scenario")) != typ.ScenarioClaudeCode {
		apierr.Failure(c, http.StatusBadRequest, "only the default Claude Code configuration can be switched to a profile")
		return false
	}
	return true
}

// GetClaudeCodeDefaultSwitch reports whether the default is switched to a profile.
func (h *Handler) GetClaudeCodeDefaultSwitch(c *gin.Context) {
	if !h.defaultSwitchReady(c) {
		return
	}
	c.JSON(http.StatusOK, ClaudeCodeDefaultSwitchResponse{Success: true, Data: h.config.GetClaudeCodeDefaultSwitch()})
}

// SwitchClaudeCodeDefault makes the default Claude Code routing a copy of a profile's.
func (h *Handler) SwitchClaudeCodeDefault(c *gin.Context) {
	if !h.defaultSwitchReady(c) {
		return
	}
	var req ClaudeCodeDefaultSwitchRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	status, err := h.config.SwitchClaudeCodeDefaultToProfile(req.Profile)
	if err != nil {
		apierr.Failure(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, ClaudeCodeDefaultSwitchResponse{Success: true, Data: status})
}

// RestoreClaudeCodeDefault puts the default's own routing back.
func (h *Handler) RestoreClaudeCodeDefault(c *gin.Context) {
	if !h.defaultSwitchReady(c) {
		return
	}
	if err := h.config.RestoreClaudeCodeDefault(); err != nil {
		apierr.Failure(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, ClaudeCodeDefaultSwitchResponse{Success: true, Data: h.config.GetClaudeCodeDefaultSwitch()})
}
