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

// LoadBalancer interface defines the load balancer operations we need. The
// status line only displays the pick, never dispatches to it, so it uses the
// side-effect-free PreviewService: SelectService would claim a half-open
// breaker's probe slot on every status poll.
type LoadBalancer interface {
	PreviewService(rule *typ.Rule) (*loadbalance.Service, error)
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
	//   row 2 (real model + consumption):     realModel @ provider | ▓▓░░░░░░ 7% | $0.05 | Cache: 87% | Quota: 60% left | Balance: $12.40
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

	// Add remaining quota and balance to the same line if available.
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
	service, err := h.loadBalancer.PreviewService(rule)
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

	// The tightest window is the one the next request will hit.
	window := usage.Tightest()
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

// buildQuotaInline shows remaining quota for inline display in statusline.
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

	return formatQuotaInline(usage)
}

func formatQuotaInline(usage *quota.ProviderUsage) string {
	var quotas, balances []string
	for _, seg := range QuotaSegments(usage) {
		if seg.Balance {
			balances = append(balances, seg.Text)
		} else {
			quotas = append(quotas, seg.Text)
		}
	}
	var sections []string
	if len(quotas) > 0 {
		sections = append(sections, "Quota: "+strings.Join(quotas, " "))
	}
	if len(balances) > 0 {
		sections = append(sections, "Balance: "+strings.Join(balances, " · "))
	}
	if len(sections) == 0 {
		return ""
	}
	return " | " + strings.Join(sections, " | ")
}

// QuotaSegment is one quota window as the status line shows it.
type QuotaSegment struct {
	Type    string // window type: session, daily, weekly, balance, ...
	Balance bool   // a balance ("$12.40"), not a used/limit window
	// Text is the rendered value, the same string the terminal status line
	// prints: "60% left", "12K/100K left", "$12.40".
	Text         string
	UsedPercent  float64
	ResetsAt     *time.Time
	LimitReached bool
}

// QuotaSegments lists a provider's windows that have something to show, in
// the provider's order. The terminal status line and Desk both render from
// this, so the two never disagree.
func QuotaSegments(usage *quota.ProviderUsage) []QuotaSegment {
	if usage == nil {
		return nil
	}
	var out []QuotaSegment
	for _, window := range usage.Windows {
		if window == nil {
			continue
		}
		seg := QuotaSegment{
			Type:         string(window.Type),
			UsedPercent:  window.Percent(),
			ResetsAt:     window.ResetsAt,
			LimitReached: window.LimitReached != nil && *window.LimitReached,
		}
		// A balance may have no reported cap. An explicit available amount
		// remains useful even when its usage percentage is unknown.
		if window.Type == quota.WindowTypeBalance && (window.Available != nil || window.Countable()) {
			seg.Balance, seg.Text = true, formatQuotaBalance(window)
		} else if window.Countable() {
			seg.Text = formatQuotaWindow(window) + " left"
		} else {
			continue
		}
		out = append(out, seg)
	}
	return out
}

// Route is where a request for a model is routed in a scenario, and the
// quota of the provider it lands on: the tingly-box half of the status line.
type Route struct {
	ProviderName string
	Model        string
	Quota        []QuotaSegment
}

// ResolveRoute resolves the route the way the terminal status line does. It
// returns nil when no rule matches the model in the scenario.
func (h *Handler) ResolveRoute(ctx context.Context, scenario, modelID string) *Route {
	mapping := h.getTBModelMapping(modelID, typ.RuleScenario(scenario))
	if mapping == nil {
		return nil
	}
	route := &Route{ProviderName: mapping.providerName, Model: mapping.model}
	if h.quotaMgr != nil {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if usage, err := h.quotaMgr.GetQuota(ctx, mapping.providerUUID); err == nil {
			route.Quota = QuotaSegments(usage)
		}
	}
	return route
}

func formatQuotaBalance(window *quota.UsageWindow) string {
	value := max(0, window.Limit-window.Used)
	if window.Available != nil {
		value = max(0, *window.Available)
	}
	switch window.Unit {
	case quota.UsageUnitCurrency:
		if window.CurrencyCode == "USD" {
			return fmt.Sprintf("$%.2f", value)
		}
		if window.CurrencyCode != "" {
			return fmt.Sprintf("%.2f %s", value, window.CurrencyCode)
		}
		return fmt.Sprintf("%.2f", value)
	case quota.UsageUnitCredits:
		return fmt.Sprintf("%.0f credits", value)
	case quota.UsageUnitPercent:
		return fmt.Sprintf("%.0f%%", value)
	default:
		return fmt.Sprintf("%.0f %s", value, window.Unit)
	}
}

// formatQuotaWindow formats a single quota window
func formatQuotaWindow(window *quota.UsageWindow) string {
	remaining, limit := max(0, window.Limit-window.Used), window.Limit
	if window.Available != nil {
		remaining = max(0, *window.Available)
	}
	if window.Unit == quota.UsageUnitPercent {
		return fmt.Sprintf("%.0f%%", remaining)
	}

	// Requests and credits always show actual numbers, never a K/M suffix.
	if window.Unit == quota.UsageUnitRequests || window.Unit == quota.UsageUnitCredits {
		return fmt.Sprintf("%.0f/%.0f", remaining, limit)
	}

	// Tokens (and any other unit) get a K/M suffix for large limits.
	switch {
	case limit >= 1000000:
		return fmt.Sprintf("%.1fM/%.1fM", remaining/1000000, limit/1000000)
	case limit >= 10000:
		return fmt.Sprintf("%.0fK/%.0fK", remaining/1000, limit/1000)
	default:
		return fmt.Sprintf("%.0f/%.0f", remaining, limit)
	}
}
