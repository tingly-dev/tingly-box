package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tingly-box/internal/obs"
	"tingly-box/internal/typ"
)

// GetRules returns all rules, require filtered by scenario
func (s *Server) GetRules(c *gin.Context) {
	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	rules := cfg.GetRequestConfigs()

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

// GetRule returns a specific rule by name
func (s *Server) GetRule(c *gin.Context) {
	ruleUUID := c.Param("uuid")
	if ruleUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Rule UUID is required",
		})
		return
	}

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	rule := cfg.GetRuleByUUID(ruleUUID)
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

func (s *Server) CreateRule(c *gin.Context) {
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

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	if err := cfg.AddRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save rule: " + err.Error(),
		})
		return
	}

	// Log the action
	if s.logger != nil {
		s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
			"name": rule.RequestModel,
		}, true, fmt.Sprintf("Rule %s updated successfully", rule.RequestModel))
	}

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
func (s *Server) UpdateRule(c *gin.Context) {
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

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	rule.UUID = uid
	if err := cfg.UpdateRule(uid, rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save rule: " + err.Error(),
		})
		return
	}

	// Log the action
	if s.logger != nil {
		s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
			"name": uid,
		}, true, fmt.Sprintf("Rule %s updated successfully", uid))
	}

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

func (s *Server) DeleteRule(c *gin.Context) {
	ruleUUID := c.Param("uuid")
	if ruleUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Rule name is required",
		})
		return
	}

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	err := cfg.DeleteRule(ruleUUID)
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
