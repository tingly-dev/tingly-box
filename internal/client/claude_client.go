package client

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClaudeClient wraps AnthropicClient with Claude Code OAuth-specific behaviors.
// It embeds AnthropicClient to inherit standard Anthropic API functionality,
// while overriding methods that require special handling for Claude Code OAuth.
//
// Claude Code (Claude Code OAuth) limitations:
// - Does NOT support /models endpoint (returns 404)
// - Requires special headers via claudeRoundTripper (already applied in transport)
// - Requires tool prefix stripping (already applied in claudeRoundTripper)
type ClaudeClient struct {
	*AnthropicClient
}

// NewClaudeClient creates a new Claude client wrapper.
// The base AnthropicClient is configured with claudeRoundTripper for header/tool transformations.
func NewClaudeClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*ClaudeClient, error) {
	base, err := NewAnthropicClient(provider, model, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create base Anthropic client: %w", err)
	}

	return &ClaudeClient{
		AnthropicClient: base,
	}, nil
}

// ListModels returns the list of available models.
// For Claude Code OAuth, this returns an error as the token cannot access /models endpoint.
func (c *ClaudeClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, &ErrModelsEndpointNotSupported{
		Provider: c.provider.Name,
		Reason:   "Claude Code OAuth token cannot access /models endpoint",
	}
}

// The following methods delegate to the embedded AnthropicClient.
// They are explicitly defined to satisfy the AnthropicClientInterface.

// MessagesNew creates a new message request.
func (c *ClaudeClient) MessagesNew(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error) {
	return c.AnthropicClient.MessagesNew(ctx, req)
}

// MessagesNewStreaming creates a new streaming message request.
func (c *ClaudeClient) MessagesNewStreaming(ctx context.Context, req *anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	return c.AnthropicClient.MessagesNewStreaming(ctx, req)
}

// BetaMessagesNew creates a new beta message request.
func (c *ClaudeClient) BetaMessagesNew(ctx context.Context, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	return c.AnthropicClient.BetaMessagesNew(ctx, req)
}

// BetaMessagesNewStreaming creates a new beta streaming message request.
func (c *ClaudeClient) BetaMessagesNewStreaming(ctx context.Context, req *anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	return c.AnthropicClient.BetaMessagesNewStreaming(ctx, req)
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

// ProbeChatEndpoint tests the messages endpoint.
// For ClaudeClient, we delegate to the embedded AnthropicClient's probe method.
func (c *ClaudeClient) ProbeChatEndpoint(ctx context.Context, model string) ProbeResult {
	return c.AnthropicClient.ProbeChatEndpoint(ctx, model)
}

// ProbeModelsEndpoint tests the models endpoint.
// For Claude Code OAuth, this returns an error as the endpoint is not supported.
func (c *ClaudeClient) ProbeModelsEndpoint(ctx context.Context) ProbeResult {
	return ProbeResult{
		Success:      false,
		ErrorMessage: "Claude Code does not support /models endpoint",
	}
}

// ProbeOptionsEndpoint tests basic connectivity with an OPTIONS request.
func (c *ClaudeClient) ProbeOptionsEndpoint(ctx context.Context) ProbeResult {
	return c.AnthropicClient.ProbeOptionsEndpoint(ctx)
}
