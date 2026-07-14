package anthropicbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

func TestV1ToBetaCompletePreservesWireAndFacts(t *testing.T) {
	t.Parallel()

	usage := protocol.NewTokenUsage(9, 4)
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
			beta, ok := call.Request.(*anthropic.BetaMessageNewParams)
			if !ok || beta == nil {
				t.Fatalf("request type = %T", call.Request)
			}
			if call.Metadata.RequestID != "v1-beta" || beta.Model != "claude-provider" || len(beta.Tools) != 1 {
				t.Fatalf("target call = %#v metadata=%+v", beta, call.Metadata)
			}
			return &stage.Response{
				Value: decodeV1BetaMessage(t, `{
					"id":"msg_1","type":"message","role":"assistant","model":"claude-provider",
					"content":[{"type":"tool_use","id":"tool-1","name":"lookup","input":{"city":"Paris"}}],
					"stop_reason":"tool_use","stop_sequence":null,
					"usage":{"input_tokens":9,"output_tokens":4}
				}`),
				Usage: usage, Model: "claude-provider", SideEffectsCommitted: true,
			}, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewV1ToBeta())
	v1 := decodeV1BetaRequest(t, `{
		"model":"claude-provider","max_tokens":128,
		"messages":[{"role":"user","content":[{"type":"text","text":"weather"}]}],
		"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`)
	response, err := adapted.Complete(context.Background(), stage.Call{
		Request: v1, Metadata: stage.CallMetadata{RequestID: "v1-beta", Attempt: 2},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	message, ok := response.Value.(*anthropic.Message)
	if !ok || message == nil {
		t.Fatalf("response type = %T", response.Value)
	}
	if message.StopReason != "tool_use" || len(message.Content) != 1 || message.Content[0].Type != "tool_use" {
		t.Fatalf("response = %#v", message)
	}
	if response.Usage != usage || response.Model != "claude-provider" || !response.SideEffectsCommitted {
		t.Fatalf("response facts = %+v", response)
	}
}

func TestV1ToBetaStreamConvertsLifecycleAndOwnsTarget(t *testing.T) {
	t.Parallel()

	wires := []string{
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-provider","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	}
	events := make([]stage.Event, 0, len(wires))
	for i, wire := range wires {
		value := any(decodeV1BetaEvent(t, wire))
		if i%2 == 0 {
			value = json.RawMessage(wire)
		}
		events = append(events, stage.Event{Value: value})
	}
	usage := protocol.NewTokenUsage(5, 2)
	target := &memoryStream{
		events: events,
		result: stage.StreamResult{Usage: usage, Model: "claude-provider", SideEffectsCommitted: true},
	}
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(_ context.Context, call stage.Call) (stage.EventStream, error) {
			if _, ok := call.Request.(*anthropic.BetaMessageNewParams); !ok {
				t.Fatalf("request type = %T", call.Request)
			}
			return target, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewV1ToBeta())
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &anthropic.MessageNewParams{Model: "claude-provider", MaxTokens: 32}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	for i, wire := range wires {
		event, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next(%d) error = %v", i, err)
		}
		v1, ok := event.Value.(anthropic.MessageStreamEventUnion)
		if !ok {
			t.Fatalf("event %d type = %T", i, event.Value)
		}
		beta := decodeV1BetaEvent(t, wire)
		if v1.Type != beta.Type || v1.Index != beta.Index {
			t.Fatalf("event %d identity = (%q,%d), want (%q,%d)", i, v1.Type, v1.Index, beta.Type, beta.Index)
		}
		switch v1.Type {
		case "message_start":
			if v1.Message.ID != "msg_1" || v1.Message.Model != "claude-provider" || v1.Message.Usage.InputTokens != 5 {
				t.Fatalf("message_start = %#v", v1.Message)
			}
		case "content_block_start":
			if v1.ContentBlock.Type != "text" {
				t.Fatalf("content_block_start = %#v", v1.ContentBlock)
			}
		case "content_block_delta":
			if v1.Delta.Type != "text_delta" || v1.Delta.Text != "hello" {
				t.Fatalf("content_block_delta = %#v", v1.Delta)
			}
		case "message_delta":
			if v1.Delta.StopReason != "end_turn" || v1.Usage.OutputTokens != 2 {
				t.Fatalf("message_delta = delta=%#v usage=%#v", v1.Delta, v1.Usage)
			}
		}
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final Next() error = %v", err)
	}
	if got := stream.Result(); got.Usage != usage || got.Model != "claude-provider" || !got.SideEffectsCommitted {
		t.Fatalf("Result() = %+v", got)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if target.closeCount != 1 {
		t.Fatalf("target close count = %d, want 1", target.closeCount)
	}
}

func TestV1ToBetaRejectsWrongTypes(t *testing.T) {
	t.Parallel()

	bridge := NewV1ToBeta()
	if _, err := bridge.Open(context.Background(), stage.Call{Request: &anthropic.BetaMessageNewParams{}}, stage.OperationComplete); err == nil {
		t.Fatal("Open() accepted a Beta request on the v1 side")
	}

	session, err := bridge.Open(context.Background(), stage.Call{Request: &anthropic.MessageNewParams{}}, stage.OperationComplete)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := session.ConvertComplete(context.Background(), &stage.Response{Value: &anthropic.Message{}}); err == nil {
		t.Fatal("ConvertComplete() accepted a v1 response on the Beta side")
	}
}

func decodeV1BetaRequest(t *testing.T, raw string) *anthropic.MessageNewParams {
	t.Helper()
	var value anthropic.MessageNewParams
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("decode v1 request: %v", err)
	}
	return &value
}

func decodeV1BetaMessage(t *testing.T, raw string) *anthropic.BetaMessage {
	t.Helper()
	var value anthropic.BetaMessage
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("decode Beta message: %v", err)
	}
	return &value
}

func decodeV1BetaEvent(t *testing.T, raw string) anthropic.BetaRawMessageStreamEventUnion {
	t.Helper()
	var value anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("decode Beta event: %v", err)
	}
	return value
}
