package statusline

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// LoadBalancer interface defines the load balancer operations we need
type LoadBalancer interface {
	SelectService(rule *typ.Rule) (*loadbalance.Service, error)
}

// Handler handles Claude Code status HTTP requests
type Handler struct {
	config       *config.Config
	loadBalancer LoadBalancer
	cache        *Cache
	quotaMgr     QuotaManager // quota manager for fetching quota
}

// QuotaManager defines the quota manager interface
type QuotaManager interface {
	GetQuota(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
}

// NewHandler creates a new Claude Code handler
func NewHandler(cfg *config.Config, lb LoadBalancer, cache *Cache, quotaMgr QuotaManager) *Handler {
	return &Handler{
		config:       cfg,
		loadBalancer: lb,
		cache:        cache,
		quotaMgr:     quotaMgr, // Can be nil if quota not enabled
	}
}

// GetClaudeCodeStatus returns combined status from Claude Code input and Tingly Box
// This endpoint receives Claude Code status JSON and combines it with Tingly Box model mapping
// POST /tingly/:scenario/status
func (h *Handler) GetClaudeCodeStatus(c *gin.Context) {
	scenario := c.Param("scenario")

	var input StatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		// If no body provided, use empty defaults
		input = StatusInput{}
	}

	// Get cache and merge with cached values for zero/empty fields
	merged := h.cache.Get(&input)

	// Update cache with new input (even if partial)
	h.cache.Update(&input)

	// Build response
	resp := &CombinedStatusData{
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
	if mapping := h.getTBModelMapping(merged.Model.ID, typ.RuleScenario(scenario)); mapping != nil {
		resp.TBProviderName = mapping.providerName
		resp.TBProviderUUID = mapping.providerUUID
		resp.TBModel = mapping.model
		resp.TBRequestModel = merged.Model.ID
		resp.TBScenario = mapping.scenario

		// Fetch quota information
		h.populateQuotaData(resp, mapping.providerUUID)
	}

	c.JSON(http.StatusOK, CombinedStatus{
		Success: true,
		Data:    resp,
	})
}

// GetClaudeCodeStatusLine returns rendered status line text for Claude Code
// This endpoint receives Claude Code status JSON and returns a pre-rendered status line string
// POST /tingly/:scenario/statusline
// ref: https://code.claude.com/docs/en/statusline
func (h *Handler) GetClaudeCodeStatusLine(c *gin.Context) {
	scenario := c.Param("scenario")

	var input StatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		// If no body provided, use empty defaults
		input = StatusInput{}
	}

	// Get cache and merge with cached values for zero/empty fields
	merged := h.cache.Get(&input)

	// Update cache with new input (even if partial)
	h.cache.Update(&input)

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

	// Build profile label: "p1:name" or empty
	profileLabel := ""
	base, profileID := typ.ParseScenarioProfile(typ.RuleScenario(scenario))
	if profileID != "" {
		profileName := profileID
		if meta, ok := h.config.GetProfile(base, profileID); ok {
			profileName = profileID + ":" + meta.Name
		}
		profileLabel = profileName
	}

	// Query Tingly Box model mapping
	mapping := h.getTBModelMapping(merged.Model.ID, typ.RuleScenario(scenario))

	// Build output: [ruleModel @ p1:name] -> realModel @ providerName | bar pct | cost
	ruleModel := merged.Model.ID
	if ruleModel == "" {
		ruleModel = ccModel
	}

	output := fmt.Sprintf("[%s", ruleModel)
	if profileLabel != "" {
		output += fmt.Sprintf(" @ %s", profileLabel)
	}
	output += "]"
	if mapping != nil && mapping.model != "" {
		output += fmt.Sprintf(" → %s @ %s", mapping.model, mapping.providerName)
	}
	output += fmt.Sprintf(" | %s %d%% | $%.2f", bar, usedPct, cost)

	// Add quota info to the same line if available
	quotaInfo := h.buildQuotaInline(mapping)
	if quotaInfo != "" {
		output += quotaInfo
	}

	c.String(http.StatusOK, output)
}

// tbModelMappingResult contains the result of model mapping lookup
type tbModelMappingResult struct {
	providerName string
	providerUUID string
	model        string
	scenario     string
}

