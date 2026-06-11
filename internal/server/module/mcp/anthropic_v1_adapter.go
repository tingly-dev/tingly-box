package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// AnthropicV1Adapter implements FormatAdapter for Anthropic V1 API
type AnthropicV1Adapter struct{}

// NewAnthropicV1Adapter creates a new Anthropic V1 adapter
func NewAnthropicV1Adapter() *AnthropicV1Adapter {
	return &AnthropicV1Adapter{}
}

// Request/Response types

func (a *AnthropicV1Adapter) NewRequest() any {
	return &anthropic.MessageNewParams{}
}

func (a *AnthropicV1Adapter) NewResponse() any {
	return &anthropic.Message{}
}

// Tool extraction

func (a *AnthropicV1Adapter) ExtractTools(response any) ([]Tool, error) {
	msg, ok := response.(*anthropic.Message)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.Message, got %T", response)
	}

	tools := make([]Tool, 0, len(msg.Content))
	for _, block := range msg.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			tools = append(tools, &AnthropicV1Tool{ToolUseBlock: tu})
		}
	}
	return tools, nil
}

func (a *AnthropicV1Adapter) IsVirtualTool(tool Tool, registry *coretool.VirtualToolRegistry) bool {
	sourceID, toolName, ok := runtime.ParseNormalizedToolName(tool.Name())
	if !ok {
		return false
	}
	if sourceID == "advisor" || (sourceID == "builtin" && toolName == "advisor") {
		return true
	}
	if registry == nil {
		return false
	}

	_, exists := registry.Get(toolName)
	return exists
}

func (a *AnthropicV1Adapter) SplitVirtualExternal(
	tools []Tool,
	registry *coretool.VirtualToolRegistry,
) (virtual, external []Tool, externalIDs []string) {
	virtual = make([]Tool, 0)
	external = make([]Tool, 0)
	externalIDs = make([]string, 0)

	for _, tool := range tools {
		if a.IsVirtualTool(tool, registry) {
			virtual = append(virtual, tool)
		} else {
			external = append(external, tool)
			if tool.ID() != "" {
				externalIDs = append(externalIDs, tool.ID())
			}
		}
	}
	return
}

// Message building

func (a *AnthropicV1Adapter) BuildAssistantMessage(response any) (any, error) {
	msg, ok := response.(*anthropic.Message)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.Message, got %T", response)
	}
	return messageToParamPreservingThinking(msg), nil
}

func (a *AnthropicV1Adapter) BuildToolMessage(result ToolExecutionResult) any {
	return anthropic.ToolResultBlockParam{
		ToolUseID: result.ToolUseID,
		Content:   toolContentsToAnthropicV1(result.Contents),
		IsError:   anthropic.Bool(result.IsError),
	}
}

func (a *AnthropicV1Adapter) AppendToolResults(req, resp any, results []any) (any, error) {
	reqParams, ok := req.(*anthropic.MessageNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.MessageNewParams, got %T", req)
	}

	msg, ok := resp.(*anthropic.Message)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.Message, got %T", resp)
	}

	// Create new request with appended messages
	newReq := *reqParams
	newMessages := append([]anthropic.MessageParam{}, reqParams.Messages...)
	newMessages = append(newMessages, messageToParamPreservingThinking(msg))

	// Convert results to tool result blocks
	resultBlocks := make([]anthropic.ContentBlockParamUnion, len(results))
	for i, r := range results {
		tr := r.(ToolExecutionResult)
		resultBlocks[i] = anthropic.ContentBlockParamUnion{
			OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: tr.ToolUseID,
				Content:   toolContentsToAnthropicV1(tr.Contents),
				IsError:   anthropic.Bool(tr.IsError),
			},
		}
	}
	newMessages = append(newMessages, anthropic.NewUserMessage(resultBlocks...))

	newReq.Messages = newMessages
	return &newReq, nil
}

func (a *AnthropicV1Adapter) BuildContinuationSegment(resp any, results []ToolExecutionResult) (any, error) {
	msg, ok := resp.(*anthropic.Message)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.Message, got %T", resp)
	}
	segment := make([]anthropic.MessageParam, 0, 2)
	segment = append(segment, messageToParamPreservingThinking(msg))
	resultBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(results))
	for _, r := range results {
		resultBlocks = append(resultBlocks, anthropic.ContentBlockParamUnion{
			OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: r.ToolUseID,
				Content:   toolContentsToAnthropicV1(r.Contents),
				IsError:   anthropic.Bool(r.IsError),
			},
		})
	}
	if len(resultBlocks) > 0 {
		segment = append(segment, anthropic.NewUserMessage(resultBlocks...))
	}
	return segment, nil
}

