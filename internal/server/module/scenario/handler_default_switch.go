package scenario

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/bind"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// UseClaudeCodeProfile overwrites the default Claude Code routing with a copy of
// a profile's. Only the main claude_code scenario can be switched.
func (h *Handler) UseClaudeCodeProfile(c *gin.Context) {
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}
	if typ.RuleScenario(c.Param("scenario")) != typ.ScenarioClaudeCode {
		apierr.Failure(c, http.StatusBadRequest, "only the default Claude Code configuration can use a profile")
		return
	}
	var req ClaudeCodeUseProfileRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	if err := h.config.UseProfileAsDefault(req.Profile); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, config.ErrProfileNotFound) {
			status = http.StatusNotFound
		}
		apierr.Failure(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, ClaudeCodeUseProfileResponse{Success: true})
}
