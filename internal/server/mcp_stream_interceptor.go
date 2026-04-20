package server

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const maxInterceptorRounds = 6

// ============================================================================
// Shared helpers
// ============================================================================

func sendKeepAlive(c *gin.Context) {
	fmt.Fprint(c.Writer, ": keep-alive\n\n")
	c.Writer.Flush()
}

func sendMessageStop(c *gin.Context) {
	stopJSON, _ := json.Marshal(map[string]interface{}{"type": "message_stop"})
	c.SSEvent("", string(stopJSON))
	c.Writer.Flush()
}

func isVirtualTool(normalizedName string, registry *runtime.VirtualToolRegistry) bool {
	if registry == nil {
		return false
	}
	_, toolName, ok := runtime.ParseNormalizedToolName(normalizedName)
	if !ok {
		return false
	}
	_, ok = registry.Get(toolName)
	return ok
}

// rewriteIndex rewrites the "index" field in raw SDK event JSON by adding offset.
func rewriteIndex(rawJSON []byte, offset int) []byte {
	var m map[string]any
	if err := json.Unmarshal(rawJSON, &m); err != nil {
		return rawJSON
	}
	if idx, ok := m["index"].(float64); ok {
		m["index"] = int(idx) + offset
	}
	b, _ := json.Marshal(m)
	return b
}

// rewriteGuardrailsIndex rewrites the "index" field in a GuardrailsBufferedEvent payload.
func rewriteGuardrailsIndex(event protocol.GuardrailsBufferedEvent, offset int) []byte {
	newPayload := maps.Clone(event.Payload)
	if idx, ok := newPayload["index"].(float64); ok {
		newPayload["index"] = int(idx) + offset
	}
	b, _ := json.Marshal(newPayload)
	return b
}

// ============================================================================
// Anthropic V1 Stream Interceptor
// ============================================================================

// AnthropicV1StreamInterceptor intercepts Anthropic V1 streaming responses
// for all-virtual tool_use scenarios, executing virtual tools server-side
// while streaming text deltas to the client in real-time.
type AnthropicV1StreamInterceptor struct {
	c               *gin.Context
	s               *Server
	provider        *typ.Provider
	hc              *protocol.HandleContext
	virtualRegistry *runtime.VirtualToolRegistry
	recorder        *ProtocolRecorder

	// cross-round state (not reset)
	ttftRecorded             bool
	totalInputTokens         int64
	totalOutputTokens        int64
	totalCacheCreationTokens int64
	totalCacheReadTokens     int64
	blockIndexOffset         int

	// per-round state
	round              int
	toolUseEventBuffer map[int][]protocol.GuardrailsBufferedEvent
	virtualToolIndices map[int]bool
	roundBlockCount    int
	maxBlockIndex      int // track max index seen this round
}

// NewAnthropicV1StreamInterceptor creates a new interceptor.
func NewAnthropicV1StreamInterceptor(
	c *gin.Context,
	s *Server,
	provider *typ.Provider,
	hc *protocol.HandleContext,
	virtualRegistry *runtime.VirtualToolRegistry,
	recorder *ProtocolRecorder,
) *AnthropicV1StreamInterceptor {
	return &AnthropicV1StreamInterceptor{
		c:               c,
		s:               s,
		provider:        provider,
		hc:              hc,
		virtualRegistry: virtualRegistry,
		recorder:        recorder,
	}
}

func (i *AnthropicV1StreamInterceptor) resetRoundState() {
	i.toolUseEventBuffer = make(map[int][]protocol.GuardrailsBufferedEvent)
	i.virtualToolIndices = make(map[int]bool)
	i.roundBlockCount = 0
	i.maxBlockIndex = -1
}

func (i *AnthropicV1StreamInterceptor) advanceBlockIndex() {
	// Use the maximum index seen this round to advance offset.
	// This ensures offset doesn't conflict with suppressed virtual tool indices.
	if i.maxBlockIndex >= 0 {
		i.blockIndexOffset = i.maxBlockIndex + 1
	}
	i.roundBlockCount = 0
}

// Run executes the interceptor loop for all-virtual tool_use streaming.
func (i *AnthropicV1StreamInterceptor) Run(req *anthropic.MessageNewParams) error {
	i.hc.SetupSSEHeaders()
	defer i.reportUsage()

	for i.round = 0; i.round < maxInterceptorRounds; i.round++ {
		i.resetRoundState()

		wrapper := i.s.clientPool.GetAnthropicClient(i.c.Request.Context(), i.provider, req.Model)
		fc := NewForwardContext(i.c, i.provider)
		stream, cancel, err := ForwardAnthropicV1Stream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return err
		}

		var msg anthropic.Message
		if err := i.consumeRound(stream, &msg); err != nil {
			return err
		}

		i.accumulateUsage(msg)

		if msg.StopReason != anthropic.StopReasonToolUse {
			i.advanceBlockIndex()
			i.sendFinalMessageDelta(string(msg.StopReason))
			sendMessageStop(i.c)
			return nil
		}

		// Check if there are virtual tools to execute server-side.
		// External tools have already been passed through to the client in real-time.
		virtualToolUses := i.classifyVirtualToolUses(msg.Content)
		hasExternalToolUses := i.hasExternalToolUses(msg.Content)

		if len(virtualToolUses) == 0 {
			// No virtual tools in this round; external tools passed through.
			// Wait for client to send tool_results in a follow-up request.
			i.advanceBlockIndex()
			i.sendFinalMessageDelta(string(msg.StopReason))
			sendMessageStop(i.c)
			return nil
		}

		if hasExternalToolUses {
			// Mixed scenario (virtual + external): execute virtual tools server-side
			// and stash results for the client's follow-up request merge.
			// External tools are passed through to client in this response.
			virtualResults := i.executeMCPTools(i.c.Request.Context(), virtualToolUses, req.Messages, &msg)
			i.s.stashPendingVirtualToolResults(i.externalToolUseIDs(msg.Content), virtualResults)
			i.advanceBlockIndex()
			i.sendFinalMessageDelta(string(msg.StopReason))
			sendMessageStop(i.c)
			return nil
		}

		// Pure virtual tools: execute server-side and continue to next round
		i.advanceBlockIndex()
		sendKeepAlive(i.c)
		virtualResults := i.executeMCPTools(i.c.Request.Context(), virtualToolUses, req.Messages, &msg)
		toolResults := make([]anthropic.ContentBlockParamUnion, 0, len(virtualResults))
		for _, r := range virtualResults {
			toolResults = append(toolResults, anthropic.NewToolResultBlock(r.ToolUseID, r.Content, r.IsError))
		}

		req.Messages = append(req.Messages,
			msg.ToParam(),
			anthropic.NewUserMessage(toolResults...),
		)
	}

	i.sendFinalMessageDelta("max_rounds_exceeded")
	sendMessageStop(i.c)
	return nil
}

