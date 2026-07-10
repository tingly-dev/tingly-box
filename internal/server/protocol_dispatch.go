package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// shouldUseGenericMCPForProvider checks if the provider is allowed to use generic MCP path
func (ph *ProtocolHandler) shouldUseGenericMCPForProvider(provider *typ.Provider) bool {
	return ShouldUseGenericMCPForProvider(ph.deps.Config, provider)
}

// ShouldUseGenericMCPForProvider is the pure-Config form of
// Handler.shouldUseGenericMCPForProvider, exported so callers that only have
// a *config.Config (e.g. tests constructing a bare *Server without a wired
// aiHandler) can check the same provider-limits logic directly.
func ShouldUseGenericMCPForProvider(cfg *config.Config, provider *typ.Provider) bool {
	limits := cfg.GenericMCP.ProviderLimits
	if limits == "" || limits == "*" {
		// No limits configured, all providers can use generic path
		return true
	}

	// Check if provider is in the limits list
	// Format: comma-separated provider names (e.g., "provider1,provider2")
	if limits == provider.Name {
		return true
	}

	// Parse comma-separated limits and check if provider is in the list
	// This is a simple implementation - can be improved with proper parsing
	parts := strings.Split(limits, ",")
	for _, part := range parts {
		if strings.TrimSpace(part) == provider.Name {
			return true
		}
	}

	return false
}

