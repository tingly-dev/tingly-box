package forwarding

import "testing"

func TestDecisionEndpointURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"https://www.jevai.org/api/v1", "https://www.jevai.org/api/v1/decisions"},
		{"https://www.jevai.org/api/v1/", "https://www.jevai.org/api/v1/decisions"},
		{"https://www.jevai.org/api/v1/decisions", "https://www.jevai.org/api/v1/decisions"},
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
