package server

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// releaseReqCtxAfterStreamCommit releases reqCtx's request (both Request and
// OriginalRequest) once the failover gate commits its first chunk — the earliest
// point retry can no longer re-read it. On the failover path reqCtx is held by
// the attempt closure for the whole stream, so this is what frees the
// gjson-pinned body; releasing before the stream would break a retryable
// pre-first-chunk failure. No-op without a gate (single-service), where GC
// reclaims reqCtx on its own.
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

	return finalCtx, nil
}
