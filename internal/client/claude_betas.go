package client

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropic-beta composition for the native Claude Code profile, mirroring
// the CLI (.design/claude-code-oauth-compat.md): a model-dependent baseline, flags derived
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
	betaToolProvenance             = "tool-provenance-2026-10-26"
	betaResumption                 = "resumption-2026-10-08"
	betaProgramUseCaseIDs          = "program-use-case-ids-2026-09-17"
	betaAdvisorTool                = "advisor-tool-2026-03-01"
	betaCacheKeepalive             = "cache-keepalive-2026-09-03"
	betaFallbackCredit             = "fallback-credit-2026-06-01"
	betaServerSideFallback         = "server-side-fallback-2026-06-01"
	betaServerSideFallbackCategory = "server-side-fallback-2026-07-01"
	betaMCPServers                 = "mcp-servers-2025-12-04"
	betaFilesAPI                   = "files-api-2025-04-14"
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
	betaContextManagement,
	betaPromptCachingScope,
	// query-scoped (the CLI's per-turn table); mid-conversation-system is
	// registered without a header since 2.1.328 and is never sent.
	betaPerTurnControl,
	betaPerTurnTiming,
	betaMidConversationToolChanges,
	betaInlineTools,
	betaToolProvenance,
	betaMidConvSystemClearAt,
	// main query loop, in call order. thinking-token-count left the baseline
	// in 2.1.328: it is pushed here, ahead of tool search.
	betaProgramUseCaseIDs,
	betaAdvisorTool,
	betaThinkingTokenCount,
	betaAdvancedToolUse,
	betaToolSearchTool,
	betaEffort,
	betaTaskBudgets,
	betaStructuredOutputs,
	betaDangerousToolUse,
	betaThinkingBindingControls,
	betaThinkingDisplayUpdates,
	betaThinkingResumption,
	betaResumption,
	betaFastMode,
	betaAfkMode,
	betaExtendedCacheTTL,
	betaContextHint,
	betaMCPServers,
	betaFilesAPI,
	betaPromptCachingEvict,
	betaCacheDiagnosis,
	betaCacheKeepalive,
	betaMessageThreads,
	// positions below were not observed on the wire (server-driven, rare)
	betaFallbackCredit,
	betaServerSideFallback,
	betaServerSideFallbackCategory,
}

// claudeCodeBetaOrderFor returns the emission order for a model. For haiku the
// CLI leaves claude-code-20250219 out of the baseline and appends it for
// agentic queries, i.e. after prompt-caching-scope (2.1.328 capture).
func claudeCodeBetaOrderFor(model string) []string {
	if !claudeModelIsHaiku(model) {
		return claudeCodeBetaEmissionOrder
	}
	out := make([]string, 0, len(claudeCodeBetaEmissionOrder))
	for _, flag := range claudeCodeBetaEmissionOrder {
		if flag == betaClaudeCode {
			continue
		}
		out = append(out, flag)
		if flag == betaPromptCachingScope {
			out = append(out, betaClaudeCode)
		}
	}
	return out
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
	betaResumption:                 {},
	betaProgramUseCaseIDs:          {},
	betaAdvisorTool:                {},
	betaCacheKeepalive:             {},
	betaFallbackCredit:             {},
	betaServerSideFallback:         {},
	betaServerSideFallbackCategory: {},
	betaMCPServers:                 {},
	betaFilesAPI:                   {},
}

// claudeCodeBodyExtraBetas pairs the body members the typed SDK params drop
// with the beta that enables them. A member is replayed only when its beta is
// in the composed header, so we never send a field without its beta (the API
// rejects unknown inputs) nor a beta whose field we silently dropped.
var claudeCodeBodyExtraBetas = map[string]string{
	"resumption":          betaResumption,
	"program_use_case_id": betaProgramUseCaseIDs,
	"thread":              betaMessageThreads,
}

// claudeCacheControlExtraBetas pairs the cache_control members the SDK's
// BetaCacheControlEphemeralParam lacks with their betas.
var claudeCacheControlExtraBetas = map[string]string{
	"evict_on_complete": betaPromptCachingEvict,
	"scope":             betaPromptCachingScope,
}

