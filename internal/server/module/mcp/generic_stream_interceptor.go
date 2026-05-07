package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	defaultMaxRounds = 3
)

// GenericStreamInterceptor implements format-agnostic streaming MCP tool handling
type GenericStreamInterceptor struct {
	c               *gin.Context
	s               ServerOps
	provider        *typ.Provider
	hc              *protocol.HandleContext
	virtualRegistry *runtime.VirtualToolRegistry
	recorder        ProtocolRecorder
	adapter         FormatAdapter
	forwarder       Forwarder
	toolExecutor    ToolExecutor
	config          InterceptorConfig

	// Cross-round state (not reset)
	ttftRecorded      bool
	totalInputTokens  int64
	totalOutputTokens int64
	totalCacheTokens  int64

	// Mutable request for multi-round loop
	currentReq any

	// Per-round state (reset each round)
	round int

	roundAnthropicV1       *anthropic.Message
	roundAnthropicBeta     *anthropic.BetaMessage
	roundOpenAI            *openai.ChatCompletion
	openAIToolStates       map[int]*genericOpenAIToolCallState
	roundTools             []Tool
	roundToolSeen          map[string]struct{}
	seenMessageStart       bool
	seenContentBlockStart  bool
	seenContentBlockStop   bool
	seenStopReason         bool
	roundMessageDelta      []byte
	roundMessageStop       []byte
	suppressedBlockIndices map[int]struct{}
	stopAfterRound         bool
}

type genericOpenAIToolCallState struct {
	index     int
	id        string
	name      string
	arguments strings.Builder
}

// NewGenericStreamInterceptor creates a new generic streaming interceptor
func NewGenericStreamInterceptor(
	c *gin.Context,
	s ServerOps,
	provider *typ.Provider,
	hc *protocol.HandleContext,
	virtualRegistry *runtime.VirtualToolRegistry,
	recorder ProtocolRecorder,
	adapter FormatAdapter,
	forwarder Forwarder,
	config InterceptorConfig,
) *GenericStreamInterceptor {
	if config.MaxRounds == 0 {
		config.MaxRounds = defaultMaxRounds
	}

	return &GenericStreamInterceptor{
		c:               c,
		s:               s,
		provider:        provider,
		hc:              hc,
		virtualRegistry: virtualRegistry,
		recorder:        recorder,
		adapter:         adapter,
		forwarder:       forwarder,
		config:          config,
	}
}

// Run executes the streaming interceptor loop
func (i *GenericStreamInterceptor) Run(req any) error {
	// Setup SSE headers
	i.adapter.SetupSSEHeaders(i.c)
	defer i.reportUsage()

	i.currentReq = req
	i.applyStoredContinuation()

	for i.round = 0; i.round < i.config.MaxRounds; i.round++ {
		i.resetRoundState()
		logrus.Debugf("[MCP-Interceptor] Round %d: starting", i.round)

		if i.round > 0 && i.config.OnBeforeRound != nil {
			if err := i.config.OnBeforeRound(i.round); err != nil {
				return err
			}
		}

		// Forward request to upstream
		stream, err := i.forwarder.ForwardStream(i.c.Request.Context(), i.provider, i.extractModel(i.currentReq), i.currentReq)
		if err != nil {
			return fmt.Errorf("forward stream failed: %w", err)
		}

		// Consume round and build response
		response, err := i.consumeRound(stream)
		stream.Close()
		if err != nil {
			return err
		}

		// Accumulate usage
		if err := i.accumulateUsage(response); err != nil {
			logrus.WithError(err).Warn("failed to accumulate usage")
		}

		// Classify response and decide next action
		decision := i.classifyResponse(response)

		switch decision {
		case DecisionNoTools:
			logrus.Debugf("[MCP-Interceptor] Round %d: no tools, ending", i.round)
			return i.adapter.SendFinalMessage(i.c)

		case DecisionPureVirtual:
			logrus.Debugf("[MCP-Interceptor] Round %d: pure virtual, continuing", i.round)
			if err := i.handlePureVirtual(response); err != nil {
				return err
			}
			// Loop continues with updated i.currentReq

		case DecisionPureExternal:
			logrus.Debugf("[MCP-Interceptor] Round %d: pure external, continuing with tool execution", i.round)
			if err := i.handlePureExternal(response); err != nil {
				return err
			}
			if i.stopAfterRound {
				logrus.Debugf("[MCP-Interceptor] Round %d: client-native handoff complete, ending stream", i.round)
				return nil
			}
			// Loop continues with updated i.currentReq after appending tool results.

		case DecisionMixed:
			logrus.Debugf("[MCP-Interceptor] Round %d: mixed, stashing and ending", i.round)
			return i.handleMixed(response, i.currentReq)
		}
	}

	// Max rounds exceeded
	logrus.Infof("[MCP-Interceptor] Max rounds (%d) exceeded", i.config.MaxRounds)
	return i.adapter.SendFinalMessage(i.c)
}

