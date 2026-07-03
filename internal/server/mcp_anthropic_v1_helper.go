package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ===================================================================
// ServerOps Adapter
// ===================================================================

// serverOpsAdapter implements mcp.ServerOps by wrapping AIHandler
type serverOpsAdapter struct {
	handler    *AIHandler
	recorder   *ProtocolRecorder
	advisorCtx *coretool.AdvisorContext // persists AdvisorContext pointer across CallMCPTool rounds
}

func newServerOpsAdapter(handler *AIHandler, recorder *ProtocolRecorder) *serverOpsAdapter {
	return &serverOpsAdapter{
		handler:  handler,
		recorder: recorder,
	}
}

func (a *serverOpsAdapter) TrackUsage(c *gin.Context, input, output, cache int) {
	usage := protocol.NewTokenUsageWithCache(input, output, cache)
	a.handler.trackUsageWithTokenUsage(c, usage, nil)
}

func (a *serverOpsAdapter) CallMCPTool(ctx context.Context, toolName, arguments string, messages []map[string]any) (string, error) {
	// Inject persisted AdvisorContext into the caller's ctx so UsesRemaining survives
	// across multi-round tool calls without losing the caller's cancellation chain.
	callCtx := ctx
	if a.advisorCtx != nil {
		callCtx = coretool.WithAdvisorContext(ctx, a.advisorCtx)
	}
	updatedCtx, result, err := a.handler.CallMCPToolWithHooks(callCtx, toolName, arguments, messages)
	// Extract and persist the AdvisorContext pointer for the next round.
	if ac, ok := coretool.GetAdvisorContext(updatedCtx); ok {
		a.advisorCtx = ac
	}
	return result.FirstText(), err
}

func (a *serverOpsAdapter) CallMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (context.Context, coretool.ToolResult, error) {
	callCtx := ctx
	if a.advisorCtx != nil {
		callCtx = coretool.WithAdvisorContext(ctx, a.advisorCtx)
	}
	updatedCtx, result, err := a.handler.CallMCPToolWithHooks(callCtx, toolName, arguments, messages)
	if ac, ok := coretool.GetAdvisorContext(updatedCtx); ok {
		a.advisorCtx = ac
	}
	return updatedCtx, result, err
}

func (a *serverOpsAdapter) GetRecorder() mcp.ProtocolRecorder {
	if a.recorder == nil {
		return nil
	}
	return &protocolRecorderAdapter{recorder: a.recorder}
}

// ===================================================================
// ProtocolRecorder Adapter
// ===================================================================

// protocolRecorderAdapter implements mcp.ProtocolRecorder by wrapping ProtocolRecorder
type protocolRecorderAdapter struct {
	recorder *ProtocolRecorder
}

func (a *protocolRecorderAdapter) RecordError(err error) {
	if a.recorder != nil {
		a.recorder.RecordError(err)
	}
}

func (a *protocolRecorderAdapter) SetAssembledResponse(resp any) {
	if a.recorder != nil {
		a.recorder.SetAssembledResponse(resp)
	}
}

func (a *protocolRecorderAdapter) RecordResponse(provider any, model string) {
	if a.recorder != nil {
		if p, ok := provider.(*typ.Provider); ok {
			a.recorder.RecordResponse(p, model)
		}
	}
}

// ===================================================================
// ForwardContextProvider
// ===================================================================

// forwardContextProvider implements mcp.ForwardContextGetter. It carries no
// state of its own — NewForwardContext only needs the per-call ctx/provider.
type forwardContextProvider struct{}

func (p *forwardContextProvider) NewForwardContext(ctx context.Context, provider *typ.Provider) *forwarding.ForwardContext {
	return forwarding.NewForwardContext(ctx, provider)
}

