package protocol

import (
	"encoding/json"
	"github.com/tidwall/gjson"
)

// AnthropicToolInputJSON preserves the original JSON number precision. SDK
// response unions decode Input through interface{}, which can round large IDs.
// A programmatically constructed block has no raw JSON and uses the fallback.
func AnthropicToolInputJSON(rawBlock string, input any) string {
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
