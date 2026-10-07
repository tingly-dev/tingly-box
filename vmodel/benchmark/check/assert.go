package check

import (
	"encoding/json"
	"fmt"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"regexp"
	"slices"
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

// AssertContentNonEmpty returns an Assertion that the response carries some
// payload — either text content or at least one tool call. Useful as an
// upstream-independent structural check when the exact response text is not
// controlled by the test (e.g. vmodel or real upstreams).
func AssertContentNonEmpty() Assertion {
	return Assertion{
		Name: "content_non_empty",
		Check: func(r *RoundTripResult) error {
			if strings.TrimSpace(r.Content) == "" && len(r.ToolCalls) == 0 {
				return fmt.Errorf("response has neither text content nor tool calls")
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

// AssertFinishReasonOneOf returns an Assertion that the finish_reason matches any
// of the accepted values. Useful for scenarios where the expected finish reason
// varies by source protocol (e.g. "length" for Chat, "max_tokens" for Anthropic,
// "incomplete" for Responses).
func AssertFinishReasonOneOf(accepted ...string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("finish_reason_one_of(%v)", accepted),
		Check: func(r *RoundTripResult) error {
			if slices.Contains(accepted, r.FinishReason) {
				return nil
			}
			return fmt.Errorf("finish_reason: got %q, want one of %v", r.FinishReason, accepted)
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
// For streaming responses, usage extraction is protocol-dependent and may not
// be available, so this assertion is skipped in streaming mode.
func AssertUsageNonZero() Assertion {
	return Assertion{
		Name: "usage_non_zero",
		Check: func(r *RoundTripResult) error {
			if r.IsStreaming {
				return nil
			}
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

// AssertStreamError returns an Assertion that a streaming client explicitly
// observed an error after the response had started.
func AssertStreamError() Assertion {
	return Assertion{
		Name: "stream_error",
		Check: func(r *RoundTripResult) error {
			if r.StreamError == "" {
				return fmt.Errorf("stream ended without a reported error")
			}
			return nil
		},
	}
}

// AssertStreamNotCompleted returns an Assertion that no normal completion
// marker was observed. Error events and a clean terminal event are deliberately
// separate states: a truncated turn must never be presented as completed.
func AssertStreamNotCompleted() Assertion {
	return Assertion{
		Name: "stream_not_completed",
		Check: func(r *RoundTripResult) error {
			if r.StreamCompleted {
				return fmt.Errorf("stream was marked completed despite the expected failure")
			}
			return nil
		},
	}
}

// AssertHTTPStatusAtLeast returns an Assertion that the HTTP status code is >= min.
func AssertHTTPStatusAtLeast(min int) Assertion {
	return Assertion{
		Name: fmt.Sprintf("http_status(>=%d)", min),
		Check: func(r *RoundTripResult) error {
			if r.HTTPStatus < min {
				return fmt.Errorf("http_status: got %d, want >= %d", r.HTTPStatus, min)
			}
			return nil
		},
	}
}

// AssertErrorMessageContains returns an Assertion that the raw response body
// contains the given substring. Useful for validating error responses where
// the semantic fields (Content, Role, etc.) are not populated.
func AssertErrorMessageContains(substring string) Assertion {
	return Assertion{
		Name: fmt.Sprintf("error_message_contains(%q)", substring),
		Check: func(r *RoundTripResult) error {
			if !strings.Contains(string(r.RawBody), substring) {
				return fmt.Errorf("raw body does not contain %q", substring)
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

// AssertStreamEventsContain returns an Assertion that every marker appears
// somewhere in the raw SSE event lines. Use it to pin a streaming response's
// event shape (e.g. the Anthropic message_start…message_stop frame sequence)
// independently of its content.
func AssertStreamEventsContain(markers ...string) Assertion {
	return Assertion{
		Name: "stream_events_contain",
		Check: func(r *RoundTripResult) error {
			joined := strings.Join(r.StreamEvents, "\n")
			var missing []string
			for _, m := range markers {
				if !strings.Contains(joined, m) {
					missing = append(missing, m)
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf("stream missing expected event markers %v (%d event lines)", missing, len(r.StreamEvents))
			}
			return nil
		},
	}
}

// AssertFinishReasonNonEmpty returns an Assertion that a finish/stop reason was
// extracted at all — upstream-independent, unlike AssertFinishReason which pins
// an exact value.
func AssertFinishReasonNonEmpty() Assertion {
	return Assertion{
		Name: "finish_reason_non_empty",
		Check: func(r *RoundTripResult) error {
			if r.FinishReason == "" {
				return fmt.Errorf("no finish/stop reason extracted")
			}
			return nil
		},
	}
}

// AssertUsagePropagated returns an Assertion that token usage was extracted
// with both input and output counts > 0 — including for streaming responses,
// where usage rides the terminal frames (e.g. Anthropic message_delta). Use it
// when the pipeline under test must propagate usage end-to-end;
// AssertUsageNonZero is the softer variant that skips streaming.
func AssertUsagePropagated() Assertion {
	return Assertion{
		Name: "usage_propagated",
		Check: func(r *RoundTripResult) error {
			if r.Usage == nil {
				return fmt.Errorf("no usage extracted")
			}
			if r.Usage.InputTokens <= 0 || r.Usage.OutputTokens <= 0 {
				return fmt.Errorf("usage not fully propagated (input=%d, output=%d)", r.Usage.InputTokens, r.Usage.OutputTokens)
			}
			return nil
		},
	}
}

// responsesIDShape is the canonical form for one Responses item type: its
// id prefix and a compiled "<prefix>_<32 hex>" matcher built from it, so the
// prefix used in an error message is the one value both were built from
// rather than something re-derived from the regexp source.
type responsesIDShape struct {
	prefix string
	re     *regexp.Regexp
}

func newResponsesIDShape(prefix string) responsesIDShape {
	return responsesIDShape{prefix: prefix, re: regexp.MustCompile(`^` + prefix + `_[0-9a-f]{32}$`)}
}

// responsesIDShapes are the canonical id forms the gateway mints for a
// Responses response it synthesizes from a non-Responses upstream
// (.design/protocol-responses.md §2): "<prefix>_<32 lowercase hex>".
var responsesIDShapes = map[string]responsesIDShape{
	"response":      newResponsesIDShape("resp"),
	"message":       newResponsesIDShape("msg"),
	"function_call": newResponsesIDShape("fc"),
	"reasoning":     newResponsesIDShape("rs"),
}

// FinalResponsesBody returns the terminal Responses API response body of a
// round trip: RawBody parsed directly when the trip was not streaming, or
// the "response" payload of the terminal response.completed /
// response.incomplete SSE event when it was. Shared by this package's own
// assertions and by harness test cases that need the same body (e.g.
// internal/protocoltest/content_shapes.go), so the terminal-event scan is
// implemented once.
func FinalResponsesBody(r *RoundTripResult) (map[string]any, error) {
	if !r.IsStreaming {
		var resp map[string]any
		if err := json.Unmarshal(r.RawBody, &resp); err != nil {
			return nil, fmt.Errorf("responses body: %w", err)
		}
		return resp, nil
	}
	for _, line := range r.StreamEvents {
		payload := strings.TrimPrefix(line, "data: ")
		var ev map[string]any
		if json.Unmarshal([]byte(payload), &ev) != nil {
			continue
		}
		if ev["type"] == "response.completed" || ev["type"] == "response.incomplete" {
			if resp, ok := ev["response"].(map[string]any); ok {
				return resp, nil
			}
		}
	}
	// Some client drivers (the subprocess ones: python/node) report a stream
	// event count rather than shipping every raw frame across the process
	// boundary (client_subprocess.go sets StreamEvents to that many empty
	// placeholder strings), so there is no literal response.completed/
	// incomplete line to scan for above. Those drivers already extract the
	// terminal response body themselves — via the real SDK's own
	// response.completed event — and report it as RawBody in the same shape
	// the non-streaming path parses; fall back to that before giving up.
	var resp map[string]any
	if err := json.Unmarshal(r.RawBody, &resp); err == nil {
		return resp, nil
	}
	return nil, fmt.Errorf("stream has no response.completed/incomplete event")
}

// AssertResponsesItemIDsCanonical returns an Assertion that a Responses
// response synthesized by the gateway (source Responses, target not
// Responses) carries canonical ids: the response id and every output item
// id have the type prefix and 32-hex shape OpenAI validates on replay, ids
// are unique within the response, a function_call's call_id is set and
// distinct from its item id, and in streaming the same item id appears in
// output_item.added, output_item.done and the final response output.
//
// It is a no-op for other pairs: on a Responses passthrough the ids come
// from the upstream, and a non-Responses source never sees these ids.
func AssertResponsesItemIDsCanonical() Assertion {
	return Assertion{
		Name: "responses_item_ids_canonical",
		Check: func(r *RoundTripResult) error {
			if r.SourceProtocol != protocol.TypeOpenAIResponses || r.TargetProtocol == protocol.TypeOpenAIResponses {
				return nil
			}
			final, err := FinalResponsesBody(r)
			if err != nil {
				return err
			}
			if err := checkResponsesIDs(final); err != nil {
				return err
			}
			if !r.IsStreaming {
				return nil
			}

			added := map[string]string{}
			done := map[string]string{}
			sawStreamContent := false
			for _, line := range r.StreamEvents {
				payload := strings.TrimPrefix(line, "data: ")
				var ev map[string]any
				if json.Unmarshal([]byte(payload), &ev) != nil {
					continue
				}
				sawStreamContent = true
				switch ev["type"] {
				case "response.output_item.added":
					item, _ := ev["item"].(map[string]any)
					added[str(item["id"])] = str(item["type"])
				case "response.output_item.done":
					item, _ := ev["item"].(map[string]any)
					done[str(item["id"])] = str(item["type"])
				}
			}
			if !sawStreamContent {
				// Some client drivers (the subprocess ones: python/node)
				// report a stream event count rather than shipping every raw
				// frame across the process boundary, so there is nothing
				// here to pair added/done against — see the same note on
				// FinalResponsesBody. The id-shape check above already ran
				// against the terminal body, which is everything checkable
				// without raw frames.
				return nil
			}
			for id, typ := range added {
				if _, ok := done[id]; !ok {
					return fmt.Errorf("output_item.added %s %s has no matching output_item.done", typ, id)
				}
			}
			for id := range done {
				if _, ok := added[id]; !ok {
					return fmt.Errorf("output_item.done %s was never added", id)
				}
			}
			for _, o := range outputItems(final) {
				if _, ok := added[str(o["id"])]; !ok {
					return fmt.Errorf("final output item %s %s was not streamed under that id", str(o["type"]), str(o["id"]))
				}
			}
			return nil
		},
	}
}

func checkResponsesIDs(resp map[string]any) error {
	if id := str(resp["id"]); !responsesIDShapes["response"].re.MatchString(id) {
		return fmt.Errorf("response id %q is not %s_<32 hex>", id, responsesIDShapes["response"].prefix)
	}
	seen := map[string]bool{}
	for _, item := range outputItems(resp) {
		typ, id := str(item["type"]), str(item["id"])
		if shape, ok := responsesIDShapes[typ]; ok && !shape.re.MatchString(id) {
			return fmt.Errorf("%s item id %q is not %s_<32 hex>", typ, id, shape.prefix)
		}
		if seen[id] {
			return fmt.Errorf("duplicate output item id %q", id)
		}
		seen[id] = true
		if typ == "function_call" {
			callID := str(item["call_id"])
			if callID == "" || callID == id {
				return fmt.Errorf("function_call %s call_id %q must be set and distinct from the item id", id, callID)
			}
		}
	}
	return nil
}

func outputItems(resp map[string]any) []map[string]any {
	raw, _ := resp["output"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, o := range raw {
		if m, ok := o.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
