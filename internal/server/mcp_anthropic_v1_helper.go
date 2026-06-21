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

// serverOpsAdapter implements mcp.ServerOps by wrapping Server
type serverOpsAdapter struct {
	server     *Server
	recorder   *ProtocolRecorder
	advisorCtx *coretool.AdvisorContext // persists AdvisorContext pointer across CallMCPTool rounds
}

func newServerOpsAdapter(server *Server, recorder *ProtocolRecorder) *serverOpsAdapter {
	return &serverOpsAdapter{
		server:   server,
		recorder: recorder,
	}
}

func (a *serverOpsAdapter) TrackUsage(c *gin.Context, input, output, cache int) {
	usage := protocol.NewTokenUsageWithCache(input, output, cache)
	a.server.trackUsageWithTokenUsage(c, usage, nil)
}

func (a *serverOpsAdapter) CallMCPTool(ctx context.Context, toolName, arguments string, messages []map[string]any) (string, error) {
	// Inject persisted AdvisorContext into the caller's ctx so UsesRemaining survives
	// across multi-round tool calls without losing the caller's cancellation chain.
	callCtx := ctx
	if a.advisorCtx != nil {
		callCtx = coretool.WithAdvisorContext(ctx, a.advisorCtx)
	}
	updatedCtx, result, err := a.server.callMCPToolWithHooks(callCtx, toolName, arguments, messages)
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
	updatedCtx, result, err := a.server.callMCPToolWithHooks(callCtx, toolName, arguments, messages)
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

// forwardContextProvider implements mcp.ForwardContextGetter
type forwardContextProvider struct {
	server *Server
}

func (p *forwardContextProvider) NewForwardContext(ctx context.Context, provider *typ.Provider) *forwarding.ForwardContext {
	return forwarding.NewForwardContext(ctx, provider)
}

func (s *Server) runGenericOpenAIChatNonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *openai.ChatCompletionNewParams,
	recorder *ProtocolRecorder,
) (*openai.ChatCompletion, *mcp.TokenUsage, error) {
	adapter := mcp.NewOpenAIChatAdapter()
	forwarder := mcp.NewOpenAIChatForwarder(s.clientPool, &forwardContextProvider{server: s})
	virtualRegistry := s.mcpRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(s, recorder)
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

func (s *Server) runGenericAnthropicV1NonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.MessageNewParams,
	recorder *ProtocolRecorder,
) (*anthropic.Message, *mcp.TokenUsage, error) {
	adapter := mcp.NewAnthropicV1Adapter()
	forwarder := mcp.NewAnthropicV1Forwarder(s.clientPool, &forwardContextProvider{server: s})
	virtualRegistry := s.mcpRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(s, recorder)
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

func (s *Server) runGenericAnthropicBetaNonStream(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.BetaMessageNewParams,
	recorder *ProtocolRecorder,
) (*anthropic.BetaMessage, *mcp.TokenUsage, error) {

	adapter := mcp.NewAnthropicBetaAdapter()
	forwarder := mcp.NewAnthropicBetaForwarder(s.clientPool, &forwardContextProvider{server: s})
	virtualRegistry := s.mcpRuntime.VirtualRegistry()
	serverOps := newServerOpsAdapter(s, recorder)
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

// dispatchGenericAnthropicV1NonStream handles A→A non-streaming with generic processor
func (s *Server) dispatchGenericAnthropicV1NonStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *ProtocolRecorder,
) {
	req := reqCtx.Request.(*anthropic.MessageNewParams)
	actualModel := reqCtx.RequestModel

	ctx := c.Request.Context()

	// Create adapter
	adapter := mcp.NewAnthropicV1Adapter()

	// Create forwarder
	forwarder := mcp.NewAnthropicV1Forwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create tool executor
	serverOps := newServerOpsAdapter(s, recorder)
	toolExecutor := mcp.NewServerToolExecutor(serverOps)

	// Create recorder adapter
	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	// Create processor
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

	// Run processor
	response, err := processor.Run(req)
	if err != nil {
		recordMCPError(s, c, err, recorder)
		return
	}

	// Extract usage
	usage, err := adapter.ExtractUsage(response)
	if err == nil {
		serverOps.TrackUsage(c, usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
	}

	// Update affinity and get typed message
	var anthropicMsg *anthropic.Message
	if msg, ok := response.(*anthropic.Message); ok {
		s.updateAffinityMessageID(c, rule, string(msg.ID))
		anthropicMsg = msg
	}

	// Response guardrails
	scenario := GetTrackingContextScenario(c)
	if anthropicMsg != nil && s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1NonStreamResponse(c, req, actualModel, provider, anthropicMsg)
	}

	// Record response if not already recorded
	if recorder != nil && recorderAdapter == nil {
		recorder.SetAssembledResponse(response)
		recorder.RecordResponse(provider, actualModel)
	}

	// Return response
	nonstream.WriteAnthropicMessage(c, response)
}

// dispatchGenericAnthropicV1Stream handles A→A streaming with generic interceptor
func (s *Server) dispatchGenericAnthropicV1Stream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *ProtocolRecorder,
) {
	req := reqCtx.Request.(*anthropic.MessageNewParams)
	actualModel := reqCtx.RequestModel
	responseModel := reqCtx.ResponseModel

	// Create adapter
	adapter := mcp.NewAnthropicV1Adapter()

	// Create forwarder
	forwarder := mcp.NewAnthropicV1Forwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)
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
	guardrailsEnabled := s.guardrailsEnabledForScenario(scenario)
	interceptorCfg := mcp.InterceptorConfig{MaxRounds: 3, EnableGuardrails: guardrailsEnabled}
	if guardrailsEnabled {
		hc.EnsureGuardrails().Enabled = true
		messages := guardrailsadapter.AdaptMessagesFromAnthropicV1(req.System, req.Messages)
		baseEventHooks := len(hc.OnStreamEventHooks)
		baseErrorHooks := len(hc.OnStreamErrorHooks)
		s.attachGuardrailsHooks(c, hc, actualModel, provider, messages)
		interceptorCfg.OnBeforeRound = func(round int) error {
			s.reattachGuardrailsHooks(c, hc, actualModel, provider, messages, baseEventHooks, baseErrorHooks)
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
		recordMCPError(s, c, err, recorder)
		return
	}
}

