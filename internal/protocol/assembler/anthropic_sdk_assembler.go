package assembler

import "github.com/anthropics/anthropic-sdk-go"

// AnthropicSDKAssembler wraps the SDK's Message.Accumulate method
// providing a unified interface for stream accumulation.
type AnthropicSDKAssembler struct {
	msg anthropic.Message
}

// NewAnthropicSDKAssembler creates a new assembler using the SDK's accumulate pattern.
func NewAnthropicSDKAssembler() *AnthropicSDKAssembler {
	return &AnthropicSDKAssembler{}
}

// Accumulate processes a stream event using the SDK's built-in accumulation logic.
// Returns an error if accumulation fails.
func (a *AnthropicSDKAssembler) Accumulate(event anthropic.MessageStreamEventUnion) error {
	return a.msg.Accumulate(event)
}

// Finish returns the accumulated Message.
func (a *AnthropicSDKAssembler) Finish() *anthropic.Message {
	return &a.msg
}

// Result returns the internal message for direct access if needed.
func (a *AnthropicSDKAssembler) Result() *anthropic.Message {
	return &a.msg
}

// AnthropicBetaSDKAssembler wraps the SDK's BetaMessage.Accumulate method
// for v1 beta API streams.
type AnthropicBetaSDKAssembler struct {
	msg anthropic.BetaMessage
}

// NewAnthropicBetaSDKAssembler creates a new assembler for v1 beta streams.
func NewAnthropicBetaSDKAssembler() *AnthropicBetaSDKAssembler {
	return &AnthropicBetaSDKAssembler{}
}

// Accumulate processes a v1 beta stream event using the SDK's accumulation logic.
func (a *AnthropicBetaSDKAssembler) Accumulate(event anthropic.BetaRawMessageStreamEventUnion) error {
	return a.msg.Accumulate(event)
}

// Finish returns the accumulated BetaMessage.
func (a *AnthropicBetaSDKAssembler) Finish() *anthropic.BetaMessage {
	return &a.msg
}

// Result returns the internal message for direct access if needed.
func (a *AnthropicBetaSDKAssembler) Result() *anthropic.BetaMessage {
	return &a.msg
}
