// Package openai provides OpenAI-protocol virtual models.
//
// All concrete virtual models in this package implement VirtualModel —
// a base vmodel.VirtualModel extended with HandleOpenAIChat /
// HandleOpenAIChatStream. Models are stored in a Registry that is scoped
// to this protocol; lookups never cross over to Anthropic.
package openai

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// VirtualModel handles OpenAI Chat Completions requests.
type VirtualModel interface {
	vmodel.VirtualModel
	HandleOpenAIChat(req *protocol.OpenAIChatCompletionRequest) (VModelResponse, error)
	HandleOpenAIChatStream(req *protocol.OpenAIChatCompletionRequest, emit func(any)) error
}

// VToolCall is a protocol-agnostic tool call returned by OpenAI Chat models.
// Using an intermediate type avoids coupling to the OpenAI SDK in this package;
// callers (e.g. virtualserver) translate it to the SDK form when serializing.
type VToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON-encoded arguments
}
