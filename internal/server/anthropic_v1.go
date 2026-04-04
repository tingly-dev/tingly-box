package server

import (
	"context"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/toolinterceptor"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicMessagesV1 implements standard v1 messages API
func (s *Server) AnthropicMessagesV1(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, provider *typ.Provider, actualModel string, rule *typ.Rule) {
	// Get or create recorder for dual-stage recording (when V2 flag is enabled)
	var recorder *ProtocolRecorder
	scenarioType := rule.GetScenario()
	recorder = s.GetOrCreateScenarioRecorderV2(c, string(scenarioType), provider, actualModel, s.recordMode)

	// Check if streaming is requested
	isStreaming := req.Stream

	req.Model = anthropic.Model(actualModel)

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, proxyModel, isStreaming)

	// === Check if provider has built-in web_search ===
	hasBuiltInWebSearch := s.templateManager.ProviderHasBuiltInWebSearch(provider)

	// === Tool Interceptor: Check if enabled and should be used ===
	shouldIntercept, shouldStripTools, _ := s.resolveToolInterceptor(provider, hasBuiltInWebSearch)

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

	// === PRE-REQUEST INTERCEPTION: Strip tools before sending to provider ===
	if shouldIntercept {
		preparedReq, _ := s.toolInterceptor.PrepareAnthropicRequest(provider, &req.MessageNewParams)
		req.MessageNewParams = *preparedReq
	} else if shouldStripTools {
		req.MessageNewParams.Tools = toolinterceptor.StripSearchFetchToolsAnthropic(req.MessageNewParams.Tools)
	}

	session := s.guardrailsSessionFromContext(c, actualModel, provider)
	if s.guardrailsEnabledForSession(session) {
		s.applyGuardrailsToToolResultV1(c, &req.MessageNewParams, session)
		// Run credential masking after terminal tool_result filtering so block/review
		// decisions still inspect the original tool output while the upstream model
		// only receives alias tokens.
		s.applyGuardrailsCredentialMasksV1WithSession(c, &req.MessageNewParams, session)
	}

	// Check provider's API style to decide which path to take
	apiStyle := provider.APIStyle

	target := protocol.TypeAnthropicV1
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicV1
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		preferredEndpoint := s.GetPreferredEndpointForModel(provider, actualModel)
		logrus.Debugf("[AnthropicV1] Probe cache preferred endpoint for model=%s: %s", actualModel, preferredEndpoint)
		useResponsesAPI := preferredEndpoint == "responses"
		if useResponsesAPI {
			target = protocol.TypeOpenAIResponses
		} else {
			target = protocol.TypeOpenAIChat
		}
	}

	reqCtx, err := s.transformAnthropicV1(c, req, target, provider, isStreaming, recorder, scenarioType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	reqCtx.RequestModel = actualModel
	reqCtx.ResponseModel = proxyModel

	s.dispatchChainResult(c, reqCtx, rule, provider, isStreaming, recorder)
}

// handleAnthropicV1ViaResponsesAPINonStreaming handles non-streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and converts back to v1
func (s *Server) handleAnthropicV1ViaResponsesAPINonStreaming(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder if exists
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}

	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	// Use standard OpenAI Responses API
	wrapper := s.clientPool.GetOpenAIClient(provider, responsesReq.Model)
	fc := NewForwardContext(nil, provider)

	response, cancel, err = ForwardOpenAIResponses(fc, wrapper, responsesReq)
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

	// Extract usage from response
	inputTokens := int(response.Usage.InputTokens)
	outputTokens := int(response.Usage.OutputTokens)
	cacheTokens := int(response.Usage.InputTokensDetails.CachedTokens)
	usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)

	// Track usage
	s.trackUsageWithTokenUsage(c, usage, nil)

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

// handleAnthropicV1ViaResponsesAPIStreaming handles streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and streams back in v1 format
func (s *Server) handleAnthropicV1ViaResponsesAPIStreaming(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ProtocolRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ProtocolRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}

	// For standard OpenAI providers, use the OpenAI SDK
	wrapper := s.clientPool.GetOpenAIClient(provider, responsesReq.Model)
	fc := NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
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

	// Handle the streaming response
	// Use the dedicated stream handler to convert Responses API to Anthropic v1 format
	usage, err := stream.HandleResponsesToAnthropicV1Stream(c, streamResp, proxyModel)

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
		streamRec.Finish(proxyModel, usage.InputTokens, usage.OutputTokens)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// handleAnthropicV1ViaResponsesAPIStreaming handles streaming Responses API request
func (s *Server) handleAnthropicV1ViaResponsesAPIAssembly(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ProtocolRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ProtocolRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}

	// For standard OpenAI providers, use the OpenAI SDK
	wrapper := s.clientPool.GetOpenAIClient(provider, responsesReq.Model)
	fc := NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
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

	// Handle the streaming response
	// Use the dedicated stream handler to convert Responses API to Anthropic beta format
	usage, err := stream.HandleResponsesToAnthropicV1Assembly(c, streamResp, proxyModel)

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
		streamRec.Finish(proxyModel, usage.InputTokens, usage.OutputTokens)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// updateAffinityMessageID updates the affinity entry with the latest message ID
func (s *Server) updateAffinityMessageID(c *gin.Context, rule *typ.Rule, messageID string) {
	if !rule.SmartAffinity || messageID == "" {
		return
	}

	sessionID, exists := c.Get(ContextKeySessionID)
	if !exists {
		return
	}

	s.affinityStore.UpdateMessageID(rule.UUID, sessionID.(string), messageID)
	logrus.Debugf("[affinity] updated message ID %s for session %s, rule %s", messageID, sessionID.(string), rule.UUID)
}
