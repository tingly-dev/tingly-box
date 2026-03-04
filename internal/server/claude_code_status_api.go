package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// tbModelMappingResult contains the result of model mapping lookup
type tbModelMappingResult struct {
	providerName string
	providerUUID string
	model        string
	scenario     string
}

// getTBModelMapping looks up the model mapping from Tingly Box configuration
// It queries the routing rules to find which provider/model would be used for the given model and scenario
func (s *Server) getTBModelMapping(modelID string, scenario typ.RuleScenario) *tbModelMappingResult {
	if s.config == nil || modelID == "" {
		return nil
	}

	rule := s.config.MatchRuleByModelAndScenario(modelID, scenario)
	if rule == nil {
		return nil
	}

	// Get the service that would be selected
	service, err := s.loadBalancer.SelectService(rule)
	if err != nil || service == nil {
		return nil
	}

	// Find the provider (service.Provider stores the provider UUID)
	provider, err := s.config.GetProviderByUUID(service.Provider)
	if err != nil || provider == nil {
		return nil
	}

	return &tbModelMappingResult{
		providerName: provider.Name,
		providerUUID: provider.UUID,
		model:        service.Model,
		scenario:     string(scenario),
	}
}

// GetClaudeCodeStatus returns combined status from Claude Code input and Tingly Box
// This endpoint receives Claude Code status JSON and combines it with Tingly Box model mapping
// POST /tingly/claude_code/status
func (s *Server) GetClaudeCodeStatus(c *gin.Context) {
	scenario := c.Param("scenario")

	var input ClaudeCodeStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		// If no body provided, use empty defaults
		input = ClaudeCodeStatusInput{}
	}

	// Get cache and merge with cached values for zero/empty fields
	cache := GetGlobalClaudeCodeStatusCache()
	merged := cache.Get(&input)

	// Update cache with new input (even if partial)
	cache.Update(&input)

	// Build response
	resp := &ClaudeCodeCombinedStatusData{
		CCModel:             merged.Model.DisplayName,
		CCUsedPct:           int(merged.ContextWindow.UsedPercentage),
		CCUsedTokens:        merged.ContextWindow.TotalInputTokens + merged.ContextWindow.TotalOutputTokens,
		CCMaxTokens:         merged.ContextWindow.ContextWindowSize,
		CCCost:              merged.Cost.TotalCostUSD,
		CCDurationMs:        merged.Cost.TotalDurationMs,
		CCAPIDurationMs:     merged.Cost.TotalAPIDurationMs,
		CCLinesAdded:        merged.Cost.TotalLinesAdded,
		CCLinesRemoved:      merged.Cost.TotalLinesRemoved,
		CCSessionID:         merged.SessionID,
		CCExceeds200kTokens: merged.Exceeds200kTokens,
	}

	// Query Tingly Box model mapping
	if mapping := s.getTBModelMapping(merged.Model.ID, typ.RuleScenario(scenario)); mapping != nil {
		resp.TBProviderName = mapping.providerName
		resp.TBProviderUUID = mapping.providerUUID
		resp.TBModel = mapping.model
		resp.TBRequestModel = merged.Model.ID
		resp.TBScenario = mapping.scenario
	}

	c.JSON(http.StatusOK, ClaudeCodeCombinedStatus{
		Success: true,
		Data:    resp,
	})
}

// GetClaudeCodeStatusLine returns rendered status line text for Claude Code
// This endpoint receives Claude Code status JSON and returns a pre-rendered status line string
// POST /tingly/:scenario/statusline
func (s *Server) GetClaudeCodeStatusLine(c *gin.Context) {
	scenario := c.Param("scenario")

	var input ClaudeCodeStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		// If no body provided, use empty defaults
		input = ClaudeCodeStatusInput{}
	}

	// Get cache and merge with cached values for zero/empty fields
	cache := GetGlobalClaudeCodeStatusCache()
	merged := cache.Get(&input)

	// Update cache with new input (even if partial)
	cache.Update(&input)

	// Build status line
	// Format: [CC Model] → TB Model@Provider | ▓▓▓░░░░░ 7% | $0.05
	ccModel := merged.Model.DisplayName
	if ccModel == "" {
		ccModel = "unknown"
	}

	usedPct := int(merged.ContextWindow.UsedPercentage)
	cost := merged.Cost.TotalCostUSD

	// Build context bar (8 characters wide)
	barWidth := 8
	filled := usedPct * barWidth / 100
	empty := barWidth - filled
	bar := ""
	for i := 0; i < filled; i++ {
		bar += "▓"
	}
	for i := 0; i < empty; i++ {
		bar += "░"
	}

	// Build output
	output := fmt.Sprintf("[%s]", ccModel)

	// Query Tingly Box model mapping and add info if available
	if mapping := s.getTBModelMapping(merged.Model.ID, typ.RuleScenario(scenario)); mapping != nil && mapping.model != "" {
		output += fmt.Sprintf(" → %s@%s", mapping.model, mapping.providerName)
	}

	// Add context bar and cost
	output += fmt.Sprintf(" | %s %d%% | $%.2f", bar, usedPct, cost)

	c.String(http.StatusOK, output)
}
