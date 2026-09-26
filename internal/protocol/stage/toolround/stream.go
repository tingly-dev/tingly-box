package toolround

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

func (e *endpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	request, err := e.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	s := &roundStream{endpoint: e, ctx: ctx, call: call, request: request}
	if err := s.startRound(); err != nil {
		return nil, err
	}
	return s, nil
}

// heldBlock is a tool_use block withheld until its round is decided.
type heldBlock struct {
	index  int64
	events []anthropic.BetaRawMessageStreamEventUnion
	call   ToolCall
	input  strings.Builder
}

// roundStream presents every round of one request as a single client
// message: message_start once, content blocks renumbered contiguously, and
// the final round's message_delta and message_stop.
type roundStream struct {
	endpoint *endpoint
	ctx      context.Context // execution context, threaded through owned calls
	call     stage.Call
	request  *anthropic.BetaMessageNewParams

	current        stage.EventStream
	queue          []stage.Event
	done           bool
	started        bool
	nextIndex      int64
	executedRounds int
	committed      bool
	usage          *protocol.TokenUsage
	model          string

	// Per round.
	message anthropic.BetaMessage
	live    map[int64]int64 // upstream index -> client index
	held    []*heldBlock
	delta   *anthropic.BetaRawMessageStreamEventUnion
	stop    *anthropic.BetaRawMessageStreamEventUnion

	closeOnce sync.Once
	closeErr  error
}

func (s *roundStream) startRound() error {
	current, err := s.endpoint.next.Stream(s.ctx, stage.Call{Request: s.request, Metadata: s.call.Metadata, State: s.call.State})
	if err != nil {
		return stage.WrapCommitted(err, s.committed)
	}
	s.current = current
	s.message = anthropic.BetaMessage{}
	s.live = map[int64]int64{}
	s.held = nil
	s.delta, s.stop = nil, nil
	return nil
}

func (s *roundStream) Next(ctx context.Context) (stage.Event, error) {
	for {
		if len(s.queue) > 0 {
			event := s.queue[0]
			s.queue = s.queue[1:]
			return event, nil
		}
		if s.done {
			return stage.Event{}, io.EOF
		}
		if err := ctx.Err(); err != nil {
			return stage.Event{}, err
		}
		event, err := s.current.Next(ctx)
		if errors.Is(err, io.EOF) {
			if err := s.finishRound(ctx); err != nil {
				return stage.Event{}, err
			}
			continue
		}
		if err != nil {
			return stage.Event{}, stage.WrapCommitted(err, s.committed)
		}
		if err := s.consume(event.Value); err != nil {
			return stage.Event{}, stage.WrapCommitted(err, s.committed)
		}
	}
}

func (s *roundStream) consume(value any) error {
	event, err := betaEvent(value)
	if err != nil {
		return err
	}
	if err := s.message.Accumulate(event); err != nil {
		return fmt.Errorf("tool round: accumulate %s: %w", event.Type, err)
	}
	switch event.Type {
	case "message_start":
		if !s.started {
			s.started = true
			s.emit(event)
		}
	case "content_block_start":
		if event.ContentBlock.Type == "tool_use" {
			s.held = append(s.held, &heldBlock{
				index:  event.Index,
				events: []anthropic.BetaRawMessageStreamEventUnion{event},
				call:   ToolCall{ID: event.ContentBlock.ID, Name: event.ContentBlock.Name},
			})
			return nil
		}
		s.live[event.Index] = s.nextIndex
		s.nextIndex++
		return s.emitAt(event, s.live[event.Index])
	case "content_block_delta", "content_block_stop":
		if block := s.heldBlock(event.Index); block != nil {
			block.events = append(block.events, event)
			if event.Type == "content_block_delta" && event.Delta.Type == "input_json_delta" {
				block.input.WriteString(event.Delta.PartialJSON)
			}
			return nil
		}
		index, ok := s.live[event.Index]
		if !ok {
			return fmt.Errorf("tool round: %s for unknown block %d", event.Type, event.Index)
		}
		return s.emitAt(event, index)
	case "message_delta":
		s.delta = &event
	case "message_stop":
		s.stop = &event
	default:
		s.emit(event)
	}
	return nil
}

func (s *roundStream) heldBlock(index int64) *heldBlock {
	for _, block := range s.held {
		if block.index == index {
			return block
		}
	}
	return nil
}

// finishRound decides the round that just ended: another round, or the
// client's view of this one followed by the end of the message.
func (s *roundStream) finishRound(ctx context.Context) error {
	s.absorb()
	e := s.endpoint
	calls := make([]ToolCall, len(s.held))
	for i, block := range s.held {
		block.call.Input = json.RawMessage(block.input.String())
		if len(block.call.Input) == 0 {
			block.call.Input = json.RawMessage("{}")
		}
		calls[i] = block.call
	}

	p := e.decide(s.ctx, s.request, calls, s.executedRounds)
	if len(p.execute) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		var results []anthropic.BetaToolResultBlockParam
		s.ctx, results = e.execute(s.ctx, s.request, p.execute)
		s.committed = true
		s.executedRounds++
		turn := s.message.ToParam()
		if p.continueLoop {
			s.request = continued(s.request, turn, results)
			return s.startRound()
		}
		e.config.Owner.Suspend(s.ctx, turn, results)
	}

	for _, block := range s.held {
		if err := s.emitHeld(block, p); err != nil {
			return stage.WrapCommitted(err, s.committed)
		}
	}
	if err := s.emitEnd(p.stopReason); err != nil {
		return stage.WrapCommitted(err, s.committed)
	}
	s.done = true
	return nil
}

