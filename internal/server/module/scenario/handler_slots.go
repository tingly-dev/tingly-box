package scenario

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/bind"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// slotScenario reads the :scenario param and rejects anything that is not the
// Claude Code main routing or one of its profiles.
func (h *Handler) slotScenario(c *gin.Context) (typ.RuleScenario, bool) {
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return "", false
	}
	scenario := typ.RuleScenario(c.Param("scenario"))
	if !scenario.Is(typ.ScenarioClaudeCode) {
		apierr.Failure(c, http.StatusBadRequest, "Model slots are only available for claude_code")
		return "", false
	}
	return scenario, true
}

func (h *Handler) respondClaudeCodeSlots(c *gin.Context, scenario typ.RuleScenario, rule *typ.Rule) {
	slots := h.config.ResolveClaudeCodeSlots(scenario)
	c.JSON(http.StatusOK, ClaudeCodeSlotsResponse{
		Success: true,
		Data:    ClaudeCodeSlotsData{Slots: slots, Unified: config.ClaudeCodeSlotsUnified(slots), Rule: rule},
	})
}

// GetClaudeCodeSlots lists the Claude Code model slots with their rules.
func (h *Handler) GetClaudeCodeSlots(c *gin.Context) {
	scenario, ok := h.slotScenario(c)
	if !ok {
		return
	}
	h.respondClaudeCodeSlots(c, scenario, nil)
}

// SetClaudeCodeSlot binds one slot to a rule, or unbinds it.
func (h *Handler) SetClaudeCodeSlot(c *gin.Context) {
	scenario, ok := h.slotScenario(c)
	if !ok {
		return
	}
	var req ClaudeCodeSlotUpdateRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	if err := h.config.SetClaudeCodeSlot(scenario, c.Param("slot"), req.RuleUUID); err != nil {
		apierr.Failure(c, http.StatusBadRequest, err.Error())
		return
	}
	h.respondClaudeCodeSlots(c, scenario, nil)
}

// CreateClaudeCodeSlotRule gives a slot its own rule and binds it.
func (h *Handler) CreateClaudeCodeSlotRule(c *gin.Context) {
	scenario, ok := h.slotScenario(c)
	if !ok {
		return
	}
	rule, err := h.config.CreateClaudeCodeSlotRule(scenario, c.Param("slot"))
	if err != nil {
		apierr.Failure(c, http.StatusBadRequest, err.Error())
		return
	}
	h.respondClaudeCodeSlots(c, scenario, &rule)
}

// ApplyClaudeCodeSlotPreset rebinds every slot to a preset.
func (h *Handler) ApplyClaudeCodeSlotPreset(c *gin.Context) {
	scenario, ok := h.slotScenario(c)
	if !ok {
		return
	}
	var req ClaudeCodeSlotPresetRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	if err := h.config.ApplyClaudeCodeSlotPreset(scenario, req.Preset); err != nil {
		apierr.Failure(c, http.StatusBadRequest, err.Error())
		return
	}
	h.respondClaudeCodeSlots(c, scenario, nil)
}
