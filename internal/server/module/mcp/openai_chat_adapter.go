package mcp

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

// OpenAIChatAdapter implements FormatAdapter for OpenAI Chat Completions API
type OpenAIChatAdapter struct{}

// NewOpenAIChatAdapter creates a new OpenAI Chat adapter
func NewOpenAIChatAdapter() *OpenAIChatAdapter {
	return &OpenAIChatAdapter{}
}

// Request/Response types

func (o *OpenAIChatAdapter) NewRequest() any {
	return &openai.ChatCompletionNewParams{}
}

func (o *OpenAIChatAdapter) NewResponse() any {
	return &openai.ChatCompletion{}
}

// Tool extraction

func (o *OpenAIChatAdapter) ExtractTools(response any) ([]Tool, error) {
	completion, ok := response.(*openai.ChatCompletion)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletion, got %T", response)
	}

	if len(completion.Choices) == 0 {
		return []Tool{}, nil
	}

	tools := make([]Tool, 0, len(completion.Choices[0].Message.ToolCalls))
	for _, tc := range completion.Choices[0].Message.ToolCalls {
		tools = append(tools, &OpenAITool{ToolCall: tc})
	}
	return tools, nil
}

func (o *OpenAIChatAdapter) IsVirtualTool(tool Tool, registry *runtime.VirtualToolRegistry) bool {
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

func (o *OpenAIChatAdapter) SplitVirtualExternal(
	tools []Tool,
	registry *runtime.VirtualToolRegistry,
) (virtual, external []Tool, externalIDs []string) {
	virtual = make([]Tool, 0)
	external = make([]Tool, 0)
	externalIDs = make([]string, 0)

	for _, tool := range tools {
		if o.IsVirtualTool(tool, registry) {
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

func (o *OpenAIChatAdapter) BuildAssistantMessage(response any) (any, error) {
	completion, ok := response.(*openai.ChatCompletion)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletion, got %T", response)
	}
	return completion.Choices[0].Message.ToParam(), nil
}

func (o *OpenAIChatAdapter) BuildToolMessage(result ToolExecutionResult) any {
	return openai.ToolMessage(result.Content, result.ToolUseID)
}

func (o *OpenAIChatAdapter) AppendToolResults(req, resp any, results []any) (any, error) {
	reqParams, ok := req.(*openai.ChatCompletionNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletionNewParams, got %T", req)
	}

	completion, ok := resp.(*openai.ChatCompletion)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletion, got %T", resp)
	}

	// Create new request with appended messages
	newReq := *reqParams
	newMessages := append([]openai.ChatCompletionMessageParamUnion{}, reqParams.Messages...)
	newMessages = append(newMessages, completion.Choices[0].Message.ToParam())

	// Convert results to tool messages
	for _, r := range results {
		result := r.(ToolExecutionResult)
		// ToolMessage returns ChatCompletionMessageParamUnion
		newMessages = append(newMessages, openai.ToolMessage(result.Content, result.ToolUseID))
	}

	newReq.Messages = newMessages
	return &newReq, nil
}

func (o *OpenAIChatAdapter) BuildContinuationSegment(resp any, results []ToolExecutionResult) (any, error) {
	completion, ok := resp.(*openai.ChatCompletion)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletion, got %T", resp)
	}
	segment := make([]openai.ChatCompletionMessageParamUnion, 0, 1+len(results))
	segment = append(segment, completion.Choices[0].Message.ToParam())
	for _, r := range results {
		segment = append(segment, openai.ToolMessage(r.Content, r.ToolUseID))
	}
	return segment, nil
}

func (o *OpenAIChatAdapter) ApplyContinuation(req any, segment any) (any, error) {
	reqParams, ok := req.(*openai.ChatCompletionNewParams)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletionNewParams, got %T", req)
	}
	seg, ok := segment.([]openai.ChatCompletionMessageParamUnion)
	if !ok || len(seg) == 0 {
		return req, nil
	}
	newReq := *reqParams
	newReq.Messages = append(append([]openai.ChatCompletionMessageParamUnion{}, seg...), reqParams.Messages...)
	return &newReq, nil
}

func (o *OpenAIChatAdapter) FilterVirtualTools(response any, externalTools []Tool) (any, error) {
	completion, ok := response.(*openai.ChatCompletion)
	if !ok {
		return nil, fmt.Errorf("expected *openai.ChatCompletion, got %T", response)
	}

	// Build external tool ID set
	externalIDs := make(map[string]bool)
	for _, t := range externalTools {
		externalIDs[t.ID()] = true
	}

	// Filter tool calls
	filtered := make([]openai.ChatCompletionMessageToolCallUnion, 0)
	for _, tc := range completion.Choices[0].Message.ToolCalls {
		if externalIDs[tc.ID] {
			filtered = append(filtered, tc)
		}
	}

	completion.Choices[0].Message.ToolCalls = filtered
	return completion, nil
}

