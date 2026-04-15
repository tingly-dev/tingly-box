package assembler

import "github.com/openai/openai-go/v3"

// OpenAIChatStreamAssembler wraps the SDK's ChatCompletionAccumulator
// providing a unified interface for stream accumulation.
type OpenAIChatStreamAssembler struct {
	acc *openai.ChatCompletionAccumulator
}

// NewOpenAIStreamAssembler creates a new assembler for OpenAI streams
func NewOpenAIStreamAssembler() *OpenAIChatStreamAssembler {
	return &OpenAIChatStreamAssembler{
		acc: &openai.ChatCompletionAccumulator{},
	}
}

// AddChunk incorporates a chunk into the accumulation.
// Chunks must be added in order. Returns false if accumulation failed.
func (a *OpenAIChatStreamAssembler) AddChunk(chunk openai.ChatCompletionChunk) bool {
	return a.acc.AddChunk(chunk)
}

// JustFinishedContent returns the content when it was just completed.
// If the content is just completed, returns (content, true), otherwise ("", false).
func (a *OpenAIChatStreamAssembler) JustFinishedContent() (string, bool) {
	return a.acc.JustFinishedContent()
}

// JustFinishedRefusal returns the refusal when it was just completed.
func (a *OpenAIChatStreamAssembler) JustFinishedRefusal() (string, bool) {
	return a.acc.JustFinishedRefusal()
}

// JustFinishedToolCall returns a tool call when it was just completed.
// Note: Not reliable with ParallelToolCalls enabled.
func (a *OpenAIChatStreamAssembler) JustFinishedToolCall() (openai.FinishedChatCompletionToolCall, bool) {
	return a.acc.JustFinishedToolCall()
}

// Finish returns the accumulated ChatCompletion.
func (a *OpenAIChatStreamAssembler) Finish() *openai.ChatCompletion {
	return &a.acc.ChatCompletion
}

// Result returns the internal accumulator for direct access if needed.
func (a *OpenAIChatStreamAssembler) Result() *openai.ChatCompletionAccumulator {
	return a.acc
}
