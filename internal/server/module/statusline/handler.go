package statusline

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
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
		CCCacheReadTokens:   merged.ContextWindow.CurrentUsage.CacheRead,
		CCCacheWriteTokens:  merged.ContextWindow.CurrentUsage.CacheWrite,
		CCCacheHitPct:       cacheHitPct(merged.ContextWindow.CurrentUsage),
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

	// Build status line as two rows, split by semantic dimension:
	//   row 1 (session + requested routing): ruleModel @ profile  📁 <cwd>  <session>
	//   row 2 (real model + consumption):     realModel @ provider | ▓▓░░░░░░ 7% | $0.05 | Cache: 87% | Usage: 40K/100K
	ccModel := cmp.Or(merged.Model.DisplayName, "unknown")

	usedPct := int(merged.ContextWindow.UsedPercentage)
	cost := merged.Cost.TotalCostUSD

	// Build context bar (8 characters wide)
	barWidth := 8
	filled := min(max(usedPct*barWidth/100, 0), barWidth)
	bar := strings.Repeat("▓", filled) + strings.Repeat("░", barWidth-filled)

	// Build profile label: "p1:name" or "default" when none configured.
	profileLabel := "default"
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
	ruleModel := cmp.Or(merged.Model.ID, ccModel)

	// Row 1: requested routing first, then session identity.
	// @ reads as "belongs to / via" (profile, provider).
	row1 := fmt.Sprintf("%s @ %s  📁 %s%s", ruleModel, profileLabel, shortenPath(merged.CWD), sessionLabel(merged.SessionName, merged.SessionID))

	// Row 2: real model + consumption.
	row2 := ""
	if mapping != nil && mapping.model != "" {
		row2 = fmt.Sprintf("%s @ %s | ", mapping.model, mapping.providerName)
	}
	row2 += fmt.Sprintf("%s %d%% | $%.2f", bar, usedPct, cost)
	row2 += buildCacheInline(merged.ContextWindow.CurrentUsage)

	// Add usage info to the same line if available
	quotaInfo := h.buildQuotaInline(mapping)
	if quotaInfo != "" {
		row2 += quotaInfo
	}

	c.String(http.StatusOK, row1+"\n"+row2)
}

func cacheHitPct(usage CurrentUsage) int {
	if usage.CacheRead <= 0 {
		return 0
	}

	denominator := usage.InputTokens + usage.CacheRead
	if denominator <= 0 {
		return 0
	}

	return usage.CacheRead * 100 / denominator
}

func buildCacheInline(usage CurrentUsage) string {
	pct := cacheHitPct(usage)
	if pct <= 0 {
		return ""
	}

	return fmt.Sprintf(" | Cache: %d%%", pct)
}

// shortenPath collapses a long absolute path for compact statusline display.
// The home directory prefix becomes ~; long middles collapse to ... while the
// first segment and the last two segments (parent/basename) are kept.
// Examples:
//
//	/Users/yz/Project/101-project/tingly-box → ~/.../101-project/tingly-box
//	/Users/xyz/tmp            → ~/tmp
//	""                        → ~
func shortenPath(path string) string {
	if path == "" {
		return "~"
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home) {
		path = "~" + strings.TrimPrefix(path, home)
	}

	// Clean slashes so splitting is predictable, but keep the leading ~.
	segments := strings.Split(strings.TrimPrefix(path, "~"), "/")
	// Drop empty segments (leading slash, doubled slashes).
	cleaned := segments[:0]
	for _, s := range segments {
		if s != "" {
			cleaned = append(cleaned, s)
		}
	}

	const maxLen = 40
	if path == "~" || len(cleaned) == 0 {
		return "~"
	}

	short := "~/" + strings.Join(cleaned, "/")

	// Collapse the middle when there are more than 2 path segments (keeps
	// home ~ prefix + last two), or when even a short segment count blows past
	// the width budget. So /h/Project/101-project/tingly-box (3 segments) →
	// ~/.../101-project/tingly-box, while /h/repo stays ~/repo.
	if len(cleaned) > 2 || len(short) > maxLen {
		if len(cleaned) <= 2 {
			return short
		}
		tail := cleaned[len(cleaned)-2:]
		return "~/.../" + strings.Join(tail, "/")
	}

	return short
}

// firstN returns the first n bytes of s (or all of it if shorter), safe for
// short/empty input.
func firstN(s string, n int) string {
	if n < 0 {
		n = 0
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// sessionLabel renders a compact, resumeable session identifier for row 1.
// Prefers the human-readable title (quoted — literally copy-resumeable via
// `claude --resume <title>`); falls back to #<first8> of the id when no title
// exists (early / `claude -p` sessions). Returns "" when neither is available.
// The result always carries a leading space when non-empty.
func sessionLabel(name, id string) string {
	if name != "" {
		return fmt.Sprintf(" %q", name)
	}
	if id != "" {
		return " #" + firstN(id, 8)
	}
	return ""
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

// selectBestQuotaWindow selects the most relevant quota window.
// Lower tier means more important, so the first meaningful window wins.
func (h *Handler) selectBestQuotaWindow(usage *quota.ProviderUsage) *quota.UsageWindow {
	if usage == nil {
		return nil
	}

	usage.NormalizeWindows()
	for _, w := range usage.Windows {
		if w != nil && (w.Limit > 0 || w.UsedPercent > 0) {
			return w
		}
	}

	return nil
}

// buildQuotaInline builds quota (used/limit) information for inline display in statusline
// Format: " | Usage: 40/100 tokens" or " | Usage: 40K/100K tokens | 100/500 req"
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
	usage.NormalizeWindows()
	windows := make([]*quota.UsageWindow, 0, len(usage.Windows))
	for _, window := range usage.Windows {
		if window != nil && window.Limit > 0 {
			windows = append(windows, window)
		}
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

	// Join all quota parts. Label is "Usage" (not "Quota"): the values are
	// used/limit (consumption), and "Usage" is unambiguous next to the usage %.
	return " | Usage: " + strings.Join(parts, " ")
}

// formatQuotaWindow formats a single quota window
func (h *Handler) formatQuotaWindow(window *quota.UsageWindow) string {
	used := window.Used
	limit := window.Limit

	if limit <= 0 {
		return ""
	}

	// Requests and credits always show actual numbers, never a K/M suffix.
	if window.Unit == quota.UsageUnitRequests || window.Unit == quota.UsageUnitCredits {
		return fmt.Sprintf("%.0f/%.0f", used, limit)
	}

	// Tokens (and any other unit) get a K/M suffix for large limits.
	switch {
	case limit >= 1000000:
		return fmt.Sprintf("%.1fM/%.1fM", used/1000000, limit/1000000)
	case limit >= 10000:
		return fmt.Sprintf("%.0fK/%.0fK", used/1000, limit/1000)
	default:
		return fmt.Sprintf("%.0f/%.0f", used, limit)
	}
}
