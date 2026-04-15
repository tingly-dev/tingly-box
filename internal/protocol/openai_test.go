package protocol

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/packages/param"
)

func TestResponseCreateRequestUnmarshalJSON(t *testing.T) {
	jsonData := `{
		"input": [{
			"role": "user",
			"content": "Please help me calculate 3+5 and tell me the weather in Beijing"
		}],
		"model": "tingly-gpt",
		"stream": true
	}`

	var req ResponseCreateRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.Stream != true {
		t.Errorf("Expected Stream=true, got %v", req.Stream)
	}

	if param.IsOmitted(req.Input.OfInputItemList) {
		t.Fatalf("Input.OfInputItemList should not be omitted")
	}

	if len(req.Input.OfInputItemList) != 1 {
		t.Fatalf("Expected 1 input item, got %d", len(req.Input.OfInputItemList))
	}

	item := req.Input.OfInputItemList[0]
	if item.OfMessage == nil {
		t.Fatalf("Expected OfMessage to be non-nil")
	}

	if string(item.OfMessage.Role) != "user" {
		t.Errorf("Expected role 'user', got '%s'", item.OfMessage.Role)
	}

	if param.IsOmitted(item.OfMessage.Content.OfString) {
		t.Fatalf("Content.OfString should not be omitted")
	}

	if item.OfMessage.Content.OfString.Value != "Please help me calculate 3+5 and tell me the weather in Beijing" {
		t.Errorf("Expected content 'Please help me calculate 3+5 and tell me the weather in Beijing', got '%s'", item.OfMessage.Content.OfString.Value)
	}
}

func TestResponseCreateRequestUnmarshalJSONWithTools(t *testing.T) {
	jsonData := `{
		"input": [{
			"role": "user",
			"content": "Please help me calculate 3+5 and tell me the weather in Beijing"
		}],
		"model": "tingly-gpt",
		"stream": true,
		"tools": [{
			"type": "function",
			"name": "add_numbers",
			"description": "Add two numbers",
			"parameters": {
				"type": "object",
				"properties": {
					"a": {"type": "number"},
					"b": {"type": "number"}
				},
				"required": ["a", "b"]
			}
		}]
	}`

	var req ResponseCreateRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(req.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(req.Tools))
	}
}

func TestResponseCreateRequestUnmarshalJSONWithOutputTextBlocks(t *testing.T) {
	// Test case for output_text block flattening
	// When content is an array of output_text blocks, they should be flattened
	// to a single string to ensure multi-turn Codex histories are preserved
	jsonData := `{
		"input": [{
			"role": "user",
			"content": [
				{"type": "output_text", "text": "First part of response"},
				{"type": "output_text", "text": "Second part of response"}
			]
		}],
		"model": "tingly-gpt",
		"stream": false
	}`

	var req ResponseCreateRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if param.IsOmitted(req.Input.OfInputItemList) {
		t.Fatalf("Input.OfInputItemList should not be omitted")
	}

	if len(req.Input.OfInputItemList) != 1 {
		t.Fatalf("Expected 1 input item, got %d", len(req.Input.OfInputItemList))
	}

	item := req.Input.OfInputItemList[0]
	if item.OfMessage == nil {
		t.Fatalf("Expected OfMessage to be non-nil")
	}

	// After flattening, content should be a single string joined by newlines
	if param.IsOmitted(item.OfMessage.Content.OfString) {
		t.Fatalf("Content.OfString should not be omitted after flattening output_text blocks")
	}

	expectedContent := "First part of response\nSecond part of response"
	if item.OfMessage.Content.OfString.Value != expectedContent {
		t.Errorf("Expected content '%s', got '%s'", expectedContent, item.OfMessage.Content.OfString.Value)
	}
}

func TestResponseCreateRequestUnmarshalJSONWithMixedContentBlocks(t *testing.T) {
	// Test case for mixed content blocks (output_text + other types)
	// Non-output_text blocks should not be flattened
	jsonData := `{
		"input": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Regular text block"},
				{"type": "output_text", "text": "Output text block"}
			]
		}],
		"model": "tingly-gpt",
		"stream": false
	}`

	var req ResponseCreateRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	item := req.Input.OfInputItemList[0].OfMessage

	// Mixed blocks may be handled differently - this test verifies unmarshal succeeds
	if item == nil {
		t.Fatalf("Expected OfMessage to be non-nil")
	}
}

func TestResponseCreateRequestUnmarshalJSONWithStringInput(t *testing.T) {
	// Test case for input as a single string (not an array)
	// This is a valid format for the Responses API
	jsonData := `{
		"input": "What is the weather in Beijing?",
		"model": "tingly-gpt",
		"stream": false
	}`

	var req ResponseCreateRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// When input is a string, it should be available in OfString
	if param.IsOmitted(req.Input.OfString) {
		t.Fatalf("Input.OfString should not be omitted for string input")
	}

	if req.Input.OfString.Value != "What is the weather in Beijing?" {
		t.Errorf("Expected input 'What is the weather in Beijing?', got '%s'", req.Input.OfString.Value)
	}

	// OfInputItemList should be omitted when input is a string
	if !param.IsOmitted(req.Input.OfInputItemList) {
		t.Errorf("Input.OfInputItemList should be omitted for string input")
	}
}
