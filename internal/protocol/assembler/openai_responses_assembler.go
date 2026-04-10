package assembler

import (
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3/responses"
)

// ResponsesAssembler accumulates OpenAI Responses API streaming events.
// It focuses on core functionality: text accumulation, tool calls, and final response construction.
// Inspired by internal/protocol/stream/anthropic_to_openai_responses.go
type ResponsesAssembler struct {
	// Final response when completed
	response *responses.Response

	// Accumulated state
	responseID      string
	itemID          string
	outputIndex     int
	contentIndex    int
	accumulatedText string
	currentText     string
	currentRefusal  string
	status          string
	sequenceNumber  int
	createdAt       int64
	finished        bool

	// Tool call tracking
	pendingToolCalls map[int]*pendingResponseToolCall
}

// pendingResponseToolCall tracks a tool call being assembled from stream chunks
type pendingResponseToolCall struct {
	itemID    string
	name      string
	arguments strings.Builder
}

// NewResponsesAssembler creates a new Responses API stream assembler.
func NewResponsesAssembler() *ResponsesAssembler {
	return &ResponsesAssembler{
		status:           "queued",
		pendingToolCalls: make(map[int]*pendingResponseToolCall),
		createdAt:        time.Now().Unix(),
	}
}

// NewResponsesAssemblerWithID creates a new assembler with specific IDs (useful for testing)
func NewResponsesAssemblerWithID(responseID, itemID string) *ResponsesAssembler {
	return &ResponsesAssembler{
		responseID:       responseID,
		itemID:           itemID,
		status:           "queued",
		pendingToolCalls: make(map[int]*pendingResponseToolCall),
		createdAt:        time.Now().Unix(),
	}
}

// nextSequenceNumber returns the next sequence number and increments it
func (a *ResponsesAssembler) nextSequenceNumber() int {
	seq := a.sequenceNumber
	a.sequenceNumber++
	return seq
}

// Accumulate processes a Responses API stream event.
// Returns true if the event was handled, false otherwise.
func (a *ResponsesAssembler) Accumulate(event responses.ResponseStreamEventUnion) bool {
	switch event.Type {
	case "response.created":
		a.status = "created"
		a.responseID = event.Response.ID
		return true

	case "response.in_progress":
		a.status = "in_progress"
		return true

	// Output text delta events
	case "response.output_text.delta":
		a.currentText += event.Delta
		a.accumulatedText += event.Delta
		return true

	case "response.output_text.done":
		// Text is complete, stored in accumulatedText
		return true

	// Function call events (tool calls)
	case "response.function_call_arguments.delta":
		if event.OutputIndex >= 0 && int(event.OutputIndex) < len(a.pendingToolCalls) {
			if pending, exists := a.pendingToolCalls[int(event.OutputIndex)]; exists {
				pending.arguments.WriteString(event.Arguments)
			}
		}
		return true

	case "response.function_call_arguments.done":
		if event.OutputIndex >= 0 && int(event.OutputIndex) < len(a.pendingToolCalls) {
			if pending, exists := a.pendingToolCalls[int(event.OutputIndex)]; exists {
				pending.arguments.WriteString(event.Input)
			}
		}
		return true

	// Output item events
	case "response.output_item.added":
		a.itemID = event.ItemID
		a.outputIndex = int(event.OutputIndex)

		// Track function call items
		// Try to get the function call details from the item
		if fcItem, ok := event.AsAny().(responses.ResponseOutputItemAddedEvent); ok {
			// Use AsAny() to check the actual type
			if actualItem := fcItem.Item.AsAny(); actualItem != nil {
				if fc, ok := actualItem.(responses.ResponseFunctionToolCallItem); ok {
					// fc is ResponseFunctionToolCallItem which embeds ResponseFunctionToolCall
					a.pendingToolCalls[a.outputIndex] = &pendingResponseToolCall{
						itemID:    fc.ID,
						name:      fc.Name,
						arguments: strings.Builder{},
					}
				}
			}
		}
		return true

	case "response.output_item.done":
		// Item is complete
		return true

	// Content part events
	case "response.content_part.added":
		a.contentIndex = int(event.ContentIndex)
		return true

	case "response.content_part.done":
		a.contentIndex = 0
		return true

	// Refusal events
	case "response.refusal.delta":
		a.currentRefusal += event.Delta
		return true

	case "response.refusal.done":
		a.currentRefusal += event.Refusal
		return true

	// Completion events
	case "response.completed":
		a.status = "completed"
		a.finished = true
		a.response = &event.Response
		return true

	case "response.failed":
		a.status = "failed"
		a.finished = true
		return true

	case "response.incomplete":
		a.status = "incomplete"
		a.finished = true
		return true

	// Error event
	case "error":
		a.status = "error"
		a.finished = true
		return true

	// Unsupported events - can be extended as needed:
	// - response.audio.delta/done
	// - response.audio.transcript.delta/done
	// - response.code_interpreter_call.*
	// - response.file_search_call.*
	// - response.web_search_call.*
	// - response.image_generation_call.*
	// - response.mcp_call.*
	// - response.reasoning.*
	// - response.output_text.annotation.added
	// - response.custom_tool_call_input.delta/done

	default:
		// Silently ignore unsupported events
		return false
	}
}

