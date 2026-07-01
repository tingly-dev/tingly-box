package server

import (
	"encoding/json"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// nonstreamAnthropicV1 handles A→A non-streaming with generic processor
func (s *Server) nonstreamAnthropicV1(
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

// streamAnthropicV1 handles A→A streaming with generic interceptor
func (s *Server) streamAnthropicV1(
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

// streamAnthropicBeta processes the Anthropic beta streaming
// response. The resolved model is passed in as actualModel rather than read from
// the request, so the handler no longer depends on req.Model.
func (s *Server) streamAnthropicBeta(c *gin.Context, req *anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], actualModel string, responseModel string, provider *typ.Provider, recorder *ProtocolRecorder) {
	hc := protocol.NewHandleContext(c, responseModel)

	// Add recorder hooks if recorder is available
	AttachRecorderHooks(hc, recorder, actualModel, provider)

	// response guardrails
	scenario := GetTrackingContextScenario(c)
	if s.guardrailsEnabledForScenario(scenario) {
		hc.EnsureGuardrails().Enabled = true
		s.attachGuardrailsHooks(c, hc, actualModel, provider, guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages))
	}

	usageStat, err := stream.HandleAnthropicBeta(hc, streamResp)
	s.trackUsageWithTokenUsage(c, usageStat, err)
}

// nonstreamOpenAIChat handles non-streaming chat completion requests with MCP runtime support.
func (s *Server) nonstreamOpenAIChat(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, stripUsage bool) {
	req := originalReq

	// Forward request to provider
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	response, _, err := forwarding.ForwardOpenAIChat(fc, wrapper, req)
	if err != nil {
		// Track error with no usage
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		s.trackUsageWithTokenUsage(c, usage, err)
		c.JSON(protocol.UpstreamStatus(err, http.StatusInternalServerError), ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Extract usage from response
	inputTokens := int(response.Usage.PromptTokens)
	outputTokens := int(response.Usage.CompletionTokens)
	cacheTokens := int(response.Usage.PromptTokensDetails.CachedTokens)

	// Track usage
	usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
	s.trackUsageWithTokenUsage(c, usage, nil)

	// Convert response to JSON map for modification
	responseJSON, err := json.Marshal(response)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to marshal response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to process response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Update response model if configured
	responseMap["model"] = responseModel
	if stripUsage {
		delete(responseMap, "usage")
	}

	if ShouldRoundtripResponse(c, "anthropic") {
		roundtripped, err := RoundtripOpenAIResponseViaAnthropic(response, responseModel, provider, req.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to roundtrip response: " + err.Error(),
					Type:    "api_error",
				},
			})
			return
		}
		responseMap = roundtripped
		responseMap["model"] = responseModel
		if stripUsage {
			delete(responseMap, "usage")
		}
	}

	// Return modified response
	c.JSON(http.StatusOK, responseMap)
}

// streamOpenAIChat handles streaming chat completion requests.
func (s *Server) streamOpenAIChat(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, disableStreamUsage bool) {
	req := originalReq

	// Estimate input tokens up front and hand the scalar to the stream handler,
	// so it depends on the estimate rather than the request for the usage fallback.
	estimatedInputTokens := token.EstimateInputTokensSimple(req)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		// Track error with no usage
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		s.trackUsageWithTokenUsage(c, usage, err)
		c.JSON(protocol.UpstreamStatus(err, http.StatusInternalServerError), ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to create streaming request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Create handle context and handle stream
	hc := protocol.NewHandleContext(c, responseModel)
	hc.DisableStreamUsage = disableStreamUsage
	hc.EstimatedInputTokens = estimatedInputTokens

	usage, err := stream.HandleOpenAIChatStream(hc, streamResp)

	// Track usage from stream handler
	s.trackUsageWithTokenUsage(c, usage, err)
}

// nonstreamOpenAIResponses handles Responses API passthrough (non-streaming)
func (s *Server) nonstreamOpenAIResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	params := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, string(params.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	response, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *params)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to forward request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleOpenAIResponsesPassthroughNonStream(hc, response)
	s.trackUsageWithTokenUsage(c, tokenUsage, nil)
	if recorder != nil {
		recorder.SetAssembledResponse(response)
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
}

// streamOpenAIResponses handles Responses API passthrough (streaming)
// Moved from openai_responses.go:421-456
func (s *Server) streamOpenAIResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	responseModel := reqCtx.ResponseModel
	params := reqCtx.Request.(*responses.ResponseNewParams)

	// Create streaming request with request context for proper cancellation
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, params.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	respStream, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, *params)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.handlePreStreamFailure(c, err, recorder)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(respStream)
	if primeErr != nil {
		s.handlePreStreamFailure(c, primeErr, recorder)
		return
	}

	// Handle the streaming response
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleOpenAIResponsesStream(hc, primedStream, responseModel)

	// Track usage from stream handler
	s.trackUsageWithTokenUsage(c, usage, err)
}
