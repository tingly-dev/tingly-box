package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
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

func (a *AnthropicV1Adapter) IsVirtualTool(tool Tool, registry *runtime.VirtualToolRegistry) bool {
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
	registry *runtime.VirtualToolRegistry,
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
	return msg.ToParam(), nil
}

func (a *AnthropicV1Adapter) BuildToolMessage(result ToolExecutionResult) any {
	return anthropic.NewToolResultBlock(result.ToolUseID, result.Content, result.IsError)
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
	newMessages = append(newMessages, msg.ToParam())

	// Convert results to tool result blocks
	resultBlocks := make([]anthropic.ContentBlockParamUnion, len(results))
	for i, r := range results {
		tr := r.(ToolExecutionResult)
		resultBlocks[i] = anthropic.NewToolResultBlock(tr.ToolUseID, tr.Content, tr.IsError)
	}
	newMessages = append(newMessages, anthropic.NewUserMessage(resultBlocks...))

	newReq.Messages = newMessages
	return &newReq, nil
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
	c.SSEvent("", payload)
	c.Writer.Flush()
	return nil
}

func (a *AnthropicV1Adapter) SendKeepAlive(c *gin.Context) error {
	fmt.Fprint(c.Writer, ": keep-alive\n\n")
	c.Writer.Flush()
	return nil
}

func (a *AnthropicV1Adapter) SendFinalMessage(c *gin.Context) error {
	stopJSON, _ := json.Marshal(map[string]interface{}{"type": "message_stop"})
	c.SSEvent("", string(stopJSON))
	c.Writer.Flush()
	return nil
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

func (a *AnthropicV1Adapter) ShouldSuppressEvent(event any, virtualRegistry *runtime.VirtualToolRegistry) bool {
	_, okValue := event.(anthropic.MessageStreamEventUnion)
	_, okPtr := event.(*anthropic.MessageStreamEventUnion)
	if !okValue && !okPtr {
		return false
	}

	// For delta and stop events, we need to check if the index corresponds to a virtual tool
	// This requires external state tracking in the interceptor
	return false // Let interceptor decide based on tracked state
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
