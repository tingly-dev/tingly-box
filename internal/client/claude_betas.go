package client

import (
	"context"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropic-beta composition for the native Claude Code profile, mirroring
// the CLI (.design/claude-code.md Part B): a model-dependent baseline, flags derived
// from the body, and an allowlisted replay of the inbound client's flags.
// Anything else the client sends is dropped.

// Beta flags the CLI registers.
const (
	betaClaudeCode                 = "claude-code-20250219"
	betaOAuth                      = "oauth-2025-04-20"
	betaContext1M                  = "context-1m-2025-08-07"
	betaInterleavedThinking        = "interleaved-thinking-2025-05-14"
	betaRedactThinking             = "redact-thinking-2026-02-12"
	betaThinkingTokenCount         = "thinking-token-count-2026-05-13"
	betaContextManagement          = "context-management-2025-06-27"
	betaStructuredOutputs          = "structured-outputs-2025-12-15"
	betaPromptCachingScope         = "prompt-caching-scope-2026-01-05"
	betaMidConversationSystem      = "mid-conversation-system-2026-04-07"
	betaPerTurnControl             = "per-turn-control-2026-07-01"
	betaMidConversationToolChanges = "mid-conversation-tool-changes-2026-07-01"
	betaEffort                     = "effort-2025-11-24"
	betaTaskBudgets                = "task-budgets-2026-03-13"
	betaThinkingDisplayUpdates     = "thinking-display-updates-2026-08-18"
	betaFastMode                   = "fast-mode-2026-02-01"
	betaAfkMode                    = "afk-mode-2026-01-31"
	betaExtendedCacheTTL           = "extended-cache-ttl-2025-04-11"
	betaContextHint                = "context-hint-2026-04-09"
	betaPromptCachingEvict         = "prompt-caching-evict-2026-05-12"
	betaCacheDiagnosis             = "cache-diagnosis-2026-04-07"
	betaAdvancedToolUse            = "advanced-tool-use-2025-11-20"
	betaToolSearchTool             = "tool-search-tool-2025-10-19"
	betaPerTurnTiming              = "timing-2026-09-09"
	betaInlineTools                = "inline-tools-2026-09-15"
	betaMidConvSystemClearAt       = "mid-conversation-system-clear-at-2026-08-21"
	betaDangerousToolUse           = "dangerous-tool-use-2026-09-03"
	betaThinkingBindingControls    = "thinking-binding-controls-2026-08-01"
	betaThinkingResumption         = "thinking-resumption-2026-07-17"
	betaMessageThreads             = "message-threads-2026-08-12"
)

// claudeCodeBetaEmissionOrder is the CLI's push order; the composed header
// follows it so it matches a real client byte-for-byte.
var claudeCodeBetaEmissionOrder = []string{
	// model baseline
	betaClaudeCode,
	betaOAuth,
	betaContext1M,
	betaInterleavedThinking,
	betaRedactThinking,
	betaThinkingTokenCount,
	betaContextManagement,
	betaPromptCachingScope,
	betaMidConversationSystem,
	// query-scoped
	betaPerTurnControl,
	betaPerTurnTiming,
	betaMidConversationToolChanges,
	betaInlineTools,
	betaMidConvSystemClearAt,
	// tool search (pushed before the main loop)
	betaAdvancedToolUse,
	betaToolSearchTool,
	// main query loop, in call order
	betaEffort,
	betaTaskBudgets,
	betaStructuredOutputs,
	betaDangerousToolUse,
	betaThinkingBindingControls,
	betaThinkingDisplayUpdates,
	betaThinkingResumption,
	betaFastMode,
	betaAfkMode,
	betaExtendedCacheTTL,
	betaContextHint,
	betaPromptCachingEvict,
	betaCacheDiagnosis,
	betaMessageThreads,
}

// claudeCodeClientReplayableBetas are the inbound flags replayed upstream:
// every flag the CLI adds conditionally, including body-derived ones in case
// our derivation misses the feature.
var claudeCodeClientReplayableBetas = map[string]struct{}{
	betaContext1M:                  {},
	betaPerTurnControl:             {},
	betaMidConversationToolChanges: {},
	betaEffort:                     {},
	betaTaskBudgets:                {},
	betaStructuredOutputs:          {},
	betaThinkingDisplayUpdates:     {},
	betaFastMode:                   {},
	betaAfkMode:                    {},
	betaExtendedCacheTTL:           {},
	betaContextHint:                {},
	betaPromptCachingEvict:         {},
	betaCacheDiagnosis:             {},
	betaAdvancedToolUse:            {},
	betaToolSearchTool:             {},
	betaPerTurnTiming:              {},
	betaInlineTools:                {},
	betaMidConvSystemClearAt:       {},
	betaDangerousToolUse:           {},
	betaThinkingBindingControls:    {},
	betaThinkingResumption:         {},
	betaMessageThreads:             {},
}

// claudeCodeCountTokensBetas is the subset the CLI sends on count_tokens.
var claudeCodeCountTokensBetas = map[string]struct{}{
	betaClaudeCode:          {},
	betaInterleavedThinking: {},
	betaContextManagement:   {},
	betaOAuth:               {},
}

// claudeBetaSignals are the request facts composeClaudeCodeBetas reads.
// Model is the outbound (provider) model id.
type claudeBetaSignals struct {
	Model     string
	OAuth     bool
	Context1M bool

	EffortSet              bool
	FormatSet              bool
	TaskBudgetSet          bool
	FastMode               bool
	ThinkingDisplayUpdates bool
	CacheTTL1h             bool
	ToolSearch             bool
	ThinkingActive         bool // thinking adaptive/enabled → thinking-binding-controls
	Diagnostics            bool // body "diagnostics" → cache-diagnosis

	ClientBetas []string // inbound anthropic-beta
}

var modelDateSuffixRe = regexp.MustCompile(`-\d{8}$`)

// normalizeClaudeModel lowercases and strips the [1m] marker and snapshot date.
func normalizeClaudeModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	m = strings.TrimSuffix(m, "[1m]")
	m = modelDateSuffixRe.ReplaceAllString(m, "")
	return m
}

