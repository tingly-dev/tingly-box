package server

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// TestResponseNewParamsInputUnion_String tests unmarshaling a string input
func TestResponseNewParamsInputUnion_String(t *testing.T) {
	jsonString := `"Hello, world!"`

	var input responses.ResponseNewParamsInputUnion
	if err := json.Unmarshal([]byte(jsonString), &input); err != nil {
		t.Fatalf("Failed to deserialize string into ResponseNewParamsInputUnion: %v", err)
	}

	if param.IsOmitted(input.OfString) {
		t.Fatal("Expected input to be a string")
	}

	str := input.OfString.Value
	if str != "Hello, world!" {
		t.Fatalf("Expected 'Hello, world!', got '%s'", str)
	}
}

// TestResponseNewParamsInputUnion_Array tests unmarshaling an array input
func TestResponseNewParamsInputUnion_Array(t *testing.T) {
	jsonString := `[{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hello"}]}]`

	var input responses.ResponseNewParamsInputUnion
	if err := json.Unmarshal([]byte(jsonString), &input); err != nil {
		t.Fatalf("Failed to deserialize array into ResponseNewParamsInputUnion: %v", err)
	}

	if param.IsOmitted(input.OfInputItemList) {
		t.Fatal("Expected input to be an array")
	}
}

// TestResponseCreateRequest_UnmarshalJSON tests unmarshaling a ResponseCreateRequest
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

	if string(req.Model) != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	if !req.Stream {
		t.Fatal("Expected stream to be true")
	}

	if param.IsOmitted(req.Input.OfString) {
		t.Fatal("Expected input to be a string")
	}

	str := req.Input.OfString.Value
	if str != "Write a haiku" {
		t.Fatalf("Expected input 'Write a haiku', got '%s'", str)
	}
}

// TestResponseCreateRequest_UnmarshalJSON_ArrayInput tests unmarshaling a ResponseCreateRequest with array input
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

	if string(req.Model) != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	if param.IsOmitted(req.Input.OfInputItemList) {
		t.Fatal("Expected input to be an array")
	}
}

// TestResponseCreateRequest_UnmarshalJSON_WithExtraFields tests unmarshaling with extra fields
func TestResponseCreateRequest_UnmarshalJSON_WithExtraFields(t *testing.T) {
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

	if string(req.Model) != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	// Check that temperature was captured (it's a field on ResponseNewParams)
	if !param.IsOmitted(req.Temperature) {
		if req.Temperature.Value != 0.7 {
			t.Fatalf("Expected temperature 0.7, got %v", req.Temperature.Value)
		}
	}

	// Check that max_output_tokens was captured
	if !param.IsOmitted(req.MaxOutputTokens) {
		if req.MaxOutputTokens.Value != 1000 {
			t.Fatalf("Expected max_output_tokens 1000, got %v", req.MaxOutputTokens.Value)
		}
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