// dispatchChainResult
// do request from source to target, and return upstream response from target to source
func (ph *ProtocolHandler) DispatchChainResult(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *recording.ProtocolRecorder,
) {
	defer func() {
		reqCtx.Release()
	}()

	// Bubble up the execution-level routing decision for probes. This is the
	// single chokepoint where the resolved upstream API + provider + matched
	// rule + applied flags are all known, before any response byte is written.
	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeaders(c, reqCtx, rule, provider)
	}

	switch reqCtx.TargetAPI {
	case protocol.TypeOpenAIChat:
		ph.dispatchOpenAIChat(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeAnthropicV1:
		if isStreaming {
			ph.StreamAnthropicV1(c, reqCtx, rule, provider, recorder)
		} else {
			ph.NonstreamAnthropicV1(c, reqCtx, rule, provider, recorder)
		}
	case protocol.TypeAnthropicBeta:
		ph.dispatchAnthropicBeta(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeOpenAIResponses:
		ph.dispatchOpenAIResponses(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeGoogle:
		ph.dispatchGoogle(c, reqCtx, rule, provider, isStreaming, recorder)
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
		if recorder != nil {
			recorder.RecordError(fmt.Errorf("invalid api style: %s", provider.APIStyle))
		}
	}
}

// dispatchAnthropicBeta routes an Anthropic-beta-bound request by the client's
// source format: Chat and Responses sources need their responses converted
// back; everything else is beta passthrough.
func (ph *ProtocolHandler) dispatchAnthropicBeta(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *recording.ProtocolRecorder,
) {
	switch reqCtx.SourceAPI {
	case protocol.TypeOpenAIChat:
		ph.dispatchAnthropicBetaToOpenAIChat(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeOpenAIResponses:
		if isStreaming {
			ph.streamAnthropicBetaToResponses(c, reqCtx, provider, recorder)
		} else {
			ph.nonstreamAnthropicBetaToResponses(c, reqCtx, provider, recorder)
		}
	default:
		ph.passthroughAnthropicBeta(c, reqCtx, rule, provider, isStreaming, recorder)
	}
}

// dispatchOpenAIResponses routes a Responses-API-bound request by the client's
// source format. Anthropic sources on Codex providers use the assembly path
// (Codex only streams), other providers use the plain non-streaming forward.
func (ph *ProtocolHandler) dispatchOpenAIResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *recording.ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	switch reqCtx.SourceAPI {
	case protocol.TypeAnthropicV1:
		logrus.Debugf("[AnthropicV1] Using Transform Chain for Responses API for model=%s", actualModel)
		if isStreaming {
			ph.streamResponsesToAnthropic(c, responseModel, actualModel, provider, *req)
		} else if provider.APIBase == protocol.CodexAPIBase {
			ph.assembleResponsesToAnthropic(c, responseModel, actualModel, provider, *req)
		} else {
			ph.nonstreamResponsesToAnthropic(c, responseModel, actualModel, provider, *req)
		}
	case protocol.TypeAnthropicBeta:
		logrus.Debugf("[Anthropic Beta] Using Transform Chain for Responses API for model=%s", actualModel)
		if isStreaming {
			ph.streamResponsesToAnthropicBeta(c, responseModel, actualModel, provider, *req)
		} else if provider.APIBase == protocol.CodexAPIBase {
			ph.assembleResponsesToAnthropicBeta(c, responseModel, actualModel, provider, *req)
		} else {
			ph.nonstreamResponsesToAnthropicBeta(c, responseModel, actualModel, provider, *req)
		}
	case protocol.TypeOpenAIChat:
		// Client sent Responses API, but provider needs Chat format
		// Forward as Chat, then convert response back to Responses format
		if isStreaming {
			ph.streamResponsesToChat(c, reqCtx, provider, recorder)
		} else {
			ph.nonstreamResponsesToChat(c, reqCtx, provider, recorder)
		}
	case protocol.TypeOpenAIResponses:
		// Responses API passthrough
		if isStreaming {
			ph.streamOpenAIResponses(c, reqCtx, provider, recorder)
		} else {
			ph.nonstreamOpenAIResponses(c, reqCtx, provider, recorder)
		}
	}
}

// setProbeUpstreamHeaders writes the execution-level routing decision as
// X-Tingly-* response headers, consumed by the probe's captureRoutingRoundTripper.
// Gated by the caller on X-Tingly-Debug-Routing so production traffic is untouched.
func setProbeUpstreamHeaders(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider) {
	c.Header("X-Tingly-Upstream-API", string(reqCtx.TargetAPI))
	if provider != nil {
		c.Header("X-Tingly-Upstream-URL", upstreamURLFor(provider, reqCtx.TargetAPI))
	}
	// Synthetic rules (provider probes) carry no meaningful rule identity.
	if rule != nil && rule.UUID != ProbeSyntheticRuleUUID {
		c.Header("X-Tingly-Matched-Rule", rule.UUID)
		if rule.Description != "" {
			// Descriptions may be non-ASCII; percent-encode for header safety.
			c.Header("X-Tingly-Matched-Rule-Desc", url.QueryEscape(rule.Description))
		}
	}
	if rule != nil {
		if flags := formatAppliedFlags(rule.Flags); flags != "" {
			c.Header("X-Tingly-Applied-Flags", flags)
		}
	}
}

// upstreamURLFor reconstructs the real upstream endpoint TB forwards to, mirroring
// the path each SDK appends to provider.APIBase.
func upstreamURLFor(provider *typ.Provider, target protocol.APIType) string {
	base := strings.TrimSuffix(provider.APIBase, "/")
	switch target {
	case protocol.TypeOpenAIChat:
		return base + "/chat/completions"
	case protocol.TypeOpenAIResponses:
		return base + "/responses"
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return base + "/v1/messages"
	default:
		return base
	}
}

// formatAppliedFlags renders the non-default rule flags as a compact,
// human-readable string (e.g. "endpoint=responses, thinking=high").
func formatAppliedFlags(f typ.RuleFlags) string {
	var parts []string
	if f.OpenAIEndpointOverride != "" && f.OpenAIEndpointOverride != "auto" {
		parts = append(parts, "endpoint="+f.OpenAIEndpointOverride)
	}
	if f.ThinkingEffort != "" {
		parts = append(parts, "thinking="+string(f.ThinkingEffort))
	}
	if f.UseMaxCompletionTokens {
		parts = append(parts, "max_completion_tokens")
	}
	if f.UseMaxTokens {
		parts = append(parts, "max_tokens")
	}
	if f.BlockTools != "" {
		parts = append(parts, "block_tools="+f.BlockTools)
	}
	if f.SkipUsage {
		parts = append(parts, "skip_usage")
	}
	if f.CursorCompat {
		parts = append(parts, "cursor_compat")
	}
	if f.CleanHeader {
		parts = append(parts, "clean_header")
	}
	if f.ClaudeCodeCompat {
		parts = append(parts, "claude_code_compat")
	}
	if f.CustomUserAgent != "" {
		parts = append(parts, "custom_ua")
	}
	if f.SessionAffinity > 0 {
		parts = append(parts, fmt.Sprintf("session_affinity=%ds", f.SessionAffinity))
	}
	if f.VisionProxyService != nil {
		parts = append(parts, "vision_proxy")
	}
	return strings.Join(parts, ", ")
}

// dispatchAnthropicBetaToOpenAIChat forwards an OpenAI-Chat-shaped client
// request to an Anthropic beta provider. The client expects OpenAI format, so
// responses are converted back.
func (ph *ProtocolHandler) dispatchAnthropicBetaToOpenAIChat(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *recording.ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()

	wrapper := ph.deps.ClientPool.GetAnthropicClient(ctx, provider, actualModel)
	fc := forwarding.NewForwardContext(ctx, provider)

	if isStreaming {
		disableStreamUsage := ShouldStripUsage(reqCtx.Extra)
		if reqCtx.ScenarioFlags != nil {
			disableStreamUsage = disableStreamUsage || reqCtx.ScenarioFlags.SkipUsage
		}

		if HasDeclaredMCPAnthropicBetaTools(req) && ph.mcpEnabled() {
			ph.StreamAnthropicBetaToOpenAIChatWithMCP(c, provider, req, actualModel, responseModel, disableStreamUsage, recorder)
			return
		}

		streamResp, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			ph.failRequest(c, recorder, err, "Failed to create streaming request")
			return
		}

		hc := protocol.NewHandleContext(c, responseModel)
		tokenUsage, err := stream.AnthropicToOpenAIStream(hc, req, streamResp, responseModel, disableStreamUsage)
		if err != nil {
			if tokenUsage != nil {
				if tokenUsage.InputTokens > 0 || tokenUsage.OutputTokens > 0 {
					ph.trackUsageWithTokenUsage(c, tokenUsage, err)
				} else {
					// Track error even when no tokens were received (e.g., early 1302 rate limit)
					ph.trackUsageFromContext(c, 0, 0, err)
				}
			}
			SendErrorResponse(c, err, "Failed to create streaming request")
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		if tokenUsage.InputTokens > 0 || tokenUsage.OutputTokens > 0 {
			ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
		}
	} else {
		var anthropicResp *anthropic.BetaMessage
		var usage *protocol.TokenUsage
		var err error

		if HasDeclaredMCPAnthropicBetaTools(req) && ph.mcpEnabled() {
			var genericUsage *mcp.TokenUsage
			anthropicResp, genericUsage, err = ph.RunGenericAnthropicBetaNonStream(ctx, provider, req, recorder)
			if err != nil {
				ph.respondMCPError(c, recorder, err, "Failed to handle MCP tool calls")
				return
			}
			if genericUsage != nil {
				usage = protocol.NewTokenUsageWithCache(genericUsage.InputTokens, genericUsage.OutputTokens, genericUsage.CacheTokens)
			}
		} else {
			var cancel context.CancelFunc
			anthropicResp, cancel, err = forwarding.ForwardAnthropicV1Beta(fc, wrapper, req)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				ph.failRequest(c, recorder, err, "Failed to forward Anthropic request")
				return
			}
			usage = usagepkg.FromAnthropicBetaMessage(anthropicResp.Usage)
		}

		ph.trackUsageWithTokenUsage(c, usage, nil)

		openaiResp := ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp, responseModel, provider, actualModel)
		if ShouldRoundtripResponse(c, "anthropic") {
			roundtripped, err := RoundtripOpenAIMapViaAnthropic(openaiResp, responseModel, provider, actualModel)
			if err != nil {
				SendErrorResponse(c, err, "Failed to roundtrip response")
				return
			}
			openaiResp = roundtripped
		}
		if ShouldStripUsage(reqCtx.Extra) {
			delete(openaiResp, "usage")
		}

		if recorder != nil {
			recorder.SetAssembledResponse(anthropicResp)
			recorder.RecordResponse(provider, reqCtx.RequestModel)
		}
		c.JSON(http.StatusOK, openaiResp)
	}
}