func (a *AnthropicV1Adapter) ApplyContinuation(req any, segment any) (any, error) {
	reqParams, ok := req.(*anthropic.MessageNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.MessageNewParams, got %T", req)
	}
	seg, ok := segment.([]anthropic.MessageParam)
	if !ok || len(seg) == 0 {
		return req, nil
	}
	newReq := *reqParams
	newReq.Messages = mergeAnthropicV1Continuation(seg, reqParams.Messages)
	return &newReq, nil
}

func mergeAnthropicV1Continuation(segment []anthropic.MessageParam, messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(segment) == 0 {
		return append([]anthropic.MessageParam{}, messages...)
	}
	if len(messages) == 0 {
		return append([]anthropic.MessageParam{}, segment...)
	}

	assistantIdx := -1
	toolResultIdx := -1
	for idx, msg := range messages {
		if assistantIdx == -1 && msg.Role == anthropic.MessageParamRoleAssistant {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					assistantIdx = idx
					break
				}
			}
		}
		if toolResultIdx == -1 && msg.Role == anthropic.MessageParamRoleUser {
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
					toolResultIdx = idx
					break
				}
			}
		}
		if assistantIdx != -1 && toolResultIdx != -1 {
			break
		}
	}
	if toolResultIdx == -1 {
		return append(append([]anthropic.MessageParam{}, segment...), messages...)
	}

	merged := append([]anthropic.MessageParam{}, segment...)
	lastIdx := len(merged) - 1
	merged[lastIdx].Content = append(append([]anthropic.ContentBlockParamUnion{}, merged[lastIdx].Content...), messages[toolResultIdx].Content...)
	if assistantIdx == -1 || toolResultIdx < assistantIdx {
		result := append([]anthropic.MessageParam{}, merged...)
		result = append(result, messages[:toolResultIdx]...)
		result = append(result, messages[toolResultIdx+1:]...)
		return result
	}

	result := append([]anthropic.MessageParam{}, messages[:assistantIdx]...)
	result = append(result, merged...)
	result = append(result, messages[toolResultIdx+1:]...)
	return result
}

func messageToParamPreservingThinking(msg *anthropic.Message) anthropic.MessageParam {
	if msg == nil {
		return anthropic.MessageParam{}
	}

	// Preserve original assistant content when building the follow-up request.
	// Raw JSON round-trip keeps provider-specific fields that ToParam() may omit.
	if param, ok := unmarshalAnthropicParamPreservingRawJSON[anthropic.MessageParam](msg.RawJSON()); ok {
		return param
	}

	return msg.ToParam()
}

func (a *AnthropicV1Adapter) FilterVirtualTools(response any, externalTools []Tool) (any, error) {
	msg, ok := response.(*anthropic.Message)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.Message, got %T", response)
	}

	// Build external tool ID set
	externalIDs := make(map[string]bool)
	for _, t := range externalTools {
		externalIDs[t.ID()] = true
	}

	// Filter content blocks
	filtered := make([]anthropic.ContentBlockUnion, 0)
	for _, block := range msg.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			if externalIDs[string(tu.ID)] {
				filtered = append(filtered, block)
			}
		} else {
			filtered = append(filtered, block)
		}
	}

	if len(filtered) != len(msg.Content) {
		// A virtual tool_use block was removed; re-parse so RawJSON reflects
		// the filtered content (downstream writers prefer RawJSON).
		msg.Content = filtered
		if b, err := json.Marshal(msg); err == nil {
			var fresh anthropic.Message
			if json.Unmarshal(b, &fresh) == nil {
				return &fresh, nil
			}
		}
	}
	msg.Content = filtered
	return msg, nil
}

// Streaming setup

func (a *AnthropicV1Adapter) SetupSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Retry-After", "5000")
}

func (a *AnthropicV1Adapter) SendEvent(c *gin.Context, eventType string, payload []byte) error {
	// Anthropic SSE requires a named `event:` line per frame: official SDKs
	// dispatch on it and silently drop frames without one. Prefer the type
	// embedded in the payload, falling back to the caller-supplied name.
	name := eventType
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err == nil {
		if typ, ok := raw["type"].(string); ok && typ != "" {
			name = typ
		}
	}
	if name == "" {
		name = "message"
	}
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", name, payload)
	c.Writer.Flush()
	return nil
}