// handlePureExternal hands non-virtual tools back to the client. Only virtual
// tools (for example advisor) are executed by tingly-box inside the stream loop.
func (i *GenericStreamInterceptor) handlePureExternal(response any) error {
	return i.finishClientNativeToolUse()
}

func (i *GenericStreamInterceptor) finishClientNativeToolUse() error {
	i.stopAfterRound = true
	if i.roundMessageDelta != nil {
		if err := i.adapter.SendEvent(i.c, "message_delta", i.roundMessageDelta); err != nil {
			return err
		}
	}
	if i.roundMessageStop != nil {
		if err := i.adapter.SendEvent(i.c, "message_stop", i.roundMessageStop); err != nil {
			return err
		}
	}
	return nil
}

// consumeRound processes all events from a stream and builds a complete response
func (i *GenericStreamInterceptor) consumeRound(stream StreamHandle) (any, error) {
	for {
		next := stream.Next()
		if !next {
			break
		}
		event := stream.Current()
		i.accumulateRoundEvent(event)

		// Call guardrails hooks if enabled
		if i.config.EnableGuardrails && i.hc != nil {
			for _, hook := range i.hc.OnStreamEventHooks {
				if err := hook(event); err != nil {
					logrus.WithError(err).Warn("guardrails hook error")
				}
			}
		}

		// Route event based on type
		eventType := i.adapter.ClassifyEvent(event)
		if err := i.routeEvent(event, eventType); err != nil {
			return nil, err
		}
	}

	err := stream.Err()
	if err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	// Build complete response from accumulated state
	return i.roundResponse(), nil
}

func (i *GenericStreamInterceptor) resetRoundState() {
	i.roundAnthropicV1 = nil
	i.roundAnthropicBeta = nil
	i.roundOpenAI = nil
	i.openAIToolStates = make(map[int]*genericOpenAIToolCallState)
	i.roundTools = nil
	i.roundToolSeen = make(map[string]struct{})
	i.seenMessageStart = false
	i.seenContentBlockStart = false
	i.seenContentBlockStop = false
	i.seenStopReason = false
	i.roundMessageDelta = nil
	i.roundMessageStop = nil
	i.suppressedBlockIndices = make(map[int]struct{})
	i.stopAfterRound = false
}

func (i *GenericStreamInterceptor) roundResponse() any {
	if i.roundAnthropicV1 != nil {
		return i.roundAnthropicV1
	}
	if i.roundAnthropicBeta != nil {
		return i.roundAnthropicBeta
	}
	if i.roundOpenAI != nil {
		return i.roundOpenAI
	}
	return i.adapter.NewResponse()
}

