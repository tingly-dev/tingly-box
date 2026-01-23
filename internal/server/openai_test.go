package server

import (
	"encoding/json"
	"testing"

	"tingly-box/internal/protocol"
)

func TestOpenAIChatCompletionRequest_UnmarshalJSON(t *testing.T) {
	jsonString := "{\"stream\":true}"

	var req protocol.OpenAIChatCompletionRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize rawBody into OpenAIChatCompletionRequest: %v", err)
	}

	if req.Stream != true {
		t.Fatal("Failed to deserialize rawBody into OpenAIChatCompletionRequest")
	}
}
