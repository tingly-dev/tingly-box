package server

import (
	"context"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	serverguardrails "github.com/tingly-dev/tingly-box/internal/server/guardrails"
	"github.com/tingly-dev/tingly-box/internal/toolinterceptor"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicMessagesV1Beta implements beta messages API
func (s *Server) AnthropicMessagesV1Beta(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, provider *typ.Provider, actualModel string, rule *typ.Rule) {

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
	if err := executeAnthropicBetaPreChain(
		&req.BetaMessageNewParams, scenarioConfig,
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
		preparedReq, _ := s.toolInterceptor.PrepareAnthropicBetaRequest(provider, &req.BetaMessageNewParams)
		req.BetaMessageNewParams = *preparedReq
	} else if shouldStripTools {
		req.BetaMessageNewParams.Tools = toolinterceptor.StripSearchFetchToolsAnthropicBeta(req.BetaMessageNewParams.Tools)
	}

	s.applyGuardrailsToToolResultV1Beta(c, &req.BetaMessageNewParams, actualModel, provider)
	// Apply alias masking after terminal tool_result filtering so the decision
	// engine sees original tool output and the upstream model sees aliases.
	s.applyGuardrailsCredentialMasksV1Beta(c, &req.BetaMessageNewParams, actualModel, provider)

	// Check provider's API style to decide which path to take
	apiStyle := provider.APIStyle

	target := protocol.TypeAnthropicBeta
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		// Check if model prefers Responses API (for models like Codex)
		// This is used for ChatGPT backend API which only supports Responses API
		preferredEndpoint := s.GetPreferredEndpointForModel(provider, actualModel)
		logrus.Debugf("[AnthropicV1] Probe cache preferred endpoint for model=%s: %s", actualModel, preferredEndpoint)
		useResponsesAPI := preferredEndpoint == "responses"
		if useResponsesAPI {
			target = protocol.TypeOpenAIResponses
		} else {
			target = protocol.TypeOpenAIChat
		}
	}

	reqCtx, err := s.transformAnthropicBeta(c, req, target, provider, isStreaming, recorder, scenarioType)
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

// handleAnthropicStreamResponseV1Beta processes the Anthropic beta streaming response and sends it to the client
func (s *Server) handleAnthropicStreamResponseV1Beta(c *gin.Context, req *anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], respModel string, provider *typ.Provider, recorder *ProtocolRecorder) {
	hc := protocol.NewHandleContext(c, respModel)
	actualModel := string(req.Model)

	// Record TTFT when the first streaming chunk arrives
	firstTokenRecorded := false
	hc.WithOnStreamEvent(func(_ interface{}) error {
		if !firstTokenRecorded {
			SetFirstTokenTime(c)
			firstTokenRecorded = true
		}
		return nil
	})

	// Add recorder hooks if recorder is available
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

	// Anthropic beta only adapts request history; the shared runtime owns all
	// enablement checks and hook wiring after this point.
	session := s.guardrailsSessionFromContext(c, actualModel, provider)
	s.attachGuardrailsHooks(c, hc, session, serverguardrails.MessagesFromAnthropicV1Beta(req.System, req.Messages))

	usageStat, err := stream.HandleAnthropicV1BetaStream(hc, streamResp)
	s.trackUsageWithTokenUsage(c, usageStat, err)
}

// handleAnthropicV1BetaViaResponsesAPINonStreaming handles non-streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPINonStreaming(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder if exists
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}

	// Get rule from context for affinity
	var rule *typ.Rule
	if r, exists := c.Get(ContextKeyRule); exists {
		rule = r.(*typ.Rule)
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

	anthropicResp := nonstream.ConvertResponsesToAnthropicBetaResponse(response, proxyModel)

	// Update affinity entry with message ID
	s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	c.JSON(http.StatusOK, anthropicResp)

}

// handleAnthropicV1BetaViaResponsesAPIStreaming handles streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPIStreaming(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
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
	usage, err := stream.HandleResponsesToAnthropicBetaStream(c, streamResp, proxyModel)

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

// handleAnthropicV1BetaViaResponsesAPIStreaming handles streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPIAssembly(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
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
	usage, err := stream.HandleResponsesToAnthropicBetaAssembly(c, streamResp, proxyModel)

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
