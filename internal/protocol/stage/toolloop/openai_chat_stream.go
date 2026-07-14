package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// openAIChatToolLoopStream only buffers a round while deciding whether it is
// an internal tool round. Once visible content appears, events are passed
// through incrementally and that public round is never intercepted.
type openAIChatToolLoopStream struct {
	endpoint *openAIChatEndpoint
	call     protocolstage.Call
	owned    map[string]struct{}
	runCtx   context.Context

	round     int
	current   protocolstage.EventStream
	assembler assembler.StreamAssembler
	buffered  []protocolstage.Event
	pending   []protocolstage.Event
	public    bool

	usage       *protocol.TokenUsage
	model       string
	sideEffects bool
	done        bool
	closed      bool
	eofPending  bool
}

func newOpenAIChatToolLoopStream(
	ctx context.Context,
	endpoint *openAIChatEndpoint,
	call protocolstage.Call,
	owned map[string]struct{},
) (protocolstage.EventStream, error) {
	stream := &openAIChatToolLoopStream{
		endpoint: endpoint,
		call:     call,
		owned:    owned,
		runCtx:   ctx,
	}
	if err := stream.startRound(ctx); err != nil {
		return nil, err
	}
	return stream, nil
}

func (s *openAIChatToolLoopStream) Next(ctx context.Context) (protocolstage.Event, error) {
	for {
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			if len(s.pending) == 0 && s.eofPending {
				s.done = true
			}
			return event, nil
		}
		if s.done || s.closed {
			return protocolstage.Event{}, io.EOF
		}

		event, err := s.current.Next(ctx)
		if err == nil {
			if assembleErr := s.assembler.Add(event.Value); assembleErr != nil {
				return protocolstage.Event{}, s.fail(assembleErr)
			}
			if s.public {
				return event, nil
			}

			visible, classifyErr := chatStreamEventVisible(event.Value)
			if classifyErr != nil {
				return protocolstage.Event{}, s.fail(classifyErr)
			}
			s.buffered = append(s.buffered, event)
			if visible {
				s.public = true
				s.pending = s.buffered
				s.buffered = nil
			}
			continue
		}

		s.absorbCurrentResult()
		closeErr := s.closeCurrent()
		if !errors.Is(err, io.EOF) {
			if closeErr != nil {
				err = errors.Join(err, closeErr)
			}
			return protocolstage.Event{}, s.fail(err)
		}
		if closeErr != nil {
			return protocolstage.Event{}, s.fail(closeErr)
		}
		if s.public {
			s.done = true
			return protocolstage.Event{}, io.EOF
		}

		complete, finishErr := s.assembler.Finish()
		if finishErr != nil {
			return protocolstage.Event{}, s.fail(finishErr)
		}
		roundResponse, parseErr := parseChatRound(complete)
		if parseErr != nil {
			return protocolstage.Event{}, s.fail(parseErr)
		}
		if !allCallsOwned(roundResponse.calls, s.owned) {
			s.pending = s.buffered
			s.buffered = nil
			s.eofPending = true
			if len(s.pending) == 0 {
				s.done = true
				return protocolstage.Event{}, io.EOF
			}
			continue
		}
		if s.round >= s.endpoint.stage.maxRounds {
			return protocolstage.Event{}, s.fail(ErrMaxRounds)
		}

		results, nextCtx, committed, executeErr := s.endpoint.executeCalls(s.runCtx, roundResponse.calls)
		s.sideEffects = s.sideEffects || committed
		if executeErr != nil {
			return protocolstage.Event{}, s.fail(executeErr)
		}
		s.runCtx = nextCtx
		nextRequest, appendErr := appendChatToolResults(s.call.Request, roundResponse.assistant, results)
		if appendErr != nil {
			return protocolstage.Event{}, s.fail(appendErr)
		}
		s.call.Request = nextRequest
		s.buffered = nil
		s.public = false
		if startErr := s.startRound(s.runCtx); startErr != nil {
			return protocolstage.Event{}, s.fail(startErr)
		}
	}
}

func (s *openAIChatToolLoopStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.done = true
	s.absorbCurrentResult()
	return s.closeCurrent()
}

func (s *openAIChatToolLoopStream) Result() protocolstage.StreamResult {
	usage := cloneTokenUsage(s.usage)
	model := s.model
	sideEffects := s.sideEffects
	if s.current != nil {
		current := s.current.Result()
		usage = mergeTokenUsage(usage, current.Usage)
		if current.Model != "" {
			model = current.Model
		}
		sideEffects = sideEffects || current.SideEffectsCommitted
	}
	return protocolstage.StreamResult{
		Usage:                usage,
		Model:                model,
		SideEffectsCommitted: sideEffects,
	}
}

func (s *openAIChatToolLoopStream) startRound(ctx context.Context) error {
	stream, err := s.endpoint.next.Stream(ctx, s.call)
	if err != nil {
		return err
	}
	if stream == nil {
		return errors.New("OpenAI Chat ToolLoop received a nil stream")
	}
	streamAssembler, err := assembler.NewStreamAssembler(protocol.TypeOpenAIChat)
	if err != nil {
		_ = stream.Close()
		return err
	}
	s.current = stream
	s.assembler = streamAssembler
	s.round++
	return nil
}

func (s *openAIChatToolLoopStream) absorbCurrentResult() {
	if s.current == nil {
		return
	}
	result := s.current.Result()
	s.usage = mergeTokenUsage(s.usage, result.Usage)
	if result.Model != "" {
		s.model = result.Model
	}
	s.sideEffects = s.sideEffects || result.SideEffectsCommitted
}

func (s *openAIChatToolLoopStream) closeCurrent() error {
	if s.current == nil {
		return nil
	}
	current := s.current
	s.current = nil
	return current.Close()
}

func (s *openAIChatToolLoopStream) fail(err error) error {
	s.done = true
	if s.current != nil {
		s.absorbCurrentResult()
		if closeErr := s.closeCurrent(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}
	return WrapError(err, s.sideEffects)
}

func cloneTokenUsage(usage *protocol.TokenUsage) *protocol.TokenUsage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

// chatStreamEventVisible reports whether an event commits the round to public
// streaming. Content, refusal, or reasoning wins over tool-call data in the
// same event so a mixed visible round can never be consumed internally.
func chatStreamEventVisible(value any) (bool, error) {
	raw, err := chatStreamEventJSON(value)
	if err != nil {
		return false, err
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				Refusal          string `json:"refusal"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &chunk); err != nil {
		return false, fmt.Errorf("classify OpenAI Chat stream event %T: %w", value, err)
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" || choice.Delta.Refusal != "" || choice.Delta.ReasoningContent != "" {
			return true, nil
		}
	}
	return false, nil
}

func chatStreamEventJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, errors.New("OpenAI Chat ToolLoop received a nil stream event")
	}
	switch event := value.(type) {
	case json.RawMessage:
		return event, nil
	case []byte:
		return event, nil
	case wire.ChatStreamChunk:
		return json.Marshal(event)
	case *wire.ChatStreamChunk:
		return json.Marshal(event)
	case interface{ RawJSON() string }:
		if raw := event.RawJSON(); raw != "" {
			return []byte(raw), nil
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAI Chat stream event %T: %w", value, err)
	}
	return raw, nil
}
