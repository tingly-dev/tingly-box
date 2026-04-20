package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GenericLoopProcessor implements format-agnostic non-streaming MCP tool handling
type GenericLoopProcessor struct {
	ctx              context.Context
	s                ServerOps
	provider         *typ.Provider
	hc               *protocol.HandleContext
	virtualRegistry  *runtime.VirtualToolRegistry
	recorder         ProtocolRecorder
	adapter          FormatAdapter
	forwarder        Forwarder
	toolExecutor     ToolExecutor
	pendingManager   PendingResultsManager
	config           InterceptorConfig

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
	pendingManager PendingResultsManager,
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
		pendingManager:  pendingManager,
		config:          config,
	}
}

// Run executes the non-streaming processor loop
func (p *GenericLoopProcessor) Run(req any) (any, error) {
	currentReq := req

	for round := 0; round < p.config.MaxRounds; round++ {
		logrus.Debugf("[MCP-Processor] Round %d: starting", round)

		// Forward request to upstream (blocking)
		response, err := p.forwarder.ForwardNonStream(p.ctx, p.provider, p.extractModel(currentReq), currentReq)
		if err != nil {
			return nil, fmt.Errorf("forward non-stream failed: %w", err)
		}

		// Extract the actual response from ForwardResult wrapper
		if fr, ok := response.(*ForwardResult); ok {
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

	// Stash results linked to external IDs
	if p.pendingManager != nil && len(externalIDs) > 0 {
		if err := p.pendingManager.Stash(externalIDs, results); err != nil {
			logrus.WithError(err).Warn("failed to stash pending results")
		}
	}

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
	result, err := p.s.CallMCPTool(p.ctx, tool.Name(), tool.Arguments(), messages)

	return ToolExecutionResult{
		ToolUseID: tool.ID(),
		Content:   result,
		IsError:   err != nil,
	}, err
}

// Helper methods

func (p *GenericLoopProcessor) extractModel(req any) string {
	// Extract model from request based on format
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		return string(r.Model)
	case *anthropic.BetaMessageNewParams:
		return string(r.Model)
	case *openai.ChatCompletionNewParams:
		return string(r.Model)
	default:
		// Fallback to provider name if available
		if len(p.provider.Models) > 0 {
			return p.provider.Models[0]
		}
		return ""
	}
}

func (p *GenericLoopProcessor) extractMessages(req any) []map[string]any {
	// Extract messages from request for tool execution hooks
	// Note: Current CallMCPTool implementation does not use messages parameter,
	// but this is implemented for future compatibility
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		if len(r.Messages) == 0 {
			return nil
		}
		b, _ := json.Marshal(r.Messages)
		var out []map[string]any
		json.Unmarshal(b, &out)
		return out
	case *anthropic.BetaMessageNewParams:
		if len(r.Messages) == 0 {
			return nil
		}
		b, _ := json.Marshal(r.Messages)
		var out []map[string]any
		json.Unmarshal(b, &out)
		return out
	case *openai.ChatCompletionNewParams:
		// For OpenAI, convert messages to map format
		if len(r.Messages) == 0 {
			return nil
		}
		messages := make([]map[string]any, len(r.Messages))
		for i, msg := range r.Messages {
			// Convert OpenAI message to map representation
			msgJSON, _ := msg.MarshalJSON()
			var msgMap map[string]any
			json.Unmarshal(msgJSON, &msgMap)
			messages[i] = msgMap
		}
		return messages
	default:
		return nil
	}
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
