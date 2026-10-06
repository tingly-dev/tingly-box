package client

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNewDecisionClient_RequiresDecisionEndpoint(t *testing.T) {
	plain := &typ.Provider{Name: "gpt", APIStyle: ai.APIStyleOpenAI, APIBase: "https://api.example.com/v1"}
	if _, err := NewDecisionClient(plain, "m", typ.SessionID{}); err == nil {
		t.Error("a chat provider without a decision fork must be refused")
	}

	native := &typ.Provider{Name: "jev", APIStyle: ai.APIStyleDecision, APIBase: "https://jev.example.com/api/v1"}
	c, err := NewDecisionClient(native, "m", typ.SessionID{})
	if err != nil {
		t.Fatalf("Jev-native provider should construct: %v", err)
	}
	if c.endpoint != "https://jev.example.com/api/v1/decisions" {
		t.Errorf("endpoint = %q, want the normalized /decisions URL", c.endpoint)
	}

	fork := &typ.Provider{Name: "gpt+decision", APIStyle: ai.APIStyleOpenAI, APIBase: "https://api.example.com/v1", APIBaseDecision: "https://jev.example.com/api/v1"}
	if c, err = NewDecisionClient(fork, "m", typ.SessionID{}); err != nil {
		t.Fatalf("fork provider should construct: %v", err)
	}
	if c.endpoint != "https://jev.example.com/api/v1/decisions" {
		t.Errorf("fork endpoint = %q, want the fork base, not the chat base", c.endpoint)
	}
}

func TestDecisionEndpointURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"https://www.jevai.org/api/v1", "https://www.jevai.org/api/v1/decisions"},
		{"https://www.jevai.org/api/v1/", "https://www.jevai.org/api/v1/decisions"},
		{"https://www.jevai.org/api/v1/decisions", "https://www.jevai.org/api/v1/decisions"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/decisions"},
		{"https://www.jevai.org/api/v1/decisions?x=1#frag", "https://www.jevai.org/api/v1/decisions"},
	}
	for _, tt := range tests {
		got, err := DecisionEndpointURL(tt.base)
		if err != nil {
			t.Fatalf("DecisionEndpointURL(%q): %v", tt.base, err)
		}
		if got != tt.want {
			t.Errorf("DecisionEndpointURL(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
	if _, err := DecisionEndpointURL("not-a-url"); err == nil {
		t.Error("DecisionEndpointURL should reject a relative URL")
	}
}
