package stream

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// TestHandleOpenAIChatToResponsesStream_TextOnly tests the Chat to Responses stream conversion
// with text-only content (no tool calls)
func TestHandleOpenAIChatToResponsesStream_TextOnly(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := "gpt-4o-mini"

	if apiKey == "" {
		t.Skip("Skipping test: OPENAI_API_KEY must be set")
	}

	client := openai.NewClient(openaiOption.WithAPIKey(apiKey))

	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		MaxTokens: openai.Opt[int64](100),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Say 'Hello, World!' in one sentence."),
		},
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	usage, err := HandleOpenAIChatToResponsesStream(protocol.NewHandleContext(c, model), stream, model)
	require.NoError(t, err)

	t.Logf("Usage stats: input=%d, output=%d", usage.InputTokens, usage.OutputTokens)

	body := w.Body.String()
	t.Logf("Response body:\n%s", body)

	events := parseResponsesSSEEvents(t, body)

	createdEvent, ok := events["response.created"]
	require.True(t, ok, "Should have response.created event")
	require.Contains(t, createdEvent, "response")
	response := createdEvent["response"].(map[string]interface{})
	assert.Equal(t, "in_progress", response["status"])

	foundTextDelta := false
	for eventType, eventData := range events {
		if eventType == "response.output_text.delta" {
			foundTextDelta = true
			delta := eventData["delta"].(string)
			t.Logf("Text delta: %s", delta)
		}
	}
	assert.True(t, foundTextDelta, "Should have response.output_text.delta event")

	completedEvent, ok := events["response.completed"]
	require.True(t, ok, "Should have response.completed event")
	completedResponse := completedEvent["response"].(map[string]interface{})
	assert.Equal(t, "completed", completedResponse["status"])

	assert.Contains(t, body, "data: [DONE]")
	assert.Equal(t, "text/event-stream; charset=utf-8", w.Header().Get("Content-Type"))
}

// TestHandleOpenAIChatToResponsesStream_WithToolCalls tests the Chat to Responses stream conversion
// with tool calls
func TestHandleOpenAIChatToResponsesStream_WithToolCalls(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := "gpt-4o-mini"

	if apiKey == "" {
		t.Skip("Skipping test: OPENAI_API_KEY must be set")
	}

	client := openai.NewClient(openaiOption.WithAPIKey(apiKey))

	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		MaxTokens: openai.Opt[int64](100),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("What's the weather like in London, UK?"),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			NewExampleTool(),
		},
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	usage, err := HandleOpenAIChatToResponsesStream(protocol.NewHandleContext(c, model), stream, model)
	require.NoError(t, err)

	t.Logf("Usage stats: input=%d, output=%d", usage.InputTokens, usage.OutputTokens)

	body := w.Body.String()
	t.Logf("Response body:\n%s", body)

	events := parseResponsesSSEEvents(t, body)

	_, ok := events["response.created"]
	require.True(t, ok, "Should have response.created event")

	foundItemAdded := false
	foundArgsDelta := false

	for eventType, eventData := range events {
		switch eventType {
		case "response.output_item.added":
			foundItemAdded = true
			item := eventData["item"].(map[string]interface{})
			assert.Equal(t, "function_call", item["type"])
			t.Logf("Tool call added: name=%s", item["name"])

		case "response.function_call_arguments.delta":
			foundArgsDelta = true
		}
	}

	assert.True(t, foundItemAdded, "Should have response.output_item.added event")
	assert.True(t, foundArgsDelta, "Should have response.function_call_arguments.delta event")

	completedEvent, ok := events["response.completed"]
	require.True(t, ok, "Should have response.completed event")
	completedResponse := completedEvent["response"].(map[string]interface{})
	output := completedResponse["output"].([]interface{})
	assert.NotEmpty(t, output, "Output should not be empty")
}

// TestSendChatToResponsesEvent tests the helper function
func TestSendChatToResponsesEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	eventData := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id":     "resp_123",
			"status": "in_progress",
		},
	}

	OpenAISSE(c, eventData)

	body := w.Body.String()
	assert.Contains(t, body, "data:")
	assert.Contains(t, body, `"type":"response.created"`)
}

// TestResponsesSSEWriter tests the responsesSSEWriter helper.
func TestResponsesSSEWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writer := responsesSSEWriter(c)

	evt := wire.ResponsesCreatedEvent{
		Type:           "response.created",
		SequenceNumber: 1,
		Response: wire.ResponsesWireResponse{
			ID:     "resp_test_123",
			Object: "response",
			Status: "in_progress",
			Output: []wire.ResponsesOutputItemWire{},
		},
	}
	err := writer(evt)
	require.NoError(t, err)

	body := w.Body.String()
	assert.Contains(t, body, `"type":"response.created"`)
	assert.Contains(t, body, `"id":"resp_test_123"`)
}

// TestChatToResponsesConverter_TextDelta tests that the converter emits
// the correct events for a text content delta.
func TestChatToResponsesConverter_TextDelta(t *testing.T) {
	conv := NewChatToResponsesConverter(nil, "gpt-4o-mini")
	// Simulate processing a chunk with text content
	conv.processChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{Delta: openai.ChatCompletionChunkChoiceDelta{Content: "Hello, World!"}},
		},
	})

	// Should emit: response.created, output_item.added, output_text.delta
	require.Len(t, conv.pending, 3)
	assert.Equal(t, "response.created", conv.pending[0].(wire.ResponsesCreatedEvent).Type)
	assert.Equal(t, "response.output_item.added", conv.pending[1].(wire.ResponsesOutputItemAddedEvent).Type)
	assert.Equal(t, "response.output_text.delta", conv.pending[2].(wire.ResponsesOutputTextDeltaEvent).Type)

	delta := conv.pending[2].(wire.ResponsesOutputTextDeltaEvent)
	assert.Equal(t, "Hello, World!", delta.Delta)
}

