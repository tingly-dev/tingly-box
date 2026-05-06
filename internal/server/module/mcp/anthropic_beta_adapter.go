package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

// AnthropicBetaAdapter implements FormatAdapter for Anthropic Beta API
type AnthropicBetaAdapter struct{}

// NewAnthropicBetaAdapter creates a new Anthropic Beta adapter
func NewAnthropicBetaAdapter() *AnthropicBetaAdapter {
	return &AnthropicBetaAdapter{}
}

// Request/Response types

func (a *AnthropicBetaAdapter) NewRequest() any {
	return &anthropic.BetaMessageNewParams{}
}

func (a *AnthropicBetaAdapter) NewResponse() any {
	return &anthropic.BetaMessage{}
}

// Tool extraction

func (a *AnthropicBetaAdapter) ExtractTools(response any) ([]Tool, error) {
	msg, ok := response.(*anthropic.BetaMessage)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessage, got %T", response)
	}

	tools := make([]Tool, 0, len(msg.Content))
	for _, block := range msg.Content {
		if tu, ok := block.AsAny().(anthropic.BetaToolUseBlock); ok {
			tools = append(tools, &AnthropicBetaTool{ToolUseBlock: tu})
		}
	}
	return tools, nil
}

func (a *AnthropicBetaAdapter) IsVirtualTool(tool Tool, registry *runtime.VirtualToolRegistry) bool {
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

func (a *AnthropicBetaAdapter) SplitVirtualExternal(
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

func (a *AnthropicBetaAdapter) BuildAssistantMessage(response any) (any, error) {
	msg, ok := response.(*anthropic.BetaMessage)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessage, got %T", response)
	}
	return betaMessageToParamPreservingThinking(msg), nil
}

func (a *AnthropicBetaAdapter) BuildToolMessage(result ToolExecutionResult) any {
	return anthropic.NewBetaToolResultBlock(result.ToolUseID, result.Content, result.IsError)
}

func (a *AnthropicBetaAdapter) AppendToolResults(req, resp any, results []any) (any, error) {
	reqParams, ok := req.(*anthropic.BetaMessageNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessageNewParams, got %T", req)
	}

	msg, ok := resp.(*anthropic.BetaMessage)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessage, got %T", resp)
	}

	// Create new request with appended messages
	newReq := *reqParams
	newMessages := append([]anthropic.BetaMessageParam{}, reqParams.Messages...)
	newMessages = append(newMessages, betaMessageToParamPreservingThinking(msg))

	// Convert results to tool result blocks
	resultBlocks := make([]anthropic.BetaContentBlockParamUnion, len(results))
	for i, r := range results {
		tr := r.(ToolExecutionResult)
		resultBlocks[i] = anthropic.NewBetaToolResultBlock(tr.ToolUseID, tr.Content, tr.IsError)
	}
	newMessages = append(newMessages, anthropic.NewBetaUserMessage(resultBlocks...))

	newReq.Messages = newMessages
	return &newReq, nil
}

func (a *AnthropicBetaAdapter) BuildContinuationSegment(resp any, results []ToolExecutionResult) (any, error) {
	msg, ok := resp.(*anthropic.BetaMessage)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessage, got %T", resp)
	}
	segment := make([]anthropic.BetaMessageParam, 0, 2)
	segment = append(segment, betaMessageToParamPreservingThinking(msg))
	resultBlocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(results))
	for _, r := range results {
		resultBlocks = append(resultBlocks, anthropic.NewBetaToolResultBlock(r.ToolUseID, r.Content, r.IsError))
	}
	if len(resultBlocks) > 0 {
		segment = append(segment, anthropic.NewBetaUserMessage(resultBlocks...))
	}
	return segment, nil
}

func (a *AnthropicBetaAdapter) ApplyContinuation(req any, segment any) (any, error) {
	reqParams, ok := req.(*anthropic.BetaMessageNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessageNewParams, got %T", req)
	}
	seg, ok := segment.([]anthropic.BetaMessageParam)
	if !ok || len(seg) == 0 {
		return req, nil
	}
	newReq := *reqParams
	newReq.Messages = mergeAnthropicBetaContinuation(seg, reqParams.Messages)
	return &newReq, nil
}

