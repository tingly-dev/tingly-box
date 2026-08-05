package assembler

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestNewStreamAssemblerRejectsUnsupportedProtocol(t *testing.T) {
	assembler, err := NewStreamAssembler("")
	require.Nil(t, assembler)
	require.ErrorContains(t, err, "unsupported protocol")

	assembler, err = NewStreamAssembler(protocol.TypeGoogle)
	require.Nil(t, assembler)
	require.ErrorContains(t, err, "google")
}

func TestCommonStreamAssemblerAnthropicV1(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeAnthropicV1)
	require.NoError(t, err)

	require.NoError(t, assembler.Add(anthropic.MessageStreamEventUnion{
		Type: "message_start",
		Message: anthropic.Message{
			ID:    "msg-v1",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-v1",
		},
	}))

	result, err := assembler.Finish()
	require.NoError(t, err)
	message, ok := result.(*anthropic.Message)
	require.True(t, ok)
	require.Equal(t, "msg-v1", string(message.ID))
}

func TestCommonStreamAssemblerAnthropicBetaFromJSON(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeAnthropicBeta)
	require.NoError(t, err)

	event := json.RawMessage(`{
		"type":"message_start",
		"message":{
			"id":"msg-beta",
			"type":"message",
			"role":"assistant",
			"content":[],
			"model":"claude-beta",
			"stop_reason":null,
			"stop_sequence":null,
			"usage":{"input_tokens":1,"output_tokens":0}
		}
	}`)
	require.NoError(t, assembler.Add(event))

	result, err := assembler.Finish()
	require.NoError(t, err)
	message, ok := result.(*anthropic.BetaMessage)
	require.True(t, ok)
	require.Equal(t, "msg-beta", string(message.ID))
}

func TestCommonStreamAssemblerAnthropicBetaFromConvertedEvent(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeAnthropicBeta)
	require.NoError(t, err)

	event := protocolstream.AnthropicEvent{
		Type: "message_start",
		Data: map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg-converted-beta", "type": "message", "role": "assistant",
				"content": []any{}, "model": "claude-beta", "stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{"input_tokens": 1, "output_tokens": 0},
			},
		},
	}
	require.NoError(t, assembler.Add(event))

	result, err := assembler.Finish()
	require.NoError(t, err)
	message, ok := result.(*anthropic.BetaMessage)
	require.True(t, ok)
	require.Equal(t, "msg-converted-beta", string(message.ID))
}

func TestCommonStreamAssemblerOpenAIChatAcceptsWireDTO(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeOpenAIChat)
	require.NoError(t, err)

	require.NoError(t, assembler.Add(wire.ChatStreamChunk{
		ID:      "chat-1",
		Object:  "chat.completion.chunk",
		Created: 1,
		Model:   "public-model",
		Choices: []wire.ChatStreamChoice{{
			Index: 0,
			Delta: wire.ChatStreamDelta{Role: "assistant", Content: "hello "},
		}},
	}))
	stop := "stop"
	require.NoError(t, assembler.Add(wire.ChatStreamChunk{
		ID:     "chat-1",
		Object: "chat.completion.chunk",
		Model:  "public-model",
		Choices: []wire.ChatStreamChoice{{
			Index:        0,
			Delta:        wire.ChatStreamDelta{Content: "world"},
			FinishReason: &stop,
		}},
	}))

	result, err := assembler.Finish()
	require.NoError(t, err)
	completion, ok := result.(*openai.ChatCompletion)
	require.True(t, ok)
	require.Equal(t, "chat-1", completion.ID)
	require.Equal(t, "public-model", completion.Model)
	require.Equal(t, "hello world", completion.Choices[0].Message.Content)
	require.True(t, assembler.Terminal())
}

func TestCommonStreamAssemblerOpenAIChatRejectsMismatchedIDs(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeOpenAIChat)
	require.NoError(t, err)
	require.NoError(t, assembler.Add(wire.ChatStreamChunk{ID: "chat-a", Object: "chat.completion.chunk"}))
	require.Error(t, assembler.Add(wire.ChatStreamChunk{ID: "chat-b", Object: "chat.completion.chunk"}))
}

