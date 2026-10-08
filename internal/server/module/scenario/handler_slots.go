package scenario

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/bind"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SetClaudeCodeSlot gives a Claude Code model slot its own rule, or hands it
// back to the main rule.
func (h *Handler) SetClaudeCodeSlot(c *gin.Context) {
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}
	var req ClaudeCodeSlotRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	rule, err := h.config.SetClaudeCodeSlot(typ.RuleScenario(c.Param("scenario")), c.Param("slot"), req.Enabled)
	if err != nil {
		apierr.Failure(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, ClaudeCodeSlotResponse{Success: true, Data: rule})
}
