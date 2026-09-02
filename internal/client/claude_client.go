package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// guard
var _ AnthropicClientInterface = (*ClaudeClient)(nil)

// ClaudeClient wraps AnthropicClient with Claude Code OAuth-specific behaviors.
// It creates an Anthropic SDK client directly with Claude Code headers and middleware,
// then embeds it for delegation.
//
// Claude Code (Claude Code OAuth) limitations:
// - Does NOT support /models endpoint (returns 404)
// - Requires special headers (applied via SDK options)
// - Requires tool prefix stripping (applied via middleware)
type ClaudeClient struct {
	*AnthropicClient
}

// NewClaudeClient creates a new Claude client wrapper.
// It builds an Anthropic SDK client with Claude Code specific headers and middleware,
// then wraps it in an AnthropicClient for delegation.
//
// ctx is the inbound request context: the pool constructs a client per request,
// so per-request flags resolved here (the claude_org_id rule flag via
// typ.GetRuleFlags) are correctly scoped to the request being served.
func NewClaudeClient(ctx context.Context, provider *typ.Provider, model string, sessionID typ.SessionID) (*ClaudeClient, error) {
	logrus.Debug("creating claude-client")

	// Handle API base URL - Anthropic SDK expects base without /v1
	apiBase := strings.TrimRight(provider.APIBase, "/")
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase = strings.TrimSuffix(apiBase, "/v1")
	}

	// Build base SDK options
	options := []anthropicOption.RequestOption{
		anthropicOption.WithBaseURL(apiBase),
		anthropicOption.WithMaxRetries(0), // Disable automatic retries for 429 errors
	}

	// Check if this is an OAuth token
	isOAuthToken := IsClaudeOAuthToken(provider.GetAccessToken())

	// Apply Claude Code specific headers
	options = applyClaudeCodeHeaders(options, provider, model, isOAuthToken, typ.GetRuleFlags(ctx).ClaudeOrgID)

	// Add beta query parameter
	options = append(options, anthropicOption.WithQuery("beta", "true"))

	// MENTION: must set timeout, otherwise nonstream and stream may work badly
	timeout := time.Duration(provider.Timeout) * time.Second
	if provider.Timeout <= 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}
	options = append(options, anthropicOption.WithRequestTimeout(timeout))

	// Create SDK client
	anthropicClient := anthropic.NewClient(options...)

	// Wrap in AnthropicClient base
	base := &AnthropicClient{
		client:   anthropicClient,
		provider: provider,
	}

	return &ClaudeClient{AnthropicClient: base}, nil
}

// applyClaudeCodeHeaders applies Claude Code specific headers via SDK options.
// model seeds the client-level anthropic-beta baseline (the per-request
// Guard/GuardBeta recompute it from the body and override it); orgOverride,
// when non-empty, replaces the login-time organization as the
// anthropic-organization-id value (the claude_org_id rule flag).
func applyClaudeCodeHeaders(options []anthropicOption.RequestOption, provider *typ.Provider, model string, isOAuthToken bool, orgOverride string) []anthropicOption.RequestOption {
	// Client-level baseline: the model-dependent flags a real CLI sends on
	// every request. Request-scoped flags are added per call in Guard.
	baseBetas := joinBetas(composeClaudeCodeBetas(claudeBetaSignals{Model: model, OAuth: isOAuthToken}))

	// Auth header
	if isOAuthToken {
		options = append(options, anthropicOption.WithHeader("Authorization", "Bearer "+provider.GetAccessToken()))
	} else {
		options = append(options, anthropicOption.WithHeader("x-api-key", provider.GetAccessToken()))
	}

	// Claude Code specific headers. No x-stainless-helper-method: the CLI
	// calls messages.create({stream:true}) directly, not the .stream() helper.
	options = append(options,
		anthropicOption.WithHeader("accept", acceptHeader),
		anthropicOption.WithHeader("anthropic-beta", baseBetas),
		anthropicOption.WithHeader("anthropic-dangerous-direct-browser-access", anthropicDangerousDirectBrowserAccess),
		anthropicOption.WithHeader("anthropic-version", anthropicVersion),
		anthropicOption.WithHeader("user-agent", claudeCLIUserAgent),
		anthropicOption.WithHeader("x-app", claudeXApp),
		anthropicOption.WithHeader("x-stainless-retry-count", stainlessRetryCount),
		anthropicOption.WithHeader("x-stainless-runtime-version", stainlessRuntimeVersion),
		anthropicOption.WithHeader("x-stainless-package-version", stainlessPackageVersion),
		anthropicOption.WithHeader("x-stainless-runtime", stainlessRuntime),
		anthropicOption.WithHeader("x-stainless-lang", stainlessLang),
		anthropicOption.WithHeader("x-stainless-arch", stainlessArch()),
		anthropicOption.WithHeader("x-stainless-os", stainlessOS()),
		anthropicOption.WithHeader("x-stainless-timeout", stainlessTimeout),
	)

	// Organization attribution is opt-in via the claude_org_id rule flag:
	// unset (empty) attaches no organization header — the classic default;
	// "auto" attributes the request to the organization the OAuth token was
	// issued for (login-time capture), which org-bound entitlements (e.g.
	// Cyber Verification) rely on; any other value attributes the request to
	// that organization instead.
	var orgID string
	switch orgOverride {
	case "":
		// default: no organization header
	case typ.ClaudeOrgIDAuto:
		orgID = provider.OAuthDetail.GetExtraFieldString("organization_id")
	default:
		orgID = orgOverride
	}
	if orgID != "" {
		options = append(options, anthropicOption.WithHeader("anthropic-organization-id", orgID))
	}

	return options
}

