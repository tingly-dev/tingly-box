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

	extra := map[string]any{}

	if provider.AuthType == typ.AuthTypeOAuth {
		extra["user_id"] = provider.OAuthDetail.UserID
	}
	extra["device"] = s.config.ClaudeCodeDeviceID

	transformCtx := transform.NewTransformContext(
		&req.BetaMessageNewParams,
		transform.WithProviderURL(provider.APIBase),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithExtra(extra),
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

	extra := map[string]any{}

	if provider.AuthType == typ.AuthTypeOAuth {
		extra["user_id"] = provider.OAuthDetail.UserID
	}
	extra["device"] = s.config.ClaudeCodeDeviceID

	transformCtx := transform.NewTransformContext(
		&req.MessageNewParams,
		transform.WithProviderURL(provider.APIBase),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithExtra(extra),
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
