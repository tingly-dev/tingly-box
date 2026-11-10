package protocol

import (
	"encoding/json"
	"errors"

	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/tidwall/gjson"
)

// contentPolicyBlockCodes are the upstream error codes for a request blocked
// on content-policy grounds. Codex never retries them
// (codex-rs/protocol/src/error.rs, retry_delay; read at openai/codex
// de6e72addd67), so retrying or failing over only re-sends the flagged
// content (#2417).
var contentPolicyBlockCodes = map[string]bool{
	"cyber_policy":                  true,
	"bio_policy":                    true,
	"misalignment_policy_violation": true,
}

// ContentPolicyBlock is an upstream content-policy block (a moderation
// refusal) that arrived as an in-stream error event. The Codex backend
// answers 200 and then sends {"type":"error","error":{"code":"cyber_policy",
// ...}}; openai-go ends the stream there with *ssestream.StreamError.
type ContentPolicyBlock struct {
	Code    string
	Type    string // the upstream error type, e.g. "invalid_request"
	Message string // client-safe: truncated like every relayed vendor message
	// Misalignment is the upstream error's "misalignment" object, which Codex
	// shows for misalignment_policy_violation. Nil when absent.
	Misalignment json.RawMessage
}

// AsContentPolicyBlock reports whether err is an upstream content-policy
// block. errors.As sees through the wrappers on the way to the client edge
// (CodexClient's fmt.Errorf("stream error: %w"), EmptyResponseError). Any
// other error, including an in-stream error with a different code, is not a
// block and keeps its current handling.
func AsContentPolicyBlock(err error) (ContentPolicyBlock, bool) {
	var se *ssestream.StreamError
	if !errors.As(err, &se) {
		return ContentPolicyBlock{}, false
	}
	upstream := gjson.GetBytes(se.Event.Data, "error")
	code := upstream.Get("code").String()
	if !contentPolicyBlockCodes[code] {
		return ContentPolicyBlock{}, false
	}
	block := ContentPolicyBlock{
		Code:    code,
		Type:    upstream.Get("type").String(),
		Message: truncateClientDetail(upstream.Get("message").String()),
	}
	if m := upstream.Get("misalignment"); m.IsObject() {
		block.Misalignment = json.RawMessage(m.Raw)
	}
	return block, true
}

// ContentPolicyBlockError is what usage tracking and health monitoring see
// for a block. Its text carries only the code: ReportHealthStatus matches
// substrings such as "429" or "403" in err.Error(), and a vendor message
// must not mark the account rate-limited or unauthorized. Unwrap keeps the
// original error, so the server log (UpstreamLogDetail) still has it.
type ContentPolicyBlockError struct {
	Code string
	Err  error
}

func (e *ContentPolicyBlockError) Error() string {
	return "upstream blocked the request by content policy (code=" + e.Code + ")"
}

func (e *ContentPolicyBlockError) Unwrap() error { return e.Err }
