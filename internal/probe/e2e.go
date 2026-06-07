package probe

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// E2EService runs SDK-level end-to-end probes against a rule, a saved
// provider, or an inline provider config. It is independent of *Server and
// is wired in NewServer.
type E2EService struct {
	config     *config.Config
	clientPool *client.ClientPool
}

// NewE2EService constructs a E2EService.
func NewE2EService(cfg *config.Config, pool *client.ClientPool) *E2EService {
	return &E2EService{
		config:     cfg,
		clientPool: pool,
	}
}

// Probe performs a non-streaming probe against the target described by req.
func (e *E2EService) Probe(ctx context.Context, req *E2ERequest) (*E2EData, error) {
	provider, model, probeHeaders, err := e.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(probeHeaders) > 0 {
		ctx = client.WithProbeHeaders(ctx, probeHeaders)
	}
	message := E2EMessage(req.TestMode, req.Message)
	return e.ProbeProviderWithSDK(ctx, provider, model, message, req.TestMode)
}

// ProbeStream performs a streaming probe against the target described by req.
func (e *E2EService) ProbeStream(ctx context.Context, req *E2ERequest) (*E2EData, error) {
	provider, model, probeHeaders, err := e.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(probeHeaders) > 0 {
		ctx = client.WithProbeHeaders(ctx, probeHeaders)
	}
	message := E2EMessage(req.TestMode, req.Message)
	return e.probeProviderStream(ctx, provider, model, message, req.TestMode)
}

// resolveTargetToProviderModel resolves an E2ERequest to a provider, model,
// and optional probe headers. Probe headers are injected into SDK HTTP calls
// via probeHeaderRoundTripper so that TB's own loopback endpoint can read them.
func (e *E2EService) resolveTargetToProviderModel(ctx context.Context, req *E2ERequest) (*typ.Provider, string, map[string]string, error) {
	var (
		provider     *typ.Provider
		model        string
		probeHeaders map[string]string
		err          error
	)
	switch req.TargetType {
	case E2ETargetProvider:
		provider, model, probeHeaders, err = e.resolveProviderTarget(ctx, req)
	case E2ETargetProviderConfig:
		provider, model, err = e.resolveProviderConfigTarget(ctx, req)
	case E2ETargetRule:
		provider, model, err = e.resolveRuleTarget(ctx, req)
	default:
		return nil, "", nil, fmt.Errorf("invalid target type: %s", req.TargetType)
	}
	if err != nil {
		return nil, "", nil, err
	}
	if provider.IsVirtual() {
		// vmodel://local can't be dialed; reroute through loopback so the
		// probe exercises the in-process handler end-to-end without mutating
		// the stored provider record.
		p, m, e2 := e.resolveVModelLoopbackTarget(ctx, provider, model)
		return p, m, nil, e2
	}
	return provider, model, probeHeaders, nil
}

func (e *E2EService) resolveVModelLoopbackTarget(ctx context.Context, provider *typ.Provider, model string) (*typ.Provider, string, error) {
	port := e.config.GetServerPort()
	if port == 0 {
		return nil, "", fmt.Errorf("server port unknown; cannot probe vmodel provider %q", provider.Name)
	}

	// Anthropic SDK trims a trailing /v1 from its BaseURL; OpenAI SDK does not.
	// Pass each base in the form its client expects so the rebuilt request URL
	// hits /v1/{messages,chat/completions} exactly once.
	var path string
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		path = "/virtual/anthropic"
	case protocol.APIStyleOpenAI:
		path = "/virtual/openai/v1"
	default:
		return nil, "", fmt.Errorf("vmodel probe unsupported for APIStyle %q", provider.APIStyle)
	}

	return e.resolveProviderConfigTarget(ctx, &E2ERequest{
		Name:     provider.Name,
		APIBase:  fmt.Sprintf("http://localhost:%d%s", port, path),
		APIStyle: string(provider.APIStyle),
		Token:    e.config.GetModelToken(),
		Model:    model,
	})
}