func (ph *ProtocolHandler) passthroughAnthropicBeta(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *recording.ProtocolRecorder,
) {
	useGeneric := ph.mcpEnabled() && ph.shouldUseGenericMCPForProvider(provider)

	if useGeneric {
		if !isStreaming {
			ph.DispatchGenericAnthropicBetaNonStream(c, reqCtx, rule, provider, recorder)
			return
		}
		ph.DispatchGenericAnthropicBetaStream(c, reqCtx, rule, provider, recorder)
		return
	}

	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()

	if isStreaming {
		if ph.mcpEnabled() {
			declaredMCP := HasDeclaredMCPAnthropicBetaTools(req)
			if declaredMCP {
				ph.DispatchGenericAnthropicBetaStream(c, reqCtx, rule, provider, recorder)
				return
			}
		}

		wrapper := ph.deps.ClientPool.GetAnthropicClient(ctx, provider, actualModel)
		fc := forwarding.NewForwardContext(ctx, provider)
		streamResp, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			ph.handlePreStreamFailure(c, err, recorder)
			return
		}

		ph.StreamAnthropicBeta(c, req, streamResp, actualModel, responseModel, provider, recorder)
		return

	} else {
		var anthropicResp *anthropic.BetaMessage
		var err error
		declaredMCP := false
		if ph.mcpEnabled() {
			declaredMCP = HasDeclaredMCPAnthropicBetaTools(req)
		}
		if declaredMCP {
			var usage *mcp.TokenUsage
			anthropicResp, usage, err = ph.RunGenericAnthropicBetaNonStream(ctx, provider, req, recorder)
			if err != nil {
				ph.failForward(c, recorder, err)
				return
			}
			if usage != nil {
				tokenUsage := protocol.NewTokenUsageWithCache(usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
				ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
			}
		} else {
			wrapper := ph.deps.ClientPool.GetAnthropicClient(ctx, provider, actualModel)
			fc := forwarding.NewForwardContext(ctx, provider)
			var cancel context.CancelFunc
			anthropicResp, cancel, err = forwarding.ForwardAnthropicV1Beta(fc, wrapper, req)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				ph.failForward(c, recorder, err)
				return
			}

			ph.trackUsageWithTokenUsage(c, usagepkg.FromAnthropicBetaMessage(anthropicResp.Usage), nil)
		}

		ph.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
		anthropicResp.Model = anthropic.Model(responseModel)

		scenario := GetTrackingContextScenario(c)
		if ph.guardrailsEnabledForScenario(scenario) {
			ApplyGuardrailsToAnthropicV1BetaNonStreamResponse(c, ph.currentGuardrailsRuntime(), req, actualModel, provider, anthropicResp)
		}

		if recorder != nil {
			recorder.SetAssembledResponse(anthropicResp)
			recorder.RecordResponse(provider, reqCtx.RequestModel)
		}
		nonstream.WriteAnthropicMessage(c, anthropicResp)
	}
}

