package server

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// openAIChatProviderEndpoint is the transport-free Chat provider terminal used
// by protocol paths whose selected provider exposes Chat Completions.
type openAIChatProviderEndpoint struct {
	ph       *ProtocolHandler
	provider *typ.Provider
	model    string
}

func (*openAIChatProviderEndpoint) Protocol() protocol.APIType { return protocol.TypeOpenAIChat }

func (e *openAIChatProviderEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	chatRequest, err := protocolStageOpenAIChatRequest(call.Request)
	if err != nil {
		return nil, err
	}
	request.CleanupOpenaiFields(chatRequest)
	wrapper := e.ph.deps.ClientPool.GetOpenAIClient(ctx, e.provider, e.model)
	fc := forwarding.NewForwardContext(ctx, e.provider)
	completion, cancel, err := forwarding.ForwardOpenAIChat(fc, wrapper, chatRequest)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		return nil, err
	}
	return &protocolstage.Response{
		Value: completion,
		Usage: protocolusage.FromOpenAIChatCompletion(completion.Usage),
		Model: e.model,
	}, nil
}

func (e *openAIChatProviderEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	chatRequest, err := protocolStageOpenAIChatRequest(call.Request)
	if err != nil {
		return nil, err
	}
	request.CleanupOpenaiFields(chatRequest)
	wrapper := e.ph.deps.ClientPool.GetOpenAIClient(ctx, e.provider, e.model)
	fc := forwarding.NewForwardContext(ctx, e.provider)
	stream, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, chatRequest)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	return &openAIChatProviderStream{
		stream: stream,
		cancel: cancel,
		model:  e.model,
	}, nil
}

func protocolStageOpenAIChatRequest(value any) (*openai.ChatCompletionNewParams, error) {
	request, ok := value.(*openai.ChatCompletionNewParams)
	if !ok || request == nil {
		return nil, &protocolStageSetupError{err: fmt.Errorf("OpenAI Chat provider endpoint received %T", value)}
	}
	return request, nil
}

type openAIChatProviderStream struct {
	stream *openaistream.Stream[openai.ChatCompletionChunk]
	cancel context.CancelFunc
	model  string
	usage  *protocol.TokenUsage

	closeOnce sync.Once
	closeErr  error
}

func (s *openAIChatProviderStream) Next(ctx context.Context) (protocolstage.Event, error) {
	if err := ctx.Err(); err != nil {
		return protocolstage.Event{}, err
	}
	if s.stream == nil {
		return protocolstage.Event{}, fmt.Errorf("OpenAI Chat provider stream is nil")
	}
	if !s.stream.Next() {
		if err := s.stream.Err(); err != nil {
			return protocolstage.Event{}, err
		}
		return protocolstage.Event{}, io.EOF
	}
	chunk := s.stream.Current()
	if usage := protocolusage.FromOpenAIChatCompletion(chunk.Usage); usage.HasUsage() {
		s.usage = usage
	}
	return protocolstage.Event{Value: chunk}, nil
}

func (s *openAIChatProviderStream) Close() error {
	s.closeOnce.Do(func() {
		if s.stream != nil {
			s.closeErr = s.stream.Close()
		}
		if s.cancel != nil {
			s.cancel()
		}
	})
	return s.closeErr
}

func (s *openAIChatProviderStream) Result() protocolstage.StreamResult {
	return protocolstage.StreamResult{Usage: s.usage, Model: s.model}
}