func mergeAnthropicBetaContinuation(segment []anthropic.BetaMessageParam, messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	if len(segment) == 0 {
		return append([]anthropic.BetaMessageParam{}, messages...)
	}
	if len(messages) == 0 {
		return append([]anthropic.BetaMessageParam{}, segment...)
	}

	assistantIdx := -1
	toolResultIdx := -1
	for idx, msg := range messages {
		if assistantIdx == -1 && msg.Role == anthropic.BetaMessageParamRoleAssistant {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					assistantIdx = idx
					break
				}
			}
		}
		if toolResultIdx == -1 && msg.Role == anthropic.BetaMessageParamRoleUser {
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
		return append(append([]anthropic.BetaMessageParam{}, segment...), messages...)
	}

	merged := append([]anthropic.BetaMessageParam{}, segment...)
	lastIdx := len(merged) - 1
	merged[lastIdx].Content = append(append([]anthropic.BetaContentBlockParamUnion{}, merged[lastIdx].Content...), messages[toolResultIdx].Content...)
	if assistantIdx == -1 || toolResultIdx < assistantIdx {
		result := append([]anthropic.BetaMessageParam{}, merged...)
		result = append(result, messages[:toolResultIdx]...)
		result = append(result, messages[toolResultIdx+1:]...)
		return result
	}

	result := append([]anthropic.BetaMessageParam{}, messages[:assistantIdx]...)
	result = append(result, merged...)
	result = append(result, messages[toolResultIdx+1:]...)
	return result
}

func betaMessageToParamPreservingThinking(msg *anthropic.BetaMessage) anthropic.BetaMessageParam {
	if msg == nil {
		return anthropic.BetaMessageParam{}
	}

	// Preserve original assistant content when building the follow-up request.
	// Raw JSON round-trip keeps provider-specific fields that ToParam() may omit.
	if param, ok := unmarshalAnthropicParamPreservingRawJSON[anthropic.BetaMessageParam](msg.RawJSON()); ok {
		return param
	}

	return msg.ToParam()
}

func (a *AnthropicBetaAdapter) FilterVirtualTools(response any, externalTools []Tool) (any, error) {
	msg, ok := response.(*anthropic.BetaMessage)
	if !ok {
		return nil, fmt.Errorf("expected *anthropic.BetaMessage, got %T", response)
	}

	// Build external tool ID set
	externalIDs := make(map[string]bool)
	for _, t := range externalTools {
		externalIDs[t.ID()] = true
	}

	// Filter content blocks
	filtered := make([]anthropic.BetaContentBlockUnion, 0)
	for _, block := range msg.Content {
		if tu, ok := block.AsAny().(anthropic.BetaToolUseBlock); ok {
			if externalIDs[string(tu.ID)] {
				filtered = append(filtered, block)
			}
			continue
		}
		if stu, ok := block.AsAny().(anthropic.BetaServerToolUseBlock); ok {
			if externalIDs[string(stu.ID)] {
				filtered = append(filtered, block)
			}
			continue
		}
		filtered = append(filtered, block)
	}

	msg.Content = filtered
	return msg, nil
}

// Streaming setup

func (a *AnthropicBetaAdapter) SetupSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Retry-After", "5000")
}

func (a *AnthropicBetaAdapter) SendEvent(c *gin.Context, eventType string, payload []byte) error {
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

func (a *AnthropicBetaAdapter) SendKeepAlive(c *gin.Context) error {
	fmt.Fprint(c.Writer, ": keep-alive\n\n")
	c.Writer.Flush()
	return nil
}

func (a *AnthropicBetaAdapter) SendFinalMessage(c *gin.Context) error {
	deltaJSON, _ := json.Marshal(map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
	})
	if err := a.SendEvent(c, "message_delta", deltaJSON); err != nil {
		return err
	}
	stopJSON, _ := json.Marshal(map[string]interface{}{"type": "message_stop"})
	return a.SendEvent(c, "message_stop", stopJSON)
}

