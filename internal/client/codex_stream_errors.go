package client

import (
	"bytes"
	"io"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/tidwall/gjson"
)

// Content-policy blocks inside a Codex stream (#2417).
//
// The ChatGPT backend blocks a request on content-policy grounds by answering
// HTTP 200 and then sending an error event inside the stream:
//
//	{"type":"error","error":{"type":"invalid_request","code":"cyber_policy","message":"..."}}
//
// openai-go reports that event as *ssestream.StreamError, which has no status,
// so the gateway took it for an upstream failure: a 5xx that clients retry and
// that fails over to the next account, which receives the same content. The
// Codex client reports it instead as the error an HTTP-level block produces: a
// 400 *openai.Error with the upstream code and message. The gateway then
// handles it like any upstream 400 (no retry, no failover, no breaker
// failure) without knowing the codes.

// codexSSEContentType is the Content-Type codexRoundTripper sets on a Codex
// SSE response so that ssestream.NewDecoder selects codexEventDecoder.
// openai-go creates the decoder inside Responses.NewStreaming from the
// response Content-Type, so a registered decoder is the hook that needs no
// copy of NewStreaming, and one header covers every Codex stream (streaming,
// the non-streaming assembly, image generation, and an image edit with a mask;
// an edit without a mask uses the plain-JSON images/edits endpoint). The
// registry matches the whole value, parameter included, so no other response
// is affected. TestCodexStreamDecoderSelected pins this wiring: if the value
// stops matching, the SDK falls back to its plain decoder without an error.
const codexSSEContentType = "text/event-stream; tingly-codex=1"

func init() {
	ssestream.RegisterDecoder(codexSSEContentType, newCodexEventDecoder)
}

// codexPolicyBlockCodes are the content-policy codes Codex CLI treats as
// terminal (codex-rs/protocol/src/error.rs, retry_delay, at openai/codex
// de6e72addd67); cyber_policy and bio_policy have been seen in-stream (#2417).
// Retrying or failing over re-sends the blocked content, so they are reported
// as a non-retryable 400.
var codexPolicyBlockCodes = map[string]bool{
	"cyber_policy":                  true,
	"bio_policy":                    true,
	"misalignment_policy_violation": true,
}

// codexEventDecoder wraps the SDK's text/event-stream decoder. It passes every
// event through unchanged and ends the stream at a content-policy block,
// reporting the block from Err().
type codexEventDecoder struct {
	ssestream.Decoder
	err error
}

func newCodexEventDecoder(body io.ReadCloser) ssestream.Decoder {
	return &codexEventDecoder{Decoder: ssestream.NewDecoder(&http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   body,
	})}
}

func (d *codexEventDecoder) Next() bool {
	if d.err != nil || !d.Decoder.Next() {
		return false
	}
	if err := codexPolicyBlock(d.Decoder.Event().Data); err != nil {
		d.err = err
		return false
	}
	return true
}

func (d *codexEventDecoder) Err() error {
	if d.err != nil {
		return d.err
	}
	return d.Decoder.Err()
}

// codexPolicyBlock returns the error for a content-policy block event, or nil
// for any other event. The backend nests the error object, the shape seen in
// production. The flat shape of the documented Responses error event, with the
// code at the top level, is accepted as well. A response.failed event is left
// alone: it already ends the stream, and Responses clients read its code.
func codexPolicyBlock(data []byte) error {
	// Every code contains "_policy"; this keeps the frequent delta events away
	// from the JSON lookups below.
	if !bytes.Contains(data, []byte("_policy")) {
		return nil
	}
	if nested := gjson.GetBytes(data, "error"); nested.IsObject() {
		return newCodexPolicyBlockError(nested.Get("code").String(), nested.Raw, false)
	}
	if gjson.GetBytes(data, "type").String() == "error" {
		return newCodexPolicyBlockError(gjson.GetBytes(data, "code").String(), string(bytes.TrimSpace(data)), true)
	}
	return nil
}

func newCodexPolicyBlockError(code, raw string, flat bool) error {
	if !codexPolicyBlockCodes[code] {
		return nil
	}
	api := &openai.Error{}
	_ = api.UnmarshalJSON([]byte(raw))
	api.StatusCode = http.StatusBadRequest
	api.Code = code
	if flat {
		// The flat event's "type" is the event name, not an error type.
		api.Type = ""
	}
	return &codexPolicyBlockError{api: api}
}

// codexPolicyBlockError is a content-policy block reported as an upstream 400:
// errors.As finds the *openai.Error, so status mapping, the client message and
// the server log treat it like any upstream 400. Error() adds the code but not
// the upstream message, because usage tracking stores Error() as error_code
// and ReportHealthStatus matches substrings such as "429" in it. The
// *openai.Error has no Request or Response (the decoder sees only the body),
// so its DumpRequest and DumpResponse must not be called.
type codexPolicyBlockError struct{ api *openai.Error }

func (e *codexPolicyBlockError) Error() string {
	return e.api.Error() + " (code=" + e.api.Code + ", in-stream)"
}

func (e *codexPolicyBlockError) Unwrap() error { return e.api }
