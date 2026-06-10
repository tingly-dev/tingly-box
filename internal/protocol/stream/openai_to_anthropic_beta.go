package stream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// HandleOpenAIToAnthropicBetaStream processes OpenAI streaming events and converts them to Anthropic beta format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIToAnthropicBetaStream(hc *protocol.HandleContext, req *openai.ChatCompletionNewParams, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicBetaStream(hc, req, stream, responseModel, nil)
}

// HandleOpenAIToAnthropicBetaStreamWithMCPHooks enables MCP-aware tool suppression/finalization during conversion.
func HandleOpenAIToAnthropicBetaStreamWithMCPHooks(
	hc *protocol.HandleContext,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	return handleOpenAIToAnthropicBetaStream(hc, req, stream, responseModel, hooks)
}

func handleOpenAIToAnthropicBetaStream(
	hc *protocol.HandleContext,
	req *openai.ChatCompletionNewParams,
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	hooks *OpenAIToAnthropicMCPHooks,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	logrus.WithContext(c.Request.Context()).Debug("Starting OpenAI to Anthropic beta streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in OpenAI to Anthropic beta streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.WithContext(c.Request.Context()).Errorf("Error closing OpenAI stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished OpenAI to Anthropic beta streaming response handler")
	}()

	conv := newOpenAIToAnthropicConverter(stream, responseModel, req, hooks, mapOpenAIFinishReasonToAnthropicBeta)
	_, err := RunConverter(hc, conv, anthropicSSEWriter(c))

	if hookErr := conv.HookErr(); hookErr != nil {
		return conv.Usage(), hookErr
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic beta stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("OpenAI stream error: %v", err)
		hc.DispatchStreamError(err)
		if !conv.MessageStarted() {
			SendStreamingError(c, err)
			return conv.Usage(), err
		}
		sendAnthropicStreamEvent(c, "error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}, nil)
		return conv.Usage(), err
	}
	if streamErr := stream.Err(); streamErr != nil {
		if errors.Is(streamErr, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("OpenAI to Anthropic beta stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("OpenAI stream error: %v", streamErr)
		hc.DispatchStreamError(streamErr)
		if !conv.MessageStarted() {
			SendStreamingError(c, streamErr)
			return conv.Usage(), streamErr
		}
		sendAnthropicStreamEvent(c, "error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": streamErr.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}, nil)
		return conv.Usage(), streamErr
	}
	return conv.Usage(), nil
}

// HandleResponsesToAnthropicBetaStream processes OpenAI Responses API streaming events and converts them to Anthropic beta format.
// Returns TokenUsage containing token usage information for tracking.
func HandleResponsesToAnthropicBetaStream(hc *protocol.HandleContext, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	return handleResponsesToAnthropicStream(hc, stream, responseModel)
}

func HandleResponsesToAnthropicBetaAssembly(c *gin.Context, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	blocks := make(map[int]*anthropic.BetaContentBlockUnion)

	msg := anthropic.BetaMessage{
		Type: constant.Message("message"),
		Role: constant.Assistant("assistant"),
	}

	return handlerResponsesToAnthropicStream(c, stream, responseModel, responsesAPIEventSenders{
		SendMessageStart: func(event map[string]interface{}, flusher http.Flusher) {
			if msgData, ok := event["message"].(map[string]interface{}); ok {
				if id, ok := msgData["id"].(string); ok {
					msg.ID = id
				}
				if model, ok := msgData["model"].(string); ok {
					msg.Model = anthropic.Model(model)
				}
			}
		},
		SendContentBlockStart: func(index int, blockType string, content map[string]interface{}, flusher http.Flusher) {
			block := anthropic.BetaContentBlockUnion{Type: blockType}
			if id, ok := content["id"].(string); ok {
				block.ID = id
			}
			if name, ok := content["name"].(string); ok {
				block.Name = name
			}
			blocks[index] = &block
		},
		SendContentBlockDelta: func(index int, content map[string]interface{}, flusher http.Flusher) {
			block, ok := blocks[index]
			if !ok {
				return
			}
			if deltaType, ok := content["type"].(string); ok {
				switch deltaType {
				case "text_delta":
					if text, ok := content["text"].(string); ok {
						block.Text += text
					}
				case "thinking_delta":
					if thinking, ok := content["thinking"].(string); ok {
						block.Thinking += thinking
					}
				case "input_json_delta":
					if partialJSON, ok := content["partial_json"].(string); ok {
						if block.Input == nil {
							block.Input = json.RawMessage(partialJSON)
						} else {
							block.Input = append(block.Input, partialJSON...)
						}
					}
				}
			}
			blocks[index] = block
		},
		SendContentBlockStop: func(state *streamState, index int, flusher http.Flusher) {
			if block, ok := blocks[index]; ok {
				msg.Content = append(msg.Content, *block)
				delete(blocks, index)
			}
		},
		SendStopEvents: func(state *streamState, flusher http.Flusher) {
			msg.StopReason = anthropic.BetaStopReasonEndTurn
		},
		SendMessageDelta: func(state *streamState, stopReason string, flusher http.Flusher) {
			msg.StopReason = anthropic.BetaStopReason(stopReason)
		},
		SendMessageStop: func(messageID, model string, state *streamState, stopReason string, flusher http.Flusher) {
			msg.ID = messageID
			//TODO: the id is special
			//msg.ID = fmt.Sprintf("msg_%s", uuid.New().String())
			msg.Model = anthropic.Model(model)
			msg.StopReason = anthropic.BetaStopReason(mapOpenAIFinishReasonToAnthropicBeta(stopReason))

			// Set usage
			msg.Usage.InputTokens = state.inputTokens
			msg.Usage.OutputTokens = state.outputTokens
			if state.cacheTokens > 0 {
				msg.Usage.CacheReadInputTokens = state.cacheTokens
			}

			// Send result
			c.JSON(200, wire.AnthropicBetaMessageMap(&msg))
			flusher.Flush()
			return
		},
		SendErrorEvent: func(event map[string]interface{}, flusher http.Flusher) {
			// For error, still try to send what we have
			c.JSON(200, wire.AnthropicBetaMessageMap(&msg))
		},
	})
}

// mapOpenAIFinishReasonToAnthropicBeta converts OpenAI finish_reason to Anthropic beta stop_reason
func mapOpenAIFinishReasonToAnthropicBeta(finishReason string) string {
	switch finishReason {
	case string(openai.CompletionChoiceFinishReasonStop):
		return string(anthropic.BetaStopReasonEndTurn)
	case string(openai.CompletionChoiceFinishReasonLength):
		return string(anthropic.BetaStopReasonMaxTokens)
	case openaiFinishReasonToolCalls, openaiFinishReasonFunctionCall:
		return string(anthropic.BetaStopReasonToolUse)
	case string(openai.CompletionChoiceFinishReasonContentFilter):
		// MENTION: we may use `refusal` but it works badly - then we use end turn as normal
		return string(anthropic.BetaStopReasonEndTurn)
	default:
		return string(anthropic.BetaStopReasonEndTurn)
	}
}
