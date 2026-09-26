package toolengine

import (
	"context"
	"fmt"
	"io"

	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClientPoolGetter matches client.ClientPool interface
type ClientPoolGetter interface {
	GetAnthropicClient(ctx context.Context, provider *typ.Provider, model string) client.AnthropicClientInterface
	GetOpenAIClient(ctx context.Context, provider *typ.Provider, model string) client.OpenAIClientInterface
}

// ForwardContextGetter provides ForwardContext
type ForwardContextGetter interface {
	NewForwardContext(ctx context.Context, provider *typ.Provider) *forwarding.ForwardContext
}

// ===================================================================
// Anthropic V1 Forwarder
// ===================================================================

// ForwardResult wraps non-streaming response with cancel func
type ForwardResult struct {
	Message         any
	Cancel          context.CancelFunc
	AnthropicClient client.AnthropicClientInterface
	OpenAIClient    client.OpenAIClientInterface
}

// ===================================================================
// Anthropic Beta Forwarder
// ===================================================================

// OpenAIChatForwarder implements Forwarder for OpenAI Chat API
type OpenAIChatForwarder struct {
	clientPool ClientPoolGetter
	ctxGetter  ForwardContextGetter
}

func NewOpenAIChatForwarder(clientPool ClientPoolGetter, ctxGetter ForwardContextGetter) *OpenAIChatForwarder {
	return &OpenAIChatForwarder{
		clientPool: clientPool,
		ctxGetter:  ctxGetter,
	}
}

func (f *OpenAIChatForwarder) ForwardStream(
	ctx context.Context,
	provider any,
	model string,
	req any,
) (StreamHandle, error) {
	reqParams, ok := req.(*openai.ChatCompletionNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletionNewParams, got %T", req)
	}

	prov, ok := provider.(*typ.Provider)
	if !ok {
		return nil, fmt.Errorf("expected *typ.Provider, got %T", provider)
	}

	fc := f.ctxGetter.NewForwardContext(ctx, prov)
	wrapper := f.clientPool.GetOpenAIClient(ctx, prov, model)
	stream, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, reqParams)
	if err != nil {
		return nil, err
	}

	return &OpenAIChatStreamHandle{stream: stream, cancel: cancel}, nil
}

func (f *OpenAIChatForwarder) ForwardNonStream(
	ctx context.Context,
	provider any,
	model string,
	req any,
) (any, error) {
	reqParams, ok := req.(*openai.ChatCompletionNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletionNewParams, got %T", req)
	}

	prov, ok := provider.(*typ.Provider)
	if !ok {
		return nil, fmt.Errorf("expected *typ.Provider, got %T", provider)
	}

	fc := f.ctxGetter.NewForwardContext(ctx, prov)
	wrapper := f.clientPool.GetOpenAIClient(ctx, prov, model)
	completion, cancel, err := forwarding.ForwardOpenAIChat(fc, wrapper, reqParams)
	if err != nil {
		return nil, err
	}

	return &ForwardResult{Message: completion, Cancel: cancel}, nil
}

// OpenAIChatStreamHandle wraps OpenAI stream
type OpenAIChatStreamHandle struct {
	stream *openaistream.Stream[openai.ChatCompletionChunk]
	cancel context.CancelFunc
}

func (h *OpenAIChatStreamHandle) Next() bool {
	return h.stream.Next()
}

func (h *OpenAIChatStreamHandle) Current() any {
	return h.stream.Current()
}

func (h *OpenAIChatStreamHandle) Err() error {
	err := h.stream.Err()
	if err == io.EOF {
		return nil
	}
	return err
}

func (h *OpenAIChatStreamHandle) Close() error {
	if h.cancel != nil {
		h.cancel()
	}
	return nil
}
