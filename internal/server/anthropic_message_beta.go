package server

import (
	"context"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/constant"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicMessagesV1Beta implements beta messages API.
//
// Like AnthropicMessagesV1, the provider-independent prologue runs once and the
// per-attempt callback re-runs the provider-dependent pipeline so failover can
// rotate across heterogeneous API styles.
func (s *Server) AnthropicMessagesV1Beta(c *gin.Context, req *protocol.AnthropicBetaMessagesRequest, requestModel string, responseModel string, rule *typ.Rule, provider *typ.Provider) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	// (Mutates the shared rule only, so it must run before any attempt.)
	detectAndApplyContext1MFromIncomingRequest(c, rule)

	scenarioType := rule.GetScenario()
	isStreaming := req.Stream
	scenarioConfig := s.config.GetScenarioConfig(scenarioType)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, req.BetaMessageNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata. Provider/model are refreshed per
	// attempt by the failover loop (UpdateTrackingForFailover).
	SetTrackingContext(c, rule, provider, requestModel, responseModel, isStreaming)

	// Get or create the recorder for dual-stage recording (pristine request body).
	var recorder *ProtocolRecorder
	if s.ApplyRecording(scenarioType) {
		bs, err := req.MarshalJSON()
		if err != nil {
			bs = []byte("{}")
		}
		recorder = s.EnsureProtocolRecorder(c, string(scenarioType), provider, requestModel, s.GetScenarioRecordMode(scenarioType), bs)
	}

	// Snapshot a pristine template only when failover is possible.
	multi := len(rule.GetActiveServices()) > 1
	var template []byte
	if multi {
		bs, err := req.MarshalJSON()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		template = bs
	}

	// ── Per-attempt pipeline (provider-dependent) ──
	s.dispatchWithPriorityFailover(c, rule, provider, requestModel,
		func(p *typ.Provider, retryModel string) {
			areq := req
			if multi {
				cloned, err := cloneAnthropicBetaRequest(template)
				if err != nil {
					s.failAttemptSetup(c, err)
					return
				}
				areq = cloned
			}
			s.runAnthropicBetaAttempt(c, areq, responseModel, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig, recorder)
		})
}

// runAnthropicBetaAttempt executes the provider-dependent half of an Anthropic
// beta request for one failover attempt. See runAnthropicV1Attempt.
func (s *Server) runAnthropicBetaAttempt(c *gin.Context, req *protocol.AnthropicBetaMessagesRequest, responseModel string, provider *typ.Provider, requestModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig, recorder *ProtocolRecorder) {
	// Resolve dual endpoint: when the provider has an Anthropic-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = s.resolveProviderForClient(provider, protocol.APIStyleAnthropic)
	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	req.Model = anthropic.Model(requestModel)

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", requestModel)

	// Build and run server-side pre-transform chain (scenario-driven flags)
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, requestModel)
	if err := executeAnthropicBetaPreChain(
		req.BetaMessageNewParams, scenarioConfig,
		s.config.GetDefaultMaxTokens(), maxAllowed, isStreaming,
	); err != nil {
		s.failAttemptSetup(c, err)
		return
	}

	// request guardrails
	scenario := GetTrackingContextScenario(c)
	if s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1BetaRequest(c, req.BetaMessageNewParams, requestModel, provider)
	}

	// Determine target API type for protocol transformation detection
	target := protocol.TypeAnthropicBeta
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, resolveRuleFlags(c, rule), IncomingAPIResponses)
		if routeErr != nil {
			s.failAttemptSetup(c, routeErr)
			return
		}
		target = resolvedTarget
	}

	// Resolve flags with scenario injection and auto-apply for CleanHeader.
	// (This also applies the custom User-Agent to the request context.)
	ruleFlags := resolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeAnthropicBeta, target, provider)

	reqCtx, err := s.transformAnthropicBeta(c, req, target, provider, isStreaming, recorder, scenarioType, rulePreBaseTransforms(ruleFlags), rulePreVendorTransforms(ruleFlags))
	if err != nil {
		s.failAttemptSetup(c, err)
		return
	}
	defer reqCtx.Release()

	reqCtx.RequestModel = requestModel
	reqCtx.ResponseModel = responseModel

	s.dispatchChainResult(c, reqCtx, rule, provider, isStreaming, recorder)
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

// nonstreamResponsesToAnthropicBeta handles non-streaming Responses API request
func (s *Server) nonstreamResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get protocol recorder if exists
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}

	// Get rule from context for affinity
	var rule *typ.Rule
	if r, exists := c.Get(ContextKeyRule); exists {
		rule = r.(*typ.Rule)
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

	anthropicResp := nonstream.ConvertResponsesToAnthropicBetaResponse(response, proxyModel)

	// Update affinity entry with message ID
	s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	nonstream.WriteAnthropicMessage(c, anthropicResp)

}

// streamResponsesToAnthropicBeta handles streaming Responses API request
func (s *Server) streamResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
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

	hc := protocol.NewHandleContext(c, proxyModel)
	usage, err := stream.HandleResponsesToAnthropicBetaStream(hc, primedStream, proxyModel)

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

// streamResponsesToAnthropicBeta handles streaming Responses API request
func (s *Server) assembleResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
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

	usage, err := stream.HandleResponsesToAnthropicBetaAssembly(c, primedStream, proxyModel)

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
