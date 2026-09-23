package client

import (
	"context"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
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

	// Registered in 2.1.280 (absent from the 2.1.258 registry).
	betaPerTurnTiming           = "timing-2026-09-09"
	betaInlineTools             = "inline-tools-2026-09-15"
	betaMidConvSystemClearAt    = "mid-conversation-system-clear-at-2026-08-21"
	betaDangerousToolUse        = "dangerous-tool-use-2026-09-03"
	betaThinkingBindingControls = "thinking-binding-controls-2026-08-01"
	betaThinkingResumption      = "thinking-resumption-2026-07-17"
	betaMessageThreads          = "message-threads-2026-08-12"
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
	// query-scoped (rEt / the 2.1.280 Tw table)
	betaPerTurnControl,
	betaPerTurnTiming,
	betaMidConversationToolChanges,
	betaInlineTools,
	betaMidConvSystemClearAt,
	// tool search rides the tools list and is pushed before the main loop
	// (2.1.280 direct capture: …prompt-caching-scope,advanced-tool-use,effort,…).
	betaAdvancedToolUse,
	betaToolSearchTool,
	// main query loop, in call order (2.1.280; 2.1.258 is the same order
	// minus the flags it does not register): effort → task budget →
	// structured outputs → dangerous tool use → thinking binding → thinking
	// display → thinking resumption → fast → afk → 1h cache → context hint →
	// cache evict → cache diagnosis → message threads.
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

// claudeCodeReplayableBetasSince lists replayable flags that only exist from
// a given version on; a client replaying one against an older impersonated
// version would produce a header shape that version never sends.
var claudeCodeReplayableBetasSince = map[string]string{
	betaPerTurnTiming:           typ.ClaudeCodeVersion2_1_280,
	betaInlineTools:             typ.ClaudeCodeVersion2_1_280,
	betaMidConvSystemClearAt:    typ.ClaudeCodeVersion2_1_280,
	betaDangerousToolUse:        typ.ClaudeCodeVersion2_1_280,
	betaThinkingBindingControls: typ.ClaudeCodeVersion2_1_280,
	betaThinkingResumption:      typ.ClaudeCodeVersion2_1_280,
	betaMessageThreads:          typ.ClaudeCodeVersion2_1_280,
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
	betaPerTurnTiming:              {},
	betaInlineTools:                {},
	betaMidConvSystemClearAt:       {},
	betaDangerousToolUse:           {},
	betaThinkingBindingControls:    {},
	betaThinkingResumption:         {},
	betaMessageThreads:             {},
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
	// Version is the impersonated release (typ.ClaudeCodeVersion*); it
	// gates the flags that only exist from a given version on. Empty means
	// the oldest native profile (2.1.258).
	Version   string
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
	// ThinkingActive (2.1.280+): thinking adaptive/enabled → the CLI sends
	// thinking-binding-controls on direct first-party traffic.
	ThinkingActive bool
	// Diagnostics (2.1.280+): body carries the top-level "diagnostics"
	// request → cache-diagnosis.
	Diagnostics bool

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
	if sig.versionAtLeast(typ.ClaudeCodeVersion2_1_280) {
		if sig.ThinkingActive {
			add(betaThinkingBindingControls)
		}
		if sig.Diagnostics {
			add(betaCacheDiagnosis)
		}
	}

	// 3. replayed from the inbound client
	for _, flag := range sig.ClientBetas {
		flag = strings.TrimSpace(flag)
		if _, ok := claudeCodeClientReplayableBetas[flag]; !ok {
			continue
		}
		if since, gated := claudeCodeReplayableBetasSince[flag]; gated && !sig.versionAtLeast(since) {
			continue
		}
		add(flag)
	}

	out := make([]string, 0, len(want))
	for _, flag := range claudeCodeBetaEmissionOrder {
		if _, ok := want[flag]; ok {
			out = append(out, flag)
		}
	}
	return out
}

// versionAtLeast reports whether the impersonated version is v or newer.
// Only the versions the chain knows are ordered; an unknown/empty version
// counts as the oldest native profile.
func (sig claudeBetaSignals) versionAtLeast(v string) bool {
	rank := func(x string) int {
		switch x {
		case typ.ClaudeCodeVersion2_1_280:
			return 2
		default:
			return 1
		}
	}
	return rank(sig.Version) >= rank(v)
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
	flags := typ.GetRuleFlags(ctx)
	return claudeBetaSignals{
		Version:     flags.ClaudeCodeVersion,
		Model:       model,
		OAuth:       oauth,
		Context1M:   flags.Context1M,
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
		// Deferred tool loading (defer_loading) rides the same beta; Claude
		// Code's own ToolSearch tool is a plain named tool alongside
		// DeferredToolPlaceholder entries.
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