func (i *AnthropicV1StreamInterceptor) consumeRound(
	stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion],
	msg *anthropic.Message,
) error {
	for stream.Next() {
		event := stream.Current()

		// Step 1: Accumulate
		msg.Accumulate(event)

		// Step 2: guardrails hooks
		for _, hook := range i.hc.OnStreamEventHooks {
			hook(event)
		}

		// Step 3: guardrails mutation
		var handled bool
		var rewritten []protocol.GuardrailsBufferedEvent
		var err error
		if i.hc.Guardrails != nil {
			handled, rewritten, err = guardrailsmutate.RewriteAnthropicToolUseEvent(
				i.hc.Guardrails.CredentialMask,
				i.hc.Guardrails.Stream,
				&event,
			)
			if err != nil {
				return err
			}
		}

		// Step 4: route
		if handled {
			for _, r := range rewritten {
				i.routeGuardrailsEvent(r)
			}
		} else {
			i.filterAndSend(&event)
		}
	}
	return stream.Err()
}

func (i *AnthropicV1StreamInterceptor) filterAndSend(
	event *anthropic.MessageStreamEventUnion,
) {
	switch e := event.AsAny().(type) {
	case anthropic.MessageStartEvent:
		if i.round == 0 {
			i.c.SSEvent(event.Type, event.RawJSON())
			i.c.Writer.Flush()
		}

	case anthropic.ContentBlockStartEvent:
		if block, ok := e.ContentBlock.AsAny().(anthropic.ToolUseBlock); ok {
			if isVirtualTool(block.Name, i.virtualRegistry) {
				i.virtualToolIndices[int(e.Index)] = true
				if int(e.Index) > i.maxBlockIndex {
					i.maxBlockIndex = int(e.Index)
				}
				return // suppress virtual tool_use start
			}
		}
		if int(e.Index) > i.maxBlockIndex {
			i.maxBlockIndex = int(e.Index)
		}
		i.c.SSEvent(event.Type, rewriteIndex([]byte(event.RawJSON()), i.blockIndexOffset))
		i.c.Writer.Flush()
		i.roundBlockCount++

	case anthropic.ContentBlockDeltaEvent:
		if i.virtualToolIndices[int(e.Index)] {
			return // suppress virtual tool delta
		}
		switch delta := e.Delta.AsAny().(type) {
		case anthropic.TextDelta:
			if delta.Text != "" {
				if !i.ttftRecorded {
					SetFirstTokenTime(i.c)
					i.ttftRecorded = true
				}
				logrus.Debugf("[MCP-SSE-DEBUG] Round %d: Sending text delta length=%d index=%d", i.round, len(delta.Text), e.Index)
				i.c.SSEvent(event.Type, rewriteIndex([]byte(event.RawJSON()), i.blockIndexOffset))
				i.c.Writer.Flush()
			}
		case anthropic.InputJSONDelta:
			// External tool parameter deltas must be sent to client
			logrus.Debugf("[MCP-SSE-DEBUG] Round %d: Sending input_json_delta for external tool index=%d partial_json=%s", i.round, e.Index, delta.PartialJSON)
			i.c.SSEvent(event.Type, rewriteIndex([]byte(event.RawJSON()), i.blockIndexOffset))
			i.c.Writer.Flush()
		}

	case anthropic.ContentBlockStopEvent:
		if i.virtualToolIndices[int(e.Index)] {
			delete(i.virtualToolIndices, int(e.Index))
			return // suppress virtual tool stop
		}
		i.c.SSEvent(event.Type, rewriteIndex([]byte(event.RawJSON()), i.blockIndexOffset))
		i.c.Writer.Flush()

	case anthropic.MessageDeltaEvent:
		// Silently accumulate; interceptor controls end timing

	case anthropic.MessageStopEvent:
		// Suppress mid-stream; interceptor controls end timing

	default:
		i.c.SSEvent(event.Type, event.RawJSON())
		i.c.Writer.Flush()
	}
}

func (i *AnthropicV1StreamInterceptor) routeGuardrailsEvent(
	event protocol.GuardrailsBufferedEvent,
) {
	switch event.EventType {
	case "content_block_start":
		block, _ := event.Payload["content_block"].(map[string]any)
		index := int(event.Payload["index"].(float64))
		if block["type"] == "tool_use" {
			name, _ := block["name"].(string)
			if isVirtualTool(name, i.virtualRegistry) {
				// virtual tool_use -> buffer for server-side execution
				logrus.Debugf("[MCP-SSE-DEBUG] Round %d: Buffering virtual tool_use index=%d name=%s", i.round, index, name)
				i.toolUseEventBuffer[index] = append(i.toolUseEventBuffer[index], event)
				return
			}
			// external tool_use -> pass through with index rewrite
			rewritten := rewriteGuardrailsIndex(event, i.blockIndexOffset)
			logrus.Debugf("[MCP-SSE-DEBUG] Round %d: Passing external tool_use index=%d->%d name=%s", i.round, index, i.blockIndexOffset+int(index), name)
			i.c.SSEvent(event.EventType, string(rewritten))
			i.c.Writer.Flush()
			i.roundBlockCount++
			return
		}
		// synthetic text block -> pass through
		rewritten := rewriteGuardrailsIndex(event, i.blockIndexOffset)
		logrus.Debugf("[MCP-SSE-DEBUG] Round %d: Passing text block index=%d", i.round, i.blockIndexOffset+int(index))
		i.c.SSEvent(event.EventType, string(rewritten))
		i.c.Writer.Flush()
		i.roundBlockCount++

	case "content_block_delta", "content_block_stop":
		index := int(event.Payload["index"].(float64))
		if _, buffered := i.toolUseEventBuffer[index]; buffered {
			logrus.Debugf("[MCP-SSE-DEBUG] Round %d: Buffering %s for index=%d", i.round, event.EventType, index)
			i.toolUseEventBuffer[index] = append(i.toolUseEventBuffer[index], event)
			return
		}
		rewritten := rewriteGuardrailsIndex(event, i.blockIndexOffset)
		logrus.Debugf("[MCP-SSE-DEBUG] Round %d: Passing %s index=%d", i.round, event.EventType, i.blockIndexOffset+int(index))
		i.c.SSEvent(event.EventType, string(rewritten))
		i.c.Writer.Flush()

	default:
		// message_delta etc -> passthrough
		rewritten := rewriteGuardrailsIndex(event, i.blockIndexOffset)
		logrus.Debugf("[MCP-SSE-DEBUG] Round %d: Passing %s", i.round, event.EventType)
		i.c.SSEvent(event.EventType, string(rewritten))
		i.c.Writer.Flush()
	}
}

