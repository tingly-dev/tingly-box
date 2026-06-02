package stream

import "github.com/tingly-dev/tingly-box/internal/protocol/wire"

// Type aliases so existing code within this package compiles without change.
type responsesEvent = wire.ResponsesEvent
type responsesStreamErrorEvent = wire.ResponsesStreamErrorEvent
type responsesStreamErrorBody = wire.ResponsesStreamErrorBody
type responsesCreatedEvent = wire.ResponsesCreatedEvent
type responsesInProgressEvent = wire.ResponsesInProgressEvent
type responsesCompletedEvent = wire.ResponsesCompletedEvent
type responsesWireResponse = wire.ResponsesWireResponse
type responsesUsageWire = wire.ResponsesUsageWire
type responsesInputTokensDetailsWire = wire.ResponsesInputTokensDetailsWire
type responsesOutputTokensDetailsWire = wire.ResponsesOutputTokensDetailsWire
type responsesOutputItemAddedEvent = wire.ResponsesOutputItemAddedEvent
type responsesOutputItemDoneEvent = wire.ResponsesOutputItemDoneEvent
type responsesOutputItemWire = wire.ResponsesOutputItemWire
type responsesContentPartWire = wire.ResponsesContentPartWire
type responsesContentPartAddedEvent = wire.ResponsesContentPartAddedEvent
type responsesContentPartDoneEvent = wire.ResponsesContentPartDoneEvent
type responsesOutputTextDeltaEvent = wire.ResponsesOutputTextDeltaEvent
type responsesOutputTextDoneEvent = wire.ResponsesOutputTextDoneEvent
type responsesFunctionCallArgumentsDeltaEvent = wire.ResponsesFunctionCallArgumentsDeltaEvent
type responsesFunctionCallArgumentsDoneEvent = wire.ResponsesFunctionCallArgumentsDoneEvent
