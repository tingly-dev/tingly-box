package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// responsesToChatState tracks the streaming conversion state from Responses API to Chat Completions
type responsesToChatState struct {
	chatID          string
	createdAt       int64
	accumulated     strings.Builder
	inputTokens     int64
	outputTokens    int64
	cacheTokens     int64
	reasoningTokens int64
	totalTokens     int64
	hasSentCreated  bool
	hasToolCalls    bool
	completed       bool
	toolCallIndexes map[string]int
	toolCalls       map[int]*responsesToChatToolCall
}

type responsesToChatToolCall struct {
	id        string
	callID    string
	name      string
	arguments strings.Builder
}

// HandleResponsesToOpenAIChatStream converts Responses API streaming to Chat Completions format.
// Returns TokenUsage containing token usage information for tracking.
func HandleResponsesToOpenAIChatStream(
	hc *protocol.HandleContext,
	stream ResponsesStreamIter,
	responseModel string,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	logrus.WithContext(c.Request.Context()).Debug("Starting Responses to Chat streaming conversion handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Responses to Chat streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.WithContext(c.Request.Context()).Errorf("Error closing Responses stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished Responses to Chat streaming conversion handler")
	}()

	// Set SSE headers for Chat Completions
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported by this connection")
	}

	state := &responsesToChatState{
		chatID:          fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		createdAt:       time.Now().Unix(),
		toolCallIndexes: make(map[string]int),
		toolCalls:       make(map[int]*responsesToChatToolCall),
	}

	// Trigger stream event hook
	for _, hook := range hc.OnStreamEventHooks {
		if err := hook(nil); err != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Stream event hook error: %v", err)
		}
	}

	// Process the stream
	StreamLoop(c, func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("Client disconnected, stopping Responses to Chat stream")
			return false
		default:
		}

		if !stream.Next() {
			return false
		}

		evt := stream.Current()

		switch evt.Type {
		case "response.created":
			state.chatID = evt.Response.ID
			if !state.hasSentCreated {
				writeResponsesToChatRoleChunk(c, flusher, state, responseModel)
			}

		case "response.output_text.delta":
			if !state.hasSentCreated {
				writeResponsesToChatRoleChunk(c, flusher, state, responseModel)
			}
			state.accumulated.WriteString(evt.Delta)
			writeResponsesToChatTextChunk(c, flusher, state, responseModel, evt.Delta)

		case "response.output_text.done":
			// Text output is complete - handled in response.completed

		case "response.output_item.added":
			if evt.Item.Type == "function_call" {
				if !state.hasSentCreated {
					writeResponsesToChatRoleChunk(c, flusher, state, responseModel)
				}
				index := int(evt.OutputIndex)
				callID := evt.Item.CallID
				if callID == "" {
					callID = evt.Item.ID
				}
				state.toolCallIndexes[evt.Item.ID] = index
				state.toolCalls[index] = &responsesToChatToolCall{id: evt.Item.ID, callID: callID, name: evt.Item.Name}
				state.hasToolCalls = true
				writeResponsesToChatToolCallStart(c, flusher, state, responseModel, index, callID, evt.Item.Name)
			}

		case "response.function_call_arguments.delta":
			if !state.hasSentCreated {
				writeResponsesToChatRoleChunk(c, flusher, state, responseModel)
			}
			index, ok := state.toolCallIndexes[evt.ItemID]
			if !ok {
				index = int(evt.OutputIndex)
			}
			if toolCall, ok := state.toolCalls[index]; ok {
				toolCall.arguments.WriteString(evt.Delta)
			}
			state.hasToolCalls = true
			writeResponsesToChatToolCallDelta(c, flusher, state, responseModel, index, evt.Delta)

		case "response.function_call_arguments.done":
			index, ok := state.toolCallIndexes[evt.ItemID]
			if !ok {
				index = int(evt.OutputIndex)
			}
			if toolCall, ok := state.toolCalls[index]; ok && toolCall.arguments.Len() == 0 {
				toolCall.arguments.WriteString(evt.Arguments)
			}

		case "response.output_item.done":
			if evt.Item.Type == "function_call" {
				index, ok := state.toolCallIndexes[evt.Item.ID]
				if !ok {
					index = int(evt.OutputIndex)
				}
				callID := evt.Item.CallID
				if callID == "" {
					callID = evt.Item.ID
				}
				toolCall, ok := state.toolCalls[index]
				if !ok {
					toolCall = &responsesToChatToolCall{id: evt.Item.ID}
					state.toolCalls[index] = toolCall
				}
				toolCall.id = evt.Item.ID
				toolCall.callID = callID
				toolCall.name = evt.Item.Name
				if toolCall.arguments.Len() == 0 {
					toolCall.arguments.WriteString(evt.Item.Arguments.OfString)
				}
				state.hasToolCalls = true
			}

		case "response.completed":
			state.cacheTokens = evt.Response.Usage.InputTokensDetails.CachedTokens
			state.inputTokens = evt.Response.Usage.InputTokens - state.cacheTokens
			state.outputTokens = evt.Response.Usage.OutputTokens
			state.reasoningTokens = evt.Response.Usage.OutputTokensDetails.ReasoningTokens
			if evt.Response.Usage.TotalTokens != 0 {
				state.totalTokens = evt.Response.Usage.TotalTokens
			} else {
				state.totalTokens = state.inputTokens + state.outputTokens
			}
			flushResponsesToChatCompletedOutput(c, flusher, state, responseModel, evt.Response.Output)
			finishReason := "stop"
			if state.hasToolCalls {
				finishReason = openaiFinishReasonToolCalls
			}
			writeResponsesToChatFinalChunk(c, flusher, state, responseModel, finishReason, !hc.DisableStreamUsage)
			state.completed = true

		case "error":
			writeSSEChunk(c, flusher, wire.ChatStreamErrorChunk{
				Error: wire.ChatStreamError{
					Message: evt.Message,
					Type:    "error",
					Code:    evt.Param,
				},
			})
			return false
		}

		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Responses to Chat stream canceled by client")
			return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), err
		}
		logrus.WithContext(c.Request.Context()).Errorf("Responses to Chat stream error: %v", err)
		// Stream failed before any content reached the client: surface a
		// retryable 5xx so mid-request failover can try the next tier.
		if !c.Writer.Written() {
			SendStreamingError(c, err)
		}
		return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), err
	}

	if !state.completed {
		if !state.hasSentCreated {
			writeResponsesToChatRoleChunk(c, flusher, state, responseModel)
		}
		finishReason := "stop"
		if state.hasToolCalls {
			finishReason = openaiFinishReasonToolCalls
		}
		writeResponsesToChatFinalChunk(c, flusher, state, responseModel, finishReason, !hc.DisableStreamUsage)
	}

	// Send final [DONE] message
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()

	return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), nil
}

