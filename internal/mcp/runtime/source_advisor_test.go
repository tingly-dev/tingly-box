package runtime

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestDetectAdvisorFormat(t *testing.T) {
	tests := []struct {
		baseURL string
		model   string
		want    AdvisorFormat
	}{
		{"https://api.openai.com/v1", "gpt-4", FormatOpenAI},
		{"https://api.anthropic.com/v1", "claude-opus-4-6", FormatAnthropic},
		{"https://custom.com", "claude-sonnet", FormatAnthropic},
	}
	for _, tt := range tests {
		cfg := typ.AdvisorConfig{BaseURL: tt.baseURL, Model: tt.model}
		got := detectAdvisorFormat(cfg)
		if got != tt.want {
			t.Errorf("detectAdvisorFormat(%q,%q)=%v, want %v", tt.baseURL, tt.model, got, tt.want)
		}
	}
}

func TestNormalizeAdvisorResponse(t *testing.T) {
	valid := `{"assessment":"ok","recommendation":"do it"}`
	got := normalizeAdvisorResponse(valid)
	if got != valid {
		t.Errorf("normalizeAdvisorResponse(valid)=%q, want %q", got, valid)
	}

	invalid := "just some text"
	got = normalizeAdvisorResponse(invalid)
	if !strings.Contains(got, "non-JSON") || !strings.Contains(got, "just some text") {
		t.Errorf("normalizeAdvisorResponse(invalid) unexpected: %q", got)
	}
}