func (i *AnthropicV1StreamInterceptor) classifyVirtualToolUses(
	content []anthropic.ContentBlockUnion,
) []anthropic.ToolUseBlock {
	var toolUses []anthropic.ToolUseBlock
	for _, block := range content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			if isVirtualTool(tu.Name, i.virtualRegistry) {
				toolUses = append(toolUses, tu)
			}
		}
	}
	return toolUses
}

func (i *AnthropicV1StreamInterceptor) hasExternalToolUses(
	content []anthropic.ContentBlockUnion,
) bool {
	for _, block := range content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			if !isVirtualTool(tu.Name, i.virtualRegistry) {
				return true
			}
		}
	}
	return false
}

func (i *AnthropicV1StreamInterceptor) externalToolUseIDs(
	content []anthropic.ContentBlockUnion,
) []string {
	out := make([]string, 0)
	for _, block := range content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			if isVirtualTool(tu.Name, i.virtualRegistry) {
				continue
			}
			if tu.ID != "" {
				out = append(out, string(tu.ID))
			}
		}
	}
	return out
}

func (i *AnthropicV1StreamInterceptor) executeMCPTools(
	ctx context.Context,
	toolUses []anthropic.ToolUseBlock,
	messages []anthropic.MessageParam,
	msg *anthropic.Message,
) []virtualToolExecutionResult {
	allMessages := make([]anthropic.MessageParam, 0, len(messages)+1)
	allMessages = append(allMessages, messages...)
	allMessages = append(allMessages, msg.ToParam())
	hookMessages := extractAnthropicV1Messages(allMessages)

	results := make([]virtualToolExecutionResult, 0, len(toolUses))
	for _, tu := range toolUses {
		args := string(tu.Input)
		if args == "" {
			args = "{}"
		}
		logrus.Debugf("[MCP-SSE-DEBUG] Executing virtual tool: name=%s id=%s args=%s", tu.Name, tu.ID, args)
		result, err := i.s.callMCPToolWithHooks(ctx, tu.Name, args, hookMessages)
		if err != nil {
			logrus.WithError(err).Warnf("mcp: virtual tool call failed name=%s", tu.Name)
		}
		logrus.Debugf("[MCP-SSE-DEBUG] Virtual tool result: name=%s result=%s hasError=%v", tu.Name, result, err != nil)
		results = append(results, virtualToolExecutionResult{
			ToolUseID: string(tu.ID),
			Content:   result,
			IsError:   err != nil,
		})
	}
	return results
}

func (i *AnthropicV1StreamInterceptor) accumulateUsage(msg anthropic.Message) {
	i.totalInputTokens += msg.Usage.InputTokens
	i.totalOutputTokens += msg.Usage.OutputTokens
	i.totalCacheCreationTokens += msg.Usage.CacheCreationInputTokens
	i.totalCacheReadTokens += msg.Usage.CacheReadInputTokens
}

func (i *AnthropicV1StreamInterceptor) sendFinalMessageDelta(stopReason string) {
	payload := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"input_tokens":                i.totalInputTokens,
			"output_tokens":               i.totalOutputTokens,
			"cache_creation_input_tokens": i.totalCacheCreationTokens,
			"cache_read_input_tokens":     i.totalCacheReadTokens,
		},
	}
	b, _ := json.Marshal(payload)
	i.c.SSEvent("message_delta", string(b))
	i.c.Writer.Flush()
}

func (i *AnthropicV1StreamInterceptor) reportUsage() {
	usage := protocol.NewTokenUsageWithCache(
		int(i.totalInputTokens),
		int(i.totalOutputTokens),
		int(i.totalCacheCreationTokens+i.totalCacheReadTokens),
	)
	i.s.trackUsageWithTokenUsage(i.c, usage, nil)
}

// ============================================================================
// Anthropic Beta Stream Interceptor
// ============================================================================

// AnthropicBetaStreamInterceptor intercepts Anthropic Beta streaming responses
// for all-virtual tool_use scenarios.
type AnthropicBetaStreamInterceptor struct {
	c               *gin.Context
	s               *Server
	provider        *typ.Provider
	hc              *protocol.HandleContext
	virtualRegistry *runtime.VirtualToolRegistry
	recorder        *ProtocolRecorder

	// cross-round state (not reset)
	ttftRecorded             bool
	totalInputTokens         int64
	totalOutputTokens        int64
	totalCacheCreationTokens int64
	totalCacheReadTokens     int64
	blockIndexOffset         int

	// per-round state
	round              int
	toolUseEventBuffer map[int][]protocol.GuardrailsBufferedEvent
	virtualToolIndices map[int]bool
	roundBlockCount    int
	maxBlockIndex      int // track max index seen this round
}

// NewAnthropicBetaStreamInterceptor creates a new beta interceptor.
func NewAnthropicBetaStreamInterceptor(
	c *gin.Context,
	s *Server,
	provider *typ.Provider,
	hc *protocol.HandleContext,
	virtualRegistry *runtime.VirtualToolRegistry,
	recorder *ProtocolRecorder,
) *AnthropicBetaStreamInterceptor {
	return &AnthropicBetaStreamInterceptor{
		c:               c,
		s:               s,
		provider:        provider,
		hc:              hc,
		virtualRegistry: virtualRegistry,
		recorder:        recorder,
	}
}

func (i *AnthropicBetaStreamInterceptor) resetRoundState() {
	i.toolUseEventBuffer = make(map[int][]protocol.GuardrailsBufferedEvent)
	i.virtualToolIndices = make(map[int]bool)
	i.roundBlockCount = 0
	i.maxBlockIndex = -1
}

