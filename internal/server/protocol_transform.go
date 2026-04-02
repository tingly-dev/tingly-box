package server

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (s *Server) transformAnthropicBeta(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *ProtocolRecorder, scenarioType typ.RuleScenario) (*transform.TransformContext, error) {
	// Build transform chain with recording support
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, nil, protocolRecorder)
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
	if provider.AuthType == typ.AuthTypeOAuth {
		opts = append(opts, transform.WithUserID(provider.OAuthDetail.UserID))
	}

	transformCtx := transform.NewTransformContext(
		&req.BetaMessageNewParams,
		opts...,
	)
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
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, nil, protocolRecorder)
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
	if provider.AuthType == typ.AuthTypeOAuth {
		opts = append(opts, transform.WithUserID(provider.OAuthDetail.UserID))
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
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, nil, protocolRecorder)
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
	if provider.AuthType == typ.AuthTypeOAuth {
		opts = append(opts, transform.WithUserID(provider.OAuthDetail.UserID))
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
	chain, err := s.BuildTransformChain(c, target, provider.APIBase, nil, protocolRecorder)
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
	if provider.AuthType == typ.AuthTypeOAuth {
		opts = append(opts, transform.WithUserID(provider.OAuthDetail.UserID))
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
