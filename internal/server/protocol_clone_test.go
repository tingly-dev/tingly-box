package server

import (
	"encoding/json"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// The clone helpers back per-attempt failover: a retry must re-transform from a
// pristine request, so the clone has to faithfully round-trip the request body
// (model, stream flag, messages, token caps) and must not alias the original.

func TestCloneAnthropicV1Request_RoundTrip(t *testing.T) {
	body := []byte(`{"model":"claude-3","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	var orig protocol.AnthropicMessagesRequest
	if err := json.Unmarshal(body, &orig); err != nil {
		t.Fatal(err)
	}
	tmpl, err := orig.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	clone, err := CloneAnthropicV1Request(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if string(clone.Model) != "claude-3" {
		t.Fatalf("clone Model = %q, want claude-3", clone.Model)
	}
	if !clone.Stream {
		t.Fatal("clone Stream = false, want true")
	}
	if len(clone.Messages) != 1 {
		t.Fatalf("clone Messages = %d, want 1", len(clone.Messages))
	}

	// Mutating the clone must not affect the original (no shared backing).
	clone.Model = "changed"
	if string(orig.Model) == "changed" {
		t.Fatal("clone aliases the original request")
	}
}

func TestCloneAnthropicBetaRequest_RoundTrip(t *testing.T) {
	body := []byte(`{"model":"claude-3-beta","max_tokens":42,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	var orig protocol.AnthropicBetaMessagesRequest
	if err := json.Unmarshal(body, &orig); err != nil {
		t.Fatal(err)
	}
	tmpl, err := orig.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	clone, err := CloneAnthropicBetaRequest(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if string(clone.Model) != "claude-3-beta" {
		t.Fatalf("clone Model = %q, want claude-3-beta", clone.Model)
	}
	if !clone.Stream {
		t.Fatal("clone Stream = false, want true")
	}
	if len(clone.Messages) != 1 {
		t.Fatalf("clone Messages = %d, want 1", len(clone.Messages))
	}
}

func TestCloneOpenAIChatRequest_RoundTrip(t *testing.T) {
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	var orig protocol.OpenAIChatCompletionRequest
	if err := json.Unmarshal(body, &orig); err != nil {
		t.Fatal(err)
	}
	tmpl, err := orig.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	clone, err := CloneOpenAIChatRequest(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if string(clone.Model) != "gpt-4" {
		t.Fatalf("clone Model = %q, want gpt-4", clone.Model)
	}
	if !clone.Stream {
		t.Fatal("clone Stream = false, want true")
	}
	if len(clone.Messages) != 1 {
		t.Fatalf("clone Messages = %d, want 1", len(clone.Messages))
	}
}

func TestCloneResponsesParams_RoundTrip(t *testing.T) {
	var orig protocol.ResponseCreateRequest
	if err := json.Unmarshal([]byte(`{"model":"gpt-4o","input":"hello"}`), &orig); err != nil {
		t.Fatal(err)
	}

	clone, err := CloneResponsesParams(orig.ResponseNewParams)
	if err != nil {
		t.Fatal(err)
	}
	if string(clone.Model) != "gpt-4o" {
		t.Fatalf("clone Model = %q, want gpt-4o", clone.Model)
	}

	// Mutating the clone must not affect the original params.
	clone.Model = "changed"
	if string(orig.Model) == "changed" {
		t.Fatal("clone aliases the original params")
	}
}
