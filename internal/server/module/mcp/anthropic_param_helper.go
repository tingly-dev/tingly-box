package mcp

import "encoding/json"

func unmarshalAnthropicParamPreservingRawJSON[T any](raw string) (T, bool) {
	var zero T
	if raw == "" {
		return zero, false
	}
	var param T
	if err := json.Unmarshal([]byte(raw), &param); err != nil {
		return zero, false
	}
	return param, true
}