func (i *GenericStreamInterceptor) accumulateRoundEvent(event any) {
	switch e := event.(type) {
	case anthropic.MessageStreamEventUnion:
		if i.roundAnthropicV1 == nil {
			i.roundAnthropicV1 = &anthropic.Message{}
		}
		i.roundAnthropicV1.Accumulate(e)
	case *anthropic.MessageStreamEventUnion:
		if i.roundAnthropicV1 == nil {
			i.roundAnthropicV1 = &anthropic.Message{}
		}
		i.roundAnthropicV1.Accumulate(*e)
	case anthropic.BetaRawMessageStreamEventUnion:
		if i.roundAnthropicBeta == nil {
			i.roundAnthropicBeta = &anthropic.BetaMessage{}
		}
		i.roundAnthropicBeta.Accumulate(e)
	case *anthropic.BetaRawMessageStreamEventUnion:
		if i.roundAnthropicBeta == nil {
			i.roundAnthropicBeta = &anthropic.BetaMessage{}
		}
		i.roundAnthropicBeta.Accumulate(*e)
	case openai.ChatCompletionChunk:
		i.accumulateOpenAIChunk(e)
	}
}

func (i *GenericStreamInterceptor) accumulateOpenAIChunk(chunk openai.ChatCompletionChunk) {
	if i.roundOpenAI == nil {
		i.roundOpenAI = &openai.ChatCompletion{}
	}
	if len(chunk.Choices) == 0 {
		return
	}
	if len(i.roundOpenAI.Choices) == 0 {
		i.roundOpenAI.Choices = append(i.roundOpenAI.Choices, openai.ChatCompletionChoice{})
	}
	choice := chunk.Choices[0]
	msg := &i.roundOpenAI.Choices[0].Message
	if choice.Delta.Content != "" {
		msg.Content += choice.Delta.Content
	}
	for _, tc := range choice.Delta.ToolCalls {
		idx := int(tc.Index)
		state := i.openAIToolStates[idx]
		if state == nil {
			state = &genericOpenAIToolCallState{index: idx}
			i.openAIToolStates[idx] = state
		}
		if tc.ID != "" {
			state.id = tc.ID
		}
		if tc.Function.Name != "" {
			state.name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			state.arguments.WriteString(tc.Function.Arguments)
		}
	}

	if len(i.openAIToolStates) > 0 {
		indices := make([]int, 0, len(i.openAIToolStates))
		for idx := range i.openAIToolStates {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		toolCalls := make([]openai.ChatCompletionMessageToolCallUnion, 0, len(indices))
		for _, idx := range indices {
			state := i.openAIToolStates[idx]
			toolCall := map[string]any{
				"id":   state.id,
				"type": "function",
				"function": map[string]any{
					"name":      state.name,
					"arguments": state.arguments.String(),
				},
			}
			b, _ := json.Marshal(toolCall)
			var union openai.ChatCompletionMessageToolCallUnion
			if err := json.Unmarshal(b, &union); err == nil {
				toolCalls = append(toolCalls, union)
			}
		}
		msg.ToolCalls = toolCalls
	}

	if choice.FinishReason != "" {
		i.roundOpenAI.Choices[0].FinishReason = choice.FinishReason
	}
	if chunk.Usage.PromptTokens != 0 {
		i.roundOpenAI.Usage.PromptTokens = chunk.Usage.PromptTokens
	}
	if chunk.Usage.CompletionTokens != 0 {
		i.roundOpenAI.Usage.CompletionTokens = chunk.Usage.CompletionTokens
	}
	if chunk.Usage.PromptTokensDetails.CachedTokens != 0 {
		i.roundOpenAI.Usage.PromptTokensDetails.CachedTokens = chunk.Usage.PromptTokensDetails.CachedTokens
	}
}

// routeEvent routes a single event based on its type
func (i *GenericStreamInterceptor) routeEvent(event any, eventType EventType) error {
	switch eventType {
	case EventText:
		if isAnthropicMessageStartEvent(event) {
			i.seenMessageStart = true
		}
		if isAnthropicContentBlockStartEvent(event) {
			i.seenContentBlockStart = true
		}
		if isAnthropicContentBlockStopEvent(event) {
			i.seenContentBlockStop = true
		}
		return i.handleTextEvent(event)

	case EventToolStart:
		return i.handleToolStartEvent(event)

	case EventToolDelta:
		return i.handleToolDeltaEvent(event)

	case EventToolStop:
		return i.handleToolStopEvent(event)

	case MessageDelta:
		if stopReason := extractAnthropicStopReason(event); stopReason != "" {
			i.seenStopReason = true
			if payload, err := i.extractEventPayload(event); err == nil {
				i.roundMessageDelta = payload
			}
			logrus.Debugf("[MCP-Interceptor] Round %d: forwarded message_delta stop_reason=%s", i.round, stopReason)
		}
		// Silently accumulate, interceptor controls end timing
		return nil

	case MessageStop:
		if payload, err := i.extractEventPayload(event); err == nil {
			i.roundMessageStop = payload
		}
		return nil

	default:
		// Pass through unknown events
		return i.adapter.SendEvent(i.c, "message", []byte{})
	}
}

// handleTextEvent sends text delta to client
func (i *GenericStreamInterceptor) handleTextEvent(event any) error {
	payload, err := i.extractEventPayload(event)
	if err != nil {
		return err
	}

	// Record TTFT
	if !i.ttftRecorded {
		i.recordTTFT()
		i.ttftRecorded = true
	}

	return i.adapter.SendEvent(i.c, "content_block_delta", payload)
}

// handleToolStartEvent handles tool use start event
func (i *GenericStreamInterceptor) handleToolStartEvent(event any) error {
	i.seenContentBlockStart = true
	tool, ok := i.adapter.ExtractToolFromEvent(event)
	if ok {
		logrus.Debugf("[MCP-TOOLS] round=%d tool_start name=%s id=%s virtual=%v", i.round, tool.Name(), tool.ID(), i.adapter.IsVirtualTool(tool, i.virtualRegistry))
	}
	if !ok {
		// Not a tool event, pass through
		payload, err := i.extractEventPayload(event)
		if err != nil {
			return err
		}
		return i.adapter.SendEvent(i.c, "content_block_start", payload)
	}
	i.recordRoundTool(tool)

	// Check if virtual tool
	if i.adapter.IsVirtualTool(tool, i.virtualRegistry) {
		if index, ok := extractContentBlockIndex(event); ok {
			i.suppressedBlockIndices[index] = struct{}{}
		}
		return nil
	}

	// External tool: pass through to client
	payload, err := i.extractEventPayload(event)
	if err != nil {
		return err
	}
	return i.adapter.SendEvent(i.c, "content_block_start", payload)
}

// handleToolDeltaEvent handles tool parameter delta event
func (i *GenericStreamInterceptor) handleToolDeltaEvent(event any) error {
	if tool, ok := i.adapter.ExtractToolFromEvent(event); ok {
		i.recordRoundTool(tool)
	}
	if i.isSuppressedContentBlockEvent(event) || i.adapter.ShouldSuppressEvent(event, i.virtualRegistry) {
		return nil
	}

	// External tool delta: pass through
	payload, err := i.extractEventPayload(event)
	if err != nil {
		return err
	}
	return i.adapter.SendEvent(i.c, "content_block_delta", payload)
}

// handleToolStopEvent handles tool stop event
func (i *GenericStreamInterceptor) handleToolStopEvent(event any) error {
	i.seenContentBlockStop = true
	if i.isSuppressedContentBlockEvent(event) || i.adapter.ShouldSuppressEvent(event, i.virtualRegistry) {
		return nil
	}

	// External tool stop: pass through
	payload, err := i.extractEventPayload(event)
	if err != nil {
		return err
	}
	return i.adapter.SendEvent(i.c, "content_block_stop", payload)
}

// classifyResponse classifies the response to determine next action
func (i *GenericStreamInterceptor) classifyResponse(response any) ResponseDecision {
	tools, err := i.extractRoundTools(response)
	if err != nil || len(tools) == 0 {
		return DecisionNoTools
	}
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Name())
	}
	logrus.Debugf("[MCP-TOOLS] round=%d extracted=%v", i.round, toolNames)

	hasVirtual := false
	hasExternal := false

	for _, tool := range tools {
		if i.adapter.IsVirtualTool(tool, i.virtualRegistry) {
			hasVirtual = true
		} else {
			hasExternal = true
		}
	}

	if hasVirtual && hasExternal {
		return DecisionMixed
	}
	if hasVirtual {
		return DecisionPureVirtual
	}
	return DecisionPureExternal
}

