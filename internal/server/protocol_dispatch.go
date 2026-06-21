// Package server
// since we do refactoring and migrating step by step, some api names are not unified, this will be updated in future
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func extractAnthropicV1Messages(messages []anthropic.MessageParam) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	b, _ := json.Marshal(messages)
	var out []map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func extractAnthropicBetaMessages(messages []anthropic.BetaMessageParam) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	b, _ := json.Marshal(messages)
	var out []map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// shouldUseGenericMCPForProvider checks if the provider is allowed to use generic MCP path
func (s *Server) shouldUseGenericMCPForProvider(provider *typ.Provider) bool {
	limits := s.config.GenericMCP.ProviderLimits
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
func (s *Server) dispatchChainResult(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel

	// Bubble up the execution-level routing decision for probes. This is the
	// single chokepoint where the resolved upstream API + provider + matched
	// rule + applied flags are all known, before any response byte is written.
	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeaders(c, reqCtx, rule, provider)
	}

	switch reqCtx.TargetAPI {
	case protocol.TypeOpenAIChat:
		s.dispatchOpenAIChat(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeAnthropicV1:
		s.passthroughAnthropicV1(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeAnthropicBeta:
		switch reqCtx.SourceAPI {
		case protocol.TypeOpenAIChat:
			s.dispatchAnthropicBetaToOpenAIChat(c, reqCtx, rule, provider, isStreaming, recorder)
		case protocol.TypeOpenAIResponses:
			if isStreaming {
				s.streamAnthropicBetaFromResponses(c, reqCtx, rule, provider, recorder)
			} else {
				s.nonstreamAnthropicBetaFromResponses(c, reqCtx, rule, provider, recorder)
			}
		default:
			s.passthroughAnthropicBeta(c, reqCtx, rule, provider, isStreaming, recorder)
		}
	case protocol.TypeOpenAIResponses:
		req := reqCtx.Request.(*responses.ResponseNewParams)
		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			logrus.Debugf("[AnthropicV1] Using Transform Chain for Responses API for model=%s", actualModel)
			if isStreaming {
				s.streamResponsesToAnthropic(c, responseModel, actualModel, provider, *req)
			} else {
				if provider.APIBase == protocol.CodexAPIBase {
					s.assembleResponsesToAnthropic(c, responseModel, actualModel, provider, *req)
				} else {
					s.nonstreamResponsesToAnthropic(c, responseModel, actualModel, provider, *req)
				}
			}
		case protocol.TypeAnthropicBeta:
			logrus.Debugf("[Anthropic Beta] Using Transform Chain for Responses API for model=%s", actualModel)
			if isStreaming {
				s.streamResponsesToAnthropicBeta(c, responseModel, actualModel, provider, *req)
			} else {
				if provider.APIBase == protocol.CodexAPIBase {
					s.assembleResponsesToAnthropicBeta(c, responseModel, actualModel, provider, *req)
				} else {
					s.nonstreamResponsesToAnthropicBeta(c, responseModel, actualModel, provider, *req)
				}
			}
		case protocol.TypeOpenAIChat:
			// Client sent Responses API, but provider needs Chat format
			// Forward as Chat, then convert response back to Responses format
			if isStreaming {
				s.streamResponsesToChat(c, reqCtx, rule, provider, recorder)
			} else {
				s.nonstreamResponsesToChat(c, reqCtx, rule, provider, recorder)
			}
		case protocol.TypeOpenAIResponses:
			// Responses API passthrough
			if isStreaming {
				s.streamOpenAIResponses(c, reqCtx, rule, provider, recorder)
			} else {
				s.nonstreamOpenAIResponses(c, reqCtx, rule, provider, recorder)
			}
		}
	case protocol.TypeGoogle:
		s.dispatchGoogle(c, reqCtx, rule, provider, isStreaming, recorder)
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
		if recorder != nil {
			recorder.RecordError(fmt.Errorf("invalid api style: %s", provider.APIStyle))
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
	if rule != nil && rule.UUID != probeSyntheticRuleUUID {
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

// passthroughAnthropicV1 handles Anthropic→Anthropic v1 passthrough (original behavior)
func (s *Server) passthroughAnthropicV1(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	if !isStreaming {
		s.dispatchGenericAnthropicV1NonStream(c, reqCtx, rule, provider, recorder)
		return
	}
	s.dispatchGenericAnthropicV1Stream(c, reqCtx, rule, provider, recorder)
}

// dispatchOpenAIChatFromAnthropicV1 handles OpenAI→Anthropic v1 conversion.
// The client expects OpenAI format, so responses are converted back.
func (s *Server) dispatchAnthropicBetaToOpenAIChat(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()

	wrapper := s.clientPool.GetAnthropicClient(ctx, provider, actualModel)
	fc := forwarding.NewForwardContext(ctx, provider)

	if isStreaming {
		disableStreamUsage := shouldStripUsage(reqCtx.Extra)
		if reqCtx.ScenarioFlags != nil {
			disableStreamUsage = disableStreamUsage || reqCtx.ScenarioFlags.SkipUsage
		}

		if hasDeclaredMCPAnthropicBetaTools(req) && s.mcpEnabled() {
			s.streamAnthropicBetaToOpenAIChatWithMCP(c, provider, req, actualModel, responseModel, disableStreamUsage, recorder)
			return
		}

		streamResp, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			s.trackUsageFromContext(c, 0, 0, err)
			SendErrorResponse(c, upstreamForwardStatus(err), fmt.Errorf("Failed to create streaming request: : %w", err), "api_error")
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		hc := protocol.NewHandleContext(c, responseModel)
		tokenUsage, err := stream.AnthropicToOpenAIStream(hc, req, streamResp, responseModel, disableStreamUsage)
		if err != nil {
			if tokenUsage.InputTokens > 0 || tokenUsage.OutputTokens > 0 {
				s.trackUsageWithTokenUsage(c, tokenUsage, err)
			} else {
				// Track error even when no tokens were received (e.g., early 1302 rate limit)
				s.trackUsageFromContext(c, 0, 0, err)
			}
			SendErrorResponse(c, upstreamForwardStatus(err), fmt.Errorf("Failed to create streaming request: : %w", err), "api_error")
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		if tokenUsage.InputTokens > 0 || tokenUsage.OutputTokens > 0 {
			s.trackUsageWithTokenUsage(c, tokenUsage, nil)
		}
	} else {
		wrapper := s.clientPool.GetAnthropicClient(ctx, provider, actualModel)
		fc := forwarding.NewForwardContext(ctx, provider)

		var anthropicResp *anthropic.BetaMessage
		var usage *protocol.TokenUsage
		var err error

		if hasDeclaredMCPAnthropicBetaTools(req) && s.mcpEnabled() {
			var genericUsage *mcp.TokenUsage
			anthropicResp, genericUsage, err = s.runGenericAnthropicBetaNonStream(ctx, provider, req, recorder)
			if err != nil {
				respondMCPError(s, c, recorder, err, "Failed to handle MCP tool calls")
				return
			}
			if genericUsage != nil {
				usage = protocol.NewTokenUsageWithCache(genericUsage.InputTokens, genericUsage.OutputTokens, genericUsage.CacheTokens)
			}
		} else {
			var cancel context.CancelFunc
			var forwardErr error
			anthropicResp, cancel, forwardErr = forwarding.ForwardAnthropicV1Beta(fc, wrapper, req)
			if cancel != nil {
				defer cancel()
			}
			if forwardErr != nil {
				s.trackUsageFromContext(c, 0, 0, forwardErr)
				SendErrorResponse(c, upstreamForwardStatus(forwardErr), fmt.Errorf("Failed to forward Anthropic request: : %w", forwardErr), "api_error")
				if recorder != nil {
					recorder.RecordError(forwardErr)
				}
				return
			}
			usage = usagepkg.FromAnthropicBetaMessage(anthropicResp.Usage)
		}

		s.trackUsageWithTokenUsage(c, usage, nil)

		openaiResp := ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp, responseModel, provider, actualModel)
		if ShouldRoundtripResponse(c, "anthropic") {
			roundtripped, err := RoundtripOpenAIMapViaAnthropic(openaiResp, responseModel, provider, actualModel)
			if err != nil {
				SendErrorResponse(c, http.StatusInternalServerError, fmt.Errorf("Failed to roundtrip response: : %w", err), "api_error")
				return
			}
			openaiResp = roundtripped
		}
		if shouldStripUsage(reqCtx.Extra) {
			delete(openaiResp, "usage")
		}

		if recorder != nil {
			recorder.SetAssembledResponse(anthropicResp)
			recorder.RecordResponse(provider, reqCtx.RequestModel)
		}
		c.JSON(http.StatusOK, openaiResp)
	}
}

func (s *Server) passthroughAnthropicBeta(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	useGeneric := s.mcpEnabled() && s.shouldUseGenericMCPForProvider(provider)

	if useGeneric {
		if !isStreaming {
			s.dispatchGenericAnthropicBetaNonStream(c, reqCtx, rule, provider, recorder)
			return
		}
		s.dispatchGenericAnthropicBetaStream(c, reqCtx, rule, provider, recorder)
		return
	}

	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)
	declaredMCP := hasDeclaredMCPAnthropicBetaTools(req)

	ctx := c.Request.Context()

	switch reqCtx.SourceAPI {
	case protocol.TypeOpenAIChat:
		s.dispatchAnthropicBetaToOpenAIChat(c, reqCtx, rule, provider, isStreaming, recorder)
	default:
		if isStreaming {
			if declaredMCP && s.mcpEnabled() {
				s.dispatchGenericAnthropicBetaStream(c, reqCtx, rule, provider, recorder)
				return
			}

			wrapper := s.clientPool.GetAnthropicClient(ctx, provider, actualModel)
			fc := forwarding.NewForwardContext(ctx, provider)
			streamResp, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, req)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				s.trackUsageFromContext(c, 0, 0, err)
				stream.SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			s.handleAnthropicStreamResponseV1Beta(c, req, streamResp, actualModel, responseModel, provider, recorder)
			return
		}

		var anthropicResp *anthropic.BetaMessage
		var err error
		if declaredMCP && s.mcpEnabled() {
			var usage *mcp.TokenUsage
			anthropicResp, usage, err = s.runGenericAnthropicBetaNonStream(ctx, provider, req, recorder)
			if err != nil {
				recordMCPForwardingError(s, c, err, recorder)
				return
			}
			if usage != nil {
				tokenUsage := protocol.NewTokenUsageWithCache(usage.InputTokens, usage.OutputTokens, usage.CacheTokens)
				s.trackUsageWithTokenUsage(c, tokenUsage, nil)
			}
		} else {
			wrapper := s.clientPool.GetAnthropicClient(ctx, provider, actualModel)
			fc := forwarding.NewForwardContext(ctx, provider)
			var cancel context.CancelFunc
			anthropicResp, cancel, err = forwarding.ForwardAnthropicV1Beta(fc, wrapper, req)
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

			s.trackUsageWithTokenUsage(c, usagepkg.FromAnthropicBetaMessage(anthropicResp.Usage), nil)
		}

		s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
		anthropicResp.Model = anthropic.Model(responseModel)

		scenario := GetTrackingContextScenario(c)
		if s.guardrailsEnabledForScenario(scenario) {
			s.applyGuardrailsToAnthropicV1BetaNonStreamResponse(c, req, actualModel, provider, anthropicResp)
		}

		if recorder != nil {
			recorder.SetAssembledResponse(anthropicResp)
			recorder.RecordResponse(provider, reqCtx.RequestModel)
		}
		nonstream.WriteAnthropicMessage(c, anthropicResp)
	}
}

func hasDeclaredMCPAnthropicV1Tools(req *anthropic.MessageNewParams) bool {
	if req == nil || len(req.Tools) == 0 {
		return false
	}
	for _, t := range req.Tools {
		if t.OfTool != nil && mcpruntime.IsMCPToolName(t.OfTool.Name) {
			return true
		}
	}
	return false
}

func hasDeclaredMCPAnthropicBetaTools(req *anthropic.BetaMessageNewParams) bool {
	// FIXME: we can not use such a simple logic to check
	if req == nil || len(req.Tools) == 0 {
		return false
	}

	for _, t := range req.Tools {
		if t.OfTool != nil && mcpruntime.IsMCPToolName(t.OfTool.Name) {
			return true
		}
	}

	return false
}

func (s *Server) buildOpenAIToAnthropicMCPHooks(
	ctx context.Context,
	providerUUID string,
	req *openai.ChatCompletionNewParams,
) *stream.OpenAIToAnthropicMCPHooks {
	if s == nil || s.mcpRuntime == nil || req == nil {
		return nil
	}

	registry := s.mcpRuntime.VirtualRegistry()
	hookMessages := extractOpenAIMessages(req.Messages)
	return &stream.OpenAIToAnthropicMCPHooks{
		ShouldSuppressTool: func(name string) bool {
			return mcp.IsVirtualTool(name, registry)
		},
		OnToolCallsFinal: func(calls []stream.OpenAIToAnthropicToolCall) error {
			if len(calls) == 0 {
				return nil
			}

			externalIDs := make([]string, 0, len(calls))
			virtualResults := make([]mcp.ToolExecutionResult, 0, len(calls))

			for _, tc := range calls {
				if !mcp.IsVirtualTool(tc.Name, registry) {
					if tc.ID != "" {
						externalIDs = append(externalIDs, tc.ID)
					}
					continue
				}

				arguments := tc.Arguments
				if arguments == "" {
					arguments = "{}"
				}
				// callMCPToolWithHooks updates context (e.g., advisor quota), so we must propagate it
				var toolResult coretool.ToolResult
				var err error
				ctx, toolResult, err = s.callMCPToolWithHooks(ctx, tc.Name, arguments, hookMessages)
				if err != nil {
					logrus.WithError(err).Warnf("mcp: tool call failed name=%s arguments=%s", tc.Name, arguments)
				}
				virtualResults = append(virtualResults, mcp.ToolExecutionResult{
					ToolUseID: tc.ID,
					Contents:  toolResult.Contents,
					IsError:   err != nil,
				})
			}

			if len(virtualResults) == 0 {
				return nil
			}

			segment := buildOpenAIContinuationSegment(calls, virtualResults)
			if len(segment) == 0 {
				return nil
			}

			if len(externalIDs) == 0 {
				req.Messages = append(req.Messages, segment...)
				return stream.ErrMCPStreamContinue
			}

			mcp.StoreOpenAIContinuationSegment(typ.GetSessionID(ctx), providerUUID, segment)
			return nil
		},
	}
}

func buildOpenAIContinuationSegment(
	calls []stream.OpenAIToAnthropicToolCall,
	virtualResults []mcp.ToolExecutionResult,
) []openai.ChatCompletionMessageParamUnion {
	if len(calls) == 0 || len(virtualResults) == 0 {
		return nil
	}
	toolCalls := make([]map[string]any, 0, len(calls))
	for _, tc := range calls {
		toolCalls = append(toolCalls, map[string]any{
			"id":   tc.ID,
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		})
	}
	assistantMsg := map[string]any{
		"role":       "assistant",
		"content":    "",
		"tool_calls": toolCalls,
	}
	b, err := json.Marshal(assistantMsg)
	if err != nil {
		return nil
	}
	var assistantUnion openai.ChatCompletionMessageParamUnion
	if err := json.Unmarshal(b, &assistantUnion); err != nil {
		return nil
	}
	segment := []openai.ChatCompletionMessageParamUnion{assistantUnion}
	for _, r := range virtualResults {
		if r.ToolUseID == "" {
			continue
		}
		segment = append(segment, openai.ToolMessage(r.TextContent(), r.ToolUseID))
	}
	return segment
}

// ── Google ──────────────────────────────────────────────────────────────

func (s *Server) dispatchGoogle(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	googleReq := reqCtx.Request.(*protocol.GoogleRequest)
	model, req, cfg := actualModel, googleReq.Contents, googleReq.Config

	if isStreaming {
		wrapper := s.clientPool.GetGoogleClient(c.Request.Context(), provider, model)
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
			s.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}
		s.trackUsageWithTokenUsage(c, usage, nil)
	} else {
		wrapper := s.clientPool.GetGoogleClient(c.Request.Context(), provider, model)
		fc := forwarding.NewForwardContext(nil, provider)
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
		s.trackUsageWithTokenUsage(c, usage, nil)

		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			anthropicResp := nonstream.ConvertGoogleToAnthropicResponse(resp, responseModel)
			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(anthropicResp, responseModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip resp: "+err.Error())
					return
				}
				anthropicResp = roundtripped
			}
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			nonstream.WriteAnthropicMessage(c, anthropicResp)
		case protocol.TypeAnthropicBeta:
			anthropicResp := nonstream.ConvertGoogleToAnthropicBetaResponse(resp, responseModel)
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			nonstream.WriteAnthropicMessage(c, anthropicResp)
		}
	}
}

