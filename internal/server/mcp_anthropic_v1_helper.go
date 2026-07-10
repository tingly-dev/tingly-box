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
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ===================================================================
// ServerOps Adapter
// ===================================================================

// serverOpsAdapter implements mcp.ServerOps by wrapping ProtocolHandler
type serverOpsAdapter struct {
	handler    *ProtocolHandler
	recorder   *recording.ProtocolRecorder
	advisorCtx *coretool.AdvisorContext // persists AdvisorContext pointer across CallMCPTool rounds
}

func newServerOpsAdapter(handler *ProtocolHandler, recorder *recording.ProtocolRecorder) *serverOpsAdapter {
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
	recorder *recording.ProtocolRecorder
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

func (ph *ProtocolHandler) RunGenericOpenAIChatNonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *openai.ChatCompletionNewParams,
	recorder *recording.ProtocolRecorder,
) (*openai.ChatCompletion, *mcp.TokenUsage, error) {
	adapter := mcp.NewOpenAIChatAdapter()
	forwarder := mcp.NewOpenAIChatForwarder(ph.deps.ClientPool, &forwardContextProvider{})
	virtualRegistry := ph.deps.MCPRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(ph, recorder)
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

func (ph *ProtocolHandler) RunGenericAnthropicV1NonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.MessageNewParams,
	recorder *recording.ProtocolRecorder,
) (*anthropic.Message, *mcp.TokenUsage, error) {
	adapter := mcp.NewAnthropicV1Adapter()
	forwarder := mcp.NewAnthropicV1Forwarder(ph.deps.ClientPool, &forwardContextProvider{})
	virtualRegistry := ph.deps.MCPRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(ph, recorder)
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

func (ph *ProtocolHandler) RunGenericAnthropicBetaNonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.BetaMessageNewParams,
	recorder *recording.ProtocolRecorder,
) (*anthropic.BetaMessage, *mcp.TokenUsage, error) {

	adapter := mcp.NewAnthropicBetaAdapter()
	forwarder := mcp.NewAnthropicBetaForwarder(ph.deps.ClientPool, &forwardContextProvider{})
	virtualRegistry := ph.deps.MCPRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(ph, recorder)
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
func (ph *ProtocolHandler) DispatchGenericOpenAIChatNonStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *recording.ProtocolRecorder,
) {
	req := reqCtx.Request.(*openai.ChatCompletionNewParams)

	response, usage, err := ph.RunGenericOpenAIChatNonStream(c.Request.Context(), provider, req, recorder)
	if err != nil {
		ph.handlePreStreamFailure(c, err, recorder)
		return
	}

	if usage != nil {
		tokenUsage := protocol.NewTokenUsageWithCache(usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
		ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
	}

	// Update affinity
	ph.updateAffinityMessageID(c, rule, string(response.ID))

	// Return response (OpenAI format)
	c.JSON(http.StatusOK, response)
}

// DispatchGenericOpenAIChatStream handles O→O streaming with generic interceptor
func (ph *ProtocolHandler) DispatchGenericOpenAIChatStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *recording.ProtocolRecorder,
) {
	req := reqCtx.Request.(*openai.ChatCompletionNewParams)
	actualModel := reqCtx.RequestModel
	responseModel := reqCtx.ResponseModel

	// Create adapter
	adapter := mcp.NewOpenAIChatAdapter()

	// Create forwarder
	forwarder := mcp.NewOpenAIChatForwarder(ph.deps.ClientPool, &forwardContextProvider{})

	// Get virtual registry
	virtualRegistry := ph.deps.MCPRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(ph, recorder)
	toolExecutor := mcp.NewServerToolExecutor(serverOps)

	// Create recorder adapter
	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	// Create HandleContext for streaming
	hc := protocol.NewHandleContext(c, responseModel)

	// Add recorder hooks if available
	recording.AttachRecorderHooks(hc, recorder, actualModel, provider)

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
		ph.handlePreStreamFailure(c, err, recorder)
	}
}

// DispatchGenericAnthropicBetaNonStream handles Aβ→Aβ non-streaming with generic processor
func (ph *ProtocolHandler) DispatchGenericAnthropicBetaNonStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *recording.ProtocolRecorder,
) {
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)
	actualModel := reqCtx.RequestModel

	response, usage, err := ph.RunGenericAnthropicBetaNonStream(c.Request.Context(), provider, req, recorder)
	if err != nil {
		ph.handlePreStreamFailure(c, err, recorder)
		return
	}

	if usage != nil {
		tokenUsage := protocol.NewTokenUsageWithCache(usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
		ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
	}

	// Update affinity and get typed message
	ph.updateAffinityMessageID(c, rule, string(response.ID))

	// Response guardrails
	scenario := GetTrackingContextScenario(c)
	if ph.guardrailsEnabledForScenario(scenario) {
		ApplyGuardrailsToAnthropicV1BetaNonStreamResponse(c, ph.currentGuardrailsRuntime(), req, actualModel, provider, response)
	}

	// Return response
	nonstream.WriteAnthropicMessage(c, response)
}

// DispatchGenericAnthropicBetaStream handles Aβ→Aβ streaming with generic interceptor
func (ph *ProtocolHandler) DispatchGenericAnthropicBetaStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *recording.ProtocolRecorder,
) {
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)
	actualModel := reqCtx.RequestModel
	responseModel := reqCtx.ResponseModel

	// Create adapter
	adapter := mcp.NewAnthropicBetaAdapter()

	// Create forwarder
	forwarder := mcp.NewAnthropicBetaForwarder(ph.deps.ClientPool, &forwardContextProvider{})

	// Get virtual registry
	virtualRegistry := ph.deps.MCPRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(ph, recorder)
	toolExecutor := mcp.NewServerToolExecutor(serverOps)

	// Create recorder adapter
	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	// Create HandleContext for streaming
	hc := protocol.NewHandleContext(c, responseModel)

	// Add recorder hooks if available
	recording.AttachRecorderHooks(hc, recorder, actualModel, provider)

	// Response guardrails
	scenario := GetTrackingContextScenario(c)
	guardrailsEnabled := ph.guardrailsEnabledForScenario(scenario)
	interceptorCfg := mcp.InterceptorConfig{MaxRounds: 3, EnableGuardrails: guardrailsEnabled}
	if guardrailsEnabled {
		hc.EnsureGuardrails().Enabled = true
		messages := guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages)
		baseEventHooks := len(hc.OnStreamEventHooks)
		baseErrorHooks := len(hc.OnStreamErrorHooks)
		runtime := ph.currentGuardrailsRuntime()
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
		ph.handlePreStreamFailure(c, err, recorder)
	}
}