// handlePureVirtual executes virtual tools and updates i.currentReq for next round
func (i *GenericStreamInterceptor) handlePureVirtual(response any) error {
	tools, err := i.extractRoundTools(response)
	if err != nil {
		return err
	}
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Name())
	}
	logrus.Debugf("[MCP-TOOLS] round=%d pure_virtual=%v", i.round, toolNames)

	// Execute virtual tools
	results := make([]ToolExecutionResult, 0, len(tools))
	for _, tool := range tools {
		result, err := i.executeTool(tool, i.currentReq)
		if err != nil {
			logrus.WithError(err).Warnf("tool execution failed: %s", tool.Name())
		}
		results = append(results, result)
	}

	// Append tool results to request; updated req used in next round
	updatedReq, err := i.adapter.AppendToolResults(i.currentReq, response, i.resultsToAny(results))
	if err != nil {
		return err
	}
	i.currentReq = updatedReq

	// Send keep-alive to client
	return i.adapter.SendKeepAlive(i.c)
}

// handleMixed executes virtual tools, stashes results, returns external to client
func (i *GenericStreamInterceptor) handleMixed(response, req any) error {
	tools, err := i.extractRoundTools(response)
	if err != nil {
		return err
	}

	virtual, _, externalIDs := i.adapter.SplitVirtualExternal(tools, i.virtualRegistry)
	virtualNames := make([]string, 0, len(virtual))
	for _, tool := range virtual {
		virtualNames = append(virtualNames, tool.Name())
	}
	logrus.Debugf("[MCP-TOOLS] round=%d split virtual=%v external_ids=%v", i.round, virtualNames, externalIDs)

	// Execute virtual tools
	results := make([]ToolExecutionResult, 0, len(virtual))
	for _, tool := range virtual {
		result, err := i.executeTool(tool, req)
		if err != nil {
			logrus.WithError(err).Warnf("tool execution failed: %s", tool.Name())
		}
		results = append(results, result)
	}

	normalizedResults, err := validateAndNormalizeMixedStash(externalIDs, results)
	if err != nil {
		logrus.WithError(err).Warn("[MCP-Interceptor] mixed consistency validation failed; ending current round for continuation")
		return i.adapter.SendFinalMessage(i.c)
	}

	segment, err := i.adapter.BuildContinuationSegment(response, normalizedResults)
	if err != nil {
		logrus.WithError(err).Warn("[MCP-Interceptor] failed to build mixed continuation segment; ending current round for continuation")
		return i.adapter.SendFinalMessage(i.c)
	}
	key := continuationKey(typ.GetSessionID(i.c.Request.Context()), i.provider.UUID, i.adapterID())
	mixedContinuationStore.put(key, segment)

	// Send final message (external tools already streamed to client)
	return i.adapter.SendFinalMessage(i.c)
}