func TestCommonStreamAssemblerOpenAIChatWaitsForEveryChoice(t *testing.T) {
	request := &openai.ChatCompletionNewParams{N: param.NewOpt(int64(2))}
	assembler, err := NewStreamAssemblerForRequest(protocol.TypeOpenAIChat, request)
	require.NoError(t, err)
	stop := "stop"
	require.NoError(t, assembler.Add(wire.ChatStreamChunk{
		ID: "chat-multi", Object: "chat.completion.chunk",
		Choices: []wire.ChatStreamChoice{{
			Index: 0, FinishReason: &stop,
		}},
	}))
	require.False(t, assembler.Terminal())
	require.NoError(t, assembler.Add(wire.ChatStreamChunk{
		ID: "chat-multi", Object: "chat.completion.chunk",
		Choices: []wire.ChatStreamChoice{{Index: 2, FinishReason: &stop}, {Index: 3, FinishReason: &stop}},
	}))
	require.False(t, assembler.Terminal(), "out-of-range choice indexes must not complete the stream")
	require.NoError(t, assembler.Add(wire.ChatStreamChunk{
		ID: "chat-multi", Object: "chat.completion.chunk",
		Choices: []wire.ChatStreamChoice{{
			Index: 1, FinishReason: &stop,
		}},
	}))
	require.True(t, assembler.Terminal())
}

func TestCommonStreamAssemblerOpenAIChatDoesNotPreallocateUntrustedN(t *testing.T) {
	request := &openai.ChatCompletionNewParams{N: param.NewOpt(int64(^uint64(0) >> 1))}
	assembler, err := NewStreamAssemblerForRequest(protocol.TypeOpenAIChat, request)
	require.NoError(t, err)
	stop := "stop"
	require.NoError(t, assembler.Add(wire.ChatStreamChunk{
		ID: "chat-large-n", Object: "chat.completion.chunk",
		Choices: []wire.ChatStreamChoice{{Index: 0, FinishReason: &stop}},
	}))
	require.False(t, assembler.Terminal())
}

func TestCommonStreamAssemblerAnthropicRequiresMessageStart(t *testing.T) {
	for _, api := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		t.Run(string(api), func(t *testing.T) {
			assembler, err := NewStreamAssembler(api)
			require.NoError(t, err)
			require.NoError(t, assembler.Add(json.RawMessage(`{"type":"message_stop"}`)))
			require.False(t, assembler.Terminal())
		})
	}
}

func TestCommonStreamAssemblerOpenAIResponsesAcceptsWireDTO(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeOpenAIResponses)
	require.NoError(t, err)

	require.NoError(t, assembler.Add(wire.ResponsesCreatedEvent{
		Type: "response.created",
		Response: wire.ResponsesWireResponse{
			ID:     "resp-1",
			Object: "response",
			Status: "in_progress",
			Model:  "public-model",
		},
	}))
	require.NoError(t, assembler.Add(wire.ResponsesOutputTextDeltaEvent{
		Type:         "response.output_text.delta",
		OutputIndex:  0,
		ContentIndex: 0,
		Delta:        "hello responses",
	}))
	require.NoError(t, assembler.Add(wire.ResponsesCompletedEvent{
		Type: "response.completed",
		Response: wire.ResponsesWireResponse{
			ID:     "resp-1",
			Object: "response",
			Status: "completed",
			Model:  "public-model",
			Output: []wire.ResponsesOutputItemWire{},
		},
	}))

	result, err := assembler.Finish()
	require.NoError(t, err)
	response, ok := result.(*responses.Response)
	require.True(t, ok)
	require.Equal(t, "resp-1", response.ID)
	require.Equal(t, responses.ResponsesModel("public-model"), response.Model)
	require.Equal(t, "hello responses", response.OutputText())
}

func TestCommonStreamAssemblerRejectsInvalidEvent(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeOpenAIChat)
	require.NoError(t, err)
	require.ErrorContains(t, assembler.Add([]byte(`not-json`)), "not valid JSON")
	require.ErrorContains(t, assembler.Add(nil), "nil")
}

func TestCommonStreamAssemblerRejectsTypedNilEvent(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeOpenAIChat)
	require.NoError(t, err)
	var event *openai.ChatCompletionChunk
	require.NotPanics(t, func() {
		require.ErrorContains(t, assembler.Add(event), "nil")
	})
}

func TestCommonStreamAssemblerPreservesResponsesFailedPayload(t *testing.T) {
	assembler, err := NewStreamAssembler(protocol.TypeOpenAIResponses)
	require.NoError(t, err)
	require.NoError(t, assembler.Add(json.RawMessage(
		`{"type":"response.failed","sequence_number":1,"response":{"id":"resp-failed","object":"response","status":"failed","model":"provider-model","output":[],"error":{"code":"server_error","message":"provider failed"}}}`,
	)))
	require.True(t, assembler.Terminal())
	require.ErrorContains(t, assembler.TerminalError(), "response.failed")

	result, err := assembler.Finish()
	require.NoError(t, err)
	response, ok := result.(*responses.Response)
	require.True(t, ok)
	require.Equal(t, "resp-failed", response.ID)
	require.Equal(t, responses.ResponseStatusFailed, response.Status)
}
