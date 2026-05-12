package server

import (
	"context"
	"time"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// testProviderConnectivity tests if a provider's API key and connectivity are working using cascading validation.
// Kept for use by provider_handler.go provider onboarding/verification.
func (s *Server) testProviderConnectivity(req *ProbeProviderRequest) (bool, string, int, error) {
	provider := &typ.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: protocol.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var prober client.Prober
	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		prober = s.clientPool.GetOpenAIClient(context.Background(), provider, "")
	case protocol.APIStyleAnthropic:
		prober = s.clientPool.GetAnthropicClient(context.Background(), provider, "")
	default:
		return false, "unsupported API style", 0, nil
	}

	if prober == nil {
		return false, "failed to create client for provider", 0, nil
	}

	result := prober.ProbeModelsEndpoint(ctx)
	if result.Success {
		return true, result.Message, result.ModelsCount, nil
	}

	defaultModel := s.getDefaultModelForAPIStyle(provider.APIStyle)
	result = prober.ProbeChatEndpoint(ctx, defaultModel)
	if result.Success {
		return true, result.Message, 0, nil
	}

	errorMsg := "Provider connectivity check failed. "
	if result.ErrorMessage != "" {
		errorMsg += result.ErrorMessage
	} else {
		errorMsg += "Neither models nor chat endpoints are accessible. This provider may not be compatible."
	}
	return false, errorMsg, 0, nil
}

// getDefaultModelForAPIStyle returns a default model name for probing based on API style.
func (s *Server) getDefaultModelForAPIStyle(apiStyle protocol.APIStyle) string {
	switch apiStyle {
	case protocol.APIStyleOpenAI:
		return "gpt-3.5-turbo"
	case protocol.APIStyleAnthropic:
		return "claude-3-haiku-20240307"
	default:
		return "gpt-3.5-turbo"
	}
}

// InvalidateProviderCache invalidates cached capabilities for a provider
func (s *Server) InvalidateProviderCache(providerUUID string) {
	if s.probeCache != nil {
		s.probeCache.InvalidateProvider(providerUUID)
	}
}
