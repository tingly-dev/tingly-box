package mcp

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GenericLoopProcessor implements format-agnostic non-streaming MCP tool handling
type GenericLoopProcessor struct {
	ctx             context.Context
	s               ServerOps
	provider        *typ.Provider
	hc              *protocol.HandleContext
	virtualRegistry *runtime.VirtualToolRegistry
	recorder        ProtocolRecorder
	adapter         FormatAdapter
	forwarder       Forwarder
	toolExecutor    ToolExecutor
	config          InterceptorConfig

	// Usage tracking
	totalInputTokens  int64
	totalOutputTokens int64
	totalCacheTokens  int64
}

// NewGenericLoopProcessor creates a new generic non-streaming processor
func NewGenericLoopProcessor(
	ctx context.Context,
	s ServerOps,
	provider *typ.Provider,
	hc *protocol.HandleContext,
	virtualRegistry *runtime.VirtualToolRegistry,
	recorder ProtocolRecorder,
	adapter FormatAdapter,
	forwarder Forwarder,
	toolExecutor ToolExecutor,
	config InterceptorConfig,
) *GenericLoopProcessor {
	if config.MaxRounds == 0 {
		config.MaxRounds = defaultMaxRounds
	}

	return &GenericLoopProcessor{
		ctx:             ctx,
		s:               s,
		provider:        provider,
		hc:              hc,
		virtualRegistry: virtualRegistry,
		recorder:        recorder,
		adapter:         adapter,
		forwarder:       forwarder,
		toolExecutor:    toolExecutor,
		config:          config,
	}
}

// Run executes the non-streaming processor loop
func (p *GenericLoopProcessor) Run(req any) (any, error) {
	currentReq := p.applyStoredContinuation(req)

	for round := 0; round < p.config.MaxRounds; round++ {
		logrus.Debugf("[MCP-Processor] Round %d: starting", round)

		// Forward request to upstream (blocking)
		response, err := p.forwarder.ForwardNonStream(p.ctx, p.provider, p.extractModel(currentReq), currentReq)
		if err != nil {
			return nil, fmt.Errorf("forward non-stream failed: %w", err)
		}

		// Extract the actual response from ForwardResult wrapper
		if fr, ok := response.(*ForwardResult); ok {
			defer func() {
				if fr.Cancel != nil {
					fr.Cancel()
				}
				if fr.AnthropicClient != nil {
					_ = fr.AnthropicClient.Close()
				}
				if fr.OpenAIClient != nil {
					_ = fr.OpenAIClient.Close()
				}
			}()
			response = fr.Message
		}

		// Record response
		if p.recorder != nil {
			p.recorder.SetAssembledResponse(response)
			p.recorder.RecordResponse(p.provider, p.extractModel(currentReq))
		}

		// Accumulate usage
		if err := p.accumulateUsage(response); err != nil {
			logrus.WithError(err).Warn("failed to accumulate usage")
		}

		// Classify response and decide next action
		decision := p.classifyResponse(response)

		switch decision {
		case DecisionNoTools:
			logrus.Debugf("[MCP-Processor] Round %d: no tools, ending", round)
			p.reportUsage()
			return response, nil

		case DecisionPureVirtual:
			logrus.Debugf("[MCP-Processor] Round %d: pure virtual, continuing", round)
			updatedReq, err := p.handlePureVirtual(response, currentReq)
			if err != nil {
				return nil, err
			}
			currentReq = updatedReq
			continue

		case DecisionPureExternal:
			logrus.Debugf("[MCP-Processor] Round %d: pure external, ending", round)
			p.reportUsage()
			return response, nil

		case DecisionMixed:
			logrus.Debugf("[MCP-Processor] Round %d: mixed, stashing and ending", round)
			filteredResponse, err := p.handleMixed(response, currentReq)
			if err != nil {
				return nil, err
			}
			p.reportUsage()
			return filteredResponse, nil
		}
	}

	// Max rounds exceeded
	logrus.Infof("[MCP-Processor] Max rounds (%d) exceeded", p.config.MaxRounds)
	p.reportUsage()
	return p.adapter.NewResponse(), nil
}

