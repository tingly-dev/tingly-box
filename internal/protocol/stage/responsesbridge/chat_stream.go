package responsesbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func newChatResponsesStream(target stage.EventStream, sourceModel string) (stage.EventStream, error) {
	if target == nil {
		return nil, fmt.Errorf("convert Chat stream to OpenAI Responses: target stream is nil")
	}
	iterator := &openAIChatStreamIterator{target: target}
	return &chatResponsesStream{
		iterator:  iterator,
		converter: protocolstream.NewChatToResponsesConverter(iterator, sourceModel),
		model:     sourceModel,
	}, nil
}

type openAIChatStreamIterator struct {
	target  stage.EventStream
	ctx     context.Context
	current openai.ChatCompletionChunk
	err     error

	closeOnce sync.Once
	closeErr  error
}

func (s *openAIChatStreamIterator) setContext(ctx context.Context) { s.ctx = ctx }
func (s *openAIChatStreamIterator) Next() bool {
	if s.err != nil {
		return false
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	event, err := s.target.Next(ctx)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			s.err = err
		}
		return false
	}
	switch value := event.Value.(type) {
	case openai.ChatCompletionChunk:
		s.current = value
	case *openai.ChatCompletionChunk:
		if value == nil {
			s.err = fmt.Errorf("convert Chat stream to OpenAI Responses: event is nil")
			return false
		}
		s.current = *value
	default:
		s.err = fmt.Errorf("convert Chat stream to OpenAI Responses: event has type %T", event.Value)
		return false
	}
	return true
}
func (s *openAIChatStreamIterator) Current() openai.ChatCompletionChunk { return s.current }
func (s *openAIChatStreamIterator) Err() error                          { return s.err }
func (s *openAIChatStreamIterator) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.target.Close() })
	return s.closeErr
}

type chatResponsesStream struct {
	iterator  *openAIChatStreamIterator
	converter protocolstream.StreamConverter
	model     string
}

func (s *chatResponsesStream) Next(ctx context.Context) (stage.Event, error) {
	if err := ctx.Err(); err != nil {
		return stage.Event{}, err
	}
	s.iterator.setContext(ctx)
	value, done, err := s.converter.Next()
	if err != nil {
		return stage.Event{}, err
	}
	if done {
		if err := s.iterator.Err(); err != nil {
			return stage.Event{}, err
		}
		return stage.Event{}, io.EOF
	}
	event, ok := value.(wire.ResponsesEvent)
	if !ok {
		return stage.Event{}, fmt.Errorf("convert Chat stream to OpenAI Responses: converter emitted %T", value)
	}
	return stage.Event{Value: event}, nil
}
func (s *chatResponsesStream) Close() error { return s.iterator.Close() }
func (s *chatResponsesStream) Result() stage.StreamResult {
	usage := s.converter.Usage()
	if usage != nil && !usage.HasUsage() {
		usage = nil
	}
	return stage.StreamResult{Usage: usage, Model: s.model}
}
