package server

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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

	var req protocol.ResponseCreateRequest
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

	var req protocol.ResponseCreateRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(req.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(req.Tools))
	}
}