// executeTool executes a single virtual tool
func (i *GenericStreamInterceptor) executeTool(tool Tool, req any) (ToolExecutionResult, error) {
	// Extract messages from request
	messages := i.extractMessages(req)

	// Execute tool
	type toolHookCaller interface {
		CallMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (context.Context, string, error)
	}
	nextCtx, result, err := i.s.(toolHookCaller).CallMCPToolWithHooks(i.c.Request.Context(), tool.Name(), tool.Arguments(), messages)
	if nextCtx != nil {
		i.c.Request = i.c.Request.WithContext(nextCtx)
	}
	return ToolExecutionResult{
		ToolUseID: tool.ID(),
		Content:   result,
		IsError:   err != nil,
	}, err
}

// Helper methods

func (i *GenericStreamInterceptor) extractModel(req any) string {
	return extractModelFromRequest(req, i.provider)
}

func (i *GenericStreamInterceptor) extractEventPayload(event any) ([]byte, error) {
	// Extract raw JSON payload from streaming events
	// Different SDK types have different methods to access raw data
	switch e := event.(type) {
	case anthropic.MessageStreamEventUnion:
		return []byte(e.RawJSON()), nil
	case *anthropic.MessageStreamEventUnion:
		return []byte(e.RawJSON()), nil
	case anthropic.BetaRawMessageStreamEventUnion:
		return []byte(e.RawJSON()), nil
	case *anthropic.BetaRawMessageStreamEventUnion:
		return []byte(e.RawJSON()), nil
	case *openai.ChatCompletionChunk:
		// For OpenAI, return the raw JSON string
		return []byte(e.RawJSON()), nil
	default:
		// Fallback: return empty payload for unknown types
		return []byte{}, nil
	}
}

