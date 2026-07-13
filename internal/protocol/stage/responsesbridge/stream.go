package responsesbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func newResponsesStream(target stage.EventStream, sourceModel string) (stage.EventStream, error) {
	if target == nil {
		return nil, fmt.Errorf("convert Anthropic Beta stream to OpenAI Responses: target stream is nil")
	}
	iterator := &anthropicBetaStreamIterator{target: target}
	return &responsesStream{
		iterator:  iterator,
		converter: protocolstream.NewAnthropicBetaToOpenAIResponsesConverter(iterator, sourceModel),
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
			s.err = fmt.Errorf("convert Anthropic Beta stream to OpenAI Responses: event is nil")
			return false
		}
		s.current = *value
	case protocolstream.AnthropicEvent:
		if err := s.setNormalizedEvent(value); err != nil {
			s.err = err
			return false
		}
	default:
		s.err = fmt.Errorf("convert Anthropic Beta stream to OpenAI Responses: event has type %T", event.Value)
		return false
	}
	return true
}

func (s *anthropicBetaStreamIterator) setNormalizedEvent(event protocolstream.AnthropicEvent) error {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("convert normalized Anthropic event %q: marshal data: %w", event.Type, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return fmt.Errorf("convert normalized Anthropic event %q: data must be a JSON object", event.Type)
	}
	if _, ok := fields["type"]; !ok {
		fields["type"], err = json.Marshal(event.Type)
		if err != nil {
			return fmt.Errorf("convert normalized Anthropic event type: %w", err)
		}
		raw, err = json.Marshal(fields)
		if err != nil {
			return fmt.Errorf("convert normalized Anthropic event %q: marshal envelope: %w", event.Type, err)
		}
	}
	var decoded anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("convert normalized Anthropic event %q: decode Beta event: %w", event.Type, err)
	}
	if decoded.Type != event.Type {
		return fmt.Errorf("convert normalized Anthropic event: envelope type %q does not match data type %q", event.Type, decoded.Type)
	}
	s.current = decoded
	return nil
}

func (s *anthropicBetaStreamIterator) Current() anthropic.BetaRawMessageStreamEventUnion {
	return s.current
}
func (s *anthropicBetaStreamIterator) Err() error { return s.err }
func (s *anthropicBetaStreamIterator) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.target.Close() })
	return s.closeErr
}

type responsesStream struct {
	iterator  *anthropicBetaStreamIterator
	converter protocolstream.StreamConverter
	model     string
}

func (s *responsesStream) Next(ctx context.Context) (stage.Event, error) {
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
		return stage.Event{}, fmt.Errorf("convert Anthropic Beta stream to OpenAI Responses: converter emitted %T", value)
	}
	return stage.Event{Value: event}, nil
}

func (s *responsesStream) Close() error { return s.iterator.Close() }

func (s *responsesStream) Result() stage.StreamResult {
	usage := s.converter.Usage()
	if usage != nil && !usage.HasUsage() {
		usage = nil
	}
	return stage.StreamResult{Usage: usage, Model: s.model}
}
