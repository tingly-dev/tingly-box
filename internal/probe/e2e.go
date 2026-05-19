package probe

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// E2EService runs SDK-level end-to-end probes against a rule, a saved
// provider, or an inline provider config.
//
// Rule and saved-provider probes are loopbacked through the standard
// /tingly/:scenario endpoint so the full server pipeline (routing, smart
// routing, load balancing, logging, usage tracking) runs exactly as it would
// for a production request. Provider probes carry X-Tingly-Probe-Provider /
// X-Tingly-Probe-Model headers so the handler skips rule resolution and
// targets one specific provider; rule probes carry no such headers and let
// the standard routing select a service from rule.RequestModel.
//
// Inline provider_config probes (unsaved keys) still call the upstream
// provider directly via the SDK — they have no UUID and don't belong in
// any rule, so there's nothing meaningful for the standard endpoint to
// resolve against.
type E2EService struct {
	config           *config.Config
	clientPool       *client.ClientPool
	loopbackBaseURL  func() string
}

// NewE2EService constructs a E2EService. loopbackBaseURL is evaluated lazily
// at each probe (scheme + host + port; e.g. "http://localhost:8080") and
// must NOT include a trailing slash or any path prefix — the probe service
// appends "/tingly/{scenario}" or "/virtual/..." itself. Server owns this
// decision so probe stays agnostic to TLS, bind address, and future
// transports (unix sockets, etc.).
func NewE2EService(cfg *config.Config, pool *client.ClientPool, loopbackBaseURL func() string) *E2EService {
	return &E2EService{
		config:          cfg,
		clientPool:      pool,
		loopbackBaseURL: loopbackBaseURL,
	}
}

// Probe performs a non-streaming probe against the target described by req.
func (e *E2EService) Probe(ctx context.Context, req *E2ERequest) (*E2EData, error) {
	provider, model, err := e.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	message := E2EMessage(req.TestMode, req.Message)
	return e.ProbeProviderWithSDK(ctx, provider, model, message, req.TestMode)
}

// ProbeStream performs a streaming probe against the target described by req.
func (e *E2EService) ProbeStream(ctx context.Context, req *E2ERequest) (*E2EData, error) {
	provider, model, err := e.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	message := E2EMessage(req.TestMode, req.Message)
	return e.probeProviderStream(ctx, provider, model, message, req.TestMode)
}

func (e *E2EService) resolveTargetToProviderModel(ctx context.Context, req *E2ERequest) (*typ.Provider, string, error) {
	switch req.TargetType {
	case E2ETargetProvider:
		return e.resolveProviderTarget(ctx, req)
	case E2ETargetProviderConfig:
		return e.resolveProviderConfigTarget(ctx, req)
	case E2ETargetRule:
		return e.resolveRuleTarget(ctx, req)
	default:
		return nil, "", fmt.Errorf("invalid target type: %s", req.TargetType)
	}
}

// resolveLoopbackTarget builds an ephemeral provider pointing at the local
// server's /tingly/:scenario endpoint, optionally with extra headers that
// influence dispatch (e.g. probe-direct overrides). The SDK client built
// from this provider will hit the standard pipeline rather than the upstream
// provider directly.
func (e *E2EService) resolveLoopbackTarget(scenario, model string, extraHeaders map[string]string, name string) (*typ.Provider, string, error) {
	base, err := e.getLoopbackBaseURL()
	if err != nil {
		return nil, "", err
	}

	endpoint, apiStyle := ScenarioEndpoint(scenario)

	// Anthropic SDK trims a trailing /v1 from its BaseURL; OpenAI SDK does
	// not. Pass each base in the form its client expects so the rebuilt
	// request URL hits the right /v1 path exactly once.
	var apiBase string
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		apiBase = base + endpoint
	case protocol.APIStyleOpenAI:
		apiBase = base + endpoint + "/v1"
	default:
		return nil, "", fmt.Errorf("loopback probe unsupported for APIStyle %q", apiStyle)
	}

	return &typ.Provider{
		Name:         name,
		APIBase:      apiBase,
		APIStyle:     apiStyle,
		Token:        e.config.GetModelToken(),
		Enabled:      true,
		ExtraHeaders: extraHeaders,
	}, model, nil
}

func (e *E2EService) resolveProviderTarget(_ context.Context, req *E2ERequest) (*typ.Provider, string, error) {
	provider, err := e.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		return nil, "", fmt.Errorf("provider not found: %s", req.ProviderUUID)
	}

	if !provider.Enabled {
		return nil, "", fmt.Errorf("provider is disabled: %s", req.ProviderUUID)
	}

	model := req.Model
	if model == "" {
		if len(provider.Models) > 0 {
			model = provider.Models[0]
		} else if provider.APIStyle == protocol.APIStyleAnthropic {
			model = "claude-3-haiku-20240307"
		} else {
			model = "gpt-3.5-turbo"
		}
	}

	// vmodel can't be dialed externally; reroute through the in-process
	// /virtual/ handler instead of the standard /tingly/:scenario pipeline.
	if provider.IsVirtual() {
		return e.resolveVModelLoopbackTarget(provider, model)
	}

	// Pick the scenario whose default api-style matches this provider, then
	// instruct the handler to target this provider directly via headers.
	var scenario string
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		scenario = string(typ.ScenarioAnthropic)
	default:
		scenario = string(typ.ScenarioOpenAI)
	}
	headers := map[string]string{
		HeaderProbeProvider: req.ProviderUUID,
		HeaderProbeModel:    model,
	}
	return e.resolveLoopbackTarget(scenario, model, headers, "probe-provider:"+provider.Name)
}

