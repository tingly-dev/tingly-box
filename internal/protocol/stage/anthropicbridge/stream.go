package anthropicbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/openai/openai-go/v3"
	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

func newAnthropicStream(
	target stage.EventStream,
	source protocol.APIType,
	sourceModel string,
	targetRequest *openai.ChatCompletionNewParams,
) (stage.EventStream, error) {
	if target == nil {
		return nil, fmt.Errorf("convert OpenAI Chat stream to Anthropic: target stream is nil")
	}
	iterator := &chatStreamIterator{target: target}
	var converter protocolstream.StreamConverter
	switch source {
	case protocol.TypeAnthropicV1:
		converter = protocolstream.NewOpenAIChatToAnthropicV1Converter(iterator, sourceModel, targetRequest)
	case protocol.TypeAnthropicBeta:
		converter = protocolstream.NewOpenAIChatToAnthropicBetaConverter(iterator, sourceModel, targetRequest)
	default:
		return nil, fmt.Errorf("convert OpenAI Chat stream: unsupported Anthropic source protocol %q", source)
	}
	return &anthropicStream{
		iterator:  iterator,
		converter: converter,
		model:     sourceModel,
	}, nil
}

type chatStreamIterator struct {
	target  stage.EventStream
	ctx     context.Context
	current openai.ChatCompletionChunk
	err     error

	closeOnce sync.Once
	closeErr  error
}

func (s *chatStreamIterator) setContext(ctx context.Context) { s.ctx = ctx }

func (s *chatStreamIterator) Next() bool {
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
			s.err = fmt.Errorf("convert OpenAI Chat stream to Anthropic: chunk is nil")
			return false
		}
		s.current = *value
	default:
		s.err = fmt.Errorf("convert OpenAI Chat stream to Anthropic: event has type %T, want openai.ChatCompletionChunk", event.Value)
		return false
	}
	return true
}

func (s *chatStreamIterator) Current() openai.ChatCompletionChunk { return s.current }

func (s *chatStreamIterator) Err() error { return s.err }

func (s *chatStreamIterator) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.target.Close() })
	return s.closeErr
}

type anthropicStream struct {
	iterator  *chatStreamIterator
	converter protocolstream.StreamConverter
	model     string
}

func (s *anthropicStream) Next(ctx context.Context) (stage.Event, error) {
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
		return stage.Event{}, fmt.Errorf("convert OpenAI Chat stream to Anthropic: converter emitted %T", value)
	}
	return stage.Event{Value: event}, nil
}

func (s *anthropicStream) Close() error { return s.iterator.Close() }

func (s *anthropicStream) Result() stage.StreamResult {
	return stage.StreamResult{Usage: s.converter.Usage(), Model: s.model}
}
