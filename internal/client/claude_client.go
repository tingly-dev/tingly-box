package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

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
func NewClaudeClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*ClaudeClient, error) {
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
	options = applyClaudeCodeHeaders(options, provider, sessionID.Value, isOAuthToken)

	// Add beta query parameter
	options = append(options, anthropicOption.WithQuery("beta", "true"))

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
func applyClaudeCodeHeaders(options []anthropicOption.RequestOption, provider *typ.Provider, sessionID string, isOAuthToken bool) []anthropicOption.RequestOption {
	// Build beta header with all required flags
	baseBetas := anthropicBeta

	// Add context-1m for models that support it (Sonnet/Opus, not Haiku)
	// Note: Currently commented out as per original claude.go
	// if model != "" && supportsContext1M(model) {
	// 	baseBetas = strings.TrimRight(baseBetas, ",") + "," + anthropicContext1m
	// }

	baseBetas = strings.TrimRight(baseBetas, ",")

	// Ensure oauth is always present at the end
	if !strings.Contains(baseBetas, "oauth") {
		baseBetas = strings.TrimRight(baseBetas, ",")
		baseBetas = fmt.Sprintf("%s,%s", baseBetas, anthropicOAuthBeta)
	}

	// Auth header
	if isOAuthToken {
		options = append(options, anthropicOption.WithHeader("Authorization", "Bearer "+provider.GetAccessToken()))
	} else {
		options = append(options, anthropicOption.WithHeader("x-api-key", provider.GetAccessToken()))
	}

	// Claude Code specific headers
	options = append(options,
		anthropicOption.WithHeader("accept", acceptHeader),
		anthropicOption.WithHeader("anthropic-beta", baseBetas),
		anthropicOption.WithHeader("anthropic-dangerous-direct-browser-access", anthropicDangerousDirectBrowserAccess),
		anthropicOption.WithHeader("anthropic-version", anthropicVersion),
		anthropicOption.WithHeader("user-agent", claudeCLIUserAgent),
		anthropicOption.WithHeader("x-app", claudeXApp),
		anthropicOption.WithHeader("x-stainless-helper-method", stainlessHelperMethod),
		anthropicOption.WithHeader("x-stainless-retry-count", stainlessRetryCount),
		anthropicOption.WithHeader("x-stainless-runtime-version", stainlessRuntimeVersion),
		anthropicOption.WithHeader("x-stainless-package-version", stainlessPackageVersion),
		anthropicOption.WithHeader("x-stainless-runtime", stainlessRuntime),
		anthropicOption.WithHeader("x-stainless-lang", stainlessLang),
		anthropicOption.WithHeader("x-stainless-arch", stainlessArch()),
		anthropicOption.WithHeader("x-stainless-os", stainlessOS()),
		anthropicOption.WithHeader("x-stainless-timeout", stainlessTimeout),
	)

	return options
}

// ===================================================================
// ClaudeClient interface methods
// ===================================================================

// ListModels returns the list of available models.
// For Claude Code OAuth, this returns an error as the token cannot access /models endpoint.
func (c *ClaudeClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, &ErrModelsEndpointNotSupported{
		Provider: c.provider.Name,
		Reason:   "Claude Code OAuth token cannot access /models endpoint",
	}
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

	// Remap tool names to Claude Code TitleCase equivalents to avoid Anthropic fingerprinting
	reverseMap := remapToolNames(req.Tools)

	// Inject session ID from metadata
	meta := ops.ParseMetadataUserID(req.Metadata.UserID.String())
	if meta == nil {
		panic("invalid metadata")
	}
	options := append(c.AnthropicClient.Client().Options, anthropicOption.WithHeader("X-Claude-Code-Session-Id", meta.SessionID))
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

	// Remap tool names to Claude Code TitleCase equivalents to avoid Anthropic fingerprinting
	reverseMap := remapBetaToolNames(req.Tools)

	// Inject session ID from metadata
	meta := ops.ParseMetadataUserID(req.Metadata.UserID.String())
	if meta == nil {
		panic("invalid metadata")
	}
	options := append(c.AnthropicClient.Client().Options, anthropicOption.WithHeader("X-Claude-Code-Session-Id", meta.SessionID))
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

// MessagesNew creates a new message request.
func (c *ClaudeClient) MessagesNew(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error) {
	guard, reverseMap := c.Guard(ctx, req)
	msg, err := guard.MessagesNew(ctx, req)
	if err != nil {
		return nil, err
	}
	restoreToolNamesInMessage(msg, reverseMap)
	return msg, nil
}

// MessagesNewStreaming creates a new streaming message request.
func (c *ClaudeClient) MessagesNewStreaming(ctx context.Context, req *anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	guard, _ := c.Guard(ctx, req)
	return guard.MessagesNewStreaming(ctx, req)
}

// BetaMessagesNew creates a new beta message request.
func (c *ClaudeClient) BetaMessagesNew(ctx context.Context, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	guard, reverseMap := c.GuardBeta(ctx, req)
	msg, err := guard.BetaMessagesNew(ctx, req)
	if err != nil {
		return nil, err
	}
	restoreBetaToolNamesInMessage(msg, reverseMap)
	return msg, nil
}

// BetaMessagesNewStreaming creates a new beta streaming message request.
func (c *ClaudeClient) BetaMessagesNewStreaming(ctx context.Context, req *anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	guard, _ := c.GuardBeta(ctx, req)
	return guard.BetaMessagesNewStreaming(ctx, req)
}

// MessagesCountTokens counts tokens for a message request.
func (c *ClaudeClient) MessagesCountTokens(ctx context.Context, req *anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	return c.AnthropicClient.MessagesCountTokens(ctx, req)
}

// BetaMessagesCountTokens counts tokens for a beta message request.
func (c *ClaudeClient) BetaMessagesCountTokens(ctx context.Context, req *anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error) {
	return c.AnthropicClient.BetaMessagesCountTokens(ctx, req)
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

// SetRecordSink sets the record sink for the client.
func (c *ClaudeClient) SetRecordSink(sink *obs.Sink) {
	c.AnthropicClient.SetRecordSink(sink)
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

// remapToolNames renames OfTool tools in-place using oauthToolRenameMap.
// Returns a reverse map (TitleCase → original) for restoring names in the response.
func remapToolNames(tools []anthropic.ToolUnionParam) map[string]string {
	reverseMap := make(map[string]string)
	for i := range tools {
		t := tools[i].OfTool
		if t == nil {
			continue
		}
		if newName, ok := oauthToolRenameMap[t.Name]; ok && newName != t.Name {
			reverseMap[newName] = t.Name
			tools[i].OfTool.Name = newName
		}
	}
	return reverseMap
}

// remapBetaToolNames is the BetaToolUnionParam equivalent of remapToolNames.
func remapBetaToolNames(tools []anthropic.BetaToolUnionParam) map[string]string {
	reverseMap := make(map[string]string)
	for i := range tools {
		t := tools[i].OfTool
		if t == nil {
			continue
		}
		if newName, ok := oauthToolRenameMap[t.Name]; ok && newName != t.Name {
			reverseMap[newName] = t.Name
			tools[i].OfTool.Name = newName
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
