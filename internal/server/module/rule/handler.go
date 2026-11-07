package rule

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/bind"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles rule HTTP requests
type Handler struct {
	config *config.Config
}

// NewHandler creates a new rule handler
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		config: cfg,
	}
}

// GetRules returns all rules, filtered by scenario
func (h *Handler) GetRules(c *gin.Context) {
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}

	rules := h.config.GetRequestConfigs()

	// Filter by scenario if provided
	scenario := c.Query("scenario")
	if scenario != "" {
		filteredRules := make([]typ.Rule, 0)
		for _, rule := range rules {
			if string(rule.GetScenario()) == scenario {
				filteredRules = append(filteredRules, rule)
			}
		}
		rules = filteredRules
	} else {
		apierr.Failure(c, http.StatusBadRequest, "Scenario not found in request")
		return
	}

	response := RulesResponse{
		Success: true,
		Data:    rules,
	}

	c.JSON(http.StatusOK, response)
}

// GetRule returns a specific rule by UUID
func (h *Handler) GetRule(c *gin.Context) {
	ruleUUID := c.Param("uuid")
	if ruleUUID == "" {
		apierr.Failure(c, http.StatusBadRequest, "Rule UUID is required")
		return
	}

	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}

	rule := h.config.GetRuleByUUID(ruleUUID)
	if rule == nil {
		apierr.Failure(c, http.StatusNotFound, "Rule not found")
		return
	}

	response := RuleResponse{
		Success: true,
		Data:    rule,
	}

	c.JSON(http.StatusOK, response)
}

// CreateRule creates a new rule
func (h *Handler) CreateRule(c *gin.Context) {
	var rule typ.Rule
	if !bind.JSON(c, &rule, apierr.Failure) {
		return
	}
	if rule.Scenario == "" {
		apierr.Failure(c, http.StatusBadRequest, "Unknown scenario")
		return
	}
	if !typ.CanBindRulesToScenario(rule.Scenario) {
		apierr.Failure(c, http.StatusBadRequest, "Unknown scenario")
		return
	}
	rule.UUID = uuid.NewString()

	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}

	if err := h.config.AddRule(rule); err != nil {
		apierr.Failure(c, http.StatusInternalServerError, "Failed to save rule: "+err.Error())
		return
	}

	// Echo what was actually persisted: AddRule seeds per-scenario creation
	// defaults (e.g. team rules get claude_code_compat + clean_header), so the
	// client refreshes its local state from the stored rule, not the raw payload.
	if saved := h.config.GetRuleByUUID(rule.UUID); saved != nil {
		rule = *saved
	}

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action":        "update_rule",
		"uuid":          rule.UUID,
		"request_model": rule.RequestModel,
	}).Info(fmt.Sprintf("Rule %s created successfully", rule.UUID))

	response := UpdateRuleResponse{
		Success: true,
		Message: "Rule saved successfully",
	}
	response.Data.UUID = rule.UUID
	response.Data.Scenario = string(rule.Scenario)
	response.Data.RequestModel = rule.RequestModel
	response.Data.ResponseModel = rule.ResponseModel
	response.Data.Description = rule.Description
	response.Data.Provider = rule.GetDefaultProvider()
	response.Data.DefaultModel = rule.GetDefaultModel()
	response.Data.Active = rule.Active
	response.Data.SmartEnabled = rule.SmartEnabled
	response.Data.SmartRouting = rule.SmartRouting
	response.Data.Flags = rule.Flags

	c.JSON(http.StatusOK, response)
}

