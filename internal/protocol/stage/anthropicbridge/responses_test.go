package anthropicbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

func TestAnthropicBetaToOpenAIResponsesComplete(t *testing.T) {
	t.Parallel()

	terminal := &memoryEndpoint{
		api: protocol.TypeOpenAIResponses,
		complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
			request, ok := call.Request.(*responses.ResponseNewParams)
			if !ok || request == nil {
				t.Fatalf("request type = %T", call.Request)
			}
			if request.Model != "provider-model" || call.State.OpenAIChat != nil {
				t.Fatalf("target call = %#v state=%+v", request, call.State)
			}
			return &stage.Response{
				Value: decodeResponsesResponse(t, `{
					"id":"resp_1","object":"response","model":"provider-model","status":"completed",
					"output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello from responses","annotations":[]}]}],
					"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":0}}
				}`),
				SideEffectsCommitted: true,
			}, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewBetaToOpenAIResponses(ResponsesOptions{ResponseModel: "public-model"}))
	result, err := adapted.Complete(context.Background(), stage.Call{Request: &anthropic.BetaMessageNewParams{
		Model:     "provider-model",
		MaxTokens: 321,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello"))},
	}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	message, ok := result.Value.(*anthropic.BetaMessage)
	if !ok || message == nil {
		t.Fatalf("response type = %T", result.Value)
	}
	if message.Model != "public-model" || len(message.Content) != 1 || !strings.Contains(message.Content[0].Text, "hello from responses") {
		t.Fatalf("message = %#v", message)
	}
	if result.Usage == nil || result.Usage.InputTokens != 7 || result.Usage.CacheInputTokens != 2 || result.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %#v", result.Usage)
	}
	if !result.SideEffectsCommitted {
		t.Fatal("side effects were not preserved")
	}
}

func TestAnthropicBetaToOpenAIResponsesStream(t *testing.T) {
	t.Parallel()

	target := &memoryStream{
		events: responsesStageEvents(t,
			`{"type":"response.created","sequence_number":0,"response":{"id":"resp_stream","object":"response","model":"provider-model","status":"in_progress","output":[]}}`,
			`{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"stream responses"}`,
			`{"type":"response.output_text.done","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"text":"stream responses"}`,
			`{"type":"response.completed","sequence_number":3,"response":{"id":"resp_stream","object":"response","model":"provider-model","status":"completed","output":[],"usage":{"input_tokens":6,"output_tokens":2,"total_tokens":8,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		),
		result: stage.StreamResult{SideEffectsCommitted: true},
	}
	terminal := &memoryEndpoint{api: protocol.TypeOpenAIResponses, stream: func(context.Context, stage.Call) (stage.EventStream, error) {
		return target, nil
	}}
	adapted := mustAdapt(t, terminal, NewBetaToOpenAIResponses(ResponsesOptions{ResponseModel: "public-stream-model"}))
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &anthropic.BetaMessageNewParams{
		Model:     "provider-model",
		MaxTokens: 128,
	}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var eventTypes []string
	var sawText bool
	for {
		event, nextErr := stream.Next(context.Background())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
		anthropicEvent, ok := event.Value.(protocolstream.AnthropicEvent)
		if !ok {
			t.Fatalf("event type = %T", event.Value)
		}
		eventTypes = append(eventTypes, anthropicEvent.Type)
		encoded, _ := json.Marshal(anthropicEvent.Data)
		if anthropicEvent.Type == "content_block_delta" && strings.Contains(string(encoded), "stream responses") {
			sawText = true
		}
	}
	if !sawText || len(eventTypes) == 0 || eventTypes[0] != "message_start" || eventTypes[len(eventTypes)-1] != "message_stop" {
		t.Fatalf("events = %v, saw text = %v", eventTypes, sawText)
	}
	result := stream.Result()
	if result.Model != "public-stream-model" || result.Usage == nil || result.Usage.InputTokens != 6 || result.Usage.OutputTokens != 2 || !result.SideEffectsCommitted {
		t.Fatalf("Result() = %+v", result)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if target.closeCount != 1 {
		t.Fatalf("target close count = %d", target.closeCount)
	}
}

func decodeResponsesResponse(t *testing.T, raw string) *responses.Response {
	t.Helper()
	var response responses.Response
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatalf("decode Responses response: %v", err)
	}
	return &response
}

func responsesStageEvents(t *testing.T, values ...string) []stage.Event {
	t.Helper()
	events := make([]stage.Event, 0, len(values))
	for _, raw := range values {
		var event responses.ResponseStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatalf("decode Responses event: %v", err)
		}
		events = append(events, stage.Event{Value: event})
	}
	return events
}
