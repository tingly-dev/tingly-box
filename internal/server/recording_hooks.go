package server

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// CreateRecordingHooks creates hooks for stream handlers based on target API type
// Returns (onStreamEvent, onStreamComplete, onStreamError)
func CreateRecordingHooks(
	recorder *UnifiedRecorder,
	targetAPIType protocol.APIType,
) (onEvent func(event any) error, onComplete func(), onError func(err error)) {

	if recorder == nil {
		return nil, nil, nil
	}

	switch targetAPIType {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return createAnthropicRecordingHooks(recorder, targetAPIType)

	case protocol.TypeOpenAIChat:
		// TODO: Implement OpenAI chat hooks
		return createOpenAIChatRecordingHooks(recorder)

	case protocol.TypeOpenAIResponses:
		// TODO: Implement OpenAI Responses hooks
		return createOpenAIResponsesRecordingHooks(recorder)

	case protocol.TypeGoogle:
		// TODO: Implement Google hooks
		return nil, nil, nil

	default:
		logrus.Warnf("No recording hooks available for target API type: %s", targetAPIType)
		return nil, nil, nil
	}
}

// createAnthropicRecordingHooks creates hooks for Anthropic streaming
func createAnthropicRecordingHooks(
	recorder *UnifiedRecorder,
	targetAPIType protocol.APIType,
) (onEvent func(event any) error, onComplete func(), onError func(err error)) {

	// Get the assembler wrapper
	assemblerWrapper, ok := recorder.assembler.(*anthropicAssemblerWrapper)
	if !ok || assemblerWrapper == nil {
		logrus.Warn("Anthropic assembler not available for recording")
		return nil, nil, nil
	}

	assembler := assemblerWrapper.GetAnthropicAssembler()

	// Track usage from events
	inputTokens := 0
	outputTokens := 0

	onEvent = func(event any) error {
		if assembler == nil {
			return nil
		}

		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			// Record chunk
			chunkJSON := []byte(evt.RawJSON())
			var chunkData map[string]any
			if err := json.Unmarshal(chunkJSON, &chunkData); err == nil {
				recorder.RecordStreamChunk(evt.Type, chunkJSON, chunkData)
			}

			// Feed to assembler
			assembler.RecordV1Event(evt)

			// Track usage
			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
			}

		case *anthropic.BetaRawMessageStreamEventUnion:
			// Record chunk
			chunkJSON := []byte(evt.RawJSON())
			var chunkData map[string]any
			if err := json.Unmarshal(chunkJSON, &chunkData); err == nil {
				recorder.RecordStreamChunk(evt.Type, chunkJSON, chunkData)
			}

			// Feed to assembler
			assembler.RecordV1BetaEvent(evt)

			// Track usage
			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
			}
		}

		return nil
	}

	onComplete = func() {
		// Assemble the final response
		assembled := assemblerWrapper.Finish(recorder.model, inputTokens, outputTokens)
		if assembled != nil {
			recorder.SetAssembledResponse(assembled)
		}
		recorder.SetStatusCode(recorder.GetStatusCode())
		recorder.SetHeaders(recorder.GetResponseHeaders())
		recorder.Finalize()
	}

	onError = func(err error) {
		recorder.SetError(err)
		recorder.Finalize()
	}

	return onEvent, onComplete, onError
}