func (i *AnthropicBetaStreamInterceptor) advanceBlockIndex() {
	// Use the maximum index seen this round to advance offset.
	// This ensures offset doesn't conflict with suppressed virtual tool indices.
	if i.maxBlockIndex >= 0 {
		i.blockIndexOffset = i.maxBlockIndex + 1
	}
	i.roundBlockCount = 0
}

// Run executes the interceptor loop for all-virtual tool_use streaming (Beta).
func (i *AnthropicBetaStreamInterceptor) Run(req *anthropic.BetaMessageNewParams) error {
	i.hc.SetupSSEHeaders()
	defer i.reportUsageBeta()

	for i.round = 0; i.round < maxInterceptorRounds; i.round++ {
		i.resetRoundState()

		wrapper := i.s.clientPool.GetAnthropicClient(i.c.Request.Context(), i.provider, string(req.Model))
		fc := NewForwardContext(i.c, i.provider)
		stream, cancel, err := ForwardAnthropicV1BetaStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return err
		}

		var msg anthropic.BetaMessage
		if err := i.consumeRoundBeta(stream, &msg); err != nil {
			return err
		}

		i.accumulateUsageBeta(msg)

		if msg.StopReason != anthropic.BetaStopReasonToolUse {
			i.advanceBlockIndex()
			i.sendFinalMessageDeltaBeta(string(msg.StopReason))
			sendMessageStop(i.c)
			return nil
		}

		// Check if there are virtual tools to execute server-side.
		// External tools have already been passed through to the client in real-time.
		virtualToolUses := i.classifyVirtualToolUsesBeta(msg.Content)
		hasExternalToolUses := i.hasExternalToolUsesBeta(msg.Content)

		if len(virtualToolUses) == 0 {
			// No virtual tools in this round; external tools passed through.
			// Wait for client to send tool_results in a follow-up request.
			i.advanceBlockIndex()
			i.sendFinalMessageDeltaBeta(string(msg.StopReason))
			sendMessageStop(i.c)
			return nil
		}

		if hasExternalToolUses {
			// Mixed scenario (virtual + external): execute virtual tools server-side
			// and stash results for the client's follow-up request merge.
			// External tools are passed through to client in this response.
			virtualResults := i.executeMCPToolsBeta(i.c.Request.Context(), virtualToolUses, req.Messages, &msg)
			i.s.stashPendingVirtualToolResults(i.externalToolUseIDsBeta(msg.Content), virtualResults)
			i.advanceBlockIndex()
			i.sendFinalMessageDeltaBeta(string(msg.StopReason))
			sendMessageStop(i.c)
			return nil
		}

		// Pure virtual tools: execute server-side and continue to next round
		i.advanceBlockIndex()
		sendKeepAlive(i.c)
		virtualResults := i.executeMCPToolsBeta(i.c.Request.Context(), virtualToolUses, req.Messages, &msg)
		toolResults := make([]anthropic.BetaContentBlockParamUnion, 0, len(virtualResults))
		for _, r := range virtualResults {
			toolResults = append(toolResults, anthropic.NewBetaToolResultBlock(r.ToolUseID, r.Content, r.IsError))
		}

		req.Messages = append(req.Messages,
			msg.ToParam(),
			anthropic.NewBetaUserMessage(toolResults...),
		)
	}

	i.sendFinalMessageDeltaBeta("max_rounds_exceeded")
	sendMessageStop(i.c)
	return nil
}

func (i *AnthropicBetaStreamInterceptor) consumeRoundBeta(
	stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion],
	msg *anthropic.BetaMessage,
) error {
	for stream.Next() {
		event := stream.Current()

		// Step 1: Accumulate
		msg.Accumulate(event)

		// Step 2: guardrails hooks
		for _, hook := range i.hc.OnStreamEventHooks {
			hook(event)
		}

		// Step 3: guardrails mutation
		var handled bool
		var rewritten []protocol.GuardrailsBufferedEvent
		var err error
		if i.hc.Guardrails != nil {
			handled, rewritten, err = guardrailsmutate.RewriteAnthropicToolUseEvent(
				i.hc.Guardrails.CredentialMask,
				i.hc.Guardrails.Stream,
				&event,
			)
			if err != nil {
				return err
			}
		}

		// Step 4: route
		if handled {
			for _, r := range rewritten {
				i.routeGuardrailsEventBeta(r)
			}
		} else {
			i.filterAndSendBeta(&event)
		}
	}
	return stream.Err()
}

func (i *AnthropicBetaStreamInterceptor) filterAndSendBeta(
	event *anthropic.BetaRawMessageStreamEventUnion,
) {
	logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: filterAndSendBeta event type=%s index=%d", i.round, event.Type, event.Index)

	switch e := event.AsAny().(type) {
	case anthropic.BetaRawMessageStartEvent:
		if i.round == 0 {
			logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Sending message_start", i.round)
			i.c.SSEvent(event.Type, event.RawJSON())
			i.c.Writer.Flush()
		}

	case anthropic.BetaRawContentBlockStartEvent:
		if block, ok := e.ContentBlock.AsAny().(anthropic.BetaToolUseBlock); ok {
			if isVirtualTool(block.Name, i.virtualRegistry) {
				logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Suppressing virtual tool_use name=%s index=%d", i.round, block.Name, e.Index)
				i.virtualToolIndices[int(e.Index)] = true
				if int(e.Index) > i.maxBlockIndex {
					i.maxBlockIndex = int(e.Index)
				}
				return // suppress virtual tool_use start
			}
			logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Passing external tool_use name=%s index=%d", i.round, block.Name, e.Index)
		} else {
			logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Passing content_block_start type=text index=%d", i.round, e.Index)
		}
		if int(e.Index) > i.maxBlockIndex {
			i.maxBlockIndex = int(e.Index)
		}
		i.c.SSEvent(event.Type, rewriteIndex([]byte(event.RawJSON()), i.blockIndexOffset))
		i.c.Writer.Flush()
		i.roundBlockCount++

	case anthropic.BetaRawContentBlockDeltaEvent:
		if i.virtualToolIndices[int(e.Index)] {
			logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Suppressing virtual tool delta index=%d", i.round, e.Index)
			return // suppress virtual tool delta
		}
		switch delta := e.Delta.AsAny().(type) {
		case anthropic.BetaTextDelta:
			if delta.Text != "" {
				if !i.ttftRecorded {
					SetFirstTokenTime(i.c)
					i.ttftRecorded = true
				}
				logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Sending text delta length=%d index=%d", i.round, len(delta.Text), e.Index)
				i.c.SSEvent(event.Type, rewriteIndex([]byte(event.RawJSON()), i.blockIndexOffset))
				i.c.Writer.Flush()
			}
		case anthropic.BetaInputJSONDelta:
			// External tool parameter deltas must be sent to client
			logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Sending input_json_delta for external tool index=%d partial_json=%s", i.round, e.Index, delta.PartialJSON)
			i.c.SSEvent(event.Type, rewriteIndex([]byte(event.RawJSON()), i.blockIndexOffset))
			i.c.Writer.Flush()
		}

	case anthropic.BetaRawContentBlockStopEvent:
		if i.virtualToolIndices[int(e.Index)] {
			logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Suppressing virtual tool stop index=%d", i.round, e.Index)
			delete(i.virtualToolIndices, int(e.Index))
			return // suppress virtual tool stop
		}
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Sending content_block_stop index=%d", i.round, e.Index)
		i.c.SSEvent(event.Type, rewriteIndex([]byte(event.RawJSON()), i.blockIndexOffset))
		i.c.Writer.Flush()

	case anthropic.BetaRawMessageDeltaEvent:
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Accumulating message_delta", i.round)
		// Silently accumulate; interceptor controls end timing

	case anthropic.BetaRawMessageStopEvent:
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Suppressing message_stop", i.round)
		// Suppress mid-stream; interceptor controls end timing

	default:
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Passing default event type=%s", i.round, event.Type)
		i.c.SSEvent(event.Type, event.RawJSON())
		i.c.Writer.Flush()
	}
}

