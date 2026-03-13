package rule

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/server/config"
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
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
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario not found in request",
		})
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
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Rule UUID is required",
		})
		return
	}

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	rule := h.config.GetRuleByUUID(ruleUUID)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Rule not found",
		})
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
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	if rule.Scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Unknown scenario",
		})
		return
	}
	rule.UUID = uuid.NewString()

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	if err := h.config.AddRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save rule: " + err.Error(),
		})
		return
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
	response.Data.RequestModel = rule.RequestModel
	response.Data.ResponseModel = rule.ResponseModel
	response.Data.Description = rule.Description
	response.Data.Provider = rule.GetDefaultProvider()
	response.Data.DefaultModel = rule.GetDefaultModel()
	response.Data.Active = rule.Active
	response.Data.SmartEnabled = rule.SmartEnabled
	response.Data.SmartRouting = rule.SmartRouting

	c.JSON(http.StatusOK, response)
}

// UpdateRule creates or updates a rule
func (h *Handler) UpdateRule(c *gin.Context) {
	uid := c.Param("uuid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Rule name is required",
		})
		return
	}

	var rule typ.Rule

	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	rule.UUID = uid
	if err := h.config.UpdateRule(uid, rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save rule: " + err.Error(),
		})
		return
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
	response.Data.RequestModel = rule.RequestModel
	response.Data.ResponseModel = rule.ResponseModel
	response.Data.Description = rule.Description
	response.Data.Provider = rule.GetDefaultProvider()
	response.Data.DefaultModel = rule.GetDefaultModel()
	response.Data.Active = rule.Active
	response.Data.SmartEnabled = rule.SmartEnabled
	response.Data.SmartRouting = rule.SmartRouting

	c.JSON(http.StatusOK, response)
}

// DeleteRule deletes a rule
func (h *Handler) DeleteRule(c *gin.Context) {
	ruleUUID := c.Param("uuid")
	if ruleUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Rule name is required",
		})
		return
	}

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	err := h.config.DeleteRule(ruleUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete rule: " + err.Error(),
		})
		return
	}

	response := DeleteRuleResponse{
		Success: true,
		Message: "Rule deleted successfully",
	}

	c.JSON(http.StatusOK, response)
}