func writeResponsesToChatRoleChunk(c *gin.Context, flusher http.Flusher, state *responsesToChatState, responseModel string) {
	chunk := newResponsesToChatChunk(state, responseModel, wire.ChatStreamDelta{
		Role: "assistant",
	}, nil)
	writeSSEChunk(c, flusher, chunk)
	state.hasSentCreated = true
}

func writeResponsesToChatTextChunk(c *gin.Context, flusher http.Flusher, state *responsesToChatState, responseModel, delta string) {
	chunk := newResponsesToChatChunk(state, responseModel, wire.ChatStreamDelta{
		Content: delta,
	}, nil)
	writeSSEChunk(c, flusher, chunk)
}

func writeResponsesToChatToolCallStart(c *gin.Context, flusher http.Flusher, state *responsesToChatState, responseModel string, index int, id, name string) {
	arguments := ""
	chunk := newResponsesToChatChunk(state, responseModel, wire.ChatStreamDelta{
		ToolCalls: []wire.ChatStreamToolCall{
			{
				Index: index,
				ID:    id,
				Type:  "function",
				Function: wire.ChatStreamToolFunction{
					Name:      name,
					Arguments: &arguments,
				},
			},
		},
	}, nil)
	writeSSEChunk(c, flusher, chunk)
}

