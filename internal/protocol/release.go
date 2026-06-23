package protocol

import "github.com/anthropics/anthropic-sdk-go"

// ReleaseAnthropicBetaMessageParams clears Anthropic beta SDK params once the
// caller no longer needs the request. The SDK decoder stores gjson/apijson raw
// JSON metadata inside these params; keeping them reachable after forwarding can
// keep the original request-sized JSON alive after the HTTP request has ended.
func ReleaseAnthropicBetaMessageParams(req *anthropic.BetaMessageNewParams) {
	if req != nil {
		*req = anthropic.BetaMessageNewParams{}
	}
}

// ReleaseAnthropicBetaMessagesRequest clears the parsed beta request wrapper and
// drops the embedded SDK params pointer, so request-end cleanup releases the raw
// JSON metadata retained by the SDK decoder.
func ReleaseAnthropicBetaMessagesRequest(req *AnthropicBetaMessagesRequest) {
	if req == nil {
		return
	}
	ReleaseAnthropicBetaMessageParams(req.BetaMessageNewParams)
	req.BetaMessageNewParams = nil
	req.Stream = false
}