func (ph *ProtocolHandler) dispatchGoogle(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *recording.ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	googleReq := reqCtx.Request.(*protocol.GoogleRequest)
	model, req, cfg := actualModel, googleReq.Contents, googleReq.Config

	if isStreaming {
		wrapper := ph.deps.ClientPool.GetGoogleClient(c.Request.Context(), provider, model)
		fc := forwarding.NewForwardContext(c.Request.Context(), provider)
		streamResp, cancel, err := forwarding.ForwardGoogleStream(fc, wrapper, model, req, cfg)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			stream.SendStreamingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		var usage *protocol.TokenUsage
		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			usage, err = stream.HandleGoogleToAnthropicStreamResponse(c, streamResp, responseModel)
		case protocol.TypeAnthropicBeta:
			usage, err = stream.HandleGoogleToAnthropicBetaStreamResponse(c, streamResp, responseModel)
		}
		if err != nil {
			ph.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}
		ph.trackUsageWithTokenUsage(c, usage, nil)
	} else {
		wrapper := ph.deps.ClientPool.GetGoogleClient(c.Request.Context(), provider, model)
		fc := forwarding.NewForwardContext(c.Request.Context(), provider)
		resp, _, err := forwarding.ForwardGoogle(fc, wrapper, model, req, cfg)
		if err != nil {
			stream.SendForwardingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		inputTokens := 0
		outputTokens := 0
		cacheTokens := 0
		if resp.UsageMetadata != nil {
			inputTokens = int(resp.UsageMetadata.PromptTokenCount)
			outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			cacheTokens = int(resp.UsageMetadata.CachedContentTokenCount)
		}
		usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
		ph.trackUsageWithTokenUsage(c, usage, nil)

		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			anthropicResp := nonstream.HandleGoogleToAnthropic(resp, responseModel)
			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(anthropicResp, responseModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip resp: "+err.Error())
					return
				}
				anthropicResp = roundtripped
			}
			ph.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			nonstream.WriteAnthropicMessage(c, anthropicResp)
		case protocol.TypeAnthropicBeta:
			anthropicResp := nonstream.HandleGoogleToAnthropicBeta(resp, responseModel)
			ph.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			nonstream.WriteAnthropicMessage(c, anthropicResp)
		}
	}
}