func (e *E2EService) resolveProviderConfigTarget(_ context.Context, req *E2ERequest) (*typ.Provider, string, error) {
	if req.APIBase == "" || req.APIStyle == "" || req.Token == "" {
		return nil, "", fmt.Errorf("provider_config target requires api_base, api_style, and token")
	}

	provider := &typ.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: protocol.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  true,
	}

	model := req.Model
	if model == "" {
		switch provider.APIStyle {
		case protocol.APIStyleAnthropic:
			model = "claude-3-haiku-20240307"
		case protocol.APIStyleGoogle:
			model = "gemini-2.0-flash-exp"
		default:
			model = "gpt-3.5-turbo"
		}
	}

	return provider, model, nil
}

func (e *E2EService) resolveRuleTarget(_ context.Context, req *E2ERequest) (*typ.Provider, string, error) {
	rule := e.config.GetRuleByUUID(req.RuleUUID)
	if rule == nil {
		return nil, "", fmt.Errorf("rule not found: %s", req.RuleUUID)
	}

	// Rule probes don't override routing — they send rule.RequestModel to the
	// rule's scenario endpoint and let the standard pipeline pick a service
	// via smart routing / load balancing, matching production behaviour.
	return e.resolveLoopbackTarget(string(rule.Scenario), rule.RequestModel, nil, "probe-rule:"+rule.UUID)
}

// resolveVModelLoopbackTarget routes a vmodel-provider probe to the
// in-process /virtual/ handler. vmodel can't be dialed externally so the
// probe never goes through /tingly/:scenario.
func (e *E2EService) resolveVModelLoopbackTarget(provider *typ.Provider, model string) (*typ.Provider, string, error) {
	base, err := e.getLoopbackBaseURL()
	if err != nil {
		return nil, "", fmt.Errorf("cannot probe vmodel provider %q: %w", provider.Name, err)
	}

	var path string
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		path = "/virtual/anthropic"
	case protocol.APIStyleOpenAI:
		path = "/virtual/openai/v1"
	default:
		return nil, "", fmt.Errorf("vmodel probe unsupported for APIStyle %q", provider.APIStyle)
	}

	return e.resolveProviderConfigTarget(context.Background(), &E2ERequest{
		Name:     provider.Name,
		APIBase:  base + path,
		APIStyle: string(provider.APIStyle),
		Token:    e.config.GetModelToken(),
		Model:    model,
	})
}

// getLoopbackBaseURL evaluates the injected getter and validates the result
// non-empty. Centralized so both loopback paths fail with the same message.
func (e *E2EService) getLoopbackBaseURL() (string, error) {
	if e.loopbackBaseURL == nil {
		return "", fmt.Errorf("loopback base URL not configured")
	}
	base := e.loopbackBaseURL()
	if base == "" {
		return "", fmt.Errorf("loopback base URL not yet available")
	}
	return base, nil
}

// getClientForProvider returns a Prober for the given provider via the client pool.
func (e *E2EService) getClientForProvider(provider *typ.Provider, model string) (client.Prober, error) {
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		c := e.clientPool.GetAnthropicClient(context.Background(), provider, model)
		if c == nil {
			return nil, fmt.Errorf("failed to get Anthropic client for provider: %s", provider.Name)
		}
		return c, nil
	case protocol.APIStyleOpenAI:
		c := e.clientPool.GetOpenAIClient(context.Background(), provider, model)
		if c == nil {
			return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", provider.Name)
		}
		return c, nil
	case protocol.APIStyleGoogle:
		c := e.clientPool.GetGoogleClient(context.Background(), provider, model)
		if c == nil {
			return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}

// ProbeProviderWithSDK runs a non-streaming SDK probe. Public because the
// server's provider onboarding path (testProviderConnectivity) reuses it.
func (e *E2EService) ProbeProviderWithSDK(ctx context.Context, provider *typ.Provider, model, message string, testMode E2EMode) (*E2EData, error) {
	prober, err := e.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}
	clientMode := client.ProbeMode(testMode)
	return prober.ProbeStream(ctx, model, message, clientMode)
}

func (e *E2EService) probeProviderStream(ctx context.Context, provider *typ.Provider, model, message string, testMode E2EMode) (*E2EData, error) {
	prober, err := e.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}
	clientMode := client.ProbeMode(testMode)
	return prober.ProbeStream(ctx, model, message, clientMode)
}
