// Package decision identifies and converts supported decision wire
// formats and resolves known provider endpoints.
package decision

import (
	"fmt"

	"github.com/tidwall/gjson"
)

type Dialect string

const (
	Unknown   Dialect = "unknown"
	OpenAI    Dialect = "openai_decisions"
	SystemOne Dialect = "systemone"
)

// Request recognizes the two published shapes. Unknown permits legacy opaque
// /decisions payloads with no distinguishing fields; callers decide whether
// that fallback is appropriate for their endpoint.
func Request(body []byte) (Dialect, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return Unknown, fmt.Errorf("request must be a JSON object")
	}
	input, state, questions := gjson.GetBytes(body, "input"), gjson.GetBytes(body, "state"), gjson.GetBytes(body, "questions")
	if input.Exists() && state.Exists() {
		return Unknown, fmt.Errorf("request cannot contain both input and state")
	}
	if questions.Exists() && !questions.IsArray() && !questions.IsObject() {
		return Unknown, fmt.Errorf("questions must be an array or object")
	}
	if input.Exists() && questions.IsObject() || state.Exists() && questions.IsArray() {
		return Unknown, fmt.Errorf("input/state and questions use different decision formats")
	}
	if input.Exists() || questions.IsArray() {
		return OpenAI, nil
	}
	if state.Exists() || questions.IsObject() {
		return SystemOne, nil
	}
	return Unknown, nil
}

// Response recognizes answer containers without imposing a provider schema.
func Response(body []byte) Dialect {
	if !gjson.ValidBytes(body) {
		return Unknown
	}
	answers := gjson.GetBytes(body, "answers")
	if answers.IsArray() {
		return OpenAI
	}
	if answers.IsObject() {
		return SystemOne
	}
	return Unknown
}