// Status returns the current response status.
func (a *ResponsesAssembler) Status() string {
	if a == nil {
		return ""
	}
	return a.status
}

// OutputText returns the accumulated output text.
func (a *ResponsesAssembler) OutputText() string {
	if a == nil {
		return ""
	}
	return a.accumulatedText
}

// CurrentText returns the text being accumulated for the current part.
func (a *ResponsesAssembler) CurrentText() string {
	if a == nil {
		return ""
	}
	return a.currentText
}

// CurrentRefusal returns the refusal text if the model refused to respond.
func (a *ResponsesAssembler) CurrentRefusal() string {
	if a == nil {
		return ""
	}
	return a.currentRefusal
}

// ResponseID returns the response ID.
func (a *ResponsesAssembler) ResponseID() string {
	if a == nil {
		return ""
	}
	if a.responseID == "" {
		return fmt.Sprintf("resp_%d", a.createdAt)
	}
	return a.responseID
}

// IsCompleted returns true if the response is completed.
func (a *ResponsesAssembler) IsCompleted() bool {
	return a != nil && a.status == "completed"
}

// IsFailed returns true if the response failed.
func (a *ResponsesAssembler) IsFailed() bool {
	return a != nil && (a.status == "failed" || a.status == "error")
}

// IsIncomplete returns true if the response was incomplete.
func (a *ResponsesAssembler) IsIncomplete() bool {
	return a != nil && a.status == "incomplete"
}

// IsFinished returns true if the stream has finished (completed, failed, incomplete, or error).
func (a *ResponsesAssembler) IsFinished() bool {
	return a != nil && a.finished
}

// Response returns the final Response object when completed.
// Returns nil if the response is not yet completed or failed.
func (a *ResponsesAssembler) Response() *responses.Response {
	if a == nil || !a.IsCompleted() {
		return nil
	}
	return a.response
}

// ToolCalls returns the accumulated tool calls.
// Returns a map of output index to tool call details.
func (a *ResponsesAssembler) ToolCalls() map[int]ToolCallInfo {
	if a == nil {
		return nil
	}

	result := make(map[int]ToolCallInfo)
	for idx, pending := range a.pendingToolCalls {
		result[idx] = ToolCallInfo{
			ItemID:    pending.itemID,
			Name:      pending.name,
			Arguments: pending.arguments.String(),
		}
	}
	return result
}

// ToolCallInfo contains information about a tool call.
type ToolCallInfo struct {
	ItemID    string
	Name      string
	Arguments string
}

// Finish returns the accumulated result.
// If the stream completed successfully, returns the Response.
// Otherwise, returns nil - check Status() to determine the outcome.
func (a *ResponsesAssembler) Finish() *responses.Response {
	if a == nil {
		return nil
	}

	// If we have a completed response, return it
	if a.response != nil {
		return a.response
	}

	// For incomplete/failed streams, return nil
	// The caller can check Status() to determine what happened
	return nil
}

// GetOrCreateResponseID returns the response ID, generating one if not set.
func (a *ResponsesAssembler) GetOrCreateResponseID() string {
	if a.responseID != "" {
		return a.responseID
	}
	return fmt.Sprintf("resp_%d", a.createdAt)
}

// GetOrCreateItemID returns the item ID, generating one if not set.
func (a *ResponsesAssembler) GetOrCreateItemID() string {
	if a.itemID != "" {
		return a.itemID
	}
	return fmt.Sprintf("item_%d", a.createdAt)
}

// SetResponseID sets a custom response ID.
func (a *ResponsesAssembler) SetResponseID(id string) {
	a.responseID = id
}

// SetItemID sets a custom item ID.
func (a *ResponsesAssembler) SetItemID(id string) {
	a.itemID = id
}