func writeResponsesToChatToolCallDelta(c *gin.Context, flusher http.Flusher, state *responsesToChatState, responseModel string, index int, delta string) {
	chunk := newResponsesToChatChunk(state, responseModel, wire.ChatStreamDelta{
		ToolCalls: []wire.ChatStreamToolCall{
			{
				Index: index,
				Function: wire.ChatStreamToolFunction{
					Arguments: &delta,
				},
			},
		},
	}, nil)
	writeSSEChunk(c, flusher, chunk)
}

func flushResponsesToChatCompletedOutput(c *gin.Context, flusher http.Flusher, state *responsesToChatState, responseModel string, output []responses.ResponseOutputItemUnion) {
	if state.accumulated.Len() > 0 || len(state.toolCalls) > 0 {
		return
	}
	for outputIndex, item := range output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					if !state.hasSentCreated {
						writeResponsesToChatRoleChunk(c, flusher, state, responseModel)
					}
					state.accumulated.WriteString(content.Text)
					writeResponsesToChatTextChunk(c, flusher, state, responseModel, content.Text)
				}
			}
		case "function_call":
			if !state.hasSentCreated {
				writeResponsesToChatRoleChunk(c, flusher, state, responseModel)
			}
			index := outputIndex
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			state.hasToolCalls = true
			writeResponsesToChatToolCallStart(c, flusher, state, responseModel, index, callID, item.Name)
			if item.Arguments.OfString != "" {
				writeResponsesToChatToolCallDelta(c, flusher, state, responseModel, index, item.Arguments.OfString)
			}
		}
	}
}

func writeResponsesToChatFinalChunk(c *gin.Context, flusher http.Flusher, state *responsesToChatState, responseModel, finishReason string, includeUsage bool) {
	finalChunk := newResponsesToChatChunk(state, responseModel, wire.ChatStreamDelta{}, &finishReason)
	if includeUsage {
		totalInput := state.inputTokens + state.cacheTokens
		total := state.totalTokens
		if total == 0 {
			total = totalInput + state.outputTokens
		}
		usage := &wire.ChatStreamUsage{
			PromptTokens:     totalInput,
			CompletionTokens: state.outputTokens,
			TotalTokens:      total,
		}
		if state.cacheTokens != 0 {
			usage.PromptTokensDetails = &wire.ChatStreamPromptTokenDetails{
				CachedTokens: state.cacheTokens,
			}
		}
		if state.reasoningTokens != 0 {
			usage.CompletionTokensDetails = &wire.ChatStreamOutputTokenDetails{
				ReasoningTokens: state.reasoningTokens,
			}
		}
		finalChunk.Usage = usage
	}
	writeSSEChunk(c, flusher, finalChunk)
}

func newResponsesToChatChunk(state *responsesToChatState, responseModel string, delta wire.ChatStreamDelta, finishReason *string) wire.ChatStreamChunk {
	return wire.ChatStreamChunk{
		ID:      state.chatID,
		Object:  "chat.completion.chunk",
		Created: state.createdAt,
		Model:   responseModel,
		Choices: []wire.ChatStreamChoice{
			{
				Index:        0,
				Delta:        delta,
				FinishReason: finishReason,
			},
		},
	}
}

// writeSSEChunk writes a single SSE chunk
func writeSSEChunk(c *gin.Context, flusher http.Flusher, chunk any) {
	jsonBytes, err := json.Marshal(chunk)
	if err != nil {
		logrus.WithContext(c.Request.Context()).Errorf("Failed to marshal chunk: %v", err)
		return
	}
	c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(jsonBytes))))
	flusher.Flush()
}