// ── OpenAI Chat Completions ─────────────────────────────────────────────

func (s *Server) dispatchOpenAIChat(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
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
			s.streamOpenAIChatToAnthropicV1WithMCP(c, provider, req, actualModel, responseModel, recorder)
		case protocol.TypeAnthropicBeta:
			s.streamOpenAIChatToAnthropicBetaWithMCP(c, provider, req, actualModel, responseModel, recorder)
		case protocol.TypeOpenAIChat:
			// OpenAI passthrough: source and target are both OpenAI Chat format
			disableStreamUsage := shouldStripUsage(reqCtx.Extra)
			if reqCtx.ScenarioFlags != nil {
				disableStreamUsage = disableStreamUsage || reqCtx.ScenarioFlags.SkipUsage
			}

			if hasDeclaredMCPTools(req) && s.mcpEnabled() {
				s.dispatchGenericOpenAIChatStream(c, reqCtx, rule, provider, recorder)
				return
			}

			s.streamOpenAIChat(c, provider, req, responseModel, disableStreamUsage)
		case protocol.TypeOpenAIResponses:
			s.streamOpenAIChatToResponses(c, reqCtx, rule, provider, recorder)
		}
	} else {
		switch reqCtx.SourceAPI {
		case protocol.TypeOpenAIChat:
			// OpenAI passthrough: delegate to handleNonStreamingRequest for tool interceptor support
			stripUsage := shouldStripUsage(reqCtx.Extra)

			if hasDeclaredMCPTools(req) && s.mcpEnabled() {
				s.dispatchGenericOpenAIChatNonStream(c, reqCtx, rule, provider, recorder)
				return
			}

			s.nonstreamOpenAIChat(c, provider, req, responseModel, stripUsage)
			return
		case protocol.TypeOpenAIResponses:
			s.nonstreamOpenAIChatToResponses(c, reqCtx, rule, provider, recorder)
			return
		default:
			// Forward request to provider for format conversion
		}

		var resp *openai.ChatCompletion
		var err error
		var usage *protocol.TokenUsage
		if hasDeclaredMCPTools(req) && s.mcpEnabled() {
			var genericUsage *mcp.TokenUsage
			resp, genericUsage, err = s.runGenericOpenAIChatNonStream(c.Request.Context(), provider, req, recorder)
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
			wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
			fc := forwarding.NewForwardContext(nil, provider)
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

		s.trackUsageWithTokenUsage(c, usage, err)

		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			anthropicResp := nonstream.ConvertOpenAIToAnthropicResponse(resp, responseModel)
			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(anthropicResp, responseModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip resp: "+err.Error())
					return
				}
				anthropicResp = roundtripped
			}
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			nonstream.WriteAnthropicMessage(c, anthropicResp)
		case protocol.TypeAnthropicBeta:
			anthropicResp := nonstream.ConvertOpenAIToAnthropicBetaResponse(resp, responseModel)
			s.updateAffinityMessageID(c, rule, anthropicResp.ID)
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			nonstream.WriteAnthropicMessage(c, anthropicResp)
		}
	}
}

