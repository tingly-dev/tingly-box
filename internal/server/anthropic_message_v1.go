package server

import (
	"context"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicMessagesV1 implements standard v1 messages API.
//
// It runs the provider-independent prologue once, then drives the failover loop
// whose per-attempt callback re-runs the whole provider-dependent pipeline
// (pre-chain → guardrails → target resolution → transform → dispatch) against
// the candidate selected for that attempt. Because the transform is re-run per
// attempt, failover can rotate across heterogeneous API styles.
func (s *Server) AnthropicMessagesV1(c *gin.Context, req *protocol.AnthropicMessagesRequest, requestModel string, responseModel string, rule *typ.Rule, provider *typ.Provider) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	// (Mutates the shared rule only, so it must run before any attempt.)
	detectAndApplyContext1MFromIncomingRequest(c, rule)

	scenarioType := rule.GetScenario()
	isStreaming := req.Stream
	scenarioConfig := s.config.GetScenarioConfig(scenarioType)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, req.MessageNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata. Provider/model are refreshed per
	// attempt by the failover loop (UpdateTrackingForFailover).
	SetTrackingContext(c, rule, provider, requestModel, responseModel, isStreaming)

	// Get or create the recorder for dual-stage recording. The body is the
	// pristine request as received (post-vision-proxy, pre-pre-chain); the
	// winning attempt's provider/model is re-bound per attempt via SetActiveService.
	var recorder *ProtocolRecorder
	if s.ApplyRecording(scenarioType) {
		bs, err := req.MarshalJSON()
		if err != nil {
			bs = []byte("{}")
		}
		recorder = s.EnsureProtocolRecorder(c, string(scenarioType), provider, requestModel, s.GetScenarioRecordMode(scenarioType), bs)
	}

	// Snapshot a pristine template only when failover is possible; the single
	// service case reuses the original request with no clone overhead.
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
				cloned, err := cloneAnthropicV1Request(template)
				if err != nil {
					s.failAttemptSetup(c, err)
					return
				}
				areq = cloned
			}
			s.runAnthropicV1Attempt(c, areq, responseModel, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig, recorder)
		})
}

// runAnthropicV1Attempt executes the provider-dependent half of an Anthropic v1
// request for one failover attempt: resolve the dual endpoint, run the
// pre-transform chain and guardrails, resolve the target API for this provider's
// style, transform, and dispatch. Setup failures route through failAttemptSetup
// so the orchestrator can advance to the next candidate.
func (s *Server) runAnthropicV1Attempt(c *gin.Context, req *protocol.AnthropicMessagesRequest, responseModel string, provider *typ.Provider, requestModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig, recorder *ProtocolRecorder) {
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
	if err := executeAnthropicV1PreChain(
		req.MessageNewParams, scenarioConfig,
		s.config.GetDefaultMaxTokens(), maxAllowed, isStreaming,
	); err != nil {
		s.failAttemptSetup(c, err)
		return
	}

	scenario := GetTrackingContextScenario(c)
	if s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1Request(c, req.MessageNewParams, requestModel, provider)
	}

	// Determine target API type for protocol transformation detection
	target := protocol.TypeAnthropicV1
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicV1
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
	ruleFlags := resolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeAnthropicV1, target, provider)

	reqCtx, err := s.transformAnthropicV1(c, req, target, provider, isStreaming, recorder, scenarioType, rulePreBaseTransforms(ruleFlags), rulePreVendorTransforms(ruleFlags))
	if err != nil {
		s.failAttemptSetup(c, err)
		return
	}
	defer reqCtx.Release()

	reqCtx.RequestModel = requestModel
	reqCtx.ResponseModel = responseModel

	s.dispatchChainResult(c, reqCtx, rule, provider, isStreaming, recorder)
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
	nonstream.WriteAnthropicMessage(c, anthropicResp)
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

	hc := protocol.NewHandleContext(c, proxyModel)
	usage, err := stream.HandleResponsesToAnthropicV1Stream(hc, primedStream, proxyModel)

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