// applyClaudeCodeBodyExtras restores the inbound body members the typed
// params dropped, gated by their paired beta (see claudeCodeBodyExtraBetas).
// Anthropic's gateway protocol asks proxies to pass unknown body fields
// through unchanged; the typed SDK params would otherwise lose them.
func applyClaudeCodeBodyExtras(req *anthropic.BetaMessageNewParams, extras map[string]json.RawMessage, betas []string) {
	if len(extras) == 0 {
		return
	}
	sent := make(map[string]struct{}, len(betas))
	for _, b := range betas {
		sent[b] = struct{}{}
	}
	fields := map[string]any{}
	for name, beta := range claudeCodeBodyExtraBetas {
		if raw, ok := extras[name]; ok {
			if _, ok := sent[beta]; ok {
				fields[name] = raw
			}
		}
	}
	if len(fields) > 0 {
		merged := map[string]any{}
		for k, v := range req.ExtraFields() {
			merged[k] = v
		}
		for k, v := range fields {
			merged[k] = v
		}
		req.SetExtraFields(merged)
	}

	// top-level cache_control: only members the SDK type lacks
	raw, ok := extras["cache_control"]
	if !ok || param.IsOmitted(req.CacheControl) {
		return
	}
	var cc map[string]json.RawMessage
	if json.Unmarshal(raw, &cc) != nil {
		return
	}
	ccExtra := map[string]any{}
	for name, beta := range claudeCacheControlExtraBetas {
		if v, ok := cc[name]; ok {
			if _, ok := sent[beta]; ok {
				ccExtra[name] = v
			}
		}
	}
	if len(ccExtra) > 0 {
		req.CacheControl.SetExtraFields(ccExtra)
	}
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

// claudeModelIsHaiku: the CLI leaves claude-code-20250219 out of the baseline for haiku.
func claudeModelIsHaiku(model string) bool {
	return strings.Contains(normalizeClaudeModel(model), "haiku")
}

// claudeModelSupportsInterleavedThinking: every first-party model but claude-3
// (claude-haiku-4-5 joined in 2.1.328).
func claudeModelSupportsInterleavedThinking(model string) bool {
	return !strings.Contains(normalizeClaudeModel(model), "claude-3-")
}

// claudeModelSupportsContextManagement: all but claude-3.
func claudeModelSupportsContextManagement(model string) bool {
	return !strings.Contains(normalizeClaudeModel(model), "claude-3-")
}

// claudeNative1MModels have a 1M window without the context-1m beta (the CLI's
// model table, native_1m). The beta is not sent for them and the [1m] suffix
// is dropped from the wire model.
var claudeNative1MModels = map[string]struct{}{
	"claude-haiku-5-5":  {},
	"claude-sonnet-5":   {},
	"claude-sonnet-5-5": {},
	"claude-opus-4-7":   {},
	"claude-opus-4-8":   {},
	"claude-opus-5":     {},
	"claude-opus-5-5":   {},
	"claude-fable-5":    {},
	"claude-fable-5-1":  {},
	"claude-fable-5-2":  {},
	"claude-mythos-5":   {},
	"claude-mythos-5-1": {},
	"claude-mythos-5-2": {},
}

func claudeModelNative1M(model string) bool {
	_, ok := claudeNative1MModels[normalizeClaudeModel(model)]
	return ok
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
		add(betaToolProvenance)
	}
	if sig.Context1M && !claudeModelNative1M(sig.Model) {
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
		if flag == betaContext1M && claudeModelNative1M(sig.Model) {
			continue // the model's window is native; the CLI never sends the beta
		}
		if _, ok := claudeCodeClientReplayableBetas[flag]; ok {
			add(flag)
		}
	}

	out := make([]string, 0, len(want))
	// An agentic haiku query (the main thread) carries claude-code-20250219;
	// the inbound Claude Code client says so by sending it itself.
	if claudeModelIsHaiku(sig.Model) && slices.Contains(sig.ClientBetas, betaClaudeCode) {
		add(betaClaudeCode)
	}

	for _, flag := range claudeCodeBetaOrderFor(sig.Model) {
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
		t := &req.Tools[i]
		if t.OfToolSearchToolRegex20251119 != nil || t.OfToolSearchToolBm25_20251119 != nil {
			sig.ToolSearch = true
		}
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
