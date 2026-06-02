package server

import (
	"context"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicMessagesV1 implements standard v1 messages API
func (s *Server) AnthropicMessagesV1(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, provider *typ.Provider, actualModel string, rule *typ.Rule) {
	// Resolve fusion endpoint: when the provider has an Anthropic-compatible
	// fusion URL configured, route there natively to avoid a transform.
	provider = s.resolveProviderForClient(provider, protocol.APIStyleAnthropic)

	scenarioType := rule.GetScenario()

	// Check if streaming is requested
	isStreaming := req.Stream

	req.Model = anthropic.Model(actualModel)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, &req.MessageNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, proxyModel, isStreaming)

	// Get scenario config for flags
	scenarioConfig := s.config.GetScenarioConfig(scenarioType)

	// Build and run server-side pre-transform chain (scenario-driven flags)
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, actualModel)
	if err := executeAnthropicV1PreChain(
		&req.MessageNewParams, scenarioConfig,
		s.config.GetDefaultMaxTokens(), maxAllowed, isStreaming,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", actualModel)
	_, _, _, _, scenario, _, _ := GetTrackingContext(c)
	if s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1Request(c, &req.MessageNewParams, actualModel, provider)
	}

	// Get or create the recorder for dual-stage recording
	var recorder *ProtocolRecorder
	if s.ApplyRecording(scenarioType) {
		recorder = s.EnsureProtocolRecorder(c, string(scenarioType), provider, actualModel, s.GetScenarioRecordMode(scenarioType))
	}

	// Determine target API type for protocol transformation detection
	apiStyle := provider.APIStyle
	target := protocol.TypeAnthropicV1
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicV1
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, resolveRuleFlags(c, rule), IncomingAPIResponses)
		if routeErr != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: routeErr.Error(),
					Type:    "invalid_request_error",
					Code:    "unsupported_endpoint",
				},
			})
			return
		}
		target = resolvedTarget
	}

	// Resolve flags with scenario injection and auto-apply for CleanHeader
	ruleFlags := resolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeAnthropicV1, target)

	reqCtx, err := s.transformAnthropicV1(c, req, target, provider, isStreaming, recorder, scenarioType, rulePreBaseTransforms(ruleFlags), ruleExtraTransforms(ruleFlags)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	reqCtx.RequestModel = actualModel
	reqCtx.ResponseModel = proxyModel

	s.dispatchWithPriorityFailover(c, rule, provider, actualModel,
		func(p *typ.Provider, _ string) {
			retryProvider := s.resolveProviderForClient(p, protocol.APIStyleAnthropic)
			s.dispatchChainResult(c, reqCtx, rule, retryProvider, isStreaming, recorder)
		})
}

// nonstreamResponsesToAnthropic handles non-streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and converts back to v1
func (s *Server) nonstreamResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get protocol recorder if exists
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}

	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	// Use standard OpenAI Responses API (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(nil, provider)

	response, cancel, err = forwarding.ForwardOpenAIResponses(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}

	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		stream.SendForwardingError(c, err)
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usagepkg.FromOpenAIResponses(response.Usage), nil)

	// Convert Responses API response back to Anthropic v1 format
	anthropicResp := nonstream.ConvertResponsesToAnthropicV1Response(response, proxyModel)

	// TODO: require anthropic <-> anthropic beta
	//if ShouldRoundtripResponse(c, "openai") {
	//	roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(&anthropicResp, proxyModel, provider, actualModel)
	//	if err != nil {
	//		stream.SendInternalError(c, "Failed to roundtrip response: "+err.Error())
	//		return
	//	}
	//	anthropicResp = *roundtripped
	//}

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	c.JSON(http.StatusOK, anthropicResp)
}

// streamResponsesToAnthropic handles streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and streams back in v1 format
func (s *Server) streamResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	// For standard OpenAI providers, use the OpenAI SDK (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		stream.SendStreamingError(c, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	// Prime the stream: SDK streams are lazy, real upstream errors only
	// surface on first Next(). Forcing it here lets failover retry
	// before any byte hits the wire.
	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		s.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	usage, err := stream.HandleResponsesToAnthropicV1Stream(c, primedStream, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// streamResponsesToAnthropic handles streaming Responses API request
func (s *Server) assembleResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	// For standard OpenAI providers, use the OpenAI SDK (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.handlePreStreamFailure(c, err, streamRec)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		s.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	usage, err := stream.HandleResponsesToAnthropicV1Assembly(c, primedStream, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}
