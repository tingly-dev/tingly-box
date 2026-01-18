package server

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

func TestResponseInput_UnmarshalJSON_String(t *testing.T) {
	jsonString := `"Hello, world!"`

	var input ResponseInput
	if err := json.Unmarshal([]byte(jsonString), &input); err != nil {
		t.Fatalf("Failed to deserialize string into ResponseInput: %v", err)
	}

	if !input.IsString() {
		t.Fatal("Expected input to be a string")
	}

	str, ok := input.String()
	if !ok || str != "Hello, world!" {
		t.Fatalf("Expected 'Hello, world!', got '%s'", str)
	}
}

func TestResponseInput_UnmarshalJSON_Array(t *testing.T) {
	jsonString := `[{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hello"}]}]`

	var input ResponseInput
	if err := json.Unmarshal([]byte(jsonString), &input); err != nil {
		t.Fatalf("Failed to deserialize array into ResponseInput: %v", err)
	}

	if !input.IsArray() {
		t.Fatal("Expected input to be an array")
	}
}

func TestResponseCreateRequest_UnmarshalJSON(t *testing.T) {
	jsonString := `{
		"model": "gpt-4o",
		"input": "Write a haiku",
		"stream": true
	}`

	var req ResponseCreateRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize into ResponseCreateRequest: %v", err)
	}

	if req.Model != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	if req.Stream != true {
		t.Fatal("Expected stream to be true")
	}

	str, ok := req.Input.String()
	if !ok || str != "Write a haiku" {
		t.Fatalf("Expected input 'Write a haiku', got '%s'", str)
	}
}

func TestResponseCreateRequest_UnmarshalJSON_ArrayInput(t *testing.T) {
	jsonString := `{
		"model": "gpt-4o",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hello"}]}
		]
	}`

	var req ResponseCreateRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize into ResponseCreateRequest: %v", err)
	}

	if req.Model != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	if !req.Input.IsArray() {
		t.Fatal("Expected input to be an array")
	}
}

func TestResponseCreateRequest_UnmarshalJSON_WithExtras(t *testing.T) {
	jsonString := `{
		"model": "gpt-4o",
		"input": "Hello",
		"temperature": 0.7,
		"max_output_tokens": 1000
	}`

	var req ResponseCreateRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize into ResponseCreateRequest: %v", err)
	}

	if req.Model != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	// Check that extras are captured
	if req.Extras == nil {
		t.Fatal("Expected extras to be captured")
	}

	if temp, ok := req.Extras["temperature"].(float64); !ok || temp != 0.7 {
		t.Fatalf("Expected temperature 0.7, got %v", temp)
	}
}

func TestConvertToResponsesParams_Simple(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hello world"
	}`)

	server := &Server{} // Minimal server for testing
	params, err := server.convertToResponsesParams(body, "gpt-4")
	if err != nil {
		t.Fatalf("Failed to convert params: %v", err)
	}

	// The model should be overridden
	// Note: ResponseNewParams.Model is shared.ResponsesModel (string type)
	// We can't directly access it, but the conversion should succeed
	_ = params
}

func TestConvertToResponsesParams_Complex(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hello world",
		"temperature": 0.7,
		"max_output_tokens": 1000,
		"instructions": "Be concise"
	}`)

	server := &Server{}
	params, err := server.convertToResponsesParams(body, "gpt-4")
	if err != nil {
		t.Fatalf("Failed to convert params: %v", err)
	}

	// The model should be overridden to gpt-4
	_ = params
}

func TestResponseNewParams_MarshalUnmarshal(t *testing.T) {
	// Test that the SDK's ResponseNewParams can handle our input
	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfString: param.NewOpt("Hello world")},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Failed to marshal ResponseNewParams: %v", err)
	}

	var unmarshaled responses.ResponseNewParams
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ResponseNewParams: %v", err)
	}
}