// classifyResponse classifies the response to determine next action
func (p *GenericLoopProcessor) classifyResponse(response any) ResponseDecision {
	tools, err := p.adapter.ExtractTools(response)
	if err != nil || len(tools) == 0 {
		return DecisionNoTools
	}

	hasVirtual := false
	hasExternal := false

	for _, tool := range tools {
		if p.adapter.IsVirtualTool(tool, p.virtualRegistry) {
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

// handlePureVirtual executes virtual tools and returns updated request for next round
func (p *GenericLoopProcessor) handlePureVirtual(response, req any) (any, error) {
	tools, err := p.adapter.ExtractTools(response)
	if err != nil {
		return nil, err
	}

	// Execute virtual tools
	results := make([]ToolExecutionResult, 0, len(tools))
	for _, tool := range tools {
		result, err := p.executeTool(tool, req)
		if err != nil {
			logrus.WithError(err).Warnf("tool execution failed: %s", tool.Name())
		}
		results = append(results, result)
	}

	// Append tool results to request
	updatedReq, err := p.adapter.AppendToolResults(req, response, p.resultsToAny(results))
	if err != nil {
		return nil, err
	}

	return updatedReq, nil
}

// handleMixed executes virtual tools, stashes results, returns filtered response
func (p *GenericLoopProcessor) handleMixed(response, req any) (any, error) {
	tools, err := p.adapter.ExtractTools(response)
	if err != nil {
		return nil, err
	}

	virtual, external, externalIDs := p.adapter.SplitVirtualExternal(tools, p.virtualRegistry)

	// Execute virtual tools
	results := make([]ToolExecutionResult, 0, len(virtual))
	for _, tool := range virtual {
		result, err := p.executeTool(tool, req)
		if err != nil {
			logrus.WithError(err).Warnf("tool execution failed: %s", tool.Name())
		}
		results = append(results, result)
	}

	normalizedResults, err := validateAndNormalizeMixedStash(externalIDs, results)
	if err != nil {
		logrus.WithError(err).Warn("[MCP-Processor] mixed consistency validation failed; ending current round for continuation")
		// Return filtered external calls without stashing, so caller can continue
		// in the next round using the external tool flow only.
		return p.adapter.FilterVirtualTools(response, external)
	}

	segment, err := p.adapter.BuildContinuationSegment(response, normalizedResults)
	if err != nil {
		logrus.WithError(err).Warn("[MCP-Processor] failed to build mixed continuation segment")
		return p.adapter.FilterVirtualTools(response, external)
	}
	key := continuationKey(typ.GetSessionID(p.ctx), p.provider.UUID, p.adapterID())
	mixedContinuationStore.put(key, segment)

	// Filter response to only include external tools
	filteredResponse, err := p.adapter.FilterVirtualTools(response, external)
	if err != nil {
		return nil, err
	}

	return filteredResponse, nil
}

// executeTool executes a single virtual tool
func (p *GenericLoopProcessor) executeTool(tool Tool, req any) (ToolExecutionResult, error) {
	// Extract messages from request
	messages := p.extractMessages(req)

	// Execute tool
	type toolHookCaller interface {
		CallMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (context.Context, string, error)
	}
	nextCtx, result, err := p.s.(toolHookCaller).CallMCPToolWithHooks(p.ctx, tool.Name(), tool.Arguments(), messages)
	if nextCtx != nil {
		p.ctx = nextCtx
	}

	return ToolExecutionResult{
		ToolUseID: tool.ID(),
		Content:   result,
		IsError:   err != nil,
	}, err
}

// Helper methods

func (p *GenericLoopProcessor) extractModel(req any) string {
	return extractModelFromRequest(req, p.provider)
}

func (p *GenericLoopProcessor) extractMessages(req any) []map[string]any {
	return extractMessagesForToolCall(req)
}

func (p *GenericLoopProcessor) resultsToAny(results []ToolExecutionResult) []any {
	out := make([]any, len(results))
	for i, r := range results {
		out[i] = r
	}
	return out
}

func (p *GenericLoopProcessor) accumulateUsage(response any) error {
	usage, err := p.adapter.ExtractUsage(response)
	if err != nil {
		return err
	}

	p.totalInputTokens += int64(usage.InputTokens)
	p.totalOutputTokens += int64(usage.OutputTokens)
	p.totalCacheTokens += int64(usage.CacheTokens)
	return nil
}

func (p *GenericLoopProcessor) reportUsage() {
	if p.s != nil {
		// Note: For non-streaming, we don't have gin.Context here
		// The ServerOps interface might need adjustment or we use a different reporting method
		logrus.Debugf("[MCP-Processor] Usage: Input=%d, Output=%d, Cache=%d",
			p.totalInputTokens, p.totalOutputTokens, p.totalCacheTokens)
	}
}

func (p *GenericLoopProcessor) adapterID() string {
	switch p.adapter.(type) {
	case *OpenAIChatAdapter:
		return "openai-chat"
	case *AnthropicV1Adapter:
		return "anthropic-v1"
	case *AnthropicBetaAdapter:
		return "anthropic-beta"
	default:
		return fmt.Sprintf("%T", p.adapter)
	}
}

func (p *GenericLoopProcessor) applyStoredContinuation(req any) any {
	sessionID := typ.GetSessionID(p.ctx)
	key := continuationKey(sessionID, p.provider.UUID, p.adapterID())
	segment, ok := mixedContinuationStore.pop(key)
	if !ok {
		return req
	}
	logrus.Debugf("[MCP-CONT] processor applying stored continuation key=%s adapter=%s", key, p.adapterID())
	updated, err := p.adapter.ApplyContinuation(req, segment)
	if err != nil {
		logrus.WithError(err).Warn("[MCP-Processor] failed to apply stored continuation")
		return req
	}
	return updated
}