func (i *AnthropicBetaStreamInterceptor) routeGuardrailsEventBeta(
	event protocol.GuardrailsBufferedEvent,
) {
	switch event.EventType {
	case "content_block_start":
		block, _ := event.Payload["content_block"].(map[string]any)
		index := int(event.Payload["index"].(float64))
		if block["type"] == "tool_use" {
			name, _ := block["name"].(string)
			if isVirtualTool(name, i.virtualRegistry) {
				// virtual tool_use -> buffer for server-side execution
				logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Buffering virtual tool_use index=%d name=%s", i.round, index, name)
				i.toolUseEventBuffer[index] = append(i.toolUseEventBuffer[index], event)
				return
			}
			// external tool_use -> pass through with index rewrite
			rewritten := rewriteGuardrailsIndex(event, i.blockIndexOffset)
			logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Passing external tool_use index=%d->%d name=%s", i.round, index, i.blockIndexOffset+int(index), name)
			i.c.SSEvent(event.EventType, string(rewritten))
			i.c.Writer.Flush()
			i.roundBlockCount++
			return
		}
		// synthetic text block -> pass through
		rewritten := rewriteGuardrailsIndex(event, i.blockIndexOffset)
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Passing text block index=%d", i.round, i.blockIndexOffset+int(index))
		i.c.SSEvent(event.EventType, string(rewritten))
		i.c.Writer.Flush()
		i.roundBlockCount++

	case "content_block_delta", "content_block_stop":
		index := int(event.Payload["index"].(float64))
		if _, buffered := i.toolUseEventBuffer[index]; buffered {
			logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Buffering %s for index=%d", i.round, event.EventType, index)
			i.toolUseEventBuffer[index] = append(i.toolUseEventBuffer[index], event)
			return
		}
		rewritten := rewriteGuardrailsIndex(event, i.blockIndexOffset)
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Passing %s index=%d", i.round, event.EventType, i.blockIndexOffset+int(index))
		i.c.SSEvent(event.EventType, string(rewritten))
		i.c.Writer.Flush()

	default:
		// message_delta etc -> passthrough
		rewritten := rewriteGuardrailsIndex(event, i.blockIndexOffset)
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Passing %s", i.round, event.EventType)
		i.c.SSEvent(event.EventType, string(rewritten))
		i.c.Writer.Flush()
	}
}

func (i *AnthropicBetaStreamInterceptor) classifyVirtualToolUsesBeta(
	content []anthropic.BetaContentBlockUnion,
) []anthropic.BetaToolUseBlock {
	var toolUses []anthropic.BetaToolUseBlock
	for _, block := range content {
		if tu, ok := block.AsAny().(anthropic.BetaToolUseBlock); ok {
			if isVirtualTool(tu.Name, i.virtualRegistry) {
				toolUses = append(toolUses, tu)
			}
		}
	}
	return toolUses
}

func (i *AnthropicBetaStreamInterceptor) hasExternalToolUsesBeta(
	content []anthropic.BetaContentBlockUnion,
) bool {
	for _, block := range content {
		if tu, ok := block.AsAny().(anthropic.BetaToolUseBlock); ok {
			if !isVirtualTool(tu.Name, i.virtualRegistry) {
				return true
			}
		}
	}
	return false
}

func (i *AnthropicBetaStreamInterceptor) externalToolUseIDsBeta(
	content []anthropic.BetaContentBlockUnion,
) []string {
	out := make([]string, 0)
	for _, block := range content {
		if tu, ok := block.AsAny().(anthropic.BetaToolUseBlock); ok {
			if isVirtualTool(tu.Name, i.virtualRegistry) {
				continue
			}
			if tu.ID != "" {
				out = append(out, string(tu.ID))
			}
		}
	}
	return out
}

func (i *AnthropicBetaStreamInterceptor) executeMCPToolsBeta(
	ctx context.Context,
	toolUses []anthropic.BetaToolUseBlock,
	messages []anthropic.BetaMessageParam,
	msg *anthropic.BetaMessage,
) []virtualToolExecutionResult {
	logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: executeMCPToolsBeta called with %d tools", i.round, len(toolUses))

	allMessages := make([]anthropic.BetaMessageParam, 0, len(messages)+1)
	allMessages = append(allMessages, messages...)
	allMessages = append(allMessages, msg.ToParam())
	hookMessages := extractAnthropicBetaMessages(allMessages)

	results := make([]virtualToolExecutionResult, 0, len(toolUses))
	for _, tu := range toolUses {
		args := "{}"
		if tu.Input != nil {
			if b, ok := tu.Input.([]byte); ok {
				args = string(b)
			} else {
				b, err := json.Marshal(tu.Input)
				if err == nil {
					args = string(b)
				}
			}
		}
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Executing virtual tool: name=%s id=%s args=%s", i.round, tu.Name, tu.ID, args)
		result, err := i.s.callMCPToolWithHooks(ctx, tu.Name, args, hookMessages)
		if err != nil {
			logrus.WithError(err).Warnf("mcp: virtual tool call failed name=%s", tu.Name)
		}
		logrus.Debugf("[MCP-SSE-BETA-DEBUG] Round %d: Virtual tool result: name=%s result=%s hasError=%v", i.round, tu.Name, result, err != nil)
		results = append(results, virtualToolExecutionResult{
			ToolUseID: string(tu.ID),
			Content:   result,
			IsError:   err != nil,
		})
	}
	return results
}

