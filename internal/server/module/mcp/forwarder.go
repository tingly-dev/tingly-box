package mcp

import (
	"context"
	"fmt"
	"io"
	"runtime"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
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

// AnthropicV1Forwarder implements Forwarder for Anthropic V1 API
type AnthropicV1Forwarder struct {
	clientPool ClientPoolGetter
	ctxGetter  ForwardContextGetter
}

func NewAnthropicV1Forwarder(clientPool ClientPoolGetter, ctxGetter ForwardContextGetter) *AnthropicV1Forwarder {
	return &AnthropicV1Forwarder{
		clientPool: clientPool,
		ctxGetter:  ctxGetter,
	}
}

func (f *AnthropicV1Forwarder) ForwardStream(
	ctx context.Context,
	provider any,
	model string,
	req any,
) (StreamHandle, error) {
	reqParams, ok := req.(*anthropic.MessageNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.MessageNewParams, got %T", req)
	}

	prov, ok := provider.(*typ.Provider)
	if !ok {
		return nil, fmt.Errorf("expected *typ.Provider, got %T", provider)
	}

	fc := f.ctxGetter.NewForwardContext(ctx, prov)
	wrapper := f.clientPool.GetAnthropicClient(ctx, prov, model)
	stream, cancel, err := forwarding.ForwardAnthropicV1Stream(fc, wrapper, reqParams)
	if err != nil {
		return nil, err
	}

	return &AnthropicV1StreamHandle{stream: stream, cancel: cancel, client: wrapper}, nil
}

func (f *AnthropicV1Forwarder) ForwardNonStream(
	ctx context.Context,
	provider any,
	model string,
	req any,
) (any, error) {
	reqParams, ok := req.(*anthropic.MessageNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.MessageNewParams, got %T", req)
	}

	prov, ok := provider.(*typ.Provider)
	if !ok {
		return nil, fmt.Errorf("expected *typ.Provider, got %T", provider)
	}

	fc := f.ctxGetter.NewForwardContext(ctx, prov)
	wrapper := f.clientPool.GetAnthropicClient(ctx, prov, model)
	message, cancel, err := forwarding.ForwardAnthropicV1(fc, wrapper, reqParams)
	if err != nil {
		return nil, err
	}

	// Store cancel for cleanup
	return &ForwardResult{Message: message, Cancel: cancel, AnthropicClient: wrapper}, nil
}

// AnthropicV1StreamHandle wraps MessageStream
type AnthropicV1StreamHandle struct {
	stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion]
	cancel context.CancelFunc
	client client.AnthropicClientInterface
}

func (h *AnthropicV1StreamHandle) Next() bool {
	next := h.stream.Next()
	runtime.KeepAlive(h.client)
	return next
}

func (h *AnthropicV1StreamHandle) Current() any {
	current := h.stream.Current()
	runtime.KeepAlive(h.client)
	return current
}

func (h *AnthropicV1StreamHandle) Err() error {
	err := h.stream.Err()
	runtime.KeepAlive(h.client)
	if err == io.EOF {
		return nil
	}
	return err
}

func (h *AnthropicV1StreamHandle) Close() error {
	if h.cancel != nil {
		h.cancel()
	}
	return nil
}

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

// AnthropicBetaForwarder implements Forwarder for Anthropic Beta API
type AnthropicBetaForwarder struct {
	clientPool ClientPoolGetter
	ctxGetter  ForwardContextGetter
}

func NewAnthropicBetaForwarder(clientPool ClientPoolGetter, ctxGetter ForwardContextGetter) *AnthropicBetaForwarder {
	return &AnthropicBetaForwarder{
		clientPool: clientPool,
		ctxGetter:  ctxGetter,
	}
}

func (f *AnthropicBetaForwarder) ForwardStream(
	ctx context.Context,
	provider any,
	model string,
	req any,
) (StreamHandle, error) {
	reqParams, ok := req.(*anthropic.BetaMessageNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessageNewParams, got %T", req)
	}

	prov, ok := provider.(*typ.Provider)
	if !ok {
		return nil, fmt.Errorf("expected *typ.Provider, got %T", provider)
	}

	fc := f.ctxGetter.NewForwardContext(ctx, prov)
	wrapper := f.clientPool.GetAnthropicClient(ctx, prov, model)
	stream, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, reqParams)
	if err != nil {
		return nil, err
	}

	return &AnthropicBetaStreamHandle{stream: stream, cancel: cancel, client: wrapper}, nil
}

func (f *AnthropicBetaForwarder) ForwardNonStream(
	ctx context.Context,
	provider any,
	model string,
	req any,
) (any, error) {
	reqParams, ok := req.(*anthropic.BetaMessageNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessageNewParams, got %T", req)
	}

	prov, ok := provider.(*typ.Provider)
	if !ok {
		return nil, fmt.Errorf("expected *typ.Provider, got %T", provider)
	}

	fc := f.ctxGetter.NewForwardContext(ctx, prov)
	wrapper := f.clientPool.GetAnthropicClient(ctx, prov, model)
	message, cancel, err := forwarding.ForwardAnthropicV1Beta(fc, wrapper, reqParams)
	if err != nil {
		return nil, err
	}

	return &ForwardResult{Message: message, Cancel: cancel, AnthropicClient: wrapper}, nil
}

// AnthropicBetaStreamHandle wraps Beta stream
type AnthropicBetaStreamHandle struct {
	stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]
	cancel context.CancelFunc
	client client.AnthropicClientInterface
}

func (h *AnthropicBetaStreamHandle) Next() bool {
	next := h.stream.Next()
	runtime.KeepAlive(h.client)
	return next
}

func (h *AnthropicBetaStreamHandle) Current() any {
	current := h.stream.Current()
	runtime.KeepAlive(h.client)
	return current
}

func (h *AnthropicBetaStreamHandle) Err() error {
	err := h.stream.Err()
	runtime.KeepAlive(h.client)
	if err == io.EOF {
		return nil
	}
	return err
}

func (h *AnthropicBetaStreamHandle) Close() error {
	if h.cancel != nil {
		h.cancel()
	}
	return nil
}

// ===================================================================
// OpenAI Chat Forwarder
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
