package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"

	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// NewOpenAIChat returns the OpenAI Chat Completions terminal endpoint.
func NewOpenAIChat(config Config) (stage.Endpoint, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &openAIChatEndpoint{config: config}, nil
}

type openAIChatEndpoint struct{ config Config }

func (*openAIChatEndpoint) Protocol() protocol.APIType { return protocol.TypeOpenAIChat }

func (e *openAIChatEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	req, err := openAIChatRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.config.Clients.GetOpenAIClient(ctx, e.config.Provider, e.config.Model)
	completion, cancel, err := forwarding.ForwardOpenAIChat(e.config.forwardContext(ctx), wrapper, req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		return nil, err
	}
	return &stage.Response{
		Value: completion,
		Usage: protocolusage.FromOpenAIChatCompletion(completion.Usage),
	}, nil
}

func (e *openAIChatEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	req, err := openAIChatRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.config.Clients.GetOpenAIClient(ctx, e.config.Provider, e.config.Model)
	stream, cancel, err := forwarding.ForwardOpenAIChatStream(e.config.forwardContext(ctx), wrapper, req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	return &openAIStream[openai.ChatCompletionChunk]{
		client: wrapper,
		stream: stream,
		cancel: cancel,
		usageOf: func(chunk openai.ChatCompletionChunk) *protocol.TokenUsage {
			if u := protocolusage.FromOpenAIChatCompletion(chunk.Usage); u.HasUsage() {
				return u
			}
			return nil
		},
	}, nil
}

func openAIChatRequest(value any) (*openai.ChatCompletionNewParams, error) {
	switch req := value.(type) {
	case *openai.ChatCompletionNewParams:
		if req != nil {
			return req, nil
		}
	case openai.ChatCompletionNewParams:
		return &req, nil
	}
	return nil, fmt.Errorf("OpenAI Chat upstream endpoint: request has type %T, want openai.ChatCompletionNewParams", value)
}

// NewOpenAIResponses returns the OpenAI Responses terminal endpoint.
func NewOpenAIResponses(config Config) (stage.Endpoint, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &openAIResponsesEndpoint{config: config}, nil
}

type openAIResponsesEndpoint struct{ config Config }

func (*openAIResponsesEndpoint) Protocol() protocol.APIType { return protocol.TypeOpenAIResponses }

func (e *openAIResponsesEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	if e.config.StreamOnly {
		return e.completeFromStream(ctx, call)
	}
	req, err := openAIResponsesRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.config.Clients.GetOpenAIClient(ctx, e.config.Provider, e.config.Model)
	response, cancel, err := forwarding.ForwardOpenAIResponses(e.config.forwardContext(ctx), wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		return nil, err
	}
	return &stage.Response{
		Value: response,
		Usage: protocolusage.FromOpenAIResponses(response.Usage),
	}, nil
}

func (e *openAIResponsesEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	req, err := openAIResponsesRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.config.Clients.GetOpenAIClient(ctx, e.config.Provider, e.config.Model)
	stream, cancel, err := forwarding.ForwardOpenAIResponsesStream(e.config.forwardContext(ctx), wrapper, *req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	return &openAIStream[responses.ResponseStreamEventUnion]{
		client: wrapper,
		stream: stream,
		cancel: cancel,
		usageOf: func(event responses.ResponseStreamEventUnion) *protocol.TokenUsage {
			switch event.Type {
			case "response.created", "response.in_progress", "response.completed", "response.incomplete", "response.failed":
			default:
				return nil
			}
			if u := protocolusage.FromOpenAIResponses(event.Response.Usage); u.HasUsage() {
				return u
			}
			return nil
		},
	}, nil
}

func openAIResponsesRequest(value any) (*responses.ResponseNewParams, error) {
	switch req := value.(type) {
	case *responses.ResponseNewParams:
		if req != nil {
			return req, nil
		}
	case responses.ResponseNewParams:
		return &req, nil
	}
	return nil, fmt.Errorf("OpenAI Responses upstream endpoint: request has type %T, want responses.ResponseNewParams", value)
}

// openAIStream adapts an OpenAI SDK stream to stage.EventStream. usageOf
// extracts the usage an event reports, if any; the latest one wins.
type openAIStream[T any] struct {
	client  client.OpenAIClientInterface
	stream  *openaistream.Stream[T]
	cancel  context.CancelFunc
	usageOf func(T) *protocol.TokenUsage
	usage   *protocol.TokenUsage

	closeOnce sync.Once
	closeErr  error
}

func (s *openAIStream[T]) Next(ctx context.Context) (stage.Event, error) {
	defer runtime.KeepAlive(s.client)
	if err := ctx.Err(); err != nil {
		return stage.Event{}, err
	}
	if !s.stream.Next() {
		return stage.Event{}, streamEnd(s.stream.Err())
	}
	event := s.stream.Current()
	if usage := s.usageOf(event); usage != nil {
		s.usage = usage
	}
	return stage.Event{Value: event}, nil
}

func (s *openAIStream[T]) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.stream.Close()
		if s.cancel != nil {
			s.cancel()
		}
	})
	return s.closeErr
}

func (s *openAIStream[T]) Result() stage.StreamResult {
	return stage.StreamResult{Usage: s.usage}
}

// completeFromStream answers a complete call from a stream-only provider with
// the response its stream ends with. Some stream-only backends (ChatGPT's
// Codex) end with a response whose output is empty and deliver the items only
// as response.output_item.done events, so those items fill an empty output. A
// stream that yields no output at all is an EmptyResponseError (502), never
// an empty answer, so failover can retry it.
func (e *openAIResponsesEndpoint) completeFromStream(ctx context.Context, call stage.Call) (*stage.Response, error) {
	events, err := e.Stream(ctx, call)
	if err != nil {
		return nil, err
	}
	defer events.Close()
	var final *responses.Response
	var items []responses.ResponseOutputItemUnion
	for {
		event, err := events.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if final == nil && len(items) == 0 {
				return nil, &protocol.EmptyResponseError{Err: err}
			}
			return nil, err
		}
		union, ok := event.Value.(responses.ResponseStreamEventUnion)
		if !ok {
			continue
		}
		switch union.Type {
		case "response.output_item.done":
			items = append(items, union.Item)
		case "response.completed", "response.incomplete", "response.failed":
			response := union.Response
			final = &response
		}
	}
	if final == nil {
		return nil, &protocol.EmptyResponseError{Err: errors.New("OpenAI Responses upstream endpoint: stream ended without a final response")}
	}
	if len(final.Output) == 0 {
		final.Output = items
	}
	if len(final.Output) == 0 {
		return nil, &protocol.EmptyResponseError{Err: fmt.Errorf("OpenAI Responses upstream endpoint: stream ended with no output (status %q)", final.Status)}
	}
	return &stage.Response{Value: final, Usage: protocolusage.FromOpenAIResponses(final.Usage)}, nil
}
