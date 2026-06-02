package stream

import "github.com/tingly-dev/tingly-box/internal/protocol/wire"

// Type aliases so existing code within this package compiles without change.
type chatCompletionStreamChunk = wire.ChatStreamChunk
type chatCompletionStreamChoice = wire.ChatStreamChoice
type chatCompletionStreamDelta = wire.ChatStreamDelta
type chatCompletionStreamToolCall = wire.ChatStreamToolCall
type chatCompletionStreamToolFunction = wire.ChatStreamToolFunction
type chatCompletionStreamUsage = wire.ChatStreamUsage
type chatCompletionStreamPromptTokenDetails = wire.ChatStreamPromptTokenDetails
type chatCompletionStreamOutputTokenDetails = wire.ChatStreamOutputTokenDetails
type chatCompletionStreamErrorChunk = wire.ChatStreamErrorChunk
type chatCompletionStreamError = wire.ChatStreamError
