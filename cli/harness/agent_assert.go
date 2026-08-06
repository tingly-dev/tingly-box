package main

import "strings"

// realAgentErrorMarkers lists substrings that, when present in a real-provider
// agent CLI's output, indicate the round trip failed even though the CLI exited
// 0 (a "fake green"). Matching is case-insensitive substring. Kept as one
// shared slice so every agent executor honors the same vocabulary.
//
// Bias toward catching failures over false alarms: a stray marker in otherwise
// valid model output is a manual-override (see .design/harness-agent-testing.md §"断言误报"),
// whereas an exit-0 failure that ships silently is the regression we want to
// block. English markers only by default to avoid false-positives on legitimate
// Chinese model answers; extend this list from runbook feedback.
var realAgentErrorMarkers = []string{
	"traceback",
	"unauthorized",
	"invalid_api key",
	"invalid_api_key",
	"api key is invalid",
	"incorrect api key",
	"status: 4",
	"status: 5",
	"error: ",
	"rate limit",
	"insufficient_quota",
}

// stripANSI removes common ANSI escape sequences so escape-colored "error"
// text is still caught. Minimal — covers SGR (color) and cursor sequences.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// skip until a letter (the command byte)
			j := i + 2
			for j < len(s) && !isANSICommandByte(s[j]) {
				j++
			}
			i = j // loop's i++ advances past the command byte
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isANSICommandByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// contentAssertionRule names the rule a failing assertion tripped. Empty when
// the content passes.
type contentAssertionRule string

const (
	ruleEmpty      contentAssertionRule = "output is empty after stripping whitespace"
	ruleErrorMarker contentAssertionRule = "output contains an error marker"
)

// assertRealAgentContent validates a real-provider agent CLI's captured output.
// Returns (ok, rule): when ok is false, rule names which check failed so the
// caller can record a precise error. A pass requires non-empty visible content
// AND no error marker.
//
// This mirrors the mock path's VirtualMockAnswerMarker check (agent.go) — but
// for real upstreams the response isn't test-controlled, so we assert on
// "looks like a real answer" rather than an exact marker.
func assertRealAgentContent(output string) (ok bool, rule contentAssertionRule) {
	cleaned := strings.TrimSpace(stripANSI(output))
	if len(cleaned) == 0 {
		return false, ruleEmpty
	}
	lower := strings.ToLower(cleaned)
	for _, m := range realAgentErrorMarkers {
		if strings.Contains(lower, m) {
			return false, ruleErrorMarker
		}
	}
	return true, ""
}