func (i *AnthropicBetaStreamInterceptor) accumulateUsageBeta(msg anthropic.BetaMessage) {
	i.totalInputTokens += msg.Usage.InputTokens
	i.totalOutputTokens += msg.Usage.OutputTokens
	i.totalCacheCreationTokens += msg.Usage.CacheCreationInputTokens
	i.totalCacheReadTokens += msg.Usage.CacheReadInputTokens
}

func (i *AnthropicBetaStreamInterceptor) sendFinalMessageDeltaBeta(stopReason string) {
	payload := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"input_tokens":                i.totalInputTokens,
			"output_tokens":               i.totalOutputTokens,
			"cache_creation_input_tokens": i.totalCacheCreationTokens,
			"cache_read_input_tokens":     i.totalCacheReadTokens,
		},
	}
	b, _ := json.Marshal(payload)
	i.c.SSEvent("message_delta", string(b))
	i.c.Writer.Flush()
}

func (i *AnthropicBetaStreamInterceptor) reportUsageBeta() {
	usage := protocol.NewTokenUsageWithCache(
		int(i.totalInputTokens),
		int(i.totalOutputTokens),
		int(i.totalCacheCreationTokens+i.totalCacheReadTokens),
	)
	i.s.trackUsageWithTokenUsage(i.c, usage, nil)
}

// ============================================================================
// OpenAI Chat Stream Interceptor
// ============================================================================

// openAIToolCallState tracks the accumulation of one tool call from stream chunks.
type openAIToolCallState struct {
	index     int
	id        string
	name      string
	arguments strings.Builder
}

// OpenAIChatStreamInterceptor intercepts OpenAI Chat streaming responses
// for all-virtual tool_calls scenarios.
type OpenAIChatStreamInterceptor struct {
	c               *gin.Context
	s               *Server
	provider        *typ.Provider
	hc              *protocol.HandleContext
	virtualRegistry *runtime.VirtualToolRegistry
	recorder        *ProtocolRecorder
	responseModel   string
	disableUsage    bool

	// cross-round state (not reset)
	ttftRecorded      bool
	totalInputTokens  int64
	totalOutputTokens int64

	// per-round state
	toolCallBuffer   map[int]*openAIToolCallState
	chunkBuffer      []openai.ChatCompletionChunk
	usageAccumulated struct {
		inputTokens  int
		outputTokens int
		hasUsage     bool
	}
}

// NewOpenAIChatStreamInterceptor creates a new OpenAI chat stream interceptor.
func NewOpenAIChatStreamInterceptor(
	c *gin.Context,
	s *Server,
	provider *typ.Provider,
	hc *protocol.HandleContext,
	virtualRegistry *runtime.VirtualToolRegistry,
	recorder *ProtocolRecorder,
	responseModel string,
	disableUsage bool,
) *OpenAIChatStreamInterceptor {
	return &OpenAIChatStreamInterceptor{
		c:               c,
		s:               s,
		provider:        provider,
		hc:              hc,
		virtualRegistry: virtualRegistry,
		recorder:        recorder,
		responseModel:   responseModel,
		disableUsage:    disableUsage,
	}
}

// Run executes the interceptor loop for all-virtual tool_calls streaming.
func (i *OpenAIChatStreamInterceptor) Run(req *openai.ChatCompletionNewParams) error {
	setupOpenAISSEHeaders(i.c)
	defer i.reportUsageOpenAI()

	for round := 0; round < maxInterceptorRounds; round++ {
		i.resetRoundState()

		wrapper := i.s.clientPool.GetOpenAIClient(i.c.Request.Context(), i.provider, req.Model)
		fc := NewForwardContext(i.c.Request.Context(), i.provider)
		stream, cancel, err := ForwardOpenAIChatStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return err
		}

		completion, err := i.consumeRound(stream)
		if err != nil {
			return err
		}

		if len(completion.Choices) == 0 || completion.Choices[0].FinishReason != "tool_calls" {
			i.sendFinalChunks(completion)
			i.sendOpenAIDone()
			return nil
		}

		// Check if there are virtual tools to execute server-side.
		// External tools have already been passed through to the client in real-time.
		virtualToolCalls := i.classifyVirtualToolCallsOpenAI(completion)
		externalToolCallIDs := i.externalToolCallIDsOpenAI(completion)
		if len(virtualToolCalls) == 0 {
			// No virtual tools in this round; external tools passed through.
			// Wait for client to send tool_results in a follow-up request.
			i.sendFinalChunks(completion)
			i.sendOpenAIDone()
			return nil
		}

		if len(externalToolCallIDs) > 0 {
			// Mixed scenario (virtual + external): execute virtual tools server-side
			// and stash results for the client's follow-up request merge.
			// External tool calls are emitted to client in this response.
			virtualResults := i.executeMCPToolsOpenAI(i.c.Request.Context(), virtualToolCalls, req.Messages, &completion)
			i.s.stashPendingVirtualToolResults(externalToolCallIDs, virtualResults)
			i.sendFinalChunks(completion)
			i.sendOpenAIDone()
			return nil
		}

		// Execute virtual tools server-side and re-forward
		sendKeepAlive(i.c)
		virtualResults := i.executeMCPToolsOpenAI(i.c.Request.Context(), virtualToolCalls, req.Messages, &completion)

		req.Messages = append(req.Messages, completion.Choices[0].Message.ToParam())
		for _, r := range virtualResults {
			req.Messages = append(req.Messages, openai.ToolMessage(r.Content, r.ToolUseID))
		}
	}

	i.sendFinalChunks(openai.ChatCompletion{})
	i.sendOpenAIDone()
	return nil
}