func (a *AnthropicV1Adapter) SendKeepAlive(c *gin.Context) error {
	fmt.Fprint(c.Writer, ": keep-alive\n\n")
	c.Writer.Flush()
	return nil
}

func (a *AnthropicV1Adapter) SendFinalMessage(c *gin.Context) error {
	deltaJSON, _ := json.Marshal(map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		// The Anthropic protocol requires usage on message_delta; strict SDK
		// accumulators (e.g. Python) crash on a delta without it.
		"usage": map[string]interface{}{
			"output_tokens": 0,
		},
	})
	if err := a.SendEvent(c, "message_delta", deltaJSON); err != nil {
		return err
	}
	stopJSON, _ := json.Marshal(map[string]interface{}{"type": "message_stop"})
	return a.SendEvent(c, "message_stop", stopJSON)
}

// Event processing

func (a *AnthropicV1Adapter) ClassifyEvent(event any) EventType {
	var e anthropic.MessageStreamEventUnion
	switch v := event.(type) {
	case anthropic.MessageStreamEventUnion:
		e = v
	case *anthropic.MessageStreamEventUnion:
		if v == nil {
			return EventUnknown
		}
		e = *v
	default:
		return EventUnknown
	}

	switch v := e.AsAny().(type) {
	case anthropic.MessageStartEvent:
		return EventText

	case anthropic.ContentBlockStartEvent:
		if _, ok := v.ContentBlock.AsAny().(anthropic.ToolUseBlock); ok {
			return EventToolStart
		}
		return EventText

	case anthropic.ContentBlockDeltaEvent:
		switch v.Delta.AsAny().(type) {
		case anthropic.TextDelta:
			return EventText
		case anthropic.InputJSONDelta:
			return EventToolDelta
		}

	case anthropic.ContentBlockStopEvent:
		return EventToolStop

	case anthropic.MessageDeltaEvent:
		return MessageDelta

	case anthropic.MessageStopEvent:
		return MessageStop
	}

	return EventUnknown
}

func (a *AnthropicV1Adapter) ExtractToolFromEvent(event any) (Tool, bool) {
	var e anthropic.MessageStreamEventUnion
	switch v := event.(type) {
	case anthropic.MessageStreamEventUnion:
		e = v
	case *anthropic.MessageStreamEventUnion:
		if v == nil {
			return nil, false
		}
		e = *v
	default:
		return nil, false
	}

	start, ok := e.AsAny().(anthropic.ContentBlockStartEvent)
	if !ok {
		return nil, false
	}

	tu, ok := start.ContentBlock.AsAny().(anthropic.ToolUseBlock)
	if !ok {
		return nil, false
	}

	return &AnthropicV1Tool{ToolUseBlock: tu}, true
}

func (a *AnthropicV1Adapter) ShouldSuppressEvent(event any, virtualRegistry *coretool.VirtualToolRegistry) bool {
	// Anthropic suppresses virtual tools at the start stage (bufferToolEvent),
	// so delta/stop events never reach the client. No need to check here.
	return false
}

func (a *AnthropicV1Adapter) RewriteEventIndex(event any, offset int) ([]byte, error) {
	e, ok := event.(*anthropic.MessageStreamEventUnion)
	if !ok {
		return nil, fmt.Errorf("expected *MessageStreamEventUnion, got %T", event)
	}

	rawJSON := e.RawJSON() // This returns string
	// Parse and rewrite index
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		return nil, err
	}

	if index, ok := parsed["index"].(float64); ok {
		parsed["index"] = int(index) + offset
	}

	return json.Marshal(parsed)
}

// Usage extraction

func (a *AnthropicV1Adapter) ExtractUsage(response any) (TokenUsage, error) {
	msg, ok := response.(*anthropic.Message)
	if !ok {
		return TokenUsage{}, fmt.Errorf("expected *anthropic.Message, got %T", response)
	}

	return TokenUsage{
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
		CacheTokens:  int(msg.Usage.CacheReadInputTokens),
	}, nil
}

// AnthropicV1Tool implements Tool interface for Anthropic V1 ToolUseBlock
type AnthropicV1Tool struct {
	anthropic.ToolUseBlock
}

func (t *AnthropicV1Tool) ID() string {
	return string(t.ToolUseBlock.ID)
}

func (t *AnthropicV1Tool) Name() string {
	return t.ToolUseBlock.Name
}

func (t *AnthropicV1Tool) Arguments() string {
	return string(t.ToolUseBlock.Input)
}
