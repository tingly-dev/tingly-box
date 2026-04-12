package virtualmodel

import (
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// VirtualModel is the protocol-agnostic base interface.
// All virtual model types implement this.
//
// Protocols() declares which API types this model supports. Registry uses this
// at registration time to build a protocol index and validate that declared
// protocols match the implemented sub-interfaces (AnthropicVirtualModel,
// OpenAIChatVirtualModel). Callers use Registry typed getters instead of
// performing type assertions themselves.
type VirtualModel interface {
	GetID() string
	SimulatedDelay() time.Duration
	ToModel() Model

	// Protocols returns the set of API types this model supports.
	// Each declared type must correspond to an implemented sub-interface.
	Protocols() []protocol.APIType
}

// AnthropicVirtualModel handles Anthropic Beta Messages requests.
// Models that support the Anthropic protocol implement this in addition to VirtualModel.
// HandleAnthropicStream must always be implemented; models that are batch-only
// may delegate to DefaultAnthropicStream.
type AnthropicVirtualModel interface {
	VirtualModel
	HandleAnthropic(req *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error)
	HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error
}

// OpenAIChatVirtualModel handles OpenAI Chat Completions requests.
// Models that support the OpenAI Chat protocol implement this in addition to VirtualModel.
// HandleOpenAIChatStream must always be implemented; models that are batch-only
// may delegate to DefaultOpenAIChatStream.
type OpenAIChatVirtualModel interface {
	VirtualModel
	HandleOpenAIChat(req *protocol.OpenAIChatCompletionRequest) (OpenAIChatVModelResponse, error)
	HandleOpenAIChatStream(req *protocol.OpenAIChatCompletionRequest, emit func(any)) error
}

// VModelResponse describes what the virtual model wants to respond (Anthropic protocol).
type VModelResponse struct {
	Content    []anthropic.BetaContentBlockParamUnion
	StopReason anthropic.BetaStopReason
}

// VToolCall is a protocol-agnostic tool call used in OpenAI Chat responses.
// Using an intermediate type avoids coupling to the OpenAI SDK here.
type VToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON-encoded arguments
}

// OpenAIChatVModelResponse describes what the virtual model returns for OpenAI Chat.
type OpenAIChatVModelResponse struct {
	Content      string
	ToolCalls    []VToolCall
	FinishReason string
}
