package ai

import "testing"

func TestVModelAPIBase(t *testing.T) {
	if got := VModelAPIBase(APIStyleOpenAI); got != "vmodel://openai" {
		t.Fatalf("got %q", got)
	}
	if !IsVModelAPIBase("vmodel://anthropic") || IsVModelAPIBase("https://api.openai.com/v1") || IsVModelAPIBase("") {
		t.Fatal("IsVModelAPIBase mismatch")
	}
}

func TestVModelHTTPBase(t *testing.T) {
	cases := []struct{ base, style, want string }{
		{"vmodel://openai", "anthropic", "http://vmodel.internal/openai/v1"},
		{"vmodel://anthropic", "openai", "http://vmodel.internal/anthropic/v1"},
		// legacy sentinel and unknown host fall back to the provider style
		{"vmodel://local", "anthropic", "http://vmodel.internal/anthropic/v1"},
		{"vmodel://whatever", "openai", "http://vmodel.internal/openai/v1"},
		{"vmodel://local", "", "http://vmodel.internal/openai/v1"},
	}
	for _, c := range cases {
		if got := VModelHTTPBase(c.base, APIStyle(c.style)); got != c.want {
			t.Errorf("VModelHTTPBase(%q,%q) = %q, want %q", c.base, c.style, got, c.want)
		}
	}
}
