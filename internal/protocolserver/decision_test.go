package protocolserver

import (
	"encoding/json"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestValidateDecisionBody(t *testing.T) {
	body := map[string]any{
		"model": "jev-small",
		"state": map[string]any{"mood": "calm"},
		"questions": map[string]any{
			"q1": map[string]any{"type": "choice", "options": []string{"a", "b"}},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	model, err := validateDecisionBody(raw)
	if err != nil {
		t.Fatalf("validateDecisionBody: %v", err)
	}
	if model != "jev-small" {
		t.Errorf("model = %q, want %q", model, "jev-small")
	}
}

func TestValidateDecisionBody_Rejections(t *testing.T) {
	cases := map[string]string{
		"not json":          `[{`,
		"missing model":     `{"questions": {"q1": {"type": "score"}}}`,
		"blank model":       `{"model": "  ", "questions": {"q1": {"type": "score"}}}`,
		"missing questions": `{"model": "jev-small"}`,
		"empty questions":   `{"model": "jev-small", "questions": {}}`,
	}
	for name, raw := range cases {
		if _, err := validateDecisionBody([]byte(raw)); err == nil {
			t.Errorf("%s: expected rejection, got none", name)
		}
	}
}

func TestRewriteDecisionModel(t *testing.T) {
	// Every non-model field survives the rewrite untouched.
	raw := []byte(`{"model":"jev-small","state":{"k":1},"questions":{"q1":{"type":"noul"}}}`)
	out, err := rewriteDecisionModel(raw, "routed-model")
	if err != nil {
		t.Fatalf("rewriteDecisionModel: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("rewritten body is not JSON: %v", err)
	}
	if got["model"] != "routed-model" {
		t.Errorf("model = %v, want routed-model", got["model"])
	}
	state, ok := got["state"].(map[string]any)
	if !ok || state["k"] != float64(1) {
		t.Errorf("state not preserved: %v", got["state"])
	}
}

func TestDecisionUsageFromBody(t *testing.T) {
	t.Run("absent usage means zero", func(t *testing.T) {
		usage := decisionUsageFromBody([]byte(`{"answers": {}}`))
		if usage == nil || usage.InputTokens != 0 || usage.OutputTokens != 0 {
			t.Errorf("expected zero usage, got %+v", usage)
		}
	})
	t.Run("openai spelling", func(t *testing.T) {
		usage := decisionUsageFromBody([]byte(`{"answers": {}, "usage": {"prompt_tokens": 12, "completion_tokens": 5}}`))
		if usage == nil || usage.InputTokens != 12 || usage.OutputTokens != 5 {
			t.Errorf("expected 12/5, got %+v", usage)
		}
	})
	t.Run("anthropic spelling", func(t *testing.T) {
		usage := decisionUsageFromBody([]byte(`{"answers": {}, "usage": {"input_tokens": 7, "output_tokens": 3}}`))
		if usage == nil || usage.InputTokens != 7 || usage.OutputTokens != 3 {
			t.Errorf("expected 7/3, got %+v", usage)
		}
	})
	t.Run("invalid body falls back to zero", func(t *testing.T) {
		usage := decisionUsageFromBody([]byte(`not-json`))
		if usage == nil {
			t.Fatal("expected non-nil zero usage")
		}
	})
}

func TestRequireDecisionEndpoint(t *testing.T) {
	native := &typ.Provider{Name: "jev", APIStyle: protocol.APIStyleDecision}
	if err := requireDecisionEndpoint(native); err != nil {
		t.Errorf("Jev-native provider should pass: %v", err)
	}
	fork := &typ.Provider{Name: "gpt", APIStyle: protocol.APIStyleOpenAI, APIBaseDecision: "https://example.com/api/v1"}
	if err := requireDecisionEndpoint(fork); err != nil {
		t.Errorf("chat provider with a decision fork should pass: %v", err)
	}
	plain := &typ.Provider{Name: "gpt", APIStyle: protocol.APIStyleOpenAI}
	if err := requireDecisionEndpoint(plain); err == nil {
		t.Error("chat provider without a decision fork should be refused for decisions")
	}
}