// claudeModelIsHaiku: the CLI omits claude-code-20250219 for any haiku.
func claudeModelIsHaiku(model string) bool {
	return strings.Contains(normalizeClaudeModel(model), "haiku")
}

// claudeModelSupportsInterleavedThinking: all but claude-haiku-4-5 and claude-3.
func claudeModelSupportsInterleavedThinking(model string) bool {
	m := normalizeClaudeModel(model)
	if strings.Contains(m, "claude-3-") {
		return false
	}
	return m != "claude-haiku-4-5"
}

// claudeModelSupportsContextManagement: all but claude-3.
func claudeModelSupportsContextManagement(model string) bool {
	return !strings.Contains(normalizeClaudeModel(model), "claude-3-")
}

// claudeLegacyMidConversationModels is the CLI's mid-conversation-system denylist.
var claudeLegacyMidConversationModels = map[string]struct{}{
	"claude-opus-4":     {},
	"claude-opus-4-0":   {},
	"claude-opus-4-1":   {},
	"claude-opus-4-5":   {},
	"claude-opus-4-6":   {},
	"claude-opus-4-7":   {},
	"claude-sonnet-4":   {},
	"claude-sonnet-4-0": {},
	"claude-sonnet-4-5": {},
	"claude-sonnet-4-6": {},
	"claude-haiku-4-5":  {},
}

// claudeModelSupportsMidConversationSystem mirrors the CLI's check.
func claudeModelSupportsMidConversationSystem(model string) bool {
	m := normalizeClaudeModel(model)
	if strings.Contains(m, "claude-3-") {
		return false
	}
	_, legacy := claudeLegacyMidConversationModels[m]
	return !legacy
}

// composeClaudeCodeBetas returns the ordered anthropic-beta flags for one request.
func composeClaudeCodeBetas(sig claudeBetaSignals) []string {
	want := map[string]struct{}{}
	add := func(flag string) { want[flag] = struct{}{} }

	// model baseline
	if !claudeModelIsHaiku(sig.Model) {
		add(betaClaudeCode)
	}
	if sig.OAuth {
		add(betaOAuth)
	}
	if sig.Context1M {
		add(betaContext1M)
	}
	if claudeModelSupportsInterleavedThinking(sig.Model) {
		add(betaInterleavedThinking)
		add(betaRedactThinking)
		add(betaThinkingTokenCount)
	}
	if claudeModelSupportsContextManagement(sig.Model) {
		add(betaContextManagement)
	}
	add(betaPromptCachingScope)
	if claudeModelSupportsMidConversationSystem(sig.Model) {
		add(betaMidConversationSystem)
	}

	// body-derived
	if sig.EffortSet {
		add(betaEffort)
	}
	if sig.FormatSet {
		add(betaStructuredOutputs)
	}
	if sig.TaskBudgetSet {
		add(betaTaskBudgets)
	}
	if sig.ThinkingDisplayUpdates {
		add(betaThinkingDisplayUpdates)
	}
	if sig.FastMode {
		add(betaFastMode)
	}
	if sig.CacheTTL1h {
		add(betaExtendedCacheTTL)
	}
	if sig.ToolSearch {
		add(betaAdvancedToolUse)
	}
	if sig.ThinkingActive {
		add(betaThinkingBindingControls)
	}
	if sig.Diagnostics {
		add(betaCacheDiagnosis)
	}

	// replayed from the inbound client
	for _, flag := range sig.ClientBetas {
		flag = strings.TrimSpace(flag)
		if _, ok := claudeCodeClientReplayableBetas[flag]; ok {
			add(flag)
		}
	}

	out := make([]string, 0, len(want))
	for _, flag := range claudeCodeBetaEmissionOrder {
		if _, ok := want[flag]; ok {
			out = append(out, flag)
		}
	}
	return out
}