func (s *Server) streamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	responsesStream, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.handlePreStreamFailure(c, err, recorder)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(responsesStream)
	if primeErr != nil {
		s.handlePreStreamFailure(c, primeErr, recorder)
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleResponsesToOpenAIChatStream(hc, primedStream, responseModel)
	s.trackUsageWithTokenUsage(c, usage, err)
	if recorder != nil {
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
}

func (s *Server) nonstreamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	actualModel := reqCtx.RequestModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	responsesResp, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, upstreamForwardStatus(err), fmt.Errorf("Failed to forward request: : %w", err), "api_error")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleResponsesToOpenAIChatNonStream(hc, responsesResp)
	s.trackUsageWithTokenUsage(c, tokenUsage, nil)
	if recorder != nil {
		recorder.SetAssembledResponse(nonstream.OpenAIResponsesToChat(responsesResp, reqCtx.ResponseModel))
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
}

// nonstreamOpenAIResponses handles Responses API passthrough (non-streaming)
func (s *Server) nonstreamOpenAIResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	params := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, string(params.Model))
	fc := forwarding.NewForwardContext(nil, provider)
	response, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *params)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, upstreamForwardStatus(err), fmt.Errorf("Failed to forward request: : %w", err), "api_error")
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

// nonstreamOpenAIChatToResponses handles Chat → Responses conversion (non-streaming)
func (s *Server) nonstreamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(nil, provider)
	chatResp, _, err := forwarding.ForwardOpenAIChat(fc, wrapper, chatReq)
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, upstreamForwardStatus(err), fmt.Errorf("Failed to forward request: : %w", err), "api_error")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleOpenAIChatToResponsesNonStream(hc, chatResp, reqCtx.RequestModel)
	s.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamOpenAIChatToResponses handles Chat → Responses conversion (streaming)
