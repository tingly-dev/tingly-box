package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	stagetoolloop "github.com/tingly-dev/tingly-box/internal/protocol/stage/toolloop"
)

// anthropicBetaToolLoopStream buffers one provider round before exposing it.
// Beta permits text/thinking blocks before a later tool_use, so no earlier
// event can prove that the round is safe to expose. Buffering is the only way
// to hide internal tools while preserving one valid Anthropic message stream.
type anthropicBetaToolLoopStream struct {
	endpoint *anthropicBetaToolLoopEndpoint
	call     protocolstage.Call
	owned    map[string]struct{}
	runCtx   context.Context

	round     int
	current   protocolstage.EventStream
	assembler assembler.StreamAssembler
	buffered  []protocolstage.Event
	pending   []protocolstage.Event

	usage       *protocol.TokenUsage
	model       string
	sideEffects bool
	done        bool
	closed      bool
}

func newAnthropicBetaToolLoopStream(
	ctx context.Context,
	endpoint *anthropicBetaToolLoopEndpoint,
	call protocolstage.Call,
	owned map[string]struct{},
) (protocolstage.EventStream, error) {
	stream := &anthropicBetaToolLoopStream{
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

func (s *anthropicBetaToolLoopStream) Next(ctx context.Context) (protocolstage.Event, error) {
	for {
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
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
			s.buffered = append(s.buffered, event)
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

		complete, finishErr := s.assembler.Finish()
		if finishErr != nil {
			return protocolstage.Event{}, s.fail(finishErr)
		}
		message, parseErr := betaStageMessage(complete)
		if parseErr != nil {
			return protocolstage.Event{}, s.fail(parseErr)
		}
		tools, extractErr := s.endpoint.stage.adapter.ExtractTools(message)
		if extractErr != nil {
			return protocolstage.Event{}, s.fail(extractErr)
		}
		managed, external, externalIDs := splitBetaStageTools(tools, s.owned)
		if len(managed) == 0 {
			s.pending = s.buffered
			s.buffered = nil
			s.done = true
			continue
		}
		if len(external) > 0 {
			if s.endpoint.stage.continuations == nil {
				s.pending = s.buffered
				s.buffered = nil
				s.done = true
				continue
			}
			results, nextCtx, committed := s.endpoint.executeTools(s.runCtx, s.call.Request, managed)
			s.sideEffects = s.sideEffects || committed
			s.runCtx = nextCtx
			normalized, normalizeErr := validateAndNormalizeMixedStash(externalIDs, results)
			if normalizeErr != nil {
				return protocolstage.Event{}, s.fail(normalizeErr)
			}
			segmentValue, segmentErr := s.endpoint.stage.adapter.BuildContinuationSegment(message, normalized)
			if segmentErr != nil {
				return protocolstage.Event{}, s.fail(segmentErr)
			}
			segment, ok := segmentValue.([]anthropic.BetaMessageParam)
			if !ok || len(segment) == 0 {
				return protocolstage.Event{}, s.fail(errors.New("Anthropic Beta ToolLoop built an empty mixed continuation"))
			}
			s.endpoint.stage.continuations.Put(s.runCtx, segment)
			filtered, filterErr := filterBetaStageStreamEvents(s.buffered, s.owned)
			if filterErr != nil {
				return protocolstage.Event{}, s.fail(filterErr)
			}
			s.pending = filtered
			s.buffered = nil
			s.done = true
			continue
		}
		if s.round >= s.endpoint.stage.maxRounds {
			return protocolstage.Event{}, s.fail(stagetoolloop.ErrMaxRounds)
		}

		results, nextCtx, committed := s.endpoint.executeTools(s.runCtx, s.call.Request, managed)
		s.sideEffects = s.sideEffects || committed
		s.runCtx = nextCtx
		resultValues := make([]any, len(results))
		for i := range results {
			resultValues[i] = results[i]
		}
		nextRequest, appendErr := s.endpoint.stage.adapter.AppendToolResults(s.call.Request, message, resultValues)
		if appendErr != nil {
			return protocolstage.Event{}, s.fail(appendErr)
		}
		s.call.Request = nextRequest
		s.buffered = nil
		if startErr := s.startRound(s.runCtx); startErr != nil {
			return protocolstage.Event{}, s.fail(startErr)
		}
	}
}

func (s *anthropicBetaToolLoopStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.done = true
	s.absorbCurrentResult()
	return s.closeCurrent()
}

func (s *anthropicBetaToolLoopStream) Result() protocolstage.StreamResult {
	usage := cloneBetaStageUsage(s.usage)
	model := s.model
	sideEffects := s.sideEffects
	if s.current != nil {
		current := s.current.Result()
		usage = mergeBetaStageUsage(usage, current.Usage)
		if current.Model != "" {
			model = current.Model
		}
		sideEffects = sideEffects || current.SideEffectsCommitted
	}
	return protocolstage.StreamResult{Usage: usage, Model: model, SideEffectsCommitted: sideEffects}
}

func (s *anthropicBetaToolLoopStream) startRound(ctx context.Context) error {
	stream, err := s.endpoint.next.Stream(ctx, s.call)
	if err != nil {
		return err
	}
	if stream == nil {
		return errors.New("Anthropic Beta ToolLoop received a nil stream")
	}
	streamAssembler, err := assembler.NewStreamAssembler(protocol.TypeAnthropicBeta)
	if err != nil {
		_ = stream.Close()
		return err
	}
	s.current = stream
	s.assembler = streamAssembler
	s.round++
	return nil
}

func (s *anthropicBetaToolLoopStream) absorbCurrentResult() {
	if s.current == nil {
		return
	}
	result := s.current.Result()
	s.usage = mergeBetaStageUsage(s.usage, result.Usage)
	if result.Model != "" {
		s.model = result.Model
	}
	s.sideEffects = s.sideEffects || result.SideEffectsCommitted
}

func (s *anthropicBetaToolLoopStream) closeCurrent() error {
	if s.current == nil {
		return nil
	}
	current := s.current
	s.current = nil
	return current.Close()
}

func (s *anthropicBetaToolLoopStream) fail(err error) error {
	s.done = true
	if s.current != nil {
		s.absorbCurrentResult()
		if closeErr := s.closeCurrent(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}
	return stagetoolloop.WrapError(err, s.sideEffects)
}

func cloneBetaStageUsage(usage *protocol.TokenUsage) *protocol.TokenUsage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

func filterBetaStageStreamEvents(events []protocolstage.Event, owned map[string]struct{}) ([]protocolstage.Event, error) {
	suppressed := make(map[int]struct{})
	for _, event := range events {
		value, err := betaStageStreamEvent(event.Value)
		if err != nil {
			return nil, err
		}
		tool, ok := NewAnthropicBetaAdapter().ExtractToolFromEvent(value)
		if !ok {
			continue
		}
		if _, internal := owned[tool.Name()]; !internal {
			continue
		}
		if index, ok := extractContentBlockIndex(value); ok {
			suppressed[index] = struct{}{}
		}
	}
	if len(suppressed) == 0 {
		return append([]protocolstage.Event(nil), events...), nil
	}
	indices := make([]int, 0, len(suppressed))
	for index := range suppressed {
		indices = append(indices, index)
	}
	sort.Ints(indices)

	filtered := make([]protocolstage.Event, 0, len(events))
	for _, event := range events {
		value, err := betaStageStreamEvent(event.Value)
		if err != nil {
			return nil, err
		}
		index, indexed := extractContentBlockIndex(value)
		if indexed {
			if _, remove := suppressed[index]; remove {
				continue
			}
			offset := 0
			for _, suppressedIndex := range indices {
				if suppressedIndex < index {
					offset++
				}
			}
			if offset > 0 {
				value, err = rewriteBetaStageEventIndex(value, index-offset)
				if err != nil {
					return nil, err
				}
			}
		}
		filtered = append(filtered, protocolstage.Event{Value: value})
	}
	return filtered, nil
}

func betaStageStreamEvent(value any) (*anthropic.BetaRawMessageStreamEventUnion, error) {
	switch event := value.(type) {
	case *anthropic.BetaRawMessageStreamEventUnion:
		if event == nil {
			return nil, errors.New("Anthropic Beta ToolLoop received a nil stream event")
		}
		return event, nil
	case anthropic.BetaRawMessageStreamEventUnion:
		copy := event
		return &copy, nil
	}
	raw, err := betaStageStreamEventJSON(value)
	if err != nil {
		return nil, err
	}
	var event anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("decode Anthropic Beta ToolLoop stream event %T: %w", value, err)
	}
	return &event, nil
}

func rewriteBetaStageEventIndex(event *anthropic.BetaRawMessageStreamEventUnion, index int) (*anthropic.BetaRawMessageStreamEventUnion, error) {
	raw, err := betaStageStreamEventJSON(event)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	value["index"] = index
	rewritten, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal(rewritten, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func betaStageStreamEventJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, errors.New("Anthropic Beta ToolLoop received a nil stream event")
	}
	switch event := value.(type) {
	case json.RawMessage:
		return event, nil
	case []byte:
		return event, nil
	case interface{ RawJSON() string }:
		if raw := event.RawJSON(); raw != "" {
			return []byte(raw), nil
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal Anthropic Beta ToolLoop stream event %T: %w", value, err)
	}
	return raw, nil
}