// filterClaudeCodeCountTokensBetas keeps the count_tokens subset, in order.
func filterClaudeCodeCountTokensBetas(betas []string) []string {
	out := make([]string, 0, len(claudeCodeCountTokensBetas))
	for _, b := range betas {
		if _, ok := claudeCodeCountTokensBetas[b]; ok {
			out = append(out, b)
		}
	}
	return out
}

// joinBetas renders the header value like the JS SDK (comma-joined, no spaces).
func joinBetas(betas []string) string {
	return strings.Join(betas, ",")
}

// baseClaudeBetaSignals seeds the signals known before the body is inspected.
func baseClaudeBetaSignals(ctx context.Context, model string, oauth bool) claudeBetaSignals {
	return claudeBetaSignals{
		Model:       model,
		OAuth:       oauth,
		Context1M:   typ.GetRuleFlags(ctx).Context1M,
		ClientBetas: typ.GetClaudeCodeClientHints(ctx).Betas,
	}
}

// v1ClaudeBetaSignals inspects a Messages-API body for feature flags.
func v1ClaudeBetaSignals(ctx context.Context, req *anthropic.MessageNewParams, oauth bool) claudeBetaSignals {
	sig := baseClaudeBetaSignals(ctx, string(req.Model), oauth)
	sig.EffortSet = req.OutputConfig.Effort != ""
	sig.FormatSet = req.OutputConfig.Format.Schema != nil
	sig.ThinkingActive = req.Thinking.OfAdaptive != nil || req.Thinking.OfEnabled != nil
	if a := req.Thinking.OfAdaptive; a != nil && string(a.Display) == "updates" {
		sig.ThinkingDisplayUpdates = true
	}
	if e := req.Thinking.OfEnabled; e != nil && string(e.Display) == "updates" {
		sig.ThinkingDisplayUpdates = true
	}
	for i := range req.System {
		if string(req.System[i].CacheControl.TTL) == "1h" {
			sig.CacheTTL1h = true
		}
	}
	for i := range req.Tools {
		if cc := req.Tools[i].GetCacheControl(); cc != nil && string(cc.TTL) == "1h" {
			sig.CacheTTL1h = true
		}
	}
	for i := range req.Messages {
		for j := range req.Messages[i].Content {
			if cc := req.Messages[i].Content[j].GetCacheControl(); cc != nil && string(cc.TTL) == "1h" {
				sig.CacheTTL1h = true
			}
		}
	}
	return sig
}

// betaClaudeBetaSignals inspects a beta Messages-API body for feature flags.
func betaClaudeBetaSignals(ctx context.Context, req *anthropic.BetaMessageNewParams, oauth bool) claudeBetaSignals {
	sig := baseClaudeBetaSignals(ctx, string(req.Model), oauth)
	sig.EffortSet = req.OutputConfig.Effort != ""
	sig.FormatSet = req.OutputConfig.Format.Schema != nil || req.OutputFormat.Schema != nil
	sig.TaskBudgetSet = req.OutputConfig.TaskBudget.Total > 0
	sig.FastMode = string(req.Speed) == "fast"
	sig.ThinkingActive = req.Thinking.OfAdaptive != nil || req.Thinking.OfEnabled != nil
	sig.Diagnostics = !param.IsOmitted(req.Diagnostics)
	if a := req.Thinking.OfAdaptive; a != nil && string(a.Display) == "updates" {
		sig.ThinkingDisplayUpdates = true
	}
	if e := req.Thinking.OfEnabled; e != nil && string(e.Display) == "updates" {
		sig.ThinkingDisplayUpdates = true
	}
	for i := range req.System {
		if string(req.System[i].CacheControl.TTL) == "1h" {
			sig.CacheTTL1h = true
		}
	}
	for i := range req.Tools {
		t := &req.Tools[i]
		if t.OfToolSearchToolRegex20251119 != nil || t.OfToolSearchToolBm25_20251119 != nil {
			sig.ToolSearch = true
		}
		// defer_loading and Claude Code's own ToolSearch tool ride the same beta.
		if tool := t.OfTool; tool != nil && (tool.DeferLoading.Valid() && tool.DeferLoading.Value || tool.Name == "ToolSearch") {
			sig.ToolSearch = true
		}
		if cc := t.GetCacheControl(); cc != nil && string(cc.TTL) == "1h" {
			sig.CacheTTL1h = true
		}
	}
	for i := range req.Messages {
		for j := range req.Messages[i].Content {
			if cc := req.Messages[i].Content[j].GetCacheControl(); cc != nil && string(cc.TTL) == "1h" {
				sig.CacheTTL1h = true
			}
		}
	}
	return sig
}