// createOpenAIChatRecordingHooks creates hooks for OpenAI chat streaming
func createOpenAIChatRecordingHooks(
	recorder *UnifiedRecorder,
) (onEvent func(event any) error, onComplete func(), onError func(err error)) {

	if recorder == nil || recorder.assembler == nil {
		// Return placeholder hooks if no assembler
		onEvent = func(event any) error {
			// Try to record chunks anyway
			if recorder != nil {
				if chunkData, ok := event.(map[string]any); ok {
					data, _ := json.Marshal(chunkData)
					recorder.RecordStreamChunk("chat.completion.chunk", data, chunkData)
				}
			}
			return nil
		}
		onComplete = func() {
			if recorder != nil {
				recorder.Finalize()
			}
		}
		onError = func(err error) {
			if recorder != nil {
				recorder.SetError(err)
				recorder.Finalize()
			}
		}
		return onEvent, onComplete, onError
	}

	assembler, ok := recorder.assembler.(*openAIChatAssemblerWrapper)
	if !ok {
		// Fallback to placeholder
		return createOpenAIChatRecordingHooks(recorder)
	}

	onEvent = func(event any) error {
		if assembler == nil {
			return nil
		}

		switch evt := event.(type) {
		case map[string]any:
			// Handle raw map events (from stream handlers)
			if eventType, ok := evt["type"].(string); ok {
				data, _ := json.Marshal(evt)
				recorder.RecordStreamChunk(eventType, data, evt)
				assembler.AddChunk(eventType, data, evt)
			}
		default:
			// Try to marshal and record
			data, err := json.Marshal(evt)
			if err == nil {
				var parsed map[string]any
				if json.Unmarshal(data, &parsed) == nil {
					if eventType, ok := parsed["type"].(string); ok {
						recorder.RecordStreamChunk(eventType, data, parsed)
						assembler.AddChunk(eventType, data, parsed)
					}
				}
			}
		}
		return nil
	}

	onComplete = func() {
		if assembler != nil && recorder != nil {
			assembled := assembler.GetAssembled()
			if assembled != nil {
				recorder.SetAssembledResponse(assembled)
			}
			recorder.SetStatusCode(recorder.GetStatusCode())
			recorder.SetHeaders(recorder.GetResponseHeaders())
			recorder.Finalize()
		}
	}

	onError = func(err error) {
		if recorder != nil {
			recorder.SetError(err)
			recorder.Finalize()
		}
	}

	return onEvent, onComplete, onError
}

// createOpenAIResponsesRecordingHooks creates hooks for OpenAI Responses streaming
func createOpenAIResponsesRecordingHooks(
	recorder *UnifiedRecorder,
) (onEvent func(event any) error, onComplete func(), onError func(err error)) {

	if recorder == nil || recorder.assembler == nil {
		// Return placeholder hooks if no assembler
		onEvent = func(event any) error {
			// Try to record chunks anyway
			if recorder != nil {
				if chunkData, ok := event.(map[string]any); ok {
					data, _ := json.Marshal(chunkData)
					recorder.RecordStreamChunk("responses.event", data, chunkData)
				}
			}
			return nil
		}
		onComplete = func() {
			if recorder != nil {
				recorder.Finalize()
			}
		}
		onError = func(err error) {
			if recorder != nil {
				recorder.SetError(err)
				recorder.Finalize()
			}
		}
		return onEvent, onComplete, onError
	}

	assembler, ok := recorder.assembler.(*openAIResponsesAssemblerWrapper)
	if !ok {
		// Fallback to placeholder
		return createOpenAIResponsesRecordingHooks(recorder)
	}

	onEvent = func(event any) error {
		if assembler == nil {
			return nil
		}

		switch evt := event.(type) {
		case map[string]any:
			// Handle raw map events (from stream handlers)
			if eventType, ok := evt["type"].(string); ok {
				data, _ := json.Marshal(evt)
				recorder.RecordStreamChunk(eventType, data, evt)
				assembler.AddChunk(eventType, data, evt)
			}
		default:
			// Try to marshal and record
			data, err := json.Marshal(evt)
			if err == nil {
				var parsed map[string]any
				if json.Unmarshal(data, &parsed) == nil {
					if eventType, ok := parsed["type"].(string); ok {
						recorder.RecordStreamChunk(eventType, data, parsed)
						assembler.AddChunk(eventType, data, parsed)
					}
				}
			}
		}
		return nil
	}

	onComplete = func() {
		if assembler != nil && recorder != nil {
			assembled := assembler.GetAssembled()
			if assembled != nil {
				recorder.SetAssembledResponse(assembled)
			}
			recorder.SetStatusCode(recorder.GetStatusCode())
			recorder.SetHeaders(recorder.GetResponseHeaders())
			recorder.Finalize()
		}
	}

	onError = func(err error) {
		if recorder != nil {
			recorder.SetError(err)
			recorder.Finalize()
		}
	}

	return onEvent, onComplete, onError
}