// Extracted from openai_responses.go:202-216
func (s *Server) streamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	responseModel := reqCtx.ResponseModel
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatStream, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, chatReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, upstreamForwardStatus(err), fmt.Errorf("Failed to create streaming request: : %w", err), "api_error")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleOpenAIChatToResponsesStream(hc, chatStream, responseModel)
	s.trackUsageWithTokenUsage(c, usage, err)
}

// nonstreamAnthropicBetaFromResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (non-streaming).
func (s *Server) nonstreamAnthropicBetaFromResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	anthropicReq := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()
	wrapper := s.clientPool.GetAnthropicClient(ctx, provider, string(anthropicReq.Model))
	fc := forwarding.NewForwardContext(nil, provider)
	anthropicResp, cancel, err := forwarding.ForwardAnthropicV1Beta(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, upstreamForwardStatus(err), fmt.Errorf("Failed to forward request: : %w", err), "api_error")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleAnthropicBetaToResponsesNonStream(hc, anthropicResp, reqCtx.RequestModel)
	s.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamAnthropicBetaFromResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (streaming).
func (s *Server) streamAnthropicBetaFromResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	responseModel := reqCtx.ResponseModel
	anthropicReq := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()

	wrapper := s.clientPool.GetAnthropicClient(ctx, provider, string(anthropicReq.Model))
	fc := forwarding.NewForwardContext(ctx, provider)
	anthropicStream, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, upstreamForwardStatus(err), fmt.Errorf("Failed to create streaming request: : %w", err), "api_error")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleAnthropicBetaToOpenAIResponsesStream(hc, anthropicStream, responseModel)
	s.trackUsageWithTokenUsage(c, usage, err)
}

