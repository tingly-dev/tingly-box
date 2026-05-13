package server

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// rewriteVModelProviderForProbe returns a shallow copy of provider whose
// APIBase points at this server's in-process /virtual/{style}/v1 route over
// loopback, with the global model token. The unmodified provider record
// stores "vmodel://local" as a sentinel that is never dialable; SDK clients
// (Anthropic, OpenAI) cannot probe it directly. Rewriting here keeps the
// probe path end-to-end (request actually hits the vmodel handler) without
// mutating the stored provider.
func (s *Server) rewriteVModelProviderForProbe(provider *typ.Provider) (*typ.Provider, error) {
	port := s.config.GetServerPort()
	if port == 0 {
		return nil, fmt.Errorf("server port unknown; cannot probe vmodel provider %q", provider.Name)
	}

	var path string
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		path = "/virtual/anthropic/v1"
	case protocol.APIStyleOpenAI:
		path = "/virtual/openai/v1"
	default:
		return nil, fmt.Errorf("vmodel probe unsupported for APIStyle %q", provider.APIStyle)
	}

	clone := *provider
	clone.AuthType = typ.AuthTypeAPIKey
	clone.Token = s.config.GetModelToken()
	clone.APIBase = fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	return &clone, nil
}

// getClientForProvider gets the appropriate Prober for a provider
func (s *Server) getClientForProvider(provider *typ.Provider, model string) (client.Prober, error) {
	if provider.IsVirtual() {
		rewritten, err := s.rewriteVModelProviderForProbe(provider)
		if err != nil {
			return nil, err
		}
		provider = rewritten
	}

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
