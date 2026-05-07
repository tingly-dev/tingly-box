package server

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (s *Server) transformAnthropicBeta(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario) (*transform.TransformContext, error) {

	// Build transform chain with recording support
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, scenarioType, nil, protocolRecorder)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		scenarioFlags = &scenarioConfig.Flags
	}

	if s.ApplySmartCompact(scenarioType) {
		baseTransforms := chain.GetTransforms()
		newTransforms := append(
			[]transform.Transform{
				smart_compact.NewCompactTransform(2),
			},
			baseTransforms...,
		)
		chain.SetTransforms(newTransforms)
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

func (s *Server) transformAnthropicV1(c *gin.Context, req protocol.AnthropicMessagesRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario) (*transform.TransformContext, error) {
	// Build transform chain with recording support
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, scenarioType, nil, protocolRecorder)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		scenarioFlags = &scenarioConfig.Flags
	}

	if s.ApplySmartCompact(scenarioType) {
		baseTransforms := chain.GetTransforms()
		newTransforms := append(
			[]transform.Transform{
				smart_compact.NewCompactTransform(2),
			},
			baseTransforms...,
		)
		chain.SetTransforms(newTransforms)
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

func (s *Server) transformOpenAIChat(c *gin.Context, req protocol.OpenAIChatCompletionRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario) (*transform.TransformContext, error) {
	// Build transform chain with recording support
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, scenarioType, nil, protocolRecorder)
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

func (s *Server) transformOpenAIResponses(c *gin.Context, req protocol.ResponseCreateRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario, maxAllowed int) (*transform.TransformContext, error) {
	// Build transform chain with recording support
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, scenarioType, nil, protocolRecorder)
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