func (i *OpenAIChatStreamInterceptor) resetRoundState() {
	i.toolCallBuffer = make(map[int]*openAIToolCallState)
	i.chunkBuffer = nil
	i.usageAccumulated = struct {
		inputTokens  int
		outputTokens int
		hasUsage     bool
	}{}
}

func (i *OpenAIChatStreamInterceptor) consumeRound(
	stream *openaistream.Stream[openai.ChatCompletionChunk],
) (openai.ChatCompletion, error) {
	var completion openai.ChatCompletion

	for stream.Next() {
		chunk := stream.Current()

		// Step 1: guardrails hooks (OpenAI has hook but no mutation)
		for _, hook := range i.hc.OnStreamEventHooks {
			hook(chunk)
		}

		// Step 2: accumulate and decide whether to send
		hasToolCalls := i.accumulateChunk(&completion, chunk)

		if !hasToolCalls {
			// No tool_calls in this chunk - check if it's pure text
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				if !i.ttftRecorded {
					SetFirstTokenTime(i.c)
					i.ttftRecorded = true
				}
				i.sendOpenAIChunk(chunk)
			} else if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
				// Finish reason chunk - don't send yet, interceptor controls end
				i.chunkBuffer = append(i.chunkBuffer, chunk)
			} else {
				// Other chunks (usage-only, etc) - buffer or send
				i.chunkBuffer = append(i.chunkBuffer, chunk)
			}
		}
		// If hasToolCalls, chunk was accumulated but NOT sent (buffered internally)
	}

	return completion, stream.Err()
}

// accumulateChunk builds up the completion from stream chunks.
// Returns true if this chunk contains tool_calls that need buffering.
func (i *OpenAIChatStreamInterceptor) accumulateChunk(
	completion *openai.ChatCompletion,
	chunk openai.ChatCompletionChunk,
) bool {
	hasToolCalls := false

	if len(chunk.Choices) > 0 {
		choice := chunk.Choices[0]

		// Ensure completion has at least one choice
		if len(completion.Choices) == 0 {
			completion.Choices = append(completion.Choices, openai.ChatCompletionChoice{})
		}

		// Accumulate content
		if choice.Delta.Content != "" {
			completion.Choices[0].Message.Content += choice.Delta.Content
		}

		// Accumulate tool_calls
		for _, tc := range choice.Delta.ToolCalls {
			hasToolCalls = true
			state := i.getOrCreateToolCallState(int(tc.Index))
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

		// Accumulate finish_reason
		if choice.FinishReason != "" {
			completion.Choices[0].FinishReason = choice.FinishReason
		}
	}

	// Accumulate usage (per-round and cross-round)
	if chunk.Usage.PromptTokens != 0 {
		i.usageAccumulated.inputTokens = int(chunk.Usage.PromptTokens)
		i.totalInputTokens += chunk.Usage.PromptTokens
		i.usageAccumulated.hasUsage = true
	}
	if chunk.Usage.CompletionTokens != 0 {
		i.usageAccumulated.outputTokens = int(chunk.Usage.CompletionTokens)
		i.totalOutputTokens += chunk.Usage.CompletionTokens
		i.usageAccumulated.hasUsage = true
	}

	return hasToolCalls
}

func (i *OpenAIChatStreamInterceptor) getOrCreateToolCallState(index int) *openAIToolCallState {
	if state, ok := i.toolCallBuffer[index]; ok {
		return state
	}
	state := &openAIToolCallState{index: index}
	i.toolCallBuffer[index] = state
	return state
}

func (i *OpenAIChatStreamInterceptor) classifyVirtualToolCallsOpenAI(
	completion openai.ChatCompletion,
) []openai.ChatCompletionMessageToolCallUnion {
	if len(completion.Choices) == 0 {
		return nil
	}

	toolCalls := completion.Choices[0].Message.ToolCalls
	if len(toolCalls) == 0 {
		return nil
	}

	var virtualCalls []openai.ChatCompletionMessageToolCallUnion
	for idx, tc := range toolCalls {
		fn := tc.AsFunction()
		if fn.Function.Name == "" {
			// Try to reconstruct from buffer if available
			if state, ok := i.toolCallBuffer[idx]; ok {
				fn.Function.Name = state.name
				fn.Function.Arguments = state.arguments.String()
			}
		}
		if isVirtualTool(fn.Function.Name, i.virtualRegistry) {
			virtualCalls = append(virtualCalls, tc)
		}
	}
	return virtualCalls
}

func (i *OpenAIChatStreamInterceptor) executeMCPToolsOpenAI(
	ctx context.Context,
	toolCalls []openai.ChatCompletionMessageToolCallUnion,
	messages []openai.ChatCompletionMessageParamUnion,
	completion *openai.ChatCompletion,
) []virtualToolExecutionResult {
	// Build hook messages from conversation history
	allMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)
	allMessages = append(allMessages, messages...)
	allMessages = append(allMessages, completion.Choices[0].Message.ToParam())
	hookMessages := extractOpenAIMessages(allMessages)

	results := make([]virtualToolExecutionResult, 0, len(toolCalls))
	for idx, tc := range toolCalls {
		fn := tc.AsFunction()
		args := fn.Function.Arguments
		if args == "" {
			args = "{}"
		}
		result, err := i.s.callMCPToolWithHooks(ctx, fn.Function.Name, args, hookMessages)
		if err != nil {
			logrus.WithError(err).Warnf("mcp: virtual tool call failed name=%s", fn.Function.Name)
		}

		// Use buffered ID if the function ID is empty
		toolID := fn.ID
		if toolID == "" {
			if state, ok := i.toolCallBuffer[idx]; ok {
				toolID = state.id
			}
		}

		results = append(results, virtualToolExecutionResult{
			ToolUseID: toolID,
			Content:   result,
			IsError:   err != nil,
		})
	}
	return results
}

func (i *OpenAIChatStreamInterceptor) externalToolCallIDsOpenAI(
	completion openai.ChatCompletion,
) []string {
	if len(completion.Choices) == 0 {
		return nil
	}
	toolCalls := completion.Choices[0].Message.ToolCalls
	if len(toolCalls) == 0 {
		return nil
	}

	out := make([]string, 0, len(toolCalls))
	for idx, tc := range toolCalls {
		fn := tc.AsFunction()
		if fn.Function.Name == "" {
			if state, ok := i.toolCallBuffer[idx]; ok {
				fn.Function.Name = state.name
			}
		}
		if isVirtualTool(fn.Function.Name, i.virtualRegistry) {
			continue
		}
		toolID := fn.ID
		if toolID == "" {
			if state, ok := i.toolCallBuffer[idx]; ok {
				toolID = state.id
			}
		}
		if toolID != "" {
			out = append(out, toolID)
		}
	}
	return out
}