// UpdateRule creates or updates a rule
func (h *Handler) UpdateRule(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		apierr.Failure(c, http.StatusBadRequest, "Rule name is required")
		return
	}

	var rule typ.Rule

	if !bind.JSON(c, &rule, apierr.Failure) {
		return
	}

	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}
	if !typ.CanBindRulesToScenario(rule.Scenario) {
		apierr.Failure(c, http.StatusBadRequest, "Unknown scenario")
		return
	}

	rule.UUID = uid
	if err := h.config.UpdateRule(uid, rule); err != nil {
		apierr.Failure(c, http.StatusInternalServerError, "Failed to save rule: "+err.Error())
		return
	}

	// Echo what was actually persisted: UpdateRule may normalize the rule
	// (e.g. Claude Desktop request models get the [1m] suffix synced with
	// the context_1m flag), and the client refreshes its local state from
	// this response.
	if saved := h.config.GetRuleByUUID(uid); saved != nil {
		rule = *saved
	}

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action": "update_rule",
		"uuid":   uid,
	}).Info(fmt.Sprintf("Rule %s updated successfully", uid))

	response := UpdateRuleResponse{
		Success: true,
		Message: "Rule saved successfully",
	}
	response.Data.UUID = rule.UUID
	response.Data.Scenario = string(rule.Scenario)
	response.Data.RequestModel = rule.RequestModel
	response.Data.ResponseModel = rule.ResponseModel
	response.Data.Description = rule.Description
	response.Data.Provider = rule.GetDefaultProvider()
	response.Data.DefaultModel = rule.GetDefaultModel()
	response.Data.Active = rule.Active
	response.Data.SmartEnabled = rule.SmartEnabled
	response.Data.SmartRouting = rule.SmartRouting
	response.Data.Flags = rule.Flags

	c.JSON(http.StatusOK, response)
}

// DeleteRule deletes a rule
func (h *Handler) DeleteRule(c *gin.Context) {
	ruleUUID := c.Param("uuid")
	if ruleUUID == "" {
		apierr.Failure(c, http.StatusBadRequest, "Rule name is required")
		return
	}

	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}

	err := h.config.DeleteRule(ruleUUID)
	if err != nil {
		apierr.Failure(c, http.StatusInternalServerError, "Failed to delete rule: "+err.Error())
		return
	}

	response := DeleteRuleResponse{
		Success: true,
		Message: "Rule deleted successfully",
	}

	c.JSON(http.StatusOK, response)
}

// GetFlagRegistry returns the catalog of supported rule-level flags.
func (h *Handler) GetFlagRegistry(c *gin.Context) {
	c.JSON(http.StatusOK, FlagRegistryResponse{
		Success: true,
		Data:    typ.RuleFlagRegistry(),
	})
}

// ReorderRules applies a new list order to one scenario's rules. The order is
// hard state — it reaches /v1/models, match priority and client config
// generation — and is persisted through the same Save() choke point as every
// other rule mutation. Unknown UUIDs and UUIDs from another scenario are
// rejected; rules the caller has not named keep their relative order and
// follow the named ones, so a stale client never drops a concurrent creation.
func (h *Handler) ReorderRules(c *gin.Context) {
	var req ReorderRulesRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}

	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "Global config not available")
		return
	}
	if req.Scenario == "" {
		apierr.Failure(c, http.StatusBadRequest, "Scenario is required")
		return
	}
	if len(req.OrderedUUIDs) == 0 {
		apierr.Failure(c, http.StatusBadRequest, "ordered_uuids is required")
		return
	}

	rules, err := h.config.ReorderRules(typ.RuleScenario(req.Scenario), req.OrderedUUIDs)
	if err != nil {
		apierr.Failure(c, http.StatusBadRequest, "Failed to reorder rules: "+err.Error())
		return
	}

	logrus.WithFields(logrus.Fields{
		"action":   "reorder_rules",
		"scenario": req.Scenario,
		"count":    len(rules),
	}).Info(fmt.Sprintf("Reordered %d rule(s) in scenario %s", len(rules), req.Scenario))

	// Echo the reordered subset so the client can replace its local state
	// with what actually persisted, mirroring Create/UpdateRule.
	c.JSON(http.StatusOK, RulesResponse{
		Success: true,
		Data:    rules,
	})
}
