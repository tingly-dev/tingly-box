package client

import (
	"context"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropic-beta composition for the Claude Code OAuth chain.
//
// Anthropic fingerprints Claude Code traffic and the anthropic-beta header is
// one of the signals, so the header has to look like what the impersonated
// release (constant.ClaudeCodeVersion) sends — in content and in order. This
// file mirrors the 2.1.258 bundle's emission logic, reverse-engineered and
// verified against live captures (.design/claude-code-client-compat.md §3):
//
//  1. a model-dependent baseline (the CLI's allModelBetas) — always sent;
//  2. request-scoped flags the CLI adds when the body carries the matching
//     feature (effort, 1h cache TTL, fast mode, ...) — derived from the body;
//  3. request-scoped flags that cannot be derived from the body
//     (per-turn-control, afk-mode, ...) — replayed from the inbound client's
//     own header, allowlisted.
//
// Everything else the inbound client sends is dropped: an SDK-known flag no
// real claude-cli ever emits (message-batches, pdfs, ...) would break the
// header shape.

// Beta flag identifiers as registered in the 2.1.258 bundle.
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
)

// claudeCodeBetaEmissionOrder is the order the CLI pushes flags onto its
// betas array: allModelBetas (top block) → per-query additions (rEt) → the
// main query loop's feature pushes. composeClaudeCodeBetas emits its set in
// this order so the joined header matches a real client's byte-for-byte for
// the same feature set.
var claudeCodeBetaEmissionOrder = []string{
	// allModelBetas
	betaClaudeCode,
	betaOAuth,
	betaContext1M,
	betaInterleavedThinking,
	betaRedactThinking,
	betaThinkingTokenCount,
	betaContextManagement,
	betaPromptCachingScope,
	betaMidConversationSystem,
	// query-scoped (rEt)
	betaPerTurnControl,
	betaMidConversationToolChanges,
	// main query loop, in call order: effort → task budget → structured
	// outputs → thinking display → fast → afk → 1h cache → context hint →
	// cache evict → cache diagnosis; tool search rides the tools list.
	betaEffort,
	betaTaskBudgets,
	betaStructuredOutputs,
	betaThinkingDisplayUpdates,
	betaFastMode,
	betaAfkMode,
	betaExtendedCacheTTL,
	betaContextHint,
	betaPromptCachingEvict,
	betaCacheDiagnosis,
	betaAdvancedToolUse,
	betaToolSearchTool,
}

// claudeCodeClientReplayableBetas is the allowlist of inbound flags replayed
// upstream. Every entry is a flag the 2.1.258 CLI itself adds conditionally;
// the body-derived ones are included too so a client-negotiated flag survives
// even when tingly-box's own derivation misses the feature.
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
}

// claudeCodeCountTokensBetas is the subset the CLI keeps for
// /v1/messages/count_tokens (its countTokensBetas filter).
var claudeCodeCountTokensBetas = map[string]struct{}{
	betaClaudeCode:          {},
	betaInterleavedThinking: {},
	betaContextManagement:   {},
	betaOAuth:               {},
}

// claudeBetaSignals is everything composeClaudeCodeBetas needs to know about
// one request. Model is the outbound (provider) model id.
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

	// ClientBetas are the inbound client's flags (typ.ClaudeCodeClientHints).
	ClientBetas []string
}

// modelDateSuffixRe strips the -YYYYMMDD snapshot suffix of a model id
// (claude-haiku-4-5-20251001 → claude-haiku-4-5), the CLI's normalized form.
var modelDateSuffixRe = regexp.MustCompile(`-\d{8}$`)

// normalizeClaudeModel reproduces the CLI's model normalization enough for
// the capability checks below: lowercase, no [1m] marker, no snapshot date.
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

// claudeModelSupportsInterleavedThinking mirrors the CLI's capability check:
// everything except claude-haiku-4-5 and the claude-3 family.
func claudeModelSupportsInterleavedThinking(model string) bool {
	m := normalizeClaudeModel(model)
	if strings.Contains(m, "claude-3-") {
		return false
	}
	return m != "claude-haiku-4-5"
}

// claudeModelSupportsContextManagement: first-party models outside the
// claude-3 family.
func claudeModelSupportsContextManagement(model string) bool {
	return !strings.Contains(normalizeClaudeModel(model), "claude-3-")
}

// claudeLegacyMidConversationModels are the models the CLI explicitly keeps
// off mid-conversation-system (its hard-coded denylist); newer families
// (5-series, mythos, fable, ...) get the flag.
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

// composeClaudeCodeBetas returns the ordered anthropic-beta flags for one
// request, per the rules at the top of this file.
func composeClaudeCodeBetas(sig claudeBetaSignals) []string {
	want := map[string]struct{}{}
	add := func(flag string) { want[flag] = struct{}{} }

	// 1. allModelBetas
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

	// 2. body-derived
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

	// 3. replayed from the inbound client
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

// filterClaudeCodeCountTokensBetas reduces a composed list to the subset the
// CLI sends on count_tokens, preserving order.
func filterClaudeCodeCountTokensBetas(betas []string) []string {
	out := make([]string, 0, len(claudeCodeCountTokensBetas))
	for _, b := range betas {
		if _, ok := claudeCodeCountTokensBetas[b]; ok {
			out = append(out, b)
		}
	}
	return out
}

// joinBetas renders the header value the way the JS SDK does
// (Array.prototype.toString → comma-joined, no spaces).
func joinBetas(betas []string) string {
	return strings.Join(betas, ",")
}

// baseClaudeBetaSignals seeds the signals with the facts known before the
// body is inspected: model, auth kind, the context_1m rule flag and the
// inbound client's hints.
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