// Event processing

func (a *AnthropicBetaAdapter) ClassifyEvent(event any) EventType {
	var e anthropic.BetaRawMessageStreamEventUnion
	switch v := event.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		e = v
	case *anthropic.BetaRawMessageStreamEventUnion:
		if v == nil {
			return EventUnknown
		}
		e = *v
	default:
		return EventUnknown
	}

	switch v := e.AsAny().(type) {
	case anthropic.BetaRawMessageStartEvent:
		return EventText

	case anthropic.BetaRawContentBlockStartEvent:
		if _, ok := v.ContentBlock.AsAny().(anthropic.BetaToolUseBlock); ok {
			return EventToolStart
		}
		if stu, ok := v.ContentBlock.AsAny().(anthropic.BetaServerToolUseBlock); ok && stu.Name == "advisor" {
			return EventToolStart
		}
		return EventText

	case anthropic.BetaRawContentBlockDeltaEvent:
		switch v.Delta.AsAny().(type) {
		case anthropic.BetaTextDelta:
			return EventText
		case anthropic.BetaInputJSONDelta:
			return EventToolDelta
		}

	case anthropic.BetaRawContentBlockStopEvent:
		return EventToolStop

	case anthropic.BetaRawMessageDeltaEvent:
		return MessageDelta

	case anthropic.BetaRawMessageStopEvent:
		return MessageStop
	}

	return EventUnknown
}

func (a *AnthropicBetaAdapter) ExtractToolFromEvent(event any) (Tool, bool) {
	var e anthropic.BetaRawMessageStreamEventUnion
	switch v := event.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		e = v
	case *anthropic.BetaRawMessageStreamEventUnion:
		if v == nil {
			return nil, false
		}
		e = *v
	default:
		return nil, false
	}

	start, ok := e.AsAny().(anthropic.BetaRawContentBlockStartEvent)
	if !ok {
		return nil, false
	}

	tu, ok := start.ContentBlock.AsAny().(anthropic.BetaToolUseBlock)
	if !ok {
		return nil, false
	}
	return &AnthropicBetaTool{ToolUseBlock: tu}, true
}

func (a *AnthropicBetaAdapter) ShouldSuppressEvent(event any, virtualRegistry *runtime.VirtualToolRegistry) bool {
	// Anthropic suppresses virtual tools at the start stage (bufferToolEvent),
	// so delta/stop events never reach the client. No need to check here.
	return false
}

func (a *AnthropicBetaAdapter) RewriteEventIndex(event any, offset int) ([]byte, error) {
	e, ok := event.(*anthropic.BetaRawMessageStreamEventUnion)
	if !ok {
		return nil, fmt.Errorf("expected *BetaRawMessageStreamEventUnion, got %T", event)
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

func (a *AnthropicBetaAdapter) ExtractUsage(response any) (TokenUsage, error) {
	msg, ok := response.(*anthropic.BetaMessage)
	if !ok {
		return TokenUsage{}, fmt.Errorf("expected *anthropic.BetaMessage, got %T", response)
	}

	return TokenUsage{
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
		CacheTokens:  int(msg.Usage.CacheReadInputTokens),
	}, nil
}

// AnthropicBetaTool implements Tool interface for Anthropic Beta ToolUseBlock
type AnthropicBetaTool struct {
	ToolUseBlock anthropic.BetaToolUseBlock
}

func (t *AnthropicBetaTool) ID() string {
	return string(t.ToolUseBlock.ID)
}

func (t *AnthropicBetaTool) Name() string {
	return t.ToolUseBlock.Name
}

func (t *AnthropicBetaTool) Arguments() string {
	b, err := json.Marshal(t.ToolUseBlock.Input)
	if err != nil || len(b) == 0 {
		return "{}"
	}
	return string(b)
}
