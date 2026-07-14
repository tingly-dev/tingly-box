package request

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestConvertAnthropicV1ToBetaRequestPreservesWireSubset(t *testing.T) {
	raw := []byte(`{
		"model":"claude-sonnet",
		"max_tokens":1024,
		"metadata":{"user_id":"user-1"},
		"system":[{"type":"text","text":"system","cache_control":{"type":"ephemeral"}}],
		"messages":[
			{"role":"user","content":[
				{"type":"text","text":"look"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}
			]},
			{"role":"assistant","content":[{"type":"tool_use","id":"tool-1","name":"lookup","input":{"city":"Paris"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":[{"type":"text","text":"sunny"}]}]}
		],
		"tools":[{"name":"lookup","description":"weather lookup","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}],
		"tool_choice":{"type":"tool","name":"lookup","disable_parallel_tool_use":true},
		"stop_sequences":["done"],
		"temperature":0.2,
		"top_k":20,
		"top_p":0.8,
		"thinking":{"type":"enabled","budget_tokens":256}
	}`)

	var v1 anthropic.MessageNewParams
	if err := json.Unmarshal(raw, &v1); err != nil {
		t.Fatalf("decode v1 request: %v", err)
	}
	beta, err := ConvertAnthropicV1ToBetaRequestWithError(&v1)
	if err != nil {
		t.Fatalf("ConvertAnthropicV1ToBetaRequestWithError() error = %v", err)
	}

	want := decodeJSONObject(t, mustMarshalJSON(t, &v1))
	got := decodeJSONObject(t, mustMarshalJSON(t, beta))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Beta wire request differs from v1 subset\n got: %#v\nwant: %#v", got, want)
	}
}

func TestConvertAnthropicV1ToBetaRequestNil(t *testing.T) {
	if got := ConvertAnthropicV1ToBetaRequest(nil); got != nil {
		t.Fatalf("ConvertAnthropicV1ToBetaRequest(nil) = %#v, want nil", got)
	}
	got, err := ConvertAnthropicV1ToBetaRequestWithError(nil)
	if err != nil || got != nil {
		t.Fatalf("ConvertAnthropicV1ToBetaRequestWithError(nil) = %#v, %v", got, err)
	}
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", value, err)
	}
	return data
}

func decodeJSONObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode JSON object: %v", err)
	}
	return value
}
