package protocol_validate

import (
	"fmt"
	"strings"
)

// AssertContentEquals returns an Assertion that the response content equals expected.
func AssertContentEquals(expected string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("content_equals(%q)", expected),
		Check: func(r *RoundTripResult) error {
			if r.Content != expected {
				return fmt.Errorf("content: got %q, want %q", r.Content, expected)
			}
			return nil
		},
	}
}

// AssertContentContains returns an Assertion that the response content contains substring.
func AssertContentContains(substring string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("content_contains(%q)", substring),
		Check: func(r *RoundTripResult) error {
			if !strings.Contains(r.Content, substring) {
				return fmt.Errorf("content %q does not contain %q", r.Content, substring)
			}
			return nil
		},
	}
}

// AssertRoleEquals returns an Assertion that the response role equals expected.
func AssertRoleEquals(expected string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("role_equals(%q)", expected),
		Check: func(r *RoundTripResult) error {
			if r.Role != expected {
				return fmt.Errorf("role: got %q, want %q", r.Role, expected)
			}
			return nil
		},
	}
}

// AssertFinishReason returns an Assertion that the finish_reason equals expected.
func AssertFinishReason(expected string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("finish_reason(%q)", expected),
		Check: func(r *RoundTripResult) error {
			if r.FinishReason != expected {
				return fmt.Errorf("finish_reason: got %q, want %q", r.FinishReason, expected)
			}
			return nil
		},
	}
}

// AssertHasToolCalls returns an Assertion that exactly count tool calls are present.
func AssertHasToolCalls(count int) Assertion {
	return Assertion{
		Name: fmt.Sprintf("has_tool_calls(%d)", count),
		Check: func(r *RoundTripResult) error {
			if len(r.ToolCalls) != count {
				return fmt.Errorf("tool_calls: got %d, want %d", len(r.ToolCalls), count)
			}
			return nil
		},
	}
}

// AssertToolCallName returns an Assertion that the tool call at index has the given name.
func AssertToolCallName(index int, name string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("tool_call_name[%d](%q)", index, name),
		Check: func(r *RoundTripResult) error {
			if index >= len(r.ToolCalls) {
				return fmt.Errorf("tool_call[%d]: index out of range (have %d)", index, len(r.ToolCalls))
			}
			if r.ToolCalls[index].Name != name {
				return fmt.Errorf("tool_call[%d].name: got %q, want %q", index, r.ToolCalls[index].Name, name)
			}
			return nil
		},
	}
}

// AssertToolCallArgs returns an Assertion that tool call at index has key=value in its JSON args.
func AssertToolCallArgs(index int, key, value string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("tool_call_args[%d](%q=%q)", index, key, value),
		Check: func(r *RoundTripResult) error {
			if index >= len(r.ToolCalls) {
				return fmt.Errorf("tool_call[%d]: index out of range (have %d)", index, len(r.ToolCalls))
			}
			args := r.ToolCalls[index].Arguments
			// Simple substring check: "key":"value" — avoids a full JSON parse
			needle := fmt.Sprintf(`%q:%q`, key, value)
			altNeedle := fmt.Sprintf(`%q: %q`, key, value)
			if !strings.Contains(args, needle) && !strings.Contains(args, altNeedle) {
				return fmt.Errorf("tool_call[%d].arguments does not contain %s=%s in %q", index, key, value, args)
			}
			return nil
		},
	}
}

// AssertHasThinking returns an Assertion that thinking content is non-empty.
func AssertHasThinking() Assertion {
	return Assertion{
		Name: "has_thinking",
		Check: func(r *RoundTripResult) error {
			if r.ThinkingContent == "" {
				return fmt.Errorf("expected non-empty thinking content")
			}
			return nil
		},
	}
}

// AssertNoThinking returns an Assertion that no thinking content is present.
func AssertNoThinking() Assertion {
	return Assertion{
		Name: "no_thinking",
		Check: func(r *RoundTripResult) error {
			if r.ThinkingContent != "" {
				return fmt.Errorf("expected empty thinking content, got %q", r.ThinkingContent)
			}
			return nil
		},
	}
}

// AssertUsageNonZero returns an Assertion that at least one token count > 0.
func AssertUsageNonZero() Assertion {
	return Assertion{
		Name: "usage_non_zero",
		Check: func(r *RoundTripResult) error {
			if r.Usage == nil {
				return fmt.Errorf("usage is nil")
			}
			if r.Usage.InputTokens == 0 && r.Usage.OutputTokens == 0 {
				return fmt.Errorf("usage is zero (input=%d, output=%d)", r.Usage.InputTokens, r.Usage.OutputTokens)
			}
			return nil
		},
	}
}

// AssertHTTPStatus returns an Assertion that the HTTP status code equals expected.
func AssertHTTPStatus(expected int) Assertion {
	return Assertion{
		Name: fmt.Sprintf("http_status(%d)", expected),
		Check: func(r *RoundTripResult) error {
			if r.HTTPStatus != expected {
				return fmt.Errorf("http_status: got %d, want %d", r.HTTPStatus, expected)
			}
			return nil
		},
	}
}

// AssertStreamEventCount returns an Assertion that at least min SSE events were received.
func AssertStreamEventCount(min int) Assertion {
	return Assertion{
		Name: fmt.Sprintf("stream_event_count(>=%d)", min),
		Check: func(r *RoundTripResult) error {
			if len(r.StreamEvents) < min {
				return fmt.Errorf("stream events: got %d, want >= %d", len(r.StreamEvents), min)
			}
			return nil
		},
	}
}

// AssertModelContains returns an Assertion that the model name contains substring.
func AssertModelContains(substring string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("model_contains(%q)", substring),
		Check: func(r *RoundTripResult) error {
			if !strings.Contains(r.Model, substring) {
				return fmt.Errorf("model %q does not contain %q", r.Model, substring)
			}
			return nil
		},
	}
}