func (ah *AIHandler) RunGenericOpenAIChatNonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *openai.ChatCompletionNewParams,
	recorder *ProtocolRecorder,
) (*openai.ChatCompletion, *mcp.TokenUsage, error) {
	adapter := mcp.NewOpenAIChatAdapter()
	forwarder := mcp.NewOpenAIChatForwarder(ah.deps.ClientPool, &forwardContextProvider{})
	virtualRegistry := ah.deps.MCPRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(ah, recorder)
	toolExecutor := mcp.NewServerToolExecutor(serverOps)

	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	processor := mcp.NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil,
		virtualRegistry,
		recorderAdapter,
		adapter,
		forwarder,
		toolExecutor,
		mcp.InterceptorConfig{MaxRounds: 3},
	)

	response, err := processor.Run(req)
	if err != nil {
		return nil, nil, err
	}

	openaiResp, ok := response.(*openai.ChatCompletion)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected generic response type: %T", response)
	}

	usage, err := adapter.ExtractUsage(response)
	if err != nil {
		return openaiResp, nil, nil
	}
	return openaiResp, &usage, nil
}

func (ah *AIHandler) RunGenericAnthropicV1NonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.MessageNewParams,
	recorder *ProtocolRecorder,
) (*anthropic.Message, *mcp.TokenUsage, error) {
	adapter := mcp.NewAnthropicV1Adapter()
	forwarder := mcp.NewAnthropicV1Forwarder(ah.deps.ClientPool, &forwardContextProvider{})
	virtualRegistry := ah.deps.MCPRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(ah, recorder)
	toolExecutor := mcp.NewServerToolExecutor(serverOps)

	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	processor := mcp.NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil,
		virtualRegistry,
		recorderAdapter,
		adapter,
		forwarder,
		toolExecutor,
		mcp.InterceptorConfig{MaxRounds: 3},
	)

	response, err := processor.Run(req)
	if err != nil {
		return nil, nil, err
	}

	v1Resp, ok := response.(*anthropic.Message)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected generic response type: %T", response)
	}

	usage, err := adapter.ExtractUsage(response)
	if err != nil {
		return v1Resp, nil, nil
	}
	return v1Resp, &usage, nil
}

func (ah *AIHandler) RunGenericAnthropicBetaNonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.BetaMessageNewParams,
	recorder *ProtocolRecorder,
) (*anthropic.BetaMessage, *mcp.TokenUsage, error) {

	adapter := mcp.NewAnthropicBetaAdapter()
	forwarder := mcp.NewAnthropicBetaForwarder(ah.deps.ClientPool, &forwardContextProvider{})
	virtualRegistry := ah.deps.MCPRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(ah, recorder)
	toolExecutor := mcp.NewServerToolExecutor(serverOps)

	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	processor := mcp.NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil,
		virtualRegistry,
		recorderAdapter,
		adapter,
		forwarder,
		toolExecutor,
		mcp.InterceptorConfig{MaxRounds: 3},
	)

	response, err := processor.Run(req)
	if err != nil {
		return nil, nil, err
	}

	betaResp, ok := response.(*anthropic.BetaMessage)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected generic response type: %T", response)
	}

	usage, err := adapter.ExtractUsage(response)
	if err != nil {
		return betaResp, nil, nil
	}
	return betaResp, &usage, nil
}

// DispatchGenericOpenAIChatNonStream handles O→O non-streaming with generic processor
func (ah *AIHandler) DispatchGenericOpenAIChatNonStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *ProtocolRecorder,
) {
	req := reqCtx.Request.(*openai.ChatCompletionNewParams)

	response, usage, err := ah.RunGenericOpenAIChatNonStream(c.Request.Context(), provider, req, recorder)
	if err != nil {
		recordMCPError(ah, c, err, recorder)
		return
	}

	if usage != nil {
		tokenUsage := protocol.NewTokenUsageWithCache(usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
		ah.trackUsageWithTokenUsage(c, tokenUsage, nil)
	}

	// Update affinity
	ah.updateAffinityMessageID(c, rule, string(response.ID))

	// Return response (OpenAI format)
	c.JSON(http.StatusOK, response)
}

// DispatchGenericOpenAIChatStream handles O→O streaming with generic interceptor
func (ah *AIHandler) DispatchGenericOpenAIChatStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *ProtocolRecorder,
) {
	req := reqCtx.Request.(*openai.ChatCompletionNewParams)
	actualModel := reqCtx.RequestModel
	responseModel := reqCtx.ResponseModel

	// Create adapter
	adapter := mcp.NewOpenAIChatAdapter()

	// Create forwarder
	forwarder := mcp.NewOpenAIChatForwarder(ah.deps.ClientPool, &forwardContextProvider{})

	// Get virtual registry
	virtualRegistry := ah.deps.MCPRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(ah, recorder)
	toolExecutor := mcp.NewServerToolExecutor(serverOps)

	// Create recorder adapter
	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	// Create HandleContext for streaming
	hc := protocol.NewHandleContext(c, responseModel)

	// Add recorder hooks if available
	AttachRecorderHooks(hc, recorder, actualModel, provider)

	// Create and run generic interceptor
	interceptor := mcp.NewGenericStreamInterceptor(
		c,
		serverOps,
		provider,
		hc,
		virtualRegistry,
		recorderAdapter,
		adapter,
		forwarder,
		toolExecutor,
		mcp.InterceptorConfig{MaxRounds: 3},
	)

	if err := interceptor.Run(req); err != nil {
		recordMCPError(ah, c, err, recorder)
	}
}