// getTBModelMapping looks up the model mapping from Tingly Box configuration
// It queries the routing rules to find which provider/model would be used for the given model and scenario
func (h *Handler) getTBModelMapping(modelID string, scenario typ.RuleScenario) *tbModelMappingResult {
	if h.config == nil || modelID == "" {
		return nil
	}

	rule := h.config.MatchRuleByModelAndScenario(modelID, scenario)
	if rule == nil {
		return nil
	}

	// Get the service that would be selected
	service, err := h.loadBalancer.SelectService(rule)
	if err != nil || service == nil {
		return nil
	}

	// Find the provider
	provider, err := h.config.GetProviderByUUID(service.Provider)
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

// populateQuotaData fetches and populates quota information for the given provider
func (h *Handler) populateQuotaData(resp *CombinedStatusData, providerUUID string) {
	if h.quotaMgr == nil || providerUUID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	usage, err := h.quotaMgr.GetQuota(ctx, providerUUID)
	if err != nil {
		// Silently fail - don't populate quota data on error
		return
	}

	// Select the best quota window
	window := h.selectBestQuotaWindow(usage)
	if window == nil {
		return
	}

	resp.TBQuotaAvailable = true
	resp.TBQuotaUsed = int(window.Used)
	resp.TBQuotaLimit = int(window.Limit)
	resp.TBQuotaPercent = int(window.UsedPercent)
	resp.TBQuotaWindow = string(window.Type)
	resp.TBQuotaUnit = string(window.Unit)

	if window.ResetsAt != nil {
		resp.TBQuotaResetsAt = window.ResetsAt.Format(time.RFC3339)
	}
}

// selectBestQuotaWindow selects the most relevant quota window
// Priority: session > daily > weekly > monthly > balance
// Falls back to Primary if no matching type found
func (h *Handler) selectBestQuotaWindow(usage *quota.ProviderUsage) *quota.UsageWindow {
	if usage == nil {
		return nil
	}

	priorityOrder := []quota.WindowType{
		quota.WindowTypeSession,
		quota.WindowTypeDaily,
		quota.WindowTypeWeekly,
		quota.WindowTypeMonthly,
		quota.WindowTypeBalance,
	}

	// Check in priority order - look for windows with actual limit data
	for _, windowType := range priorityOrder {
		if w := getWindowByType(usage, windowType); w != nil {
			// Accept if it has a meaningful limit OR if it has percentage data
			if w.Limit > 0 || (w.Limit > 0 && w.UsedPercent > 0) {
				return w
			}
		}
	}

	// Fallback: try Primary window even if limit is 0 (might be percentage-based)
	if usage.Primary != nil {
		// For percentage-based quotas (like Gemini), Limit=100 means 100%
		if usage.Primary.Limit > 0 || usage.Primary.UsedPercent > 0 {
			return usage.Primary
		}
	}

	return nil
}

// getWindowByType gets a window by type from ProviderUsage
func getWindowByType(usage *quota.ProviderUsage, windowType quota.WindowType) *quota.UsageWindow {
	if usage.Primary != nil && usage.Primary.Type == windowType {
		return usage.Primary
	}
	if usage.Secondary != nil && usage.Secondary.Type == windowType {
		return usage.Secondary
	}
	if usage.Tertiary != nil && usage.Tertiary.Type == windowType {
		return usage.Tertiary
	}
	return nil
}

// buildQuotaInline builds quota information for inline display in statusline
// Format: " | Quota: 40/100 tokens" or " | Quota: 40K/100K tokens | 100/500 req"
func (h *Handler) buildQuotaInline(mapping *tbModelMappingResult) string {
	if h.quotaMgr == nil || mapping == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	usage, err := h.quotaMgr.GetQuota(ctx, mapping.providerUUID)
	if err != nil {
		// Silently fail - quota unavailable
		return ""
	}

	// Collect all windows with meaningful data
	var windows []*quota.UsageWindow
	if usage.Primary != nil && usage.Primary.Limit > 0 {
		windows = append(windows, usage.Primary)
	}
	if usage.Secondary != nil && usage.Secondary.Limit > 0 {
		windows = append(windows, usage.Secondary)
	}
	if usage.Tertiary != nil && usage.Tertiary.Limit > 0 {
		windows = append(windows, usage.Tertiary)
	}

	if len(windows) == 0 {
		return ""
	}

	// Build quota string for each window
	var parts []string
	for _, w := range windows {
		part := h.formatQuotaWindow(w)
		if part != "" {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	// Join all quota parts
	result := " | Quota:"
	for _, p := range parts {
		result += " " + p
	}
	return result
}

// formatQuotaWindow formats a single quota window
func (h *Handler) formatQuotaWindow(window *quota.UsageWindow) string {
	used := window.Used
	limit := window.Limit

	if limit <= 0 {
		return ""
	}

	// Format based on unit
	switch window.Unit {
	case quota.UsageUnitTokens:
		if limit >= 1000000 {
			return fmt.Sprintf("%.1fM/%.1fM", used/1000000, limit/1000000)
		} else if limit >= 10000 {
			return fmt.Sprintf("%.0fK/%.0fK", used/1000, limit/1000)
		}
		return fmt.Sprintf("%.0f/%.0f", used, limit)
	case quota.UsageUnitRequests:
		// For requests, don't use K suffix - show actual numbers
		return fmt.Sprintf("%.0f/%.0f", used, limit)
	case quota.UsageUnitCredits:
		return fmt.Sprintf("%.0f/%.0f", used, limit)
	default:
		if limit >= 1000000 {
			return fmt.Sprintf("%.1fM/%.1fM", used/1000000, limit/1000000)
		} else if limit >= 10000 {
			return fmt.Sprintf("%.0fK/%.0fK", used/1000, limit/1000)
		}
		return fmt.Sprintf("%.0f/%.0f", used, limit)
	}
}