// ===================================================================
// ClaudeClient interface methods
// ===================================================================

// ListModels returns the list of available models.
func (c *ClaudeClient) ListModels(ctx context.Context) (*ModelListResult, error) {
	return c.AnthropicClient.ListModels(ctx)
}

func (c *ClaudeClient) Guard(ctx context.Context, req *anthropic.MessageNewParams) (*AnthropicClient, map[string]string) {
	// Apply thinking transformation for Claude Code OAuth. Thinking can be expressed
	// either through the thinking union (enabled/adaptive/disabled) or through
	// output_config.effort (the effort-based adaptive thinking used by newer models).
	// Only default to disabled when the client specified neither, otherwise we would
	// silently turn off effort-based thinking the client explicitly requested.
	// Special models like claude-fable-5 do not support thinking.type.disabled.
	model := req.Model
	thinkingSet := req.Thinking.OfEnabled != nil || req.Thinking.OfAdaptive != nil || req.Thinking.OfDisabled != nil

	isSpecialModel := strings.Contains(model, "claude-fable")
	if isSpecialModel {
		req.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
	} else {
		if !thinkingSet && req.OutputConfig.Effort == "" {
			req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
		}
	}

	// Remap tool names towards Claude Code spelling (see claude_tool_names.go)
	reverseMap := remapRequestToolNames(req)

	// Inject session ID from metadata
	meta := ops.ParseMetadataUserID(req.Metadata.UserID.String())
	if meta == nil {
		panic("invalid metadata")
	}
	options := c.perRequestOptions(ctx, meta.SessionID, v1ClaudeBetaSignals(ctx, req, c.isOAuth()))
	// Streaming responses bypass restoreToolNamesInMessage, so undo the rename
	// on the wire instead. No-op for non-streaming responses.
	if len(reverseMap) > 0 {
		options = append(options, anthropicOption.WithMiddleware(restoreToolNamesMiddleware(reverseMap)))
	}
	logrus.WithContext(ctx).Debugf("session: %s", meta.SessionID)
	logrus.WithContext(ctx).Debugf("metadata: %s", req.Metadata.UserID)

	// Create SDK client
	anthropicClient := anthropic.NewClient(options...)

	// Wrap in AnthropicClient base
	base := &AnthropicClient{
		client:   anthropicClient,
		provider: c.AnthropicClient.provider,
	}

	return base, reverseMap
}

// isOAuth reports whether the provider credential is a Claude OAuth token
// (the oauth-2025-04-20 beta rides only on those).
func (c *ClaudeClient) isOAuth() bool {
	return IsClaudeOAuthToken(c.AnthropicClient.provider.GetAccessToken())
}

