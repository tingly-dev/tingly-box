package openaibridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func newChatStream(target stage.EventStream, sourceModel string, disableStreamUsage bool) (stage.EventStream, error) {
	if target == nil {
		return nil, fmt.Errorf("convert Anthropic Beta stream to OpenAI Chat: target stream is nil")
	}
	iterator := &anthropicBetaStreamIterator{target: target}
	return &chatStream{
		iterator:  iterator,
		converter: protocolstream.NewAnthropicBetaToOpenAIChatConverter(iterator, sourceModel, disableStreamUsage),
		model:     sourceModel,
	}, nil
}

type anthropicBetaStreamIterator struct {
	target  stage.EventStream
	ctx     context.Context
	current anthropic.BetaRawMessageStreamEventUnion
	err     error

	closeOnce sync.Once
	closeErr  error
}

func (s *anthropicBetaStreamIterator) setContext(ctx context.Context) { s.ctx = ctx }

func (s *anthropicBetaStreamIterator) Next() bool {
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
	case anthropic.BetaRawMessageStreamEventUnion:
		s.current = value
	case *anthropic.BetaRawMessageStreamEventUnion:
		if value == nil {
			s.err = fmt.Errorf("convert Anthropic Beta stream to OpenAI Chat: event is nil")
			return false
		}
		s.current = *value
	default:
		s.err = fmt.Errorf("convert Anthropic Beta stream to OpenAI Chat: event has type %T, want anthropic.BetaRawMessageStreamEventUnion", event.Value)
		return false
	}
	return true
}

func (s *anthropicBetaStreamIterator) Current() anthropic.BetaRawMessageStreamEventUnion {
	return s.current
}

func (s *anthropicBetaStreamIterator) Err() error { return s.err }

func (s *anthropicBetaStreamIterator) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.target.Close() })
	return s.closeErr
}

type chatStream struct {
	iterator  *anthropicBetaStreamIterator
	converter protocolstream.StreamConverter
	model     string
}

func (s *chatStream) Next(ctx context.Context) (stage.Event, error) {
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
	switch chunk := value.(type) {
	case wire.ChatStreamChunk:
		return stage.Event{Value: chunk}, nil
	case *wire.ChatStreamChunk:
		if chunk == nil {
			return stage.Event{}, fmt.Errorf("convert Anthropic Beta stream to OpenAI Chat: converter emitted a nil chunk")
		}
		return stage.Event{Value: *chunk}, nil
	default:
		return stage.Event{}, fmt.Errorf("convert Anthropic Beta stream to OpenAI Chat: converter emitted %T", value)
	}
}

func (s *chatStream) Close() error { return s.iterator.Close() }

func (s *chatStream) Result() stage.StreamResult {
	usage := s.converter.Usage()
	if usage != nil && !usage.HasUsage() {
		usage = nil
	}
	return stage.StreamResult{Usage: usage, Model: s.model}
}
