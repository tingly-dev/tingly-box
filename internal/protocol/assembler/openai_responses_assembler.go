package assembler

import (
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3/responses"
)

// ResponsesAssembler accumulates OpenAI Responses API streaming events.
// It focuses on core functionality: text accumulation, tool calls, image generation, and final response construction.
// Inspired by internal/protocol/stream/anthropic_to_openai_responses.go
type ResponsesAssembler struct {
	// Final response when completed
	response *responses.Response

	// Accumulated state
	responseID      string
	itemID          string
	outputIndex     int
	contentIndex    int
	accumulatedText strings.Builder
	currentText     strings.Builder
	currentRefusal  strings.Builder
	status          string
	sequenceNumber  int
	createdAt       int64
	finished        bool

	// Tool call tracking
	pendingToolCalls map[int]*pendingResponseToolCall

	// Image generation tracking - maps output index to image data
	images map[int]*pendingImageData
}

// pendingImageData tracks an image being assembled from stream chunks
type pendingImageData struct {
	callID string
	data   strings.Builder
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
		images:           make(map[int]*pendingImageData),
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
		images:           make(map[int]*pendingImageData),
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
		a.currentText.WriteString(event.Delta)
		a.accumulatedText.WriteString(event.Delta)
		return true

	case "response.output_text.done":
		// Text is complete, stored in accumulatedText
		return true

	// Function call events (tool calls)
	case "response.function_call_arguments.delta":
		if pending, exists := a.pendingToolCalls[int(event.OutputIndex)]; exists {
			pending.arguments.WriteString(event.Arguments)
		}
		return true

	case "response.function_call_arguments.done":
		// event.Input contains the full arguments string; delta events already
		// accumulated the content, so nothing extra to append here.
		return true

	// Output item events
	case "response.output_item.added":
		a.itemID = event.ItemID
		a.outputIndex = int(event.OutputIndex)

		// Track function call items by checking the item type directly
		if event.Item.Type == "function_call" {
			fc := event.Item.AsFunctionCall()
			a.pendingToolCalls[a.outputIndex] = &pendingResponseToolCall{
				itemID:    fc.ID,
				name:      fc.Name,
				arguments: strings.Builder{},
			}
		}
		return true

	case "response.output_item.done":
		// Item is complete - check for image_generation_call result
		doneItem := event.Item
		if doneItem.Type == "image_generation_call" {
			// Extract result from the done event if we haven't received partial images
			imgCall := doneItem.AsImageGenerationCall()
			idx := int(event.OutputIndex)
			if a.images[idx] == nil {
				a.images[idx] = &pendingImageData{}
			}
			if a.images[idx].data.Len() == 0 && imgCall.Result != "" {
				a.images[idx].data.WriteString(imgCall.Result)
			}
			if a.images[idx].callID == "" {
				a.images[idx].callID = imgCall.ID
			}
		}
		return true

	// Content part events
	case "response.content_part.added":
		a.contentIndex = int(event.ContentIndex)
		return true

	case "response.content_part.done":
		a.contentIndex = 0
		a.currentText.Reset()
		return true

	// Refusal events
	case "response.refusal.delta":
		a.currentRefusal.WriteString(event.Delta)
		return true

	case "response.refusal.done":
		// delta events already accumulated the refusal; nothing extra to append.
		return true

	// Image generation events
	case "response.image_generation_call.in_progress":
		idx := int(event.OutputIndex)
		if a.images[idx] == nil {
			a.images[idx] = &pendingImageData{}
		}
		a.images[idx].callID = event.ItemID
		return true

	case "response.image_generation_call.partial_image":
		// Accumulate base64 image chunks for the specific output index
		idx := int(event.OutputIndex)
		if a.images[idx] == nil {
			a.images[idx] = &pendingImageData{}
		}
		a.images[idx].data.WriteString(event.PartialImageB64)
		if a.images[idx].callID == "" {
			a.images[idx].callID = event.ItemID
		}
		return true

	case "response.image_generation_call.completed":
		// Mark this image as completed
		idx := int(event.OutputIndex)
		if a.images[idx] != nil {
			// Image is complete, keep the data
		}
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
		// Incomplete is a terminal Responses status for cases such as
		// max_output_tokens/content_filter. It is not equivalent to a broken
		// transport stream: the terminal event may contain valid output and
		// final usage, and long Codex tasks rely on that partial response.
		a.status = "incomplete"
		a.finished = true
		if responseHasPayload(&event.Response) {
			a.response = &event.Response
		}
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
	// - response.mcp_call.*
	// - response.reasoning.*
	// - response.output_text.annotation.added
	// - response.custom_tool_call_input.delta/done
	// - response.image_generation_call.generating

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
	return a.accumulatedText.String()
}