// perRequestOptions extends the client's base options with the headers a
// real Claude Code CLI derives per request: the session id, the composed
// anthropic-beta list (overriding the client-level baseline — WithHeader
// replaces) and, for subagent traffic, the agent lineage headers replayed
// from the inbound client.
func (c *ClaudeClient) perRequestOptions(ctx context.Context, sessionID string, sig claudeBetaSignals) []anthropicOption.RequestOption {
	base := c.AnthropicClient.Client().Options
	options := make([]anthropicOption.RequestOption, 0, len(base)+4)
	options = append(options, base...)
	options = append(options,
		anthropicOption.WithHeader("X-Claude-Code-Session-Id", sessionID),
		anthropicOption.WithHeader("anthropic-beta", joinBetas(composeClaudeCodeBetas(sig))),
	)
	hints := typ.GetClaudeCodeClientHints(ctx)
	if hints.AgentID != "" {
		options = append(options, anthropicOption.WithHeader("x-claude-code-agent-id", sanitizeClaudeHeaderValue(hints.AgentID)))
	}
	if hints.ParentAgentID != "" {
		options = append(options, anthropicOption.WithHeader("x-claude-code-parent-agent-id", sanitizeClaudeHeaderValue(hints.ParentAgentID)))
	}
	return options
}

// sanitizeClaudeHeaderValue reproduces the CLI's agent-id header encoder:
// '%' and every byte outside printable ASCII are percent-encoded so the value
// is always a valid HTTP header value.
func sanitizeClaudeHeaderValue(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if ch == '%' || ch < 0x20 || ch > 0x7e {
			fmt.Fprintf(&b, "%%%02X", ch)
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func (c *ClaudeClient) GuardBeta(ctx context.Context, req *anthropic.BetaMessageNewParams) (*AnthropicClient, map[string]string) {
	// Apply thinking transformation for Claude Code OAuth. Thinking can be expressed
	// either through the thinking union (enabled/adaptive/disabled) or through
	// output_config.effort (the effort-based adaptive thinking used by newer models).
	// Only default to disabled when the client specified neither, otherwise we would
	// silently turn off effort-based thinking the client explicitly requested.
	// Special models like claude-fable-5 do not support thinking.type.disabled.
	model := string(req.Model)
	effortSet := req.OutputConfig.Effort != ""
	thinkingSet := req.Thinking.OfEnabled != nil || req.Thinking.OfAdaptive != nil || req.Thinking.OfDisabled != nil

	isSpecialModel := strings.Contains(model, "claude-fable")
	if isSpecialModel {
		req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}
	} else {
		if !thinkingSet && !effortSet {
			req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
		}
	}

	// clear_thinking_20251015 is only valid when thinking is enabled or adaptive;
	// effort-based thinking counts as adaptive. An explicit disabled config wins even
	// if effort is also present. Drop the edit otherwise, since Anthropic rejects it
	// with "clear_thinking_20251015 requires `thinking` to be enabled or adaptive" —
	// which is what newer Claude Code clients hit when we force thinking off.
	thinkingActive := req.Thinking.OfDisabled == nil &&
		(req.Thinking.OfEnabled != nil || req.Thinking.OfAdaptive != nil || effortSet)
	if !thinkingActive {
		stripBetaClearThinkingEdit(req)
	}

	// Remap tool names towards Claude Code spelling (see claude_tool_names.go)
	reverseMap := remapBetaRequestToolNames(req)

	// Inject session ID from metadata
	meta := ops.ParseMetadataUserID(req.Metadata.UserID.String())
	if meta == nil {
		panic("invalid metadata")
	}
	options := c.perRequestOptions(ctx, meta.SessionID, betaClaudeBetaSignals(ctx, req, c.isOAuth()))
	// The composed header is the whole anthropic-beta story: clear the SDK's
	// per-param Betas so nothing is appended as a second header value.
	req.Betas = nil
	// Streaming responses bypass restoreBetaToolNamesInMessage, so undo the
	// rename on the wire instead. No-op for non-streaming responses.
	if len(reverseMap) > 0 {
		options = append(options, anthropicOption.WithMiddleware(restoreToolNamesMiddleware(reverseMap)))
	}
	logrus.WithContext(ctx).Debugf("session: %s", meta.SessionID)
	logrus.WithContext(ctx).Debugf("metadata: %s", req.Metadata.UserID)

	// Create SDK client
	anthropicClient := anthropic.NewClient(options...)

	// Wrap in AnthropicClient base
	base := &AnthropicClient{
		client:   anthropicClient,
		provider: c.AnthropicClient.provider,
	}
	return base, reverseMap
}

// The Guard'ed client already carries the complete anthropic-beta header
// (context_1m folded in), so the calls below go to the SDK directly instead of
// through AnthropicClient's wrappers, whose withContext1MBeta /
// context1MHeaderOpts would append context-1m again as a second header value.

// MessagesNew creates a new message request.
func (c *ClaudeClient) MessagesNew(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error) {
	guard, reverseMap := c.Guard(ctx, req)
	msg, err := guard.client.Messages.New(ctx, *req)
	if err != nil {
		return nil, err
	}
	restoreToolNamesInMessage(msg, reverseMap)
	return msg, nil
}

// MessagesNewStreaming creates a new streaming message request.
func (c *ClaudeClient) MessagesNewStreaming(ctx context.Context, req *anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	guard, _ := c.Guard(ctx, req)
	return guard.client.Messages.NewStreaming(ctx, *req)
}

// BetaMessagesNew creates a new beta message request.
func (c *ClaudeClient) BetaMessagesNew(ctx context.Context, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	guard, reverseMap := c.GuardBeta(ctx, req)
	msg, err := guard.client.Beta.Messages.New(ctx, *req)
	if err != nil {
		return nil, err
	}
	restoreBetaToolNamesInMessage(msg, reverseMap)
	return msg, nil
}

// BetaMessagesNewStreaming creates a new beta streaming message request.
func (c *ClaudeClient) BetaMessagesNewStreaming(ctx context.Context, req *anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	guard, _ := c.GuardBeta(ctx, req)
	return guard.client.Beta.Messages.NewStreaming(ctx, *req)
}

// countTokensClient builds a client whose anthropic-beta header is the
// count_tokens subset the CLI sends (claude-code, interleaved-thinking,
// context-management, oauth), for the given model.
func (c *ClaudeClient) countTokensClient(ctx context.Context, model string) anthropic.Client {
	sig := baseClaudeBetaSignals(ctx, model, c.isOAuth())
	betas := filterClaudeCodeCountTokensBetas(composeClaudeCodeBetas(sig))
	base := c.AnthropicClient.Client().Options
	options := make([]anthropicOption.RequestOption, 0, len(base)+1)
	options = append(options, base...)
	options = append(options, anthropicOption.WithHeader("anthropic-beta", joinBetas(betas)))
	return anthropic.NewClient(options...)
}

// MessagesCountTokens counts tokens for a message request.
func (c *ClaudeClient) MessagesCountTokens(ctx context.Context, req *anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	client := c.countTokensClient(ctx, string(req.Model))
	return client.Messages.CountTokens(ctx, *req)
}

// BetaMessagesCountTokens counts tokens for a beta message request.
func (c *ClaudeClient) BetaMessagesCountTokens(ctx context.Context, req *anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error) {
	client := c.countTokensClient(ctx, string(req.Model))
	req.Betas = nil
	return client.Beta.Messages.CountTokens(ctx, *req)
}

// Close closes any resources held by the client.
func (c *ClaudeClient) Close() error {
	return c.AnthropicClient.Close()
}

// GetProvider returns the provider for this client.
func (c *ClaudeClient) GetProvider() *typ.Provider {
	return c.AnthropicClient.GetProvider()
}

// APIStyle returns the API style.
func (c *ClaudeClient) APIStyle() protocol.APIStyle {
	return c.AnthropicClient.APIStyle()
}

// Client returns the underlying Anthropic SDK client.
func (c *ClaudeClient) Client() *anthropic.Client {
	return c.AnthropicClient.Client()
}

// stripBetaClearThinkingEdit removes any clear_thinking_20251015 context-management
// edit from the request. The Anthropic API rejects this edit type when thinking is not
// enabled or adaptive, so it must be dropped whenever thinking is disabled — otherwise
// Claude Code OAuth traffic that ships the edit (while we force thinking off) fails with
// "clear_thinking_20251015 requires `thinking` to be enabled or adaptive".
func stripBetaClearThinkingEdit(req *anthropic.BetaMessageNewParams) {
	edits := req.ContextManagement.Edits
	if len(edits) == 0 {
		return
	}
	filtered := make([]anthropic.BetaContextManagementConfigEditUnionParam, 0, len(edits))
	for _, edit := range edits {
		if edit.OfClearThinking20251015 != nil {
			continue
		}
		filtered = append(filtered, edit)
	}
	req.ContextManagement.Edits = filtered
}

// remapRequestToolNames renames tool names in-place to their Claude Code
// equivalents. Returns a reverse map (outbound → original) for restoring names
// in the response.
//
// The plan is computed once from req.Tools and then applied to every site in
// the request that has to agree with it: the tool definitions, an explicit
// tool_choice pin, and the tool_use blocks of prior assistant turns. Folding a
// site independently would be wrong — planToolRenames deliberately skips a
// rename whose target collides with another tool in the same request, so an
// independent fold could pin tool_choice to a name that is not in tools[], or
// to a different tool that happens to own it.
func remapRequestToolNames(req *anthropic.MessageNewParams) map[string]string {
	if req == nil {
		return nil
	}
	names := make([]string, 0, len(req.Tools))
	for i := range req.Tools {
		if t := req.Tools[i].OfTool; t != nil {
			names = append(names, t.Name)
		}
	}
	plan := planToolRenames(names)
	if len(plan) == 0 {
		return nil
	}
	reverseMap := make(map[string]string, len(plan))
	for i := range req.Tools {
		t := req.Tools[i].OfTool
		if t == nil {
			continue
		}
		if newName, ok := plan[t.Name]; ok {
			reverseMap[newName] = t.Name
			t.Name = newName
		}
	}
	// tool_choice is request-only, so it needs no entry in the reverse map.
	if pin := req.ToolChoice.OfTool; pin != nil {
		if newName, ok := plan[pin.Name]; ok {
			pin.Name = newName
		}
	}
	// Prior turns carry the name the model was given last time. Leaving them
	// at the client's spelling would contradict the renamed tools[] and put
	// the snake_case names back on the wire this rename exists to remove.
	for i := range req.Messages {
		for j := range req.Messages[i].Content {
			use := req.Messages[i].Content[j].OfToolUse
			if use == nil {
				continue
			}
			if newName, ok := plan[use.Name]; ok {
				use.Name = newName
			}
		}
	}
	return reverseMap
}

// remapBetaRequestToolNames is the BetaMessageNewParams equivalent of
// remapRequestToolNames.
func remapBetaRequestToolNames(req *anthropic.BetaMessageNewParams) map[string]string {
	if req == nil {
		return nil
	}
	names := make([]string, 0, len(req.Tools))
	for i := range req.Tools {
		if t := req.Tools[i].OfTool; t != nil {
			names = append(names, t.Name)
		}
	}
	plan := planToolRenames(names)
	if len(plan) == 0 {
		return nil
	}
	reverseMap := make(map[string]string, len(plan))
	for i := range req.Tools {
		t := req.Tools[i].OfTool
		if t == nil {
			continue
		}
		if newName, ok := plan[t.Name]; ok {
			reverseMap[newName] = t.Name
			t.Name = newName
		}
	}
	if pin := req.ToolChoice.OfTool; pin != nil {
		if newName, ok := plan[pin.Name]; ok {
			pin.Name = newName
		}
	}
	for i := range req.Messages {
		for j := range req.Messages[i].Content {
			use := req.Messages[i].Content[j].OfToolUse
			if use == nil {
				continue
			}
			if newName, ok := plan[use.Name]; ok {
				use.Name = newName
			}
		}
	}
	return reverseMap
}

// restoreToolNamesInMessage reverses tool name remapping in a Message response.
func restoreToolNamesInMessage(msg *anthropic.Message, reverseMap map[string]string) {
	if msg == nil || len(reverseMap) == 0 {
		return
	}
	for i := range msg.Content {
		if msg.Content[i].Type == "tool_use" {
			if orig, ok := reverseMap[msg.Content[i].Name]; ok {
				msg.Content[i].Name = orig
			}
		}
	}
}

// restoreBetaToolNamesInMessage reverses tool name remapping in a BetaMessage response.
func restoreBetaToolNamesInMessage(msg *anthropic.BetaMessage, reverseMap map[string]string) {
	if msg == nil || len(reverseMap) == 0 {
		return
	}
	for i := range msg.Content {
		if msg.Content[i].Type == "tool_use" {
			if orig, ok := reverseMap[msg.Content[i].Name]; ok {
				msg.Content[i].Name = orig
			}
		}
	}
}
