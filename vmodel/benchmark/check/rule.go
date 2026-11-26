package check

import (
	"encoding/json"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/vmodel/validate"
)

// observation separates normalized semantics from raw wire data. Invalid raw
// bodies remain available as text (e.g. HTTP errors and intentional malformed fixtures).
func observation(r *RoundTripResult) ([]byte, error) {
	var body json.RawMessage
	if json.Valid(r.RawBody) {
		body = r.RawBody
	}
	tools := make([]map[string]any, 0, len(r.ToolCalls))
	for _, t := range r.ToolCalls {
		v := map[string]any{"id": t.ID, "name": t.Name, "raw_arguments": t.Arguments}
		if json.Valid([]byte(t.Arguments)) {
			v["arguments"] = json.RawMessage(t.Arguments)
		}
		tools = append(tools, v)
	}
	// Preserve order of decoded payloads; raw event lines remain independently
	// available, including non-JSON frames and terminal sentinels.
	streamData := make([]json.RawMessage, 0, len(r.StreamEvents))
	for _, line := range r.StreamEvents {
		payload, ok := sse.ParseSSEDataPayload(line)
		if !ok {
			payload = line
		}
		if json.Valid([]byte(payload)) {
			streamData = append(streamData, json.RawMessage(payload))
		}
	}
	doc := map[string]any{"http_status": r.HTTPStatus, "content": r.Content, "role": r.Role, "model": r.Model, "finish_reason": r.FinishReason, "thinking": r.ThinkingContent, "tool_calls": tools, "tool_call_count": len(tools), "raw_body": string(r.RawBody), "stream_events": r.StreamEvents, "stream_data": streamData, "stream_event_count": len(r.StreamEvents), "stream_error": r.StreamError, "stream_completed": r.StreamCompleted, "streaming": r.IsStreaming}
	if body != nil {
		doc["body"] = body
	}
	if r.Usage != nil {
		doc["usage"] = map[string]int{"input_tokens": r.Usage.InputTokens, "output_tokens": r.Usage.OutputTokens}
	}
	return json.Marshal(doc)
}

func AssertRule(rule validate.Rule) Assertion {
	name := rule.Name
	if name == "" {
		name = fmt.Sprintf("rule(%s %s)", rule.Path, rule.Op)
	}
	return Assertion{Name: name, Check: func(r *RoundTripResult) error {
		raw, err := observation(r)
		if err != nil {
			return err
		}
		return rule.Match(raw)
	}}
}

// Evaluate is the common assertion execution entry point for protocol runners.
func Evaluate(r *RoundTripResult, assertions []Assertion) []validate.Failure {
	checks := make([]validate.NamedCheck[*RoundTripResult], len(assertions))
	for i, a := range assertions {
		checks[i] = validate.NamedCheck[*RoundTripResult]{Name: a.Name, Check: a.Check}
	}
	return validate.Evaluate(r, checks)
}

// NamedAssertion exposes domain-specific checks without allowing arbitrary code
// execution from configurations. Pure value comparisons belong in rules.
func NamedAssertion(name string) (Assertion, bool) {
	switch name {
	case "content_non_empty":
		return AssertContentNonEmpty(), true
	case "usage_non_zero":
		return AssertUsageNonZero(), true
	case "usage_propagated":
		return AssertUsagePropagated(), true
	case "responses_item_ids_canonical":
		return AssertResponsesItemIDsCanonical(), true
	default:
		return Assertion{}, false
	}
}