// DispatchGenericAnthropicBetaNonStream handles Aβ→Aβ non-streaming with generic processor
func (ah *AIHandler) DispatchGenericAnthropicBetaNonStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *ProtocolRecorder,
) {
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)
	actualModel := reqCtx.RequestModel

	response, usage, err := ah.RunGenericAnthropicBetaNonStream(c.Request.Context(), provider, req, recorder)
	if err != nil {
		recordMCPError(ah, c, err, recorder)
		return
	}

	if usage != nil {
		tokenUsage := protocol.NewTokenUsageWithCache(usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
		ah.trackUsageWithTokenUsage(c, tokenUsage, nil)
	}

	// Update affinity and get typed message
	ah.updateAffinityMessageID(c, rule, string(response.ID))

	// Response guardrails
	scenario := GetTrackingContextScenario(c)
	if ah.guardrailsEnabledForScenario(scenario) {
		ApplyGuardrailsToAnthropicV1BetaNonStreamResponse(c, ah.currentGuardrailsRuntime(), req, actualModel, provider, response)
	}

	// Return response
	nonstream.WriteAnthropicMessage(c, response)
}

// DispatchGenericAnthropicBetaStream handles Aβ→Aβ streaming with generic interceptor
func (ah *AIHandler) DispatchGenericAnthropicBetaStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *ProtocolRecorder,
) {
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)
	actualModel := reqCtx.RequestModel
	responseModel := reqCtx.ResponseModel

	// Create adapter
	adapter := mcp.NewAnthropicBetaAdapter()

	// Create forwarder
	forwarder := mcp.NewAnthropicBetaForwarder(ah.deps.ClientPool, &forwardContextProvider{})

	// Get virtual registry
	virtualRegistry := ah.deps.MCPRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(ah, recorder)
	toolExecutor := mcp.NewServerToolExecutor(serverOps)

	// Create recorder adapter
	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	// Create HandleContext for streaming
	hc := protocol.NewHandleContext(c, responseModel)

	// Add recorder hooks if available
	AttachRecorderHooks(hc, recorder, actualModel, provider)

	// Response guardrails
	scenario := GetTrackingContextScenario(c)
	guardrailsEnabled := ah.guardrailsEnabledForScenario(scenario)
	interceptorCfg := mcp.InterceptorConfig{MaxRounds: 3, EnableGuardrails: guardrailsEnabled}
	if guardrailsEnabled {
		hc.EnsureGuardrails().Enabled = true
		messages := guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages)
		baseEventHooks := len(hc.OnStreamEventHooks)
		baseErrorHooks := len(hc.OnStreamErrorHooks)
		runtime := ah.currentGuardrailsRuntime()
		AttachGuardrailsHooks(c, runtime, hc, actualModel, provider, messages)
		interceptorCfg.OnBeforeRound = func(round int) error {
			ReattachGuardrailsHooks(c, runtime, hc, actualModel, provider, messages, baseEventHooks, baseErrorHooks)
			return nil
		}
	}

	// Create and run generic interceptor
	interceptor := mcp.NewGenericStreamInterceptor(
		c,
		serverOps,
		provider,
		hc,
		virtualRegistry,
		recorderAdapter,
		adapter,
		forwarder,
		toolExecutor,
		interceptorCfg,
	)

	if err := interceptor.Run(req); err != nil {
		recordMCPError(ah, c, err, recorder)
	}
}
