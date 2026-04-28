package forwarding

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
)

// ForwardAnthropicV1 sends a non-streaming Anthropic v1 message request.
func ForwardAnthropicV1(fc *ForwardContext, wrapper *client.AnthropicClient, req *anthropic.MessageNewParams) (*anthropic.Message, context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get Anthropic client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(req)
	message, err := wrapper.MessagesNew(ctx, req)
	fc.Complete(ctx, message, err)

	if err != nil {
		cancel()
		return nil, nil, err
	}

	return message, cancel, nil
}

// ForwardAnthropicV1Stream sends a streaming Anthropic v1 message request.
// Note: Set BaseCtx via WithBaseCtx() to support client cancellation.
func ForwardAnthropicV1Stream(fc *ForwardContext, wrapper *client.AnthropicClient, req *anthropic.MessageNewParams) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get Anthropic client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(req)
	logrus.Debugln("Creating Anthropic v1 streaming request")
	stream := wrapper.MessagesNewStreaming(ctx, req)
	return stream, cancel, nil
}

// ForwardAnthropicV1Beta sends a non-streaming Anthropic v1 beta message request.
func ForwardAnthropicV1Beta(fc *ForwardContext, wrapper *client.AnthropicClient, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get Anthropic client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(req)
	message, err := wrapper.BetaMessagesNew(ctx, req)
	fc.Complete(ctx, message, err)

	if err != nil {
		cancel()
		return nil, nil, err
	}

	return message, cancel, nil
}

// ForwardAnthropicV1BetaStream sends a streaming Anthropic v1 beta message request.
// Note: Set BaseCtx via WithBaseCtx() to support client cancellation.
func ForwardAnthropicV1BetaStream(fc *ForwardContext, wrapper *client.AnthropicClient, req *anthropic.BetaMessageNewParams) (*anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get Anthropic client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(req)
	logrus.Debugln("Creating Anthropic v1 beta streaming request")
	stream := wrapper.BetaMessagesNewStreaming(ctx, req)
	return stream, cancel, nil
}
