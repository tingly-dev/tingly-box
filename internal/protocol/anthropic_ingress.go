package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/tidwall/gjson"
)

// unmarshalAnthropicIngress keeps the classic gateway's top-level input
// contract at the HTTP boundary. The v2 SDK fork retains every unknown key as an extra; only
// the three sampling fields removed from v2 belong on the classic messages
// wire. Declared SDK fields remain untouched, including new typed fields on
// later SDK versions. Explicit extras added by internal callers are unaffected.
func unmarshalAnthropicIngress(data []byte, params interface {
	ExtraFields() map[string]any
	SetExtraFields(map[string]any)
}, sampling bool) error {
	if err := json.Unmarshal(data, params); err != nil {
		return err
	}
	// A failed role-union match can silently discard the entire messages
	// slice. Validate the raw roles before routing, rather than forwarding an
	// empty conversation and reporting a misleading "messages required" error.
	messages := gjson.GetBytes(data, "messages")
	if messages.Exists() && !messages.IsArray() {
		return fmt.Errorf("messages: expected an array")
	}
	for i, message := range messages.Array() {
		role := message.Get("role")
		if role.Type != gjson.String {
			return fmt.Errorf("messages[%d].role: expected user, assistant or system", i)
		}
		switch role.String() {
		case "user", "assistant", "system":
		default:
			return fmt.Errorf("messages[%d].role: unexpected role %q", i, role.String())
		}
	}

	extras := params.ExtraFields()
	for key := range extras {
		if sampling && (key == "temperature" || key == "top_p" || key == "top_k") {
			continue
		}
		delete(extras, key)
	}
	params.SetExtraFields(extras)
	return nil
}

// UnmarshalAnthropicCountTokensRequest applies the same ingress validation as
// messages, without admitting sampling, stream or max_tokens to count_tokens.
// The endpoint historically decodes the beta shape regardless of the query.
func UnmarshalAnthropicCountTokensRequest(data []byte, params *anthropic.BetaMessageCountTokensParams) error {
	return unmarshalAnthropicIngress(data, params, false)
}