// TestChatToResponsesConverter_ToolCall tests that the converter emits
// tool call events correctly.
func TestChatToResponsesConverter_ToolCall(t *testing.T) {
	conv := NewChatToResponsesConverter(nil, "gpt-4o-mini")
	conv.hasSentCreated = true // skip created event

	conv.processChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{Delta: openai.ChatCompletionChunkChoiceDelta{
				ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
					{
						Index:    0,
						ID:       "call_123",
						Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: "get_weather", Arguments: `{"loc`},
					},
				},
			}},
		},
	})

	require.Len(t, conv.pending, 2)
	added := conv.pending[0].(wire.ResponsesOutputItemAddedEvent)
	assert.Equal(t, "response.output_item.added", added.Type)
	assert.Equal(t, "get_weather", added.Item.Name)

	argsDelta := conv.pending[1].(wire.ResponsesFunctionCallArgumentsDeltaEvent)
	assert.Equal(t, "response.function_call_arguments.delta", argsDelta.Type)
	assert.Equal(t, `{"loc`, argsDelta.Delta)
}

// TestChatToResponsesConverter_CompletedEvent tests usage propagation
// into the response.completed event.
func TestChatToResponsesConverter_CompletedEvent(t *testing.T) {
	conv := NewChatToResponsesConverter(nil, "gpt-4o-mini")
	conv.hasSentCreated = true
	conv.inputTokens = 10
	conv.outputTokens = 20
	conv.hasUsage = true

	conv.processChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{FinishReason: "stop"},
		},
	})

	// Should emit response.completed
	var completed *wire.ResponsesCompletedEvent
	for _, evt := range conv.pending {
		if ce, ok := evt.(wire.ResponsesCompletedEvent); ok {
			completed = &ce
		}
	}
	require.NotNil(t, completed, "should emit response.completed")
	assert.Equal(t, "completed", completed.Response.Status)
	assert.Equal(t, int64(10), completed.Response.Usage.InputTokens)
	assert.Equal(t, int64(20), completed.Response.Usage.OutputTokens)
}

// TestChatToResponsesConverter_WithReasoningTokens verifies reasoning_tokens
// are propagated into the wire response's output_tokens_details.
func TestChatToResponsesConverter_WithReasoningTokens(t *testing.T) {
	conv := NewChatToResponsesConverter(nil, "o3-mini")
	conv.hasSentCreated = true
	conv.inputTokens = 50
	conv.outputTokens = 30
	conv.cacheTokens = 5
	conv.reasoningTokens = 12
	conv.hasUsage = true

	conv.processChunk(&openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{FinishReason: "stop"},
		},
	})

	var completed *wire.ResponsesCompletedEvent
	for _, evt := range conv.pending {
		if ce, ok := evt.(wire.ResponsesCompletedEvent); ok {
			completed = &ce
		}
	}
	require.NotNil(t, completed)

	usage := completed.Response.Usage
	require.NotNil(t, usage)
	assert.Equal(t, int64(50), usage.InputTokens)
	assert.Equal(t, int64(30), usage.OutputTokens)
	assert.Equal(t, int64(80), usage.TotalTokens)
	assert.Equal(t, int64(5), usage.InputTokensDetails.CachedTokens)
	assert.Equal(t, int64(12), usage.OutputTokensDetails.ReasoningTokens)
}

// TestChatStreamUsage_DetailFields verifies that wire.ChatStreamUsage
// serialises prompt_tokens_details and completion_tokens_details when non-nil.
func TestChatStreamUsage_DetailFields(t *testing.T) {
	usage := wire.ChatStreamUsage{
		PromptTokens:     100,
		CompletionTokens: 40,
		TotalTokens:      140,
		PromptTokensDetails: &wire.ChatStreamPromptTokenDetails{
			CachedTokens: 20,
		},
		CompletionTokensDetails: &wire.ChatStreamOutputTokenDetails{
			ReasoningTokens: 15,
		},
	}

	data, err := json.Marshal(usage)
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &out))

	assert.Equal(t, float64(100), out["prompt_tokens"])
	assert.Equal(t, float64(40), out["completion_tokens"])
	assert.Equal(t, float64(140), out["total_tokens"])

	pt := out["prompt_tokens_details"].(map[string]interface{})
	assert.Equal(t, float64(20), pt["cached_tokens"])

	ct := out["completion_tokens_details"].(map[string]interface{})
	assert.Equal(t, float64(15), ct["reasoning_tokens"])
}

// TestChatStreamUsage_NilDetails verifies that nil detail fields are omitted from JSON.
func TestChatStreamUsage_NilDetails(t *testing.T) {
	usage := wire.ChatStreamUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
	}
	data, err := json.Marshal(usage)
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &out))
	assert.NotContains(t, out, "prompt_tokens_details")
	assert.NotContains(t, out, "completion_tokens_details")
}

// parseResponsesSSEEvents parses SSE response body into a map of events
func parseResponsesSSEEvents(t *testing.T, body string) map[string]map[string]interface{} {
	events := make(map[string]map[string]interface{})
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))

			if data == "[DONE]" {
				continue
			}

			if data != "" {
				var eventData map[string]interface{}
				err := json.Unmarshal([]byte(data), &eventData)
				require.NoError(t, err, "SSE data should be valid JSON")

				eventType, ok := eventData["type"].(string)
				require.True(t, ok, "Event should have a type field")

				events[eventType] = eventData
			}
		}
	}

	return events
}
