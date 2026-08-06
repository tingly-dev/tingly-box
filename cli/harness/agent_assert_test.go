package main

import "testing"

func TestAssertRealAgentContent(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool // true = pass
	}{
		{"plain answer", "The capital of France is Paris.", true},
		{"chinese answer", "法国的首都是巴黎。", true},
		{"multiline", "Line one\nLine two\n  \nLine four", true},
		{"leading/trailing space", "   Paris   ", true},
		{"ANSI-colored answer", "\x1b[32mParis\x1b[0m", true},

		{"empty", "", false},
		{"only whitespace", "   \n\t  ", false},
		{"only ANSI", "\x1b[0m", false},

		{"traceback", "Traceback (most recent call last):\n  File ...", false},
		{"unauthorized", "401 Unauthorized: invalid credentials", false},
		{"invalid api key", "invalid_api_key provided", false},
		{"status 500", "upstream status: 500 Internal Server Error", false},
		{"status 401", "status: 401", false},
		{"error prefix", "error: could not reach upstream", false},
		{"rate limit", "rate limit exceeded, retry later", false},
		{"quota", "insufficient_quota on account", false},
		{"case-insensitive marker", "RATE LIMIT hit", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, rule := assertRealAgentContent(c.out)
			if got != c.want {
				t.Fatalf("assertRealAgentContent(%q) = %v, want %v (rule=%q)", c.out, got, c.want, rule)
			}
			if !got && rule == "" {
				t.Fatalf("failure returned empty rule for %q", c.out)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[1;32mhello\x1b[0m \x1b[31mworld\x1b[0m"
	want := "hello world"
	if got := stripANSI(in); got != want {
		t.Fatalf("stripANSI(%q) = %q, want %q", in, got, want)
	}
}