// Streaming setup

func (o *OpenAIChatAdapter) SetupSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
}

func (o *OpenAIChatAdapter) SendEvent(c *gin.Context, eventType string, payload []byte) error {
	c.SSEvent("", payload)
	c.Writer.Flush()
	return nil
}

func (o *OpenAIChatAdapter) SendKeepAlive(c *gin.Context) error {
	fmt.Fprint(c.Writer, ": keep-alive\n\n")
	c.Writer.Flush()
	return nil
}

func (o *OpenAIChatAdapter) SendFinalMessage(c *gin.Context) error {
	c.SSEvent("", "[DONE]")
	c.Writer.Flush()
	return nil
}

// Event processing

func (o *OpenAIChatAdapter) ClassifyEvent(event any) EventType {
	chunk, ok := event.(openai.ChatCompletionChunk)
	if !ok {
		return EventUnknown
	}

	if len(chunk.Choices) == 0 {
		return EventUnknown
	}

	delta := chunk.Choices[0].Delta

	// Check for content
	if delta.Content != "" {
		return EventText
	}

	// Check for tool calls
	if len(delta.ToolCalls) > 0 {
		return EventToolDelta
	}

	// Check for finish reason
	if chunk.Choices[0].FinishReason != "" {
		return MessageStop
	}

	return EventUnknown
}

func (o *OpenAIChatAdapter) ExtractToolFromEvent(event any) (Tool, bool) {
	chunk, ok := event.(openai.ChatCompletionChunk)
	if !ok || len(chunk.Choices) == 0 {
		return nil, false
	}

	delta := chunk.Choices[0].Delta
	if len(delta.ToolCalls) == 0 {
		return nil, false
	}

	// For chunk events, we need to accumulate tool calls across chunks
	// Return the tool call from this chunk
	tc := delta.ToolCalls[0]
	return &OpenAIChunkTool{ToolCall: tc}, true
}

func (o *OpenAIChatAdapter) ShouldSuppressEvent(event any, virtualRegistry *runtime.VirtualToolRegistry) bool {
	chunk, ok := event.(openai.ChatCompletionChunk)
	if !ok || len(chunk.Choices) == 0 {
		return false
	}

	delta := chunk.Choices[0].Delta
	if len(delta.ToolCalls) == 0 {
		return false
	}

	// Check if this is a virtual tool call
	// Need to track which tool calls are virtual from their name
	for _, tc := range delta.ToolCalls {
		if tc.Function.Name != "" {
			_, toolName, ok := runtime.ParseNormalizedToolName(tc.Function.Name)
			if ok && virtualRegistry != nil {
				if _, exists := virtualRegistry.Get(toolName); exists {
					return true // Suppress virtual tool events
				}
			}
		}
	}

	return false
}

func (o *OpenAIChatAdapter) RewriteEventIndex(event any, offset int) ([]byte, error) {
	// OpenAI doesn't use index in the same way as Anthropic
	// Tool calls have their own index field
	return nil, nil
}

// Usage extraction

func (o *OpenAIChatAdapter) ExtractUsage(response any) (TokenUsage, error) {
	completion, ok := response.(*openai.ChatCompletion)
	if !ok {
		return TokenUsage{}, fmt.Errorf("expected *openai.ChatCompletion, got %T", response)
	}

	cacheTokens := 0
	// PromptTokensDetails is a struct, not a pointer
	// Check if CachedTokens field has a value
	if completion.Usage.PromptTokensDetails.CachedTokens > 0 {
		cacheTokens = int(completion.Usage.PromptTokensDetails.CachedTokens)
	}

	return TokenUsage{
		InputTokens:  int(completion.Usage.PromptTokens),
		OutputTokens: int(completion.Usage.CompletionTokens),
		CacheTokens:  cacheTokens,
	}, nil
}

// OpenAIMessageTool implements Tool interface for OpenAI ChatCompletionMessage tool calls
type OpenAIMessageTool struct {
	ToolCall openai.ChatCompletionMessageToolCallUnion
}

func (t *OpenAIMessageTool) ID() string {
	return t.ToolCall.ID
}

func (t *OpenAIMessageTool) Name() string {
	return string(t.ToolCall.Function.Name)
}

func (t *OpenAIMessageTool) Arguments() string {
	return string(t.ToolCall.Function.Arguments)
}

// OpenAIChunkTool implements Tool interface for OpenAI ChatCompletionChunk tool calls
type OpenAIChunkTool struct {
	ToolCall openai.ChatCompletionChunkChoiceDeltaToolCall
}

func (t *OpenAIChunkTool) ID() string {
	return t.ToolCall.ID
}

func (t *OpenAIChunkTool) Name() string {
	return string(t.ToolCall.Function.Name)
}

func (t *OpenAIChunkTool) Arguments() string {
	return string(t.ToolCall.Function.Arguments)
}

// For now, use the message tool type
type OpenAITool = OpenAIMessageTool
