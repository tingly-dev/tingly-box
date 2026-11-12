package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/tidwall/gjson"
)

// unmarshalAnthropicIngress validates the classic gateway's top-level input
// contract at the HTTP boundary. Decoding itself is lossless: the v2 SDK fork
// retains every unknown key as an extra, so caller fields survive the whole
// pipeline. Whether a field may be sent upstream is decided at egress, where
// the target wire is known (see internal/client wire egress filtering) —
// not here, where it is not.
func unmarshalAnthropicIngress(data []byte, params interface {
	ExtraFields() map[string]any
	SetExtraFields(map[string]any)
}) error {
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
	return nil
}

// UnmarshalAnthropicCountTokensRequest applies the same ingress validation as
// messages. The endpoint historically decodes the beta shape regardless of the
// query; fields count_tokens does not accept are stripped at egress.
func UnmarshalAnthropicCountTokensRequest(data []byte, params *anthropic.BetaMessageCountTokensParams) error {
	return unmarshalAnthropicIngress(data, params)
}
