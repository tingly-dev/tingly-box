package stream

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestResponsesWireResponseJSONIsMinimalComparedToSDKResponse(t *testing.T) {
	conv := NewChatToResponsesConverter(nil, "")
	conv.responseID = "resp_test"
	conv.createdAt = 123
	conv.inputTokens = 10
	conv.outputTokens = 5
	conv.cacheTokens = 3
	conv.reasoningTokens = 2

	wireEvent := wire.ResponsesCreatedEvent{
		Type:           "response.created",
		SequenceNumber: 1,
		Response:       conv.wireResponse("in_progress", nil),
	}
	wireJSON := marshalJSONForTest(t, wireEvent)

	sdkEvent := responses.ResponseCreatedEvent{
		SequenceNumber: 1,
		Response: responses.Response{
			ID:        "resp_test",
			CreatedAt: 123,
			Status:    responses.ResponseStatusInProgress,
			Output:    []responses.ResponseOutputItemUnion{},
			Usage: responses.ResponseUsage{
				InputTokens:  10,
				OutputTokens: 5,
				TotalTokens:  15,
				InputTokensDetails: responses.ResponseUsageInputTokensDetails{
					CachedTokens: 3,
				},
				OutputTokensDetails: responses.ResponseUsageOutputTokensDetails{
					ReasoningTokens: 2,
				},
			},
		},
	}
	sdkJSON := marshalJSONForTest(t, sdkEvent)

	var wireRoot map[string]any
	var sdkRoot map[string]any
	require.NoError(t, json.Unmarshal([]byte(wireJSON), &wireRoot))
	require.NoError(t, json.Unmarshal([]byte(sdkJSON), &sdkRoot))

	wireResponse := wireRoot["response"].(map[string]any)
	sdkResponse := sdkRoot["response"].(map[string]any)

	require.NotContains(t, wireResponse, "error")
	require.NotContains(t, wireResponse, "metadata")
	require.NotContains(t, wireResponse, "parallel_tool_calls")
	require.NotContains(t, wireResponse, "temperature")
	require.NotContains(t, wireResponse, "tools")
	require.NotContains(t, wireResponse, "top_p")

	require.Contains(t, sdkResponse, "error")
	require.Contains(t, sdkResponse, "metadata")
	require.Contains(t, sdkResponse, "parallel_tool_calls")
	require.Contains(t, sdkResponse, "temperature")
	require.Contains(t, sdkResponse, "tools")
	require.Contains(t, sdkResponse, "top_p")
	require.Greater(t, len(sdkResponse), len(wireResponse))

	// Verify reasoning_tokens is propagated in wire response
	wireUsage := wireResponse["usage"].(map[string]any)
	wireOutputDetails := wireUsage["output_tokens_details"].(map[string]any)
	require.Equal(t, float64(2), wireOutputDetails["reasoning_tokens"])
	wireInputDetails := wireUsage["input_tokens_details"].(map[string]any)
	require.Equal(t, float64(3), wireInputDetails["cached_tokens"])
}

func TestResponsesWireFunctionCallPreservesEmptyArguments(t *testing.T) {
	arguments := ""
	event := wire.ResponsesOutputItemAddedEvent{
		Type:           "response.output_item.added",
		SequenceNumber: 2,
		OutputIndex:    1,
		Item: wire.ResponsesOutputItemWire{
			ID:        "call_1",
			CallID:    "call_1",
			Type:      "function_call",
			Name:      "get_weather",
			Arguments: &arguments,
			Status:    "in_progress",
		},
	}

	require.JSONEq(t, `{
		"type":"response.output_item.added",
		"sequence_number":2,
		"output_index":1,
		"item":{
			"id":"call_1",
			"call_id":"call_1",
			"type":"function_call",
			"name":"get_weather",
			"arguments":"",
			"status":"in_progress"
		}
	}`, marshalJSONForTest(t, event))
}

func marshalJSONForTest(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}
