package server

import (
	"encoding/json"

	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Per-attempt request cloning for mid-request failover.
//
// The pre-transform chain, guardrails, and the protocol transform all mutate
// the request *in place* (the TransformContext wraps a pointer to the params),
// so a request is only safe for one pass. When failover is possible (more than
// one active service) the handler snapshots a pristine template once and each
// attempt clones a fresh request from it, so a retry re-transforms from clean
// input rather than re-transforming already-transformed bytes.
//
// Cloning goes through the SDK's own JSON marshal/unmarshal rather than a struct
// copy: the param structs hold internal extraFields / raw-JSON maps that a plain
// copy would alias. JSON round-trip preserves both those extras and the
// param.Opt "null vs omitted vs present" tri-state.

// CloneAnthropicV1Request rebuilds an Anthropic v1 request from a marshalled
// template (produced by AnthropicMessagesRequest.MarshalJSON).
func CloneAnthropicV1Request(template []byte) (*protocol.AnthropicMessagesRequest, error) {
	var r = &protocol.AnthropicMessagesRequest{}
	err := json.Unmarshal(template, &r)
	return r, err
}

// CloneAnthropicBetaRequest rebuilds an Anthropic beta request from a marshalled
// template (produced by AnthropicBetaMessagesRequest.MarshalJSON).
func CloneAnthropicBetaRequest(template []byte) (*protocol.AnthropicBetaMessagesRequest, error) {
	var r = &protocol.AnthropicBetaMessagesRequest{}
	err := json.Unmarshal(template, &r)
	return r, err
}

// CloneOpenAIChatRequest rebuilds an OpenAI chat request from a marshalled
// template (produced by OpenAIChatCompletionRequest.MarshalJSON).
func CloneOpenAIChatRequest(template []byte) (*protocol.OpenAIChatCompletionRequest, error) {
	var r = &protocol.OpenAIChatCompletionRequest{}
	err := json.Unmarshal(template, &r)
	return r, err
}

// CloneResponsesParams clones the typed responses.ResponseNewParams. It marshals
// and unmarshals the SDK struct directly — NOT through ResponseCreateRequest,
// whose UnmarshalJSON re-runs PreprocessInputData (input-item type injection)
// that has already been applied to the template.
func CloneResponsesParams(template *responses.ResponseNewParams) (*responses.ResponseNewParams, error) {
	b, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}
	var out responses.ResponseNewParams
	err = json.Unmarshal(b, &out)
	return &out, err
}