// dispatchGenericOpenAIChatNonStream handles O→O non-streaming with generic processor
func (s *Server) dispatchGenericOpenAIChatNonStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *ProtocolRecorder,
) {
	req := reqCtx.Request.(*openai.ChatCompletionNewParams)

	response, usage, err := s.runGenericOpenAIChatNonStream(c.Request.Context(), provider, req, recorder)
	if err != nil {
		recordMCPError(s, c, err, recorder)
		return
	}

	if usage != nil {
		tokenUsage := protocol.NewTokenUsageWithCache(usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
		s.trackUsageWithTokenUsage(c, tokenUsage, nil)
	}

	// Update affinity
	s.updateAffinityMessageID(c, rule, string(response.ID))

	// Return response (OpenAI format)
	c.JSON(http.StatusOK, response)
}

// dispatchGenericOpenAIChatStream handles O→O streaming with generic interceptor
func (s *Server) dispatchGenericOpenAIChatStream(
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
	forwarder := mcp.NewOpenAIChatForwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)
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
		recordMCPError(s, c, err, recorder)
	}
}

// dispatchGenericAnthropicBetaNonStream handles Aβ→Aβ non-streaming with generic processor
func (s *Server) dispatchGenericAnthropicBetaNonStream(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	recorder *ProtocolRecorder,
) {
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)
	actualModel := reqCtx.RequestModel

	response, usage, err := s.runGenericAnthropicBetaNonStream(c.Request.Context(), provider, req, recorder)
	if err != nil {
		recordMCPError(s, c, err, recorder)
		return
	}

	if usage != nil {
		tokenUsage := protocol.NewTokenUsageWithCache(usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
		s.trackUsageWithTokenUsage(c, tokenUsage, nil)
	}

	// Update affinity and get typed message
	s.updateAffinityMessageID(c, rule, string(response.ID))

	// Response guardrails
	scenario := GetTrackingContextScenario(c)
	if s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1BetaNonStreamResponse(c, req, actualModel, provider, response)
	}

	// Return response
	nonstream.WriteAnthropicMessage(c, response)
}

// dispatchGenericAnthropicBetaStream handles Aβ→Aβ streaming with generic interceptor
func (s *Server) dispatchGenericAnthropicBetaStream(
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
	forwarder := mcp.NewAnthropicBetaForwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)
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
	guardrailsEnabled := s.guardrailsEnabledForScenario(scenario)
	interceptorCfg := mcp.InterceptorConfig{MaxRounds: 3, EnableGuardrails: guardrailsEnabled}
	if guardrailsEnabled {
		hc.EnsureGuardrails().Enabled = true
		messages := guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages)
		baseEventHooks := len(hc.OnStreamEventHooks)
		baseErrorHooks := len(hc.OnStreamErrorHooks)
		s.attachGuardrailsHooks(c, hc, actualModel, provider, messages)
		interceptorCfg.OnBeforeRound = func(round int) error {
			s.reattachGuardrailsHooks(c, hc, actualModel, provider, messages, baseEventHooks, baseErrorHooks)
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
		recordMCPError(s, c, err, recorder)
	}
}
