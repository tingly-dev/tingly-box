package anthropicbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

func newAnthropicResponsesStream(ctx context.Context, target stage.EventStream, sourceModel string) (stage.EventStream, error) {
	if target == nil {
		return nil, fmt.Errorf("convert OpenAI Responses stream to Anthropic: target stream is nil")
	}
	iterator := &responsesStreamIterator{target: target, ctx: ctx}
	return &anthropicResponsesStream{
		iterator:  iterator,
		converter: protocolstream.NewOpenAIResponsesToAnthropicConverter(ctx, iterator, sourceModel),
		model:     sourceModel,
	}, nil
}

type responsesStreamIterator struct {
	target  stage.EventStream
	ctx     context.Context
	current responses.ResponseStreamEventUnion
	err     error

	closeOnce sync.Once
	closeErr  error
}

func (s *responsesStreamIterator) setContext(ctx context.Context) { s.ctx = ctx }
func (s *responsesStreamIterator) Next() bool {
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
			s.err = fmt.Errorf("convert OpenAI Responses stream to Anthropic: event is nil")
			return false
		}
		s.current = *value
	default:
		s.err = fmt.Errorf("convert OpenAI Responses stream to Anthropic: event has type %T", event.Value)
		return false
	}
	return true
}
func (s *responsesStreamIterator) Current() responses.ResponseStreamEventUnion { return s.current }
func (s *responsesStreamIterator) Err() error                                  { return s.err }
func (s *responsesStreamIterator) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.target.Close() })
	return s.closeErr
}

type anthropicResponsesStream struct {
	iterator  *responsesStreamIterator
	converter protocolstream.StreamConverter
	model     string
}

func (s *anthropicResponsesStream) Next(ctx context.Context) (stage.Event, error) {
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
	event, ok := protocolstream.AsAnthropicEvent(value)
	if !ok {
		return stage.Event{}, fmt.Errorf("convert OpenAI Responses stream to Anthropic: converter emitted %T", value)
	}
	return stage.Event{Value: event}, nil
}
func (s *anthropicResponsesStream) Close() error { return s.iterator.Close() }
func (s *anthropicResponsesStream) Result() stage.StreamResult {
	usage := s.converter.Usage()
	if usage != nil && !usage.HasUsage() {
		usage = nil
	}
	return stage.StreamResult{Usage: usage, Model: s.model}
}
