package server

import (
	"context"
	"time"

	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// testProviderConnectivity tests if a provider's API key and connectivity are working.
// Kept for use by provider_handler.go provider onboarding/verification.
func (s *Server) testProviderConnectivity(req *probe.ProbeProviderRequest) (bool, string, int, error) {
	provider := &typ.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: protocol.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  true,
	}

	// Reuse v2 provider-config probing: simple mode with default model
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	model := s.getDefaultModelForAPIStyle(provider.APIStyle)
	message := "Hello, this is a test message. Please respond with a short greeting."

	data, err := s.probeE2EService.ProbeProviderWithSDK(ctx, provider, model, message, probe.E2EModeSimple)
	if err != nil {
		return false, err.Error(), 0, nil
	}

	if data.Content != "" {
		return true, "Provider verified successfully", 0, nil
	}

	return false, "Provider verification failed: no response content", 0, nil
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