func (i *OpenAIChatStreamInterceptor) flushOpenAIChunkBuffer() {
	for _, chunk := range i.chunkBuffer {
		i.sendOpenAIChunk(chunk)
	}
	i.chunkBuffer = nil
}

func (i *OpenAIChatStreamInterceptor) sendFinalChunks(completion openai.ChatCompletion) {
	// Send any buffered non-tool chunks
	i.flushOpenAIChunkBuffer()

	// Emit tool_calls to client for external/mixed scenarios.
	if len(completion.Choices) > 0 && len(completion.Choices[0].Message.ToolCalls) > 0 {
		i.sendOpenAIToolCallsChunk(completion)
	}

	// Send final chunk with finish_reason
	finishReason := ""
	if len(completion.Choices) > 0 {
		finishReason = string(completion.Choices[0].FinishReason)
	}
	if finishReason != "" {
		finalChunk := openai.ChatCompletionChunk{
			ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   i.responseModel,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionChunkChoiceDelta{
						Content: "",
					},
					FinishReason: finishReason,
				},
			},
		}
		i.sendOpenAIChunk(finalChunk)
	}

	// Send aggregated usage if not disabled
	if !i.disableUsage && (i.totalInputTokens > 0 || i.totalOutputTokens > 0) {
		usageChunk := i.buildUsageChunk()
		i.sendOpenAIChunk(usageChunk)
	}
}

func (i *OpenAIChatStreamInterceptor) buildUsageChunk() openai.ChatCompletionChunk {
	return openai.ChatCompletionChunk{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   i.responseModel,
		Choices: []openai.ChatCompletionChunkChoice{},
		Usage: openai.CompletionUsage{
			PromptTokens:     i.totalInputTokens,
			CompletionTokens: i.totalOutputTokens,
			TotalTokens:      i.totalInputTokens + i.totalOutputTokens,
		},
	}
}

func (i *OpenAIChatStreamInterceptor) sendOpenAIChunk(chunk openai.ChatCompletionChunk) {
	flusher, ok := i.c.Writer.(http.Flusher)
	if !ok {
		return
	}

	// Build the chunk in OpenAI format
	delta := map[string]interface{}{}
	if len(chunk.Choices) > 0 {
		choice := chunk.Choices[0]
		if choice.Delta.Role != "" {
			delta["role"] = choice.Delta.Role
		}
		delta["content"] = choice.Delta.Content
		if choice.Delta.Refusal != "" {
			delta["refusal"] = choice.Delta.Refusal
		}
		if len(choice.Delta.ToolCalls) > 0 {
			delta["tool_calls"] = choice.Delta.ToolCalls
		}

		finishReason := &choice.FinishReason
		if finishReason != nil && *finishReason == "" {
			finishReason = nil
		}

		outChunk := map[string]interface{}{
			"id":      chunk.ID,
			"object":  "chat.completion.chunk",
			"created": chunk.Created,
			"model":   i.responseModel,
			"choices": []map[string]interface{}{
				{
					"index":         choice.Index,
					"delta":         delta,
					"finish_reason": finishReason,
					"logprobs":      choice.Logprobs,
				},
			},
		}

		if !i.disableUsage && (chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0) {
			outChunk["usage"] = chunk.Usage
		}

		if chunk.SystemFingerprint != "" {
			outChunk["system_fingerprint"] = chunk.SystemFingerprint
		}

		chunkJSON, err := json.Marshal(outChunk)
		if err != nil {
			logrus.Errorf("Failed to marshal chunk: %v", err)
			return
		}
		i.c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", chunkJSON))
		flusher.Flush()
		return
	}

	// Usage-only chunk (no choices)
	outChunk := map[string]interface{}{
		"id":      chunk.ID,
		"object":  "chat.completion.chunk",
		"created": chunk.Created,
		"model":   i.responseModel,
		"choices": []map[string]interface{}{},
	}
	if !i.disableUsage && (chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0) {
		outChunk["usage"] = chunk.Usage
	}

	chunkJSON, err := json.Marshal(outChunk)
	if err != nil {
		logrus.Errorf("Failed to marshal chunk: %v", err)
		return
	}
	i.c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", chunkJSON))
	flusher.Flush()
}

func (i *OpenAIChatStreamInterceptor) sendOpenAIDone() {
	flusher, ok := i.c.Writer.(http.Flusher)
	if !ok {
		return
	}
	i.c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()
}

func (i *OpenAIChatStreamInterceptor) sendOpenAIToolCallsChunk(completion openai.ChatCompletion) {
	if len(completion.Choices) == 0 {
		return
	}
	choice := completion.Choices[0]
	if len(choice.Message.ToolCalls) == 0 {
		return
	}

	deltaToolCalls := convertMessageToolCallsToDeltaToolCalls(choice.Message.ToolCalls)
	if len(deltaToolCalls) == 0 {
		return
	}

	toolChunk := openai.ChatCompletionChunk{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   i.responseModel,
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: choice.Index,
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Role:      "assistant",
					Content:   "",
					ToolCalls: deltaToolCalls,
				},
				FinishReason: "",
			},
		},
	}
	i.sendOpenAIChunk(toolChunk)
}

func convertMessageToolCallsToDeltaToolCalls(
	toolCalls []openai.ChatCompletionMessageToolCallUnion,
) []openai.ChatCompletionChunkChoiceDeltaToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	raw, err := json.Marshal(toolCalls)
	if err != nil {
		logrus.WithError(err).Warn("mcp: failed to marshal message tool_calls for chunk conversion")
		return nil
	}
	var out []openai.ChatCompletionChunkChoiceDeltaToolCall
	if err := json.Unmarshal(raw, &out); err != nil {
		logrus.WithError(err).Warn("mcp: failed to unmarshal tool_calls into chunk delta shape")
		return nil
	}
	return out
}

func (i *OpenAIChatStreamInterceptor) reportUsageOpenAI() {
	usage := protocol.NewTokenUsageWithCache(
		int(i.totalInputTokens),
		int(i.totalOutputTokens),
		0,
	)
	i.s.trackUsageWithTokenUsage(i.c, usage, nil)
}

func setupOpenAISSEHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
}
