package scenario

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/bind"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GetClaudeCodeSlots lists the slots with a rule of their own.
func (h *Handler) GetClaudeCodeSlots(c *gin.Context) {
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}
	slots := h.config.ClaudeCodeSlots(typ.RuleScenario(c.Param("scenario")))
	if slots == nil {
		slots = []string{}
	}
	c.JSON(http.StatusOK, ClaudeCodeSlotsResponse{Success: true, Data: ClaudeCodeSlotsData{Slots: slots}})
}

// SetClaudeCodeSlot gives a slot a rule of its own, or hands it back to the
// main rule.
func (h *Handler) SetClaudeCodeSlot(c *gin.Context) {
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}
	var req ClaudeCodeSlotRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	scenario := typ.RuleScenario(c.Param("scenario"))
	rule, err := h.config.SetClaudeCodeSlot(scenario, c.Param("slot"), req.Enabled)
	if err != nil {
		apierr.Failure(c, http.StatusBadRequest, err.Error())
		return
	}
	slots := h.config.ClaudeCodeSlots(scenario)
	if slots == nil {
		slots = []string{}
	}
	c.JSON(http.StatusOK, ClaudeCodeSlotsResponse{Success: true, Data: ClaudeCodeSlotsData{Slots: slots, Rule: &rule}})
}
