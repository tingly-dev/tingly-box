package server

import (
	"fmt"
	"net/http"
	"tingly-box/internal/config"
	"tingly-box/internal/obs"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetRules returns all rules
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

	rule := cfg.GetRequestConfigByRequestModel(ruleUUID)
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

func (s *Server) AddRule(c *gin.Context) {
	var rule config.Rule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	uid, err := uuid.NewUUID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	rule.UUID = uid.String()

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
	response.Data.Provider = rule.GetDefaultProvider()
	response.Data.DefaultModel = rule.GetDefaultModel()
	response.Data.Active = rule.Active

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

	var rule config.Rule

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
	response.Data.RequestModel = rule.RequestModel
	response.Data.ResponseModel = rule.ResponseModel
	response.Data.Provider = rule.GetDefaultProvider()
	response.Data.DefaultModel = rule.GetDefaultModel()
	response.Data.Active = rule.Active

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
