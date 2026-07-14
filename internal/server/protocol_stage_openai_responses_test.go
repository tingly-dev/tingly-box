package server

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestProtocolStageOpenAIResponsesJSONPreservesRawWireFields(t *testing.T) {
	t.Parallel()

	raw := `{"id":"resp_1","object":"response","model":"upstream-model","output":[],"usage":{"input_tokens":3,"output_tokens":2},"provider_extension":{"kept":true}}`
	var response responses.Response
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	body, err := protocolStageOpenAIResponsesJSON(&response, "public-model")
	if err != nil {
		t.Fatalf("protocolStageOpenAIResponsesJSON: %v", err)
	}
	if got := gjson.GetBytes(body, "model").String(); got != "public-model" {
		t.Fatalf("model = %q, want public-model", got)
	}
	if !gjson.GetBytes(body, "provider_extension.kept").Bool() {
		t.Fatalf("provider extension was not preserved: %s", body)
	}
}

func TestProtocolStageOpenAIResponsesValueJSONAcceptsTypedWireResponse(t *testing.T) {
	t.Parallel()

	body, err := protocolStageOpenAIResponsesValueJSON(wire.ResponsesWireResponse{
		ID:     "resp_typed",
		Object: "response",
		Model:  "provider-model",
		Status: "completed",
		Output: []wire.ResponsesOutputItemWire{{
			ID: "msg_1", Type: "message", Role: "assistant", Status: "completed",
			Content: []wire.ResponsesContentPartWire{{Type: "output_text", Text: "typed response"}},
		}},
		Usage: &wire.ResponsesUsageWire{InputTokens: 3, OutputTokens: 2, TotalTokens: 5},
	}, "public-model")
	if err != nil {
		t.Fatalf("protocolStageOpenAIResponsesValueJSON: %v", err)
	}
	if got := gjson.GetBytes(body, "model").String(); got != "public-model" {
		t.Fatalf("model = %q, want public-model", got)
	}
	if got := gjson.GetBytes(body, "output.0.content.0.text").String(); got != "typed response" {
		t.Fatalf("text = %q", got)
	}
	if !gjson.GetBytes(body, "usage.input_tokens_details.cached_tokens").Exists() ||
		!gjson.GetBytes(body, "usage.output_tokens_details.reasoning_tokens").Exists() {
		t.Fatalf("strict usage details missing: %s", body)
	}
}

func TestProtocolStageOpenAIResponsesEventJSONPreservesWireShape(t *testing.T) {
	t.Parallel()

	raw := `{"type":"response.completed","sequence_number":4,"response":{"id":"resp_1","object":"response","model":"upstream-model","output":[],"usage":{"input_tokens":9,"output_tokens":4,"input_tokens_details":{},"output_tokens_details":{}},"provider_extension":"kept"}}`
	var event responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	eventType, body, err := protocolStageOpenAIResponsesEventJSON(event, "public-model")
	if err != nil {
		t.Fatalf("protocolStageOpenAIResponsesEventJSON: %v", err)
	}
	if eventType != "response.completed" {
		t.Fatalf("event type = %q", eventType)
	}
	if got := gjson.GetBytes(body, "response.model").String(); got != "public-model" {
		t.Fatalf("model = %q, want public-model", got)
	}
	if got := gjson.GetBytes(body, "response.provider_extension").String(); got != "kept" {
		t.Fatalf("provider extension = %q", got)
	}
	if !gjson.GetBytes(body, "response.usage.input_tokens_details.cached_tokens").Exists() {
		t.Fatalf("cached_tokens was not backfilled: %s", body)
	}
	if !gjson.GetBytes(body, "response.usage.output_tokens_details.reasoning_tokens").Exists() {
		t.Fatalf("reasoning_tokens was not backfilled: %s", body)
	}

	usage := protocolStageOpenAIResponsesUsage(body)
	if usage == nil || usage.InputTokens != 9 || usage.OutputTokens != 4 {
		t.Fatalf("usage = %#v", usage)
	}
}
