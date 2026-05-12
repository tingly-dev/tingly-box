package server

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// getClientForProvider gets the appropriate Prober for a provider
func (s *Server) getClientForProvider(provider *typ.Provider, model string) (client.Prober, error) {
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		c := s.clientPool.GetAnthropicClient(context.Background(), provider, model)
		if c == nil {
			return nil, fmt.Errorf("failed to get Anthropic client for provider: %s", provider.Name)
		}
		return c, nil
	case protocol.APIStyleOpenAI:
		c := s.clientPool.GetOpenAIClient(context.Background(), provider, model)
		if c == nil {
			return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", provider.Name)
		}
		return c, nil
	case protocol.APIStyleGoogle:
		c := s.clientPool.GetGoogleClient(context.Background(), provider, model)
		if c == nil {
			return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}

// probeProviderWithSDK performs a non-streaming probe for a provider using Prober interface
func (s *Server) probeProviderWithSDK(ctx context.Context, provider *typ.Provider, model, message string, testMode ProbeMode) (*client.ProbeResult, error) {
	prober, err := s.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}

	// Use simple mode for non-streaming probe
	// Convert server.ProbeMode to client.ProbeMode
	clientMode := client.ProbeMode(testMode)
	res, err := prober.ProbeStream(ctx, model, message, clientMode)
	if err == nil {
		return res, nil
	}

	if provider.APIStyle == protocol.APIStyleOpenAI {
		if c, ok := prober.(client.OpenAIClientInterface); ok {
			res, err = c.ProbeResponsesStream(ctx, model, message, clientMode)
			return res, err
		}
	}

	return nil, err
}

// probeProviderStream performs a streaming probe for a provider using Prober interface
func (s *Server) probeProviderStream(ctx context.Context, provider *typ.Provider, model, message string, testMode ProbeMode) (*client.ProbeResult, error) {
	prober, err := s.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}

	// Convert server.ProbeMode to client.ProbeMode
	clientMode := client.ProbeMode(testMode)
	res, err := prober.ProbeStream(ctx, model, message, clientMode)
	if err == nil {
		return res, nil
	}

	if provider.APIStyle == protocol.APIStyleOpenAI {
		if c, ok := prober.(client.OpenAIClientInterface); ok {
			res, err = c.ProbeResponsesStream(ctx, model, message, clientMode)
			return res, err
		}
	}

	return nil, err
}
