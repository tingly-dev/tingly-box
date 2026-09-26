package protocolserver

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/recording"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	mcp "github.com/tingly-dev/tingly-box/internal/toolengine"
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
	parts := strings.SplitSeq(limits, ",")
	for part := range parts {
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
	isStreaming bool,
) {
	recorder := recording.FromGin(c)
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
		ph.dispatchOpenAIChat(c, reqCtx, rule, provider, isStreaming)
	case protocol.TypeAnthropicV1:
		ph.serveAnthropicV1Stage(c, reqCtx, rule, provider, isStreaming)
	case protocol.TypeAnthropicBeta:
		ph.dispatchAnthropicBeta(c, reqCtx, rule, provider, isStreaming)
	case protocol.TypeOpenAIResponses:
		ph.dispatchOpenAIResponses(c, reqCtx, rule, provider, isStreaming)
	case protocol.TypeGoogle:
		ph.dispatchGoogle(c, reqCtx, rule, provider, isStreaming)
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
		if recorder != nil {
			recorder.RecordError(fmt.Errorf("invalid api style: %s", provider.APIStyle))
		}
	}
}

// dispatchAnthropicBeta serves an Anthropic-Beta-bound request. OpenAI
// clients never reach it: they are served by the Stage pipeline
// (serveOpenAIOnAnthropic).
func (ph *ProtocolHandler) dispatchAnthropicBeta(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool,
) {
	switch reqCtx.SourceAPI {
	case protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses:
		ph.FailAttemptSetup(c, fmt.Errorf("unsupported source %s for an Anthropic Beta target", reqCtx.SourceAPI))
	default:
		ph.serveAnthropicBetaStage(c, reqCtx, rule, provider, isStreaming)
	}
}

// dispatchOpenAIResponses routes a Responses-API-bound request by the
// client's source format. Anthropic sources never reach it: they are served
// by the Stage pipeline (serveAnthropicOnOpenAI), which also assembles Codex
// (stream-only) answers for non-streaming clients.
func (ph *ProtocolHandler) dispatchOpenAIResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool,
) {
	switch reqCtx.SourceAPI {
	case protocol.TypeOpenAIChat:
		// Client sent Responses API, but provider needs Chat format
		// Forward as Chat, then convert response back to Responses format
		if isStreaming {
			ph.streamResponsesToChat(c, reqCtx, provider)
		} else {
			ph.nonstreamResponsesToChat(c, reqCtx, provider)
		}
	case protocol.TypeOpenAIResponses:
		// Responses API passthrough
		if isStreaming {
			ph.streamOpenAIResponses(c, reqCtx, provider)
		} else {
			ph.nonstreamOpenAIResponses(c, reqCtx, provider)
		}
	default:
		// Anthropic clients are served by serveAnthropicOnOpenAI.
		ph.FailAttemptSetup(c, fmt.Errorf("unsupported source %s for an OpenAI Responses target", reqCtx.SourceAPI))
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
		// Prefer the resolved flag set (scenario inheritance and provider
		// suppressions applied) — that is what actually drives the request.
		// Every handler resolves before dispatch; the raw rule.Flags fallback
		// only covers a path that skipped the merge, where it matches the old
		// behavior of this header.
		flags := typ.GetRuleFlags(c.Request.Context())
		if flags.IsZero() {
			flags = rule.Flags
		}
		if formatted := formatAppliedFlags(flags); formatted != "" {
			c.Header("X-Tingly-Applied-Flags", formatted)
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
	if f.Context1M {
		parts = append(parts, "context_1m")
	}
	if f.ClaudeOrgID != "" {
		parts = append(parts, "claude_org_id="+f.ClaudeOrgID)
	}
	if f.ClaudeCodeVersion != "" {
		parts = append(parts, "claude_code_version="+f.ClaudeCodeVersion)
	}
	// Count only — header values may carry user secrets and this string is
	// echoed back to clients (X-Tingly-Applied-Flags) and logged.
	if len(f.ExtraHeaders) > 0 {
		parts = append(parts, fmt.Sprintf("extra_headers=%d", len(f.ExtraHeaders)))
	}
	return strings.Join(parts, ", ")
}

func (ph *ProtocolHandler) dispatchGoogle(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool,
) {
	recorder := recording.FromGin(c)
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
		case protocol.TypeOpenAIChat:
			err = stream.HandleGoogleToOpenAIStreamResponse(c, streamResp, responseModel)
		default:
			err = fmt.Errorf("google target does not support %s clients", reqCtx.SourceAPI)
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
		case protocol.TypeOpenAIChat:
			chatResp := nonstream.HandleGoogleToOpenAI(resp, responseModel)
			if recorder != nil {
				recorder.SetAssembledResponse(chatResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			c.JSON(http.StatusOK, chatResp)
		default:
			err := fmt.Errorf("google target does not support %s clients", reqCtx.SourceAPI)
			stream.SendInternalError(c, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
		}
	}
}

func (ph *ProtocolHandler) dispatchOpenAIChat(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool,
) {
	responseModel := reqCtx.ResponseModel

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
		case protocol.TypeOpenAIChat:
			// OpenAI passthrough: source and target are both OpenAI Chat format
			disableStreamUsage := ShouldStripUsage(reqCtx.Extra)
			if reqCtx.ScenarioFlags != nil {
				disableStreamUsage = disableStreamUsage || reqCtx.ScenarioFlags.SkipUsage
			}

			if HasDeclaredMCPTools(req) && ph.mcpEnabled() {
				ph.DispatchGenericOpenAIChatStream(c, reqCtx, rule, provider)
				return
			}

			ph.streamOpenAIChat(c, provider, req, responseModel, disableStreamUsage)
		case protocol.TypeOpenAIResponses:
			ph.streamOpenAIChatToResponses(c, reqCtx, provider)
		default:
			// Anthropic clients reach Chat providers through the Stage pipeline
			// (serveAnthropicOnOpenAI) and never dispatch here.
			ph.FailAttemptSetup(c, fmt.Errorf("unsupported source %s for an OpenAI Chat target", reqCtx.SourceAPI))
		}
	} else {
		switch reqCtx.SourceAPI {
		case protocol.TypeOpenAIChat:
			// OpenAI passthrough: delegate to handleNonStreamingRequest for tool interceptor support
			stripUsage := ShouldStripUsage(reqCtx.Extra)

			if HasDeclaredMCPTools(req) && ph.mcpEnabled() {
				ph.DispatchGenericOpenAIChatNonStream(c, reqCtx, rule, provider)
				return
			}

			ph.nonstreamOpenAIChat(c, provider, req, responseModel, stripUsage)
			return
		case protocol.TypeOpenAIResponses:
			ph.nonstreamOpenAIChatToResponses(c, reqCtx, provider)
			return
		default:
			// Anthropic clients reach Chat providers through the Stage pipeline
			// (serveAnthropicOnOpenAI) and never dispatch here.
			ph.FailAttemptSetup(c, fmt.Errorf("unsupported source %s for an OpenAI Chat target", reqCtx.SourceAPI))
		}
	}
}

// Note: dispatchOpenAIChatToAnthropicBetaGeneric (OpenAI Chat -> Anthropic
// Beta cross-format TRUE-streaming dispatch) was dropped here — confirmed
// zero callers anywhere in the codebase at move time (Step 7), same
// dead-code disposition as smart_routing_helper.go in Step 4.
