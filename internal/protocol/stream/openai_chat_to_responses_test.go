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

	usage, err := HandleOpenAIChatToResponsesStream(c, stream, model)
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
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
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

	usage, err := HandleOpenAIChatToResponsesStream(c, stream, model)
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

	sendChatToResponsesEvent(c, eventData, w)

	body := w.Body.String()
	assert.Contains(t, body, "data:")
	assert.Contains(t, body, `"type":"response.created"`)
}

// TestSendResponsesCreatedEvent tests the response.created event helper
func TestSendResponsesCreatedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := &chatToResponsesState{
		responseID: "resp_test_123",
		createdAt:  1,
	}

	sendResponsesCreatedEvent(c, state, w)

	body := w.Body.String()
	assert.Contains(t, body, `"type":"response.created"`)
	assert.Contains(t, body, `"id":"resp_test_123"`)
}

// TestSendResponsesOutputTextDelta tests the output_text.delta event helper
func TestSendResponsesOutputTextDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := &chatToResponsesState{
		textItemID: "msg_test_1",
	}
	sendResponsesOutputTextDelta(c, state, "Hello, World!", w)

	body := w.Body.String()
	assert.Contains(t, body, `"type":"response.output_text.delta"`)
	assert.Contains(t, body, `"delta":"Hello, World!"`)
}

// TestSendResponsesOutputItemAdded tests the output_item.added event helper
func TestSendResponsesOutputItemAdded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := &chatToResponsesState{}
	sendResponsesOutputItemAdded(c, state, "fc_123", "call_123", "get_weather", 1, w)

	body := w.Body.String()
	assert.Contains(t, body, `"type":"response.output_item.added"`)
	assert.Contains(t, body, `"name":"get_weather"`)
}

// TestSendResponsesFunctionCallArgumentsDelta tests the function_call_arguments.delta event helper
func TestSendResponsesFunctionCallArgumentsDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := &chatToResponsesState{}
	sendResponsesFunctionCallArgumentsDelta(c, state, "fc_123", 1, `{"location":"London"}`, w)

	body := w.Body.String()
	assert.Contains(t, body, `"type":"response.function_call_arguments.delta"`)
	assert.Contains(t, body, `"item_id":"fc_123"`)
}

// TestSendResponsesCompletedEvent tests the response.completed event helper
func TestSendResponsesCompletedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := &chatToResponsesState{
		responseID:      "resp_test_123",
		createdAt:       1,
		textItemID:      "msg_test_1",
		accumulatedText: strings.Builder{},
		pendingToolCalls: map[int]*pendingToolCallResponse{
			0: {
				itemID:    "fc_123",
				callID:    "call_123",
				outputIdx: 1,
				name:      "get_weather",
				arguments: strings.Builder{},
			},
		},
		inputTokens:  10,
		outputTokens: 20,
	}

	sendResponsesCompletedEvent(c, state, "gpt-4o-mini", "stop", w)

	body := w.Body.String()
	assert.Contains(t, body, `"type":"response.completed"`)
	assert.Contains(t, body, `"status":"completed"`)
	assert.Contains(t, body, `"input_tokens":10`)
	assert.Contains(t, body, `"output_tokens":20`)
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

// TestChatToResponsesState tests the state struct
func TestChatToResponsesState(t *testing.T) {
	state := &chatToResponsesState{
		responseID:       "resp_test",
		outputIndex:      0,
		pendingToolCalls: make(map[int]*pendingToolCallResponse),
		inputTokens:      100,
		outputTokens:     200,
		hasSentCreated:   false,
	}

	assert.Equal(t, "resp_test", state.responseID)
	assert.Equal(t, 0, state.outputIndex)
	assert.NotNil(t, state.pendingToolCalls)
	assert.Equal(t, int64(100), state.inputTokens)
	assert.Equal(t, int64(200), state.outputTokens)
	assert.False(t, state.hasSentCreated)

	state.pendingToolCalls[0] = &pendingToolCallResponse{
		itemID:    "fc_1",
		name:      "test_func",
		arguments: strings.Builder{},
	}

	assert.Equal(t, 1, len(state.pendingToolCalls))
	assert.Equal(t, "fc_1", state.pendingToolCalls[0].itemID)
	assert.Equal(t, "test_func", state.pendingToolCalls[0].name)
}
