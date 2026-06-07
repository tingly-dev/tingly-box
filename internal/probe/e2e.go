package probe

import (
	"context"
	"fmt"
	"net/url"

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
		provider, model, probeHeaders, err = e.resolveRuleTarget(ctx, req)
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

	scenario, ok := defaultScenarioForAPIStyle(provider.APIStyle)
	if !ok {
		return nil, "", fmt.Errorf("vmodel probe unsupported for APIStyle %q", provider.APIStyle)
	}
	_, apiStyle := ScenarioEndpoint(string(scenario))
	return e.resolveProviderConfigTarget(ctx, &E2ERequest{
		Name:     provider.Name,
		APIBase:  loopbackAPIBase(port, scenario),
		APIStyle: string(apiStyle),
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

	// Direct probe: caller wants to test the upstream provider in isolation,
	// bypassing TB's middleware stack entirely. Useful for diagnosing whether
	// a failure is upstream vs TB-internal.
	if req.Direct {
		logrus.Debugf("[probe-e2e] direct probe for provider %s (bypassing TB loopback)", provider.UUID)
		return provider, model, nil, nil
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

	scenario, _ := defaultScenarioForAPIStyle(provider.APIStyle)
	_, apiStyle := ScenarioEndpoint(string(scenario))
	apiBase := loopbackAPIBase(port, scenario)
	probeHeaders := map[string]string{
		"X-Tingly-Probe-Service": req.ProviderUUID + ":" + model,
		"X-Tingly-Debug-Routing":   "1",
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

// loopbackAPIBase returns the TB loopback base URL for the given scenario.
// TB registers both /tingly/:scenario and /tingly/:scenario/v1 with identical
// handlers, so the base URL needs no /v1 suffix — each SDK appends its own
// operation path (e.g. /chat/completions, /messages).
func loopbackAPIBase(port int, scenario typ.RuleScenario) string {
	path, _ := ScenarioEndpoint(string(scenario))
	return fmt.Sprintf("http://localhost:%d%s", port, path)
}

// defaultScenarioForAPIStyle returns the default TB scenario for provider-level
// probes, where no rule scenario is specified. Returns false for API styles
// that have no matching /tingly/{scenario} endpoint (e.g. Google).
func defaultScenarioForAPIStyle(style protocol.APIStyle) (typ.RuleScenario, bool) {
	switch style {
	case protocol.APIStyleAnthropic:
		return typ.ScenarioAnthropic, true
	case protocol.APIStyleOpenAI:
		return typ.ScenarioOpenAI, true
	default:
		return "", false
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

func (e *E2EService) resolveRuleTarget(ctx context.Context, req *E2ERequest) (*typ.Provider, string, map[string]string, error) {
	rule := e.config.GetRuleByUUID(req.RuleUUID)
	if rule == nil {
		return nil, "", nil, fmt.Errorf("rule not found: %s", req.RuleUUID)
	}

	port := e.config.GetServerPort()
	if port == 0 {
		return nil, "", nil, fmt.Errorf("server port unknown; cannot probe rule %q via TB interface", rule.UUID)
	}

	// Prefer the scenario the caller is probing under (the page's scenario,
	// which may carry a profile suffix like "claude_code:p1") so the loopback
	// hits the exact /tingly/{scenario} endpoint. Fall back to the rule's own
	// scenario, then to OpenAI.
	scenario := typ.RuleScenario(req.Scenario)
	if scenario == "" {
		scenario = rule.Scenario
	}
	if scenario == "" {
		scenario = typ.ScenarioOpenAI
	}

	_, apiStyle := ScenarioEndpoint(string(scenario))
	apiBase := loopbackAPIBase(port, scenario)

	logrus.Debugf("[probe-e2e] rule %s -> TB loopback %s (model=%s)", rule.UUID, apiBase, rule.RequestModel)

	probeHeaders := map[string]string{
		"X-Tingly-Debug-Routing": "1",
	}

	provider, model, err := e.resolveProviderConfigTarget(ctx, &E2ERequest{
		Name:     string(scenario),
		APIBase:  apiBase,
		APIStyle: string(apiStyle),
		Token:    e.config.GetModelToken(),
		Model:    rule.RequestModel,
	})
	if err != nil {
		return nil, "", nil, err
	}
	return provider, model, probeHeaders, nil
}

// ProbeProviderWithSDK runs an SDK probe by dispatching a minimal request
// through the provider's real-traffic client methods. Public because the
// server's provider onboarding path (testProviderConnectivity) reuses it.
func (e *E2EService) ProbeProviderWithSDK(ctx context.Context, provider *typ.Provider, model, message string, testMode E2EMode) (*E2EData, error) {
	mode := testMode

	_, wrapProbeHeaders := client.GetProbeHeaders(ctx)

	var result *E2EData
	var err error

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		oc := e.clientPool.GetOpenAIClient(ctx, provider, model)
		if oc == nil {
			return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", provider.Name)
		}
		var routing *client.RoutingCapture
		if wrapProbeHeaders {
			client.ApplyProbeHeadersToClient(oc)
			routing = client.ApplyRoutingCaptureToClient(oc)
		}
		// Codex OAuth providers only speak the Responses API.
		if isCodexOAuth(provider) {
			result, err = probeOpenAIResponses(ctx, oc, model, message, mode)
		} else {
			result, err = probeOpenAIChat(ctx, oc, model, message, mode)
		}
		if err == nil && routing != nil {
			applyRoutingCapture(result, routing)
		}

	case protocol.APIStyleAnthropic:
		ac := e.clientPool.GetAnthropicClient(ctx, provider, model)
		if ac == nil {
			return nil, fmt.Errorf("failed to get Anthropic client for provider: %s", provider.Name)
		}
		var routing *client.RoutingCapture
		if wrapProbeHeaders {
			client.ApplyProbeHeadersToClient(ac)
			routing = client.ApplyRoutingCaptureToClient(ac)
		}
		result, err = probeAnthropicMessages(ctx, ac, model, message, mode)
		if err == nil && routing != nil {
			applyRoutingCapture(result, routing)
		}

	case protocol.APIStyleGoogle:
		gc := e.clientPool.GetGoogleClient(ctx, provider, model)
		if gc == nil {
			return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
		}
		result, err = probeGoogleGenerate(ctx, gc, model, message, mode)

	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}

	return result, err
}

// applyRoutingCapture copies the captured TB loopback routing decisions into
// the probe result so callers can see which provider/model was ultimately
// selected and via which routing path.
func applyRoutingCapture(result *E2EData, cap *client.RoutingCapture) {
	if result == nil {
		return
	}
	cap.Mu.Lock()
	defer cap.Mu.Unlock()
	result.SelectedProvider = cap.SelectedProvider
	result.SelectedProviderUUID = cap.SelectedProviderUUID
	result.SelectedModel = cap.SelectedModel
	result.RoutingSource = cap.RoutingSource
	if cap.MatchedSmartRule != "" {
		fmt.Sscanf(cap.MatchedSmartRule, "%d", &result.MatchedSmartRule)
	}
	result.UpstreamAPI = cap.UpstreamAPI
	result.UpstreamURL = cap.UpstreamURL
	result.MatchedRule = cap.MatchedRule
	result.AppliedFlags = cap.AppliedFlags
	// Description was percent-encoded server-side for header safety.
	if desc, err := url.QueryUnescape(cap.MatchedRuleDesc); err == nil {
		result.MatchedRuleDesc = desc
	} else {
		result.MatchedRuleDesc = cap.MatchedRuleDesc
	}
}

func (e *E2EService) probeProviderStream(ctx context.Context, provider *typ.Provider, model, message string, testMode E2EMode) (*E2EData, error) {
	return e.ProbeProviderWithSDK(ctx, provider, model, message, testMode)
}
