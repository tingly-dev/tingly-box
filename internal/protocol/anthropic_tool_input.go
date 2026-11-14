package protocol

import (
	"encoding/json"

	"github.com/tidwall/gjson"
)

// AnthropicToolInputJSON preserves JSON number precision. Stream-start unions
// decode Input through any, so use the original wire value in that case. The
// completed-message union uses json.RawMessage: its current field is already
// lossless and may have been updated since the RawJSON snapshot was captured.
func AnthropicToolInputJSON(rawBlock string, input any) string {
	if _, ok := input.(json.RawMessage); ok {
		if raw, err := json.Marshal(input); err == nil && string(raw) != "null" {
			return string(raw)
		}
		return "{}"
	}
	if raw := gjson.Get(rawBlock, "input"); raw.Exists() && raw.Type != gjson.Null {
		return raw.Raw
	}
	if input != nil {
		if raw, err := json.Marshal(input); err == nil {
			return string(raw)
		}
	}
	return "{}"
}
