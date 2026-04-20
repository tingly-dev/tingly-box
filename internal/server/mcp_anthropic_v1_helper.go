package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/mcp"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/anthropics/anthropic-sdk-go"
)

// ===================================================================
// ServerOps Adapter
// ===================================================================

// serverOpsAdapter implements mcp.ServerOps by wrapping Server
type serverOpsAdapter struct {
	server   *Server
	recorder *ProtocolRecorder
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
	// Call through the MCP runtime
	return a.server.mcpRuntime.CallTool(ctx, toolName, arguments)
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

	// Inject pending results if any
	s.injectPendingVirtualResultsAnthropicV1(req)

	ctx := c.Request.Context()

	// Create adapter
	adapter := mcp.NewAnthropicV1Adapter()

	// Create forwarder
	forwarder := mcp.NewAnthropicV1Forwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)

	// Create tool executor
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
		nil,
		mcp.InterceptorConfig{MaxRounds: 6},
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
	_, _, _, _, scenario, _, _ := GetTrackingContext(c)
	if anthropicMsg != nil && s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1NonStreamResponse(c, req, actualModel, provider, anthropicMsg)
	}

	// Record response if not already recorded
	if recorder != nil && recorderAdapter == nil {
		recorder.SetAssembledResponse(response)
		recorder.RecordResponse(provider, actualModel)
	}

	// Return response
	c.JSON(http.StatusOK, response)
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

	// Inject pending results if any
	s.injectPendingVirtualResultsAnthropicV1(req)

	// Create adapter
	adapter := mcp.NewAnthropicV1Adapter()

	// Create forwarder
	forwarder := mcp.NewAnthropicV1Forwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)

	// Create recorder adapter
	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	// Create HandleContext for streaming
	hc := protocol.NewHandleContext(c, responseModel)

	// Add TTFT tracking
	firstTokenRecorded := false
	hc.WithOnStreamEvent(func(_ interface{}) error {
		if !firstTokenRecorded {
			SetFirstTokenTime(c)
			firstTokenRecorded = true
		}
		return nil
	})

	// Add recorder hooks if available
	if recorder != nil {
		onEvent, onComplete, onError := NewRecorderHooksWithModel(recorder, actualModel, provider)
		if onEvent != nil {
			hc.WithOnStreamEvent(onEvent)
		}
		if onComplete != nil {
			hc.WithOnStreamComplete(onComplete)
		}
		if onError != nil {
			hc.WithOnStreamError(onError)
		}
	}

	// Response guardrails
	_, _, _, _, scenario, _, _ := GetTrackingContext(c)
	if s.guardrailsEnabledForScenario(scenario) {
		hc.EnsureGuardrails().Enabled = true
		s.attachGuardrailsHooks(c, hc, actualModel, provider, guardrailsadapter.AdaptMessagesFromAnthropicV1(req.System, req.Messages))
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
		mcp.InterceptorConfig{MaxRounds: 6},
	)

	if err := interceptor.Run(req); err != nil {
		recordMCPError(s, c, err, recorder)
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
	actualModel := reqCtx.RequestModel

	// Inject pending results if any
	s.injectPendingVirtualResultsOpenAI(req)

	ctx := c.Request.Context()

	// Create adapter
	adapter := mcp.NewOpenAIChatAdapter()

	// Create forwarder
	forwarder := mcp.NewOpenAIChatForwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)

	// Create tool executor
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
		nil,
		mcp.InterceptorConfig{MaxRounds: 6},
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

	// Update affinity
	if resp, ok := response.(*openai.ChatCompletion); ok {
		s.updateAffinityMessageID(c, rule, string(resp.ID))
	}

	// Record response if not already recorded
	if recorder != nil && recorderAdapter == nil {
		recorder.SetAssembledResponse(response)
		recorder.RecordResponse(provider, actualModel)
	}

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

	// Inject pending results if any
	s.injectPendingVirtualResultsOpenAI(req)

	// Create adapter
	adapter := mcp.NewOpenAIChatAdapter()

	// Create forwarder
	forwarder := mcp.NewOpenAIChatForwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)

	// Create recorder adapter
	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	// Create HandleContext for streaming
	hc := protocol.NewHandleContext(c, responseModel)

	// Add TTFT tracking
	firstTokenRecorded := false
	hc.WithOnStreamEvent(func(_ interface{}) error {
		if !firstTokenRecorded {
			SetFirstTokenTime(c)
			firstTokenRecorded = true
		}
		return nil
	})

	// Add recorder hooks if available
	if recorder != nil {
		onEvent, onComplete, onError := NewRecorderHooksWithModel(recorder, actualModel, provider)
		if onEvent != nil {
			hc.WithOnStreamEvent(onEvent)
		}
		if onComplete != nil {
			hc.WithOnStreamComplete(onComplete)
		}
		if onError != nil {
			hc.WithOnStreamError(onError)
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
		mcp.InterceptorConfig{MaxRounds: 6},
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

	// Inject pending results if any
	s.injectPendingVirtualResultsAnthropicBeta(req)

	ctx := c.Request.Context()

	// Create adapter
	adapter := mcp.NewAnthropicBetaAdapter()

	// Create forwarder
	forwarder := mcp.NewAnthropicBetaForwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)

	// Create tool executor
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
		nil,
		mcp.InterceptorConfig{MaxRounds: 6},
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
	var betaMsg *anthropic.BetaMessage
	if msg, ok := response.(*anthropic.BetaMessage); ok {
		s.updateAffinityMessageID(c, rule, string(msg.ID))
		betaMsg = msg
	}

	// Response guardrails
	_, _, _, _, scenario, _, _ := GetTrackingContext(c)
	if betaMsg != nil && s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1BetaNonStreamResponse(c, req, actualModel, provider, betaMsg)
	}

	// Record response if not already recorded
	if recorder != nil && recorderAdapter == nil {
		recorder.SetAssembledResponse(response)
		recorder.RecordResponse(provider, actualModel)
	}

	// Return response
	c.JSON(http.StatusOK, response)
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

	// Inject pending results if any
	s.injectPendingVirtualResultsAnthropicBeta(req)

	// Create adapter
	adapter := mcp.NewAnthropicBetaAdapter()

	// Create forwarder
	forwarder := mcp.NewAnthropicBetaForwarder(s.clientPool, &forwardContextProvider{server: s})

	// Get virtual registry
	virtualRegistry := s.mcpRuntime.VirtualRegistry()

	// Create server ops adapter
	serverOps := newServerOpsAdapter(s, recorder)

	// Create recorder adapter
	var recorderAdapter mcp.ProtocolRecorder
	if recorder != nil {
		recorderAdapter = &protocolRecorderAdapter{recorder: recorder}
	}

	// Create HandleContext for streaming
	hc := protocol.NewHandleContext(c, responseModel)

	// Add TTFT tracking
	firstTokenRecorded := false
	hc.WithOnStreamEvent(func(_ interface{}) error {
		if !firstTokenRecorded {
			SetFirstTokenTime(c)
			firstTokenRecorded = true
		}
		return nil
	})

	// Add recorder hooks if available
	if recorder != nil {
		onEvent, onComplete, onError := NewRecorderHooksWithModel(recorder, actualModel, provider)
		if onEvent != nil {
			hc.WithOnStreamEvent(onEvent)
		}
		if onComplete != nil {
			hc.WithOnStreamComplete(onComplete)
		}
		if onError != nil {
			hc.WithOnStreamError(onError)
		}
	}

	// Response guardrails
	_, _, _, _, scenario, _, _ := GetTrackingContext(c)
	if s.guardrailsEnabledForScenario(scenario) {
		hc.EnsureGuardrails().Enabled = true
		s.attachGuardrailsHooks(c, hc, actualModel, provider, guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages))
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
		mcp.InterceptorConfig{MaxRounds: 6},
	)

	if err := interceptor.Run(req); err != nil {
		recordMCPError(s, c, err, recorder)
	}
}
