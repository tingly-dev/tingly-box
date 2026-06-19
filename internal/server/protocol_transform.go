package server

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// releaseOriginalRequest drops the pre-transform request once the chain has
// finished. All readers of ctx.OriginalRequest (the StagePre recorder and the
// Codex/Responses vendor ops) run *inside* chain.Execute, so after Execute
// returns nothing reads it again.
//
// On protocol-conversion paths (e.g. Anthropic client -> OpenAI/Gemini backend,
// or OpenAI client -> Anthropic backend) the outbound ctx.Request is a freshly
// built struct, while OriginalRequest still points at the gjson-backed source
// request. Both the Anthropic and OpenAI SDK decoders pin the raw request JSON
// onto that parsed struct (its unexported `raw` / `JSON` metadata fields), so
// keeping OriginalRequest alive holds the entire request body in memory for the
// whole streaming lifetime. Dropping it here lets that be GC'd as soon as the
// chain completes.
//
// On same-protocol passthrough (e.g. Anthropic->Anthropic) Request ==
// OriginalRequest (the chain mutates in place), so this is intentionally a
// no-op there.
//
// Applies to every source builder: transformAnthropicBeta / transformAnthropicV1
// / transformOpenAIChat / transformOpenAIResponses.
func releaseOriginalRequest(ctx *transform.TransformContext) {
	if ctx != nil && ctx.Request != ctx.OriginalRequest {
		ctx.OriginalRequest = nil
	}
}

// releaseReqCtxAfterStreamCommit arranges for the transform context's parsed
// request (Request + OriginalRequest) to be released once the failover gate
// wrapping c.Writer commits its first chunk.
//
// Why at commit and not before the stream: on the failover path reqCtx is
// captured by the attempt closure and stays reachable for the whole stream, so
// without this the gjson-pinned request body is retained until the stream ends.
// But releasing it *before* the stream is unsafe — a pre-first-chunk failure is
// retryable, and the retry re-reads reqCtx.Request. The gate's commit is exactly
// the boundary past which retry is impossible, so it is the earliest safe point
// to drop the request while still freeing the body for the bulk of a long stream.
//
// No-op when c.Writer is not a failover gate (single-service requests bypass the
// gate); there the attempt closure dies immediately and GC reclaims reqCtx.
func releaseReqCtxAfterStreamCommit(c *gin.Context, reqCtx *transform.TransformContext) {
	if reqCtx == nil {
		return
	}
	if g, ok := c.Writer.(*firstChunkGate); ok {
		g.SetOnCommit(reqCtx.ReleaseRequest)
	}
}

func (s *Server) transformAnthropicBeta(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario, preBaseTransforms []transform.Transform, preVendorTransforms []transform.Transform) (*transform.TransformContext, error) {

	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, scenarioType, nil, protocolRecorder, preBaseTransforms, preVendorTransforms)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		flags := scenarioConfig.GetDefaultFlags()
		scenarioFlags = &flags
		if flags.SmartCompact {
			baseTransforms := chain.GetTransforms()
			chain.SetTransforms(append(
				[]transform.Transform{smart_compact.NewCompactTransform(2)},
				baseTransforms...,
			))
		}
	}

	opts := []transform.TransformOption{
		transform.WithContext(c.Request.Context()),
		transform.WithProviderURL(provider.APIBase),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(s.config.ClaudeCodeDeviceID),
	}

	// Advisor loopback requests carry X-Tingly-Advisor-Depth >= 1; skip MCP tool injection for them
	if c.GetHeader("X-Tingly-Advisor-Depth") != "" {
		opts = append(opts, transform.WithIsAdvisorRequest(true))
	}

	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		opts = append(opts, transform.WithUserID(provider.OAuthDetail.UserID))
		opts = append(opts, transform.WithIssuer(provider.OAuthDetail.ProviderType))
	}

	transformCtx := transform.NewTransformContext(
		&req.BetaMessageNewParams,
		opts...,
	)
	transformCtx.HasNativeAdvisor = hasNativeAdvisorBeta(req)
	transformCtx.SourceAPI = protocol.TypeAnthropicBeta
	transformCtx.TargetAPI = target

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		if protocolRecorder != nil {
			protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
			protocolRecorder.RecordError(err)
		}
		return nil, err
	}

	// Store transform steps in V2 recorder
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
	}

	releaseOriginalRequest(finalCtx)

	return finalCtx, nil
}