func (s *roundStream) emitHeld(block *heldBlock, p plan) error {
	if message, ok := p.blocked[block.call.ID]; ok {
		index := s.take()
		for _, raw := range []string{
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			mustSet(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`, "delta.text", message),
			`{"type":"content_block_stop","index":0}`,
		} {
			if err := s.emitRaw(raw, index); err != nil {
				return err
			}
		}
		return nil
	}
	if !p.keep[block.call.ID] {
		return nil
	}
	index := s.take()
	input := block.call.Input
	if gate := s.endpoint.config.Gate; gate != nil {
		if restored := gate.Restore(input); len(restored) > 0 && string(restored) != string(input) {
			// Aliases were restored: send the real input as one delta.
			if err := s.emitAt(block.events[0], index); err != nil {
				return err
			}
			if err := s.emitRaw(mustSet(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`, "delta.partial_json", string(restored)), index); err != nil {
				return err
			}
			return s.emitRaw(`{"type":"content_block_stop","index":0}`, index)
		}
	}
	for _, event := range block.events {
		if err := s.emitAt(event, index); err != nil {
			return err
		}
	}
	return nil
}

func (s *roundStream) emitEnd(stopReason string) error {
	if s.delta != nil {
		delta := *s.delta
		if stopReason != "" && string(delta.Delta.StopReason) != stopReason {
			raw, err := sjson.Set(rawEvent(delta), "delta.stop_reason", stopReason)
			if err != nil {
				return err
			}
			if delta, err = betaEvent(json.RawMessage(raw)); err != nil {
				return err
			}
		}
		s.emit(delta)
	}
	if s.stop != nil {
		s.emit(*s.stop)
	}
	return nil
}

func (s *roundStream) take() int64 {
	index := s.nextIndex
	s.nextIndex++
	return index
}

func (s *roundStream) emit(event anthropic.BetaRawMessageStreamEventUnion) {
	s.queue = append(s.queue, stage.Event{Value: event})
}

func (s *roundStream) emitAt(event anthropic.BetaRawMessageStreamEventUnion, index int64) error {
	if event.Index == index {
		s.emit(event)
		return nil
	}
	return s.emitRaw(rawEvent(event), index)
}

func (s *roundStream) emitRaw(raw string, index int64) error {
	raw, err := sjson.Set(raw, "index", index)
	if err != nil {
		return err
	}
	event, err := betaEvent(json.RawMessage(raw))
	if err != nil {
		return err
	}
	s.emit(event)
	return nil
}

// absorb closes the finished round and folds its summary into the request's.
func (s *roundStream) absorb() {
	if s.current == nil {
		return
	}
	result := s.current.Result()
	s.usage = addUsage(s.usage, result.Usage)
	if result.Model != "" {
		s.model = result.Model
	}
	s.committed = s.committed || result.SideEffectsCommitted
	_ = s.current.Close()
	s.current = nil
}

func (s *roundStream) Close() error {
	s.closeOnce.Do(func() {
		if s.current != nil {
			s.closeErr = s.current.Close()
		}
	})
	return s.closeErr
}

func (s *roundStream) Result() stage.StreamResult {
	usage, model, committed := s.usage, s.model, s.committed
	if s.current != nil {
		result := s.current.Result()
		usage = addUsage(usage, result.Usage)
		if result.Model != "" {
			model = result.Model
		}
		committed = committed || result.SideEffectsCommitted
	}
	return stage.StreamResult{Usage: usage, Model: model, SideEffectsCommitted: committed}
}

// betaEvent normalizes an inner event to the Beta union. Bridges may emit
// carriers (such as stream.AnthropicEvent) that expose their wire payload
// through RawJSON.
func betaEvent(value any) (anthropic.BetaRawMessageStreamEventUnion, error) {
	switch event := value.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		return event, nil
	case *anthropic.BetaRawMessageStreamEventUnion:
		if event != nil {
			return *event, nil
		}
	case json.RawMessage:
		var out anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal(event, &out); err != nil {
			return out, fmt.Errorf("tool round: decode event: %w", err)
		}
		return out, nil
	case interface{ RawJSON() string }:
		if raw := event.RawJSON(); raw != "" {
			return betaEvent(json.RawMessage(raw))
		}
	}
	return anthropic.BetaRawMessageStreamEventUnion{}, fmt.Errorf("tool round: event has type %T, want anthropic.BetaRawMessageStreamEventUnion", value)
}

func rawEvent(event anthropic.BetaRawMessageStreamEventUnion) string {
	if raw := event.RawJSON(); raw != "" {
		return raw
	}
	data, _ := json.Marshal(event)
	return string(data)
}

func mustSet(raw, path, value string) string {
	out, err := sjson.Set(raw, path, value)
	if err != nil {
		panic(err)
	}
	return out
}