// dispatchOpenAIChatToAnthropicBetaGeneric handles OpenAI Chat -> Anthropic Beta
// cross-format streaming using TRUE streaming forwarding (not downgrade)
func (s *Server) dispatchOpenAIChatToAnthropicBetaGeneric(
	c *gin.Context,
	reqCtx *transform.TransformContext,
	rule *typ.Rule,
	provider *typ.Provider,
	isStreaming bool,
	recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*openai.ChatCompletionNewParams)
	if seg, ok := mcp.PopOpenAIContinuationSegment(typ.GetSessionID(c.Request.Context()), provider.UUID); ok {
		req.Messages = append(append([]openai.ChatCompletionMessageParamUnion{}, seg...), req.Messages...)
	}
	transform.AlignToolMessagesForOpenAI(req)

	// Step 1: Convert OpenAI Chat request to Anthropic Beta format
	const defaultMaxTokens = 4096
	anthropicReq := request.ConvertOpenAIToAnthropicRequest(req, defaultMaxTokens)

	// Step 2: Forward to Anthropic Beta using TRUE streaming (not non-stream downgrade)
	wrapper := s.clientPool.GetAnthropicClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	anthropicStream, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, anthropicReq)
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

	// Step 3: Convert Anthropic Beta stream back to OpenAI Chat format on-the-fly
	// This achieves TRUE streaming with proper format conversion
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.AnthropicToOpenAIStream(hc, anthropicReq, anthropicStream, responseModel, false)
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		stream.SendInternalError(c, err.Error())
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)
}
