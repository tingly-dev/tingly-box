package openaibridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func newResponsesChatStream(ctx context.Context, target stage.EventStream, sourceModel string, disableUsage bool) (stage.EventStream, error) {
	if target == nil {
		return nil, fmt.Errorf("convert OpenAI Responses stream to Chat: target stream is nil")
	}
	iterator := &openAIResponsesStreamIterator{target: target, ctx: ctx}
	return &responsesChatStream{
		iterator:  iterator,
		converter: protocolstream.NewOpenAIResponsesToChatConverter(iterator, sourceModel, disableUsage),
		model:     sourceModel,
	}, nil
}

type openAIResponsesStreamIterator struct {
	target  stage.EventStream
	ctx     context.Context
	current responses.ResponseStreamEventUnion
	err     error

	closeOnce sync.Once
	closeErr  error
}

func (s *openAIResponsesStreamIterator) setContext(ctx context.Context) { s.ctx = ctx }
func (s *openAIResponsesStreamIterator) Next() bool {
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
	case responses.ResponseStreamEventUnion:
		s.current = value
	case *responses.ResponseStreamEventUnion:
		if value == nil {
			s.err = fmt.Errorf("convert OpenAI Responses stream to Chat: event is nil")
			return false
		}
		s.current = *value
	default:
		s.err = fmt.Errorf("convert OpenAI Responses stream to Chat: event has type %T", event.Value)
		return false
	}
	return true
}
func (s *openAIResponsesStreamIterator) Current() responses.ResponseStreamEventUnion {
	return s.current
}
func (s *openAIResponsesStreamIterator) Err() error { return s.err }
func (s *openAIResponsesStreamIterator) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.target.Close() })
	return s.closeErr
}

type responsesChatStream struct {
	iterator  *openAIResponsesStreamIterator
	converter protocolstream.StreamConverter
	model     string
}

func (s *responsesChatStream) Next(ctx context.Context) (stage.Event, error) {
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
	chunk, ok := value.(wire.ChatStreamChunk)
	if !ok {
		return stage.Event{}, fmt.Errorf("convert OpenAI Responses stream to Chat: converter emitted %T", value)
	}
	return stage.Event{Value: chunk}, nil
}
func (s *responsesChatStream) Close() error { return s.iterator.Close() }
func (s *responsesChatStream) Result() stage.StreamResult {
	usage := s.converter.Usage()
	if usage != nil && !usage.HasUsage() {
		usage = nil
	}
	return stage.StreamResult{Usage: usage, Model: s.model}
}
