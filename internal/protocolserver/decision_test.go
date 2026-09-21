package protocolserver

import "testing"

func TestDecisionEndpoint(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"https://www.jevai.org/api/v1", "https://www.jevai.org/api/v1/decisions"},
		{"https://www.jevai.org/api/v1/", "https://www.jevai.org/api/v1/decisions"},
		{"https://www.jevai.org/api/v1/decisions", "https://www.jevai.org/api/v1/decisions"},
	}
	for _, tt := range tests {
		got, err := decisionEndpoint(tt.base)
		if err != nil {
			t.Fatalf("decisionEndpoint(%q): %v", tt.base, err)
		}
		if got != tt.want {
			t.Errorf("decisionEndpoint(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
	if _, err := decisionEndpoint("not-a-url"); err == nil {
		t.Error("decisionEndpoint should reject a relative URL")
	}
}