// CurrentText returns the text being accumulated for the current part.
func (a *ResponsesAssembler) CurrentText() string {
	if a == nil {
		return ""
	}
	return a.currentText.String()
}

// CurrentRefusal returns the refusal text if the model refused to respond.
func (a *ResponsesAssembler) CurrentRefusal() string {
	if a == nil {
		return ""
	}
	return a.currentRefusal.String()
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

	// If the upstream sent a terminal response, preserve it. Incomplete
	// Responses can still contain useful assistant output and usage; callers
	// should inspect Status() instead of treating them as assembly failure.
	if a.response != nil {
		a.ensureResponseHasAccumulatedOutput(a.response)
		return a.response
	}

	// Some providers emit text deltas and then terminate incomplete without a
	// full response object. Return a best-effort incomplete response rather than
	// discarding the already streamed assistant message; callers can still see
	// Status()=="incomplete" and decide whether to continue/retry.
	if a.IsIncomplete() && (a.accumulatedText.Len() > 0 || a.currentRefusal.Len() > 0) {
		resp := a.syntheticResponse("incomplete")
		a.response = resp
		return resp
	}

	// Failed/error streams without a response body are not recoverable here.
	return nil
}

func responseHasPayload(resp *responses.Response) bool {
	if resp == nil {
		return false
	}
	return resp.ID != "" || len(resp.Output) > 0 || resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 || resp.Usage.TotalTokens != 0
}

func (a *ResponsesAssembler) ensureResponseHasAccumulatedOutput(resp *responses.Response) {
	if resp == nil || len(resp.Output) > 0 {
		return
	}
	if a.accumulatedText.Len() == 0 && a.currentRefusal.Len() == 0 {
		return
	}
	resp.Output = a.syntheticOutputItems(resp.Status)
}

func (a *ResponsesAssembler) syntheticResponse(status string) *responses.Response {
	return &responses.Response{
		ID:        a.GetOrCreateResponseID(),
		CreatedAt: float64(a.createdAt),
		Status:    responses.ResponseStatus(status),
		Output:    a.syntheticOutputItems(responses.ResponseStatus(status)),
	}
}

func (a *ResponsesAssembler) syntheticOutputItems(status responses.ResponseStatus) []responses.ResponseOutputItemUnion {
	itemStatus := "completed"
	if status == responses.ResponseStatusIncomplete {
		itemStatus = "incomplete"
	}
	content := make([]responses.ResponseOutputMessageContentUnion, 0, 2)
	if a.accumulatedText.Len() > 0 {
		content = append(content, responses.ResponseOutputMessageContentUnion{
			Type: "output_text",
			Text: a.accumulatedText.String(),
		})
	}
	if a.currentRefusal.Len() > 0 {
		content = append(content, responses.ResponseOutputMessageContentUnion{
			Type:    "refusal",
			Refusal: a.currentRefusal.String(),
		})
	}
	return []responses.ResponseOutputItemUnion{{
		ID:      a.GetOrCreateItemID(),
		Type:    "message",
		Role:    "assistant",
		Status:  itemStatus,
		Content: content,
	}}
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

// Images returns all accumulated images as a map of output index to image data.
func (a *ResponsesAssembler) Images() map[int]string {
	if a == nil {
		return nil
	}
	result := make(map[int]string, len(a.images))
	for idx, img := range a.images {
		result[idx] = img.data.String()
	}
	return result
}

// ImageDataAt returns the image data at the specific output index.
func (a *ResponsesAssembler) ImageDataAt(idx int) string {
	if a == nil || a.images[idx] == nil {
		return ""
	}
	return a.images[idx].data.String()
}

// ImageCallIDs returns all image generation call IDs as a map of output index to call ID.
func (a *ResponsesAssembler) ImageCallIDs() map[int]string {
	if a == nil {
		return nil
	}
	result := make(map[int]string, len(a.images))
	for idx, img := range a.images {
		result[idx] = img.callID
	}
	return result
}

// ImageCallIDAt returns the image call ID at the specific output index.
func (a *ResponsesAssembler) ImageCallIDAt(idx int) string {
	if a == nil || a.images[idx] == nil {
		return ""
	}
	return a.images[idx].callID
}

// ImageCount returns the number of images accumulated.
func (a *ResponsesAssembler) ImageCount() int {
	if a == nil {
		return 0
	}
	return len(a.images)
}

// HasImage returns true if any image data was accumulated.
func (a *ResponsesAssembler) HasImage() bool {
	return a != nil && len(a.images) > 0
}

// HasImageAt returns true if image data was accumulated at the specific output index.
func (a *ResponsesAssembler) HasImageAt(idx int) bool {
	return a != nil && a.images[idx] != nil && a.images[idx].data.Len() > 0
}