func (i *GenericStreamInterceptor) extractMessages(req any) []map[string]any {
	return extractMessagesForToolCall(req)
}

func (i *GenericStreamInterceptor) resultsToAny(results []ToolExecutionResult) []any {
	out := make([]any, len(results))
	for i, r := range results {
		out[i] = r
	}
	return out
}

func (i *GenericStreamInterceptor) recordTTFT() {
	// Time To First Token (TTFT) tracking is handled at the dispatch layer
	// through HandleContext hooks. This method is called for consistency
	// but the actual tracking is done in the dispatch functions.
	// See: mcp_anthropic_v1_helper.go - dispatchGeneric*Stream functions
}

func (i *GenericStreamInterceptor) isSuppressedContentBlockEvent(event any) bool {
	index, ok := extractContentBlockIndex(event)
	if !ok {
		return false
	}
	_, suppressed := i.suppressedBlockIndices[index]
	return suppressed
}

func (i *GenericStreamInterceptor) accumulateUsage(response any) error {
	usage, err := i.adapter.ExtractUsage(response)
	if err != nil {
		return err
	}

	i.totalInputTokens += int64(usage.InputTokens)
	i.totalOutputTokens += int64(usage.OutputTokens)
	i.totalCacheTokens += int64(usage.CacheTokens)
	return nil
}

func (i *GenericStreamInterceptor) reportUsage() {
	if i.c != nil {
		i.s.TrackUsage(i.c, int(i.totalInputTokens), int(i.totalOutputTokens), int(i.totalCacheTokens))
	}
}

func (i *GenericStreamInterceptor) recordRoundTool(tool Tool) {
	if tool == nil {
		return
	}
	key := tool.ID()
	if key == "" {
		key = tool.Name() + "|" + tool.Arguments()
	}
	if _, exists := i.roundToolSeen[key]; exists {
		return
	}
	i.roundToolSeen[key] = struct{}{}
	i.roundTools = append(i.roundTools, tool)
}

func (i *GenericStreamInterceptor) extractRoundTools(response any) ([]Tool, error) {
	tools, err := i.adapter.ExtractTools(response)
	if err == nil && len(tools) > 0 {
		return tools, nil
	}
	if len(i.roundTools) > 0 {
		return i.roundTools, nil
	}
	return tools, err
}