func (ph *ProtocolHandler) dispatchOpenAIChat(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *recording.ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel

	req := reqCtx.Request.(*openai.ChatCompletionNewParams)
	if seg, ok := mcp.PopOpenAIContinuationSegment(typ.GetSessionID(c.Request.Context()), provider.UUID); ok {
		req.Messages = append(append([]openai.ChatCompletionMessageParamUnion{}, seg...), req.Messages...)
	}
	// AlignToolMessagesForOpenAI is already performed by ConsistencyTransform
	// in the transform chain (normalizeMessages -> alignToolMessages), which
	// runs before dispatchOpenAIChat for all TypeOpenAIChat targets.
	request.CleanupOpenaiFields(req)

	if isStreaming {
		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			ph.StreamOpenAIChatToAnthropicV1WithMCP(c, provider, req, actualModel, responseModel, recorder)
		case protocol.TypeAnthropicBeta:
			ph.StreamOpenAIChatToAnthropicBetaWithMCP(c, provider, req, actualModel, responseModel, recorder)
		case protocol.TypeOpenAIChat:
			// OpenAI passthrough: source and target are both OpenAI Chat format
			disableStreamUsage := ShouldStripUsage(reqCtx.Extra)
			if reqCtx.ScenarioFlags != nil {
				disableStreamUsage = disableStreamUsage || reqCtx.ScenarioFlags.SkipUsage
			}

			if HasDeclaredMCPTools(req) && ph.mcpEnabled() {
				ph.DispatchGenericOpenAIChatStream(c, reqCtx, rule, provider, recorder)
				return
			}

			ph.streamOpenAIChat(c, provider, req, responseModel, disableStreamUsage)
		case protocol.TypeOpenAIResponses:
			ph.streamOpenAIChatToResponses(c, reqCtx, provider, recorder)
		}
	} else {
		switch reqCtx.SourceAPI {
		case protocol.TypeOpenAIChat:
			// OpenAI passthrough: delegate to handleNonStreamingRequest for tool interceptor support
			stripUsage := ShouldStripUsage(reqCtx.Extra)

			if HasDeclaredMCPTools(req) && ph.mcpEnabled() {
				ph.DispatchGenericOpenAIChatNonStream(c, reqCtx, rule, provider, recorder)
				return
			}

			ph.nonstreamOpenAIChat(c, provider, req, responseModel, stripUsage)
			return
		case protocol.TypeOpenAIResponses:
			ph.nonstreamOpenAIChatToResponses(c, reqCtx, provider, recorder)
			return
		default:
			// Forward request to provider for format conversion
		}

		var resp *openai.ChatCompletion
		var err error
		var usage *protocol.TokenUsage
		if HasDeclaredMCPTools(req) && ph.mcpEnabled() {
			var genericUsage *mcp.TokenUsage
			resp, genericUsage, err = ph.RunGenericOpenAIChatNonStream(c.Request.Context(), provider, req, recorder)
			if err != nil {
				stream.SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			if genericUsage != nil {
				usage = protocol.NewTokenUsageWithCache(genericUsage.InputTokens, genericUsage.OutputTokens, genericUsage.CacheTokens)
			}
		} else {
			wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
			fc := forwarding.NewForwardContext(c.Request.Context(), provider)
			resp, _, err = forwarding.ForwardOpenAIChat(fc, wrapper, req)
			if err != nil {
				stream.SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			usage = usagepkg.FromOpenAIChatCompletion(resp.Usage)
		}

		ph.trackUsageWithTokenUsage(c, usage, err)

		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			anthropicResp := nonstream.HandleOpenAIChatToAnthropic(resp, responseModel)
			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(anthropicResp, responseModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip resp: "+err.Error())
					return
				}
				anthropicResp = roundtripped
			}
			ph.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			nonstream.WriteAnthropicMessage(c, anthropicResp)
		case protocol.TypeAnthropicBeta:
			anthropicResp := nonstream.HandleOpenAIChatToAnthropicBeta(resp, responseModel)
			ph.updateAffinityMessageID(c, rule, anthropicResp.ID)
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			nonstream.WriteAnthropicMessage(c, anthropicResp)
		}
	}
}

// Note: dispatchOpenAIChatToAnthropicBetaGeneric (OpenAI Chat -> Anthropic
// Beta cross-format TRUE-streaming dispatch) was dropped here — confirmed
// zero callers anywhere in the codebase at move time (Step 7), same
// dead-code disposition as smart_routing_helper.go in Step 4.
