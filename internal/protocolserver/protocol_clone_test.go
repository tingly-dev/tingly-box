package protocolserver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

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

func TestCloneAnthropicRequestsPreserveInternalExtras(t *testing.T) {
	const body = `{"model":"m","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	for _, beta := range []bool{false, true} {
		t.Run(map[bool]string{false: "v1", true: "beta"}[beta], func(t *testing.T) {
			var orig interface {
				SetExtraFields(map[string]any)
				ExtraFields() map[string]any
			}
			if beta {
				orig = &protocol.AnthropicBetaMessagesRequest{}
			} else {
				orig = &protocol.AnthropicMessagesRequest{}
			}
			require.NoError(t, json.Unmarshal([]byte(body), orig))
			orig.SetExtraFields(map[string]any{"internal_extension": map[string]any{"value": "original"}})
			template, err := json.Marshal(orig)
			require.NoError(t, err)
			var extras map[string]any
			if beta {
				clone, err := CloneAnthropicBetaRequest(template)
				require.NoError(t, err)
				require.True(t, clone.Stream)
				extras = clone.ExtraFields()
			} else {
				clone, err := CloneAnthropicV1Request(template)
				require.NoError(t, err)
				require.True(t, clone.Stream)
				extras = clone.ExtraFields()
			}
			require.Equal(t, orig.ExtraFields(), extras)
			require.NotContains(t, extras, "stream")
			extras["internal_extension"].(map[string]any)["value"] = "changed"
			require.Equal(t, "original", orig.ExtraFields()["internal_extension"].(map[string]any)["value"])
		})
	}
}