func extractContentBlockIndex(event any) (int, bool) {
	switch e := event.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		return extractContentBlockIndex(e.AsAny())
	case *anthropic.BetaRawMessageStreamEventUnion:
		if e == nil {
			return 0, false
		}
		return extractContentBlockIndex(e.AsAny())
	case anthropic.BetaRawContentBlockStartEvent:
		return int(e.Index), true
	case anthropic.BetaRawContentBlockDeltaEvent:
		return int(e.Index), true
	case anthropic.BetaRawContentBlockStopEvent:
		return int(e.Index), true
	case anthropic.MessageStreamEventUnion:
		return extractContentBlockIndex(e.AsAny())
	case *anthropic.MessageStreamEventUnion:
		if e == nil {
			return 0, false
		}
		return extractContentBlockIndex(e.AsAny())
	case anthropic.ContentBlockStartEvent:
		return int(e.Index), true
	case anthropic.ContentBlockDeltaEvent:
		return int(e.Index), true
	case anthropic.ContentBlockStopEvent:
		return int(e.Index), true
	default:
		return 0, false
	}
}

func isAnthropicMessageStartEvent(event any) bool {
	switch e := event.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		_, ok := e.AsAny().(anthropic.BetaRawMessageStartEvent)
		return ok
	case *anthropic.BetaRawMessageStreamEventUnion:
		if e == nil {
			return false
		}
		_, ok := e.AsAny().(anthropic.BetaRawMessageStartEvent)
		return ok
	default:
		return false
	}
}

func isAnthropicContentBlockStartEvent(event any) bool {
	switch e := event.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		_, ok := e.AsAny().(anthropic.BetaRawContentBlockStartEvent)
		return ok
	case *anthropic.BetaRawMessageStreamEventUnion:
		if e == nil {
			return false
		}
		_, ok := e.AsAny().(anthropic.BetaRawContentBlockStartEvent)
		return ok
	default:
		return false
	}
}

func isAnthropicContentBlockStopEvent(event any) bool {
	switch e := event.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		_, ok := e.AsAny().(anthropic.BetaRawContentBlockStopEvent)
		return ok
	case *anthropic.BetaRawMessageStreamEventUnion:
		if e == nil {
			return false
		}
		_, ok := e.AsAny().(anthropic.BetaRawContentBlockStopEvent)
		return ok
	default:
		return false
	}
}

func extractAnthropicStopReason(event any) string {
	switch e := event.(type) {
	case anthropic.MessageStreamEventUnion:
		if delta, ok := e.AsAny().(anthropic.MessageDeltaEvent); ok {
			return string(delta.Delta.StopReason)
		}
	case *anthropic.MessageStreamEventUnion:
		if e != nil {
			if delta, ok := e.AsAny().(anthropic.MessageDeltaEvent); ok {
				return string(delta.Delta.StopReason)
			}
		}
	case anthropic.BetaRawMessageStreamEventUnion:
		if delta, ok := e.AsAny().(anthropic.BetaRawMessageDeltaEvent); ok {
			return string(delta.Delta.StopReason)
		}
	case *anthropic.BetaRawMessageStreamEventUnion:
		if e != nil {
			if delta, ok := e.AsAny().(anthropic.BetaRawMessageDeltaEvent); ok {
				return string(delta.Delta.StopReason)
			}
		}
	}
	return ""
}

func (i *GenericStreamInterceptor) adapterID() string {
	switch i.adapter.(type) {
	case *OpenAIChatAdapter:
		return "openai-chat"
	case *AnthropicV1Adapter:
		return "anthropic-v1"
	case *AnthropicBetaAdapter:
		return "anthropic-beta"
	default:
		return fmt.Sprintf("%T", i.adapter)
	}
}

func (i *GenericStreamInterceptor) applyStoredContinuation() {
	sessionID := typ.GetSessionID(i.c.Request.Context())
	key := continuationKey(sessionID, i.provider.UUID, i.adapterID())
	segment, ok := mixedContinuationStore.pop(key)
	if !ok {
		return
	}
	logrus.Debugf("[MCP-CONT] interceptor applying stored continuation key=%s adapter=%s", key, i.adapterID())
	updated, err := i.adapter.ApplyContinuation(i.currentReq, segment)
	if err != nil {
		logrus.WithError(err).Warn("[MCP-Interceptor] failed to apply stored continuation")
		return
	}
	i.currentReq = updated
}