func (s *Server) transformAnthropicV1(c *gin.Context, req protocol.AnthropicMessagesRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario, preBaseTransforms []transform.Transform, preVendorTransforms []transform.Transform) (*transform.TransformContext, error) {
	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, scenarioType, nil, protocolRecorder, preBaseTransforms, preVendorTransforms)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		flags := scenarioConfig.GetDefaultFlags()
		scenarioFlags = &flags
		if flags.SmartCompact {
			baseTransforms := chain.GetTransforms()
			chain.SetTransforms(append(
				[]transform.Transform{smart_compact.NewCompactTransform(2)},
				baseTransforms...,
			))
		}
	}

	opts := []transform.TransformOption{
		transform.WithProviderURL(provider.APIBase),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(s.config.ClaudeCodeDeviceID),
	}

	// Check if this is an advisor request
	if c.GetHeader("X-Tingly-Advisor-Depth") != "" {
		opts = append(opts, transform.WithIsAdvisorRequest(true))
	}

	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		opts = append(opts, transform.WithUserID(provider.OAuthDetail.UserID))
		opts = append(opts, transform.WithIssuer(provider.OAuthDetail.ProviderType))
	}

	transformCtx := transform.NewTransformContext(
		&req.MessageNewParams,
		opts...,
	)
	transformCtx.SourceAPI = protocol.TypeAnthropicV1
	transformCtx.TargetAPI = target

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		if protocolRecorder != nil {
			protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
			protocolRecorder.RecordError(err)
		}
		return nil, err
	}

	// Store transform steps in V2 recorder
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
	}

	releaseOriginalRequest(finalCtx)

	return finalCtx, nil
}

func (s *Server) transformOpenAIChat(c *gin.Context, req protocol.OpenAIChatCompletionRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario, preBaseTransforms []transform.Transform, preVendorTransforms []transform.Transform) (*transform.TransformContext, error) {
	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, scenarioType, nil, protocolRecorder, preBaseTransforms, preVendorTransforms)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		scenarioFlags = &scenarioConfig.Flags
	}

	opts := []transform.TransformOption{
		transform.WithProviderURL(provider.APIBase),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(s.config.ClaudeCodeDeviceID),
	}

	// Check if this is an advisor request
	if c.GetHeader("X-Tingly-Advisor-Depth") != "" {
		opts = append(opts, transform.WithIsAdvisorRequest(true))
	}

	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		opts = append(opts, transform.WithUserID(provider.OAuthDetail.UserID))
		opts = append(opts, transform.WithIssuer(provider.OAuthDetail.ProviderType))
	}

	transformCtx := transform.NewTransformContext(
		&req.ChatCompletionNewParams,
		opts...,
	)
	transformCtx.SourceAPI = protocol.TypeOpenAIChat
	transformCtx.TargetAPI = target

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		if protocolRecorder != nil {
			protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
			protocolRecorder.RecordError(err)
		}
		return nil, err
	}

	// Store transform steps in V2 recorder
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
	}

	releaseOriginalRequest(finalCtx)

	return finalCtx, nil
}

func (s *Server) transformOpenAIResponses(c *gin.Context, req protocol.ResponseCreateRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario, maxAllowed int, preBaseTransforms []transform.Transform, preVendorTransforms []transform.Transform) (*transform.TransformContext, error) {
	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, scenarioType, nil, protocolRecorder, preBaseTransforms, preVendorTransforms)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		scenarioFlags = &scenarioConfig.Flags
	}

	opts := []transform.TransformOption{
		transform.WithProviderURL(provider.APIBase),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(s.config.ClaudeCodeDeviceID),
		transform.WithMaxTokens(int64(maxAllowed)),
	}
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		opts = append(opts, transform.WithUserID(provider.OAuthDetail.UserID))
		opts = append(opts, transform.WithIssuer(provider.OAuthDetail.ProviderType))
	}

	transformCtx := transform.NewTransformContext(
		&req.ResponseNewParams,
		opts...,
	)
	transformCtx.SourceAPI = protocol.TypeOpenAIResponses
	transformCtx.TargetAPI = target

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		if protocolRecorder != nil {
			protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
			protocolRecorder.RecordError(err)
		}
		return nil, err
	}

	// Store transform steps in V2 recorder
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
	}

	releaseOriginalRequest(finalCtx)

	return finalCtx, nil
}
