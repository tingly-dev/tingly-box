package openaibridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestOpenAIChatToOpenAIResponsesComplete(t *testing.T) {
	t.Parallel()

	terminal := &memoryEndpoint{
		api: protocol.TypeOpenAIResponses,
		complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
			request, ok := call.Request.(*responses.ResponseNewParams)
			if !ok || request == nil {
				t.Fatalf("request type = %T", call.Request)
			}
			if request.Model != "provider-model" {
				t.Fatalf("target call = %#v", request)
			}
			return &stage.Response{
				Value: decodeOpenAIResponsesResponse(t, `{
					"id":"resp_1","object":"response","created_at":123,"model":"provider-model","status":"completed",
					"output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello from responses","annotations":[]}]}],
					"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":0}}
				}`),
			}, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewChatToOpenAIResponses(ResponsesOptions{ResponseModel: "public-model"}))
	result, err := adapted.Complete(context.Background(), stage.Call{
		Request: &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("provider-model"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	chat, ok := result.Value.(wire.ChatCompletionWire)
	if !ok {
		t.Fatalf("response type = %T", result.Value)
	}
	if chat.Model != "public-model" {
		t.Fatalf("response model = %q, want public-model", chat.Model)
	}
	if len(chat.Choices) != 1 || !strings.Contains(chat.Choices[0].Message.Content, "hello from responses") {
		t.Fatalf("choices = %+v", chat.Choices)
	}
	if result.Usage == nil || result.Usage.InputTokens != 7 || result.Usage.CacheReadTokens != 2 || result.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestOpenAIChatToOpenAIResponsesStream(t *testing.T) {
	t.Parallel()

	target := &memoryStream{
		events: openAIResponsesStageEvents(t,
			`{"type":"response.created","sequence_number":0,"response":{"id":"resp_stream","object":"response","model":"provider-model","status":"in_progress","output":[]}}`,
			`{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"stream responses"}`,
			`{"type":"response.output_text.done","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"text":"stream responses"}`,
			`{"type":"response.completed","sequence_number":3,"response":{"id":"resp_stream","object":"response","model":"provider-model","status":"completed","output":[],"usage":{"input_tokens":6,"output_tokens":2,"total_tokens":8,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		),
	}
	terminal := &memoryEndpoint{api: protocol.TypeOpenAIResponses, stream: func(_ context.Context, call stage.Call) (stage.EventStream, error) {
		if _, ok := call.Request.(*responses.ResponseNewParams); !ok {
			t.Fatalf("request type = %T", call.Request)
		}
		return target, nil
	}}
	adapted := mustAdapt(t, terminal, NewChatToOpenAIResponses(ResponsesOptions{ResponseModel: "public-stream-model"}))
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{Model: openai.ChatModel("provider-model")}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var sawText, sawFinal bool
	for {
		event, nextErr := stream.Next(context.Background())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
		chunk, ok := event.Value.(wire.ChatStreamChunk)
		if !ok {
			t.Fatalf("event type = %T", event.Value)
		}
		if chunk.Model != "public-stream-model" {
			t.Fatalf("chunk model = %q, want public-stream-model", chunk.Model)
		}
		if len(chunk.Choices) > 0 && strings.Contains(chunk.Choices[0].Delta.Content, "stream responses") {
			sawText = true
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != nil {
			sawFinal = true
		}
	}
	if !sawText || !sawFinal {
		t.Fatalf("saw text/final = %v/%v", sawText, sawFinal)
	}
	result := stream.Result()
	if result.Usage == nil || result.Usage.InputTokens != 6 || result.Usage.OutputTokens != 2 {
		t.Fatalf("Result() = %+v", result)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if target.closeCount != 1 {
		t.Fatalf("target close count = %d", target.closeCount)
	}
}

// Provider SDK streams are lazy: a failure to open the stream (a 429, say)
// surfaces on the first read. The bridged stream must return that failure
// from its first Next, before any converted chunk, so the caller can still
// answer with the provider's status or fail over.
func TestOpenAIChatToOpenAIResponsesStreamSurfacesOpenFailureFirst(t *testing.T) {
	t.Parallel()

	openFailure := errors.New("429 Too Many Requests")
	target := &memoryStream{err: openFailure}
	terminal := &memoryEndpoint{api: protocol.TypeOpenAIResponses, stream: func(context.Context, stage.Call) (stage.EventStream, error) {
		return target, nil
	}}
	adapted := mustAdapt(t, terminal, NewChatToOpenAIResponses(ResponsesOptions{}))
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{Model: openai.ChatModel("client-model")}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	event, err := stream.Next(context.Background())
	if !errors.Is(err, openFailure) {
		t.Fatalf("first Next = %T %+v, %v; want the open failure before any chunk", event.Value, event.Value, err)
	}
}

func decodeOpenAIResponsesResponse(t *testing.T, raw string) *responses.Response {
	t.Helper()
	var response responses.Response
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatalf("decode Responses response: %v", err)
	}
	return &response
}

func openAIResponsesStageEvents(t *testing.T, values ...string) []stage.Event {
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