func (e *E2EService) resolveProviderTarget(ctx context.Context, req *E2ERequest) (*typ.Provider, string, map[string]string, error) {
	provider, err := e.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		return nil, "", nil, fmt.Errorf("provider not found: %s", req.ProviderUUID)
	}

	if !provider.Enabled {
		return nil, "", nil, fmt.Errorf("provider is disabled: %s", req.ProviderUUID)
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

	// Google providers don't have a matching /tingly/{scenario} endpoint;
	// probe them directly via SDK.
	if provider.APIStyle == protocol.APIStyleGoogle {
		return provider, model, nil, nil
	}

	// Route through TB's own loopback endpoint so request-level flags
	// (openai_endpoint_override, thinking_effort, etc.) can be applied when
	// a rule is also specified via X-Tingly-Probe-Rule.
	port := e.config.GetServerPort()
	if port == 0 {
		// Server port unknown — fall back to direct SDK probe.
		logrus.Debugf("[probe-e2e] server port unknown, falling back to direct SDK for provider %s", provider.UUID)
		return provider, model, nil, nil
	}

	_, apiStyle := ScenarioEndpoint(deriveScenarioForAPIStyle(provider.APIStyle))
	var apiBase string
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		apiBase = fmt.Sprintf("http://localhost:%d/tingly/%s", port, typ.ScenarioAnthropic)
	default:
		apiBase = fmt.Sprintf("http://localhost:%d/tingly/%s/v1", port, typ.ScenarioOpenAI)
	}

	probeHeaders := map[string]string{
		"X-Tingly-Probe-Service": req.ProviderUUID + ":" + model,
	}
	logrus.Debugf("[probe-e2e] provider %s -> TB loopback %s (service pin=%s:%s)", provider.UUID, apiBase, req.ProviderUUID, model)

	loopbackProvider, loopbackModel, err := e.resolveProviderConfigTarget(ctx, &E2ERequest{
		Name:     provider.Name,
		APIBase:  apiBase,
		APIStyle: string(apiStyle),
		Token:    e.config.GetModelToken(),
		Model:    model,
	})
	if err != nil {
		return nil, "", nil, err
	}
	return loopbackProvider, loopbackModel, probeHeaders, nil
}

// deriveScenarioForAPIStyle maps a provider API style to a default scenario name
// used when routing service probes through the TB loopback.
func deriveScenarioForAPIStyle(style protocol.APIStyle) string {
	switch style {
	case protocol.APIStyleAnthropic:
		return string(typ.ScenarioAnthropic)
	default:
		return string(typ.ScenarioOpenAI)
	}
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

func (e *E2EService) resolveRuleTarget(ctx context.Context, req *E2ERequest) (*typ.Provider, string, error) {
	rule := e.config.GetRuleByUUID(req.RuleUUID)
	if rule == nil {
		return nil, "", fmt.Errorf("rule not found: %s", req.RuleUUID)
	}

	port := e.config.GetServerPort()
	if port == 0 {
		return nil, "", fmt.Errorf("server port unknown; cannot probe rule %q via TB interface", rule.UUID)
	}

	scenario := rule.Scenario
	if scenario == "" {
		scenario = typ.ScenarioOpenAI
	}

	_, apiStyle := ScenarioEndpoint(string(scenario))

	// Route through the TB's own /tingly/{scenario} endpoint so that all
	// rule processing — flags, smart routing, load balancing — is exercised.
	// Path conventions mirror resolveVModelLoopbackTarget:
	//   Anthropic SDK trims a trailing /v1 from BaseURL → omit /v1 suffix.
	//   OpenAI SDK does not add /v1 → include /v1 in the path.
	var apiBase string
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		apiBase = fmt.Sprintf("http://localhost:%d/tingly/%s", port, scenario)
	default:
		apiBase = fmt.Sprintf("http://localhost:%d/tingly/%s/v1", port, scenario)
	}

	logrus.Debugf("[probe-e2e] rule %s -> TB loopback %s (model=%s)", rule.UUID, apiBase, rule.RequestModel)

	return e.resolveProviderConfigTarget(ctx, &E2ERequest{
		Name:     string(scenario),
		APIBase:  apiBase,
		APIStyle: string(apiStyle),
		Token:    e.config.GetModelToken(),
		Model:    rule.RequestModel,
	})
}

// ProbeProviderWithSDK runs an SDK probe by dispatching a minimal request
// through the provider's real-traffic client methods. Public because the
// server's provider onboarding path (testProviderConnectivity) reuses it.
func (e *E2EService) ProbeProviderWithSDK(ctx context.Context, provider *typ.Provider, model, message string, testMode E2EMode) (*E2EData, error) {
	mode := testMode

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		oc := e.clientPool.GetOpenAIClient(ctx, provider, model)
		if oc == nil {
			return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", provider.Name)
		}
		// Codex OAuth providers only speak the Responses API.
		if isCodexOAuth(provider) {
			return probeOpenAIResponses(ctx, oc, model, message, mode)
		}
		return probeOpenAIChat(ctx, oc, model, message, mode)

	case protocol.APIStyleAnthropic:
		ac := e.clientPool.GetAnthropicClient(ctx, provider, model)
		if ac == nil {
			return nil, fmt.Errorf("failed to get Anthropic client for provider: %s", provider.Name)
		}
		return probeAnthropicMessages(ctx, ac, model, message, mode)

	case protocol.APIStyleGoogle:
		gc := e.clientPool.GetGoogleClient(ctx, provider, model)
		if gc == nil {
			return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
		}
		return probeGoogleGenerate(ctx, gc, model, message, mode)

	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}

func (e *E2EService) probeProviderStream(ctx context.Context, provider *typ.Provider, model, message string, testMode E2EMode) (*E2EData, error) {
	return e.ProbeProviderWithSDK(ctx, provider, model, message, testMode)
}
