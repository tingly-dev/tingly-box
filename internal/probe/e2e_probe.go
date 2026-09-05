package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// E2EProber runs SDK-level end-to-end probes against a rule, a saved
// provider, or an inline provider config. It is independent of *Server and
// is wired in NewServer.
type E2EProber struct {
	config        *config.Config
	clientPool    *client.ClientPool
	endpointCache *endpointProbeCache
}

// NewE2EProber constructs a E2EProber.
func NewE2EProber(cfg *config.Config, pool *client.ClientPool) *E2EProber {
	return &E2EProber{
		config:        cfg,
		clientPool:    pool,
		endpointCache: newEndpointProbeCache(),
	}
}

// Probe performs an SDK probe against the target described by req. It serves
// all probe shapes — non-stream/stream × plain/tool — the stream decision is
// made inside the SDK helpers from req.ResolveAxes(). Only the narrow
// direct-endpoint capability-check shape is cached; everything else
// dispatches for real.
func (e *E2EProber) Probe(ctx context.Context, req *E2ERequest) (*E2EData, error) {
	provider, model, probeHeaders, err := e.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	// Resolve the wire axes into flat decisions once, here, so the SDK
	// helpers read booleans instead of re-branching. Tool composes with both
	// stream values: non-stream lifts structured tool_calls; stream keeps the
	// raw chunk array.
	stream, tool := req.ResolveAxes()
	endpointOverride := req.ResolveOpenAIEndpointOverride()

	// Narrow cache: only the direct provider+model+endpoint capability-check
	// shape (target_type=provider, direct=true, endpoint forced) is
	// cacheable — see endpointProbeCache's doc comment for why. Every other
	// probe shape (rule tests, tool-mode/streaming checks, generic
	// connectivity) always dispatches for real. shapeKey guards against a
	// cached success from one stream/tool combination short-circuiting a
	// differently-shaped check against the same provider/model/endpoint.
	cacheable := req.TargetType == E2ETargetProvider && req.Direct && !req.Customized() &&
		(endpointOverride == "chat" || endpointOverride == "responses")
	shapeKey := fmt.Sprintf("%v-%v", stream, tool)
	if req.Vision.Enabled() {
		// Vision probes are a distinct shape; the non-vision key format stays
		// unchanged so existing cache entries remain valid.
		shapeKey += "-" + string(req.Vision)
	}
	if cacheable && e.endpointCache.hit(provider.UUID, model, endpointOverride, shapeKey) {
		return &Result{Success: true, Message: "Verified recently (cached)"}, nil
	}

	if err := checkClientSimulation(req, provider, probeHeaders); err != nil {
		return nil, err
	}
	if len(probeHeaders) > 0 {
		ctx = client.WithProbeHeaders(ctx, probeHeaders)
	}
	if rw := req.probeRewrite(); rw != nil {
		ctx = client.WithProbeRewrite(ctx, rw)
	}
	params := req.probeParams(model)
	result, err := e.probeProviderWithSDK(ctx, provider, params, endpointOverride)
	if cacheable && err == nil && result != nil && result.Success {
		e.endpointCache.remember(provider.UUID, model, endpointOverride, shapeKey)
	}
	return result, err
}

// probeParams resolves the request into the flat shape the SDK helpers read.
// Shared by Probe and BuildCurl so both build from the same decisions.
func (req *E2ERequest) probeParams(model string) probeParams {
	stream, tool := req.ResolveAxes()
	return probeParams{
		Model:    model,
		Message:  E2EMessage(tool, req.Message),
		Stream:   stream,
		Tool:     tool,
		Thinking: req.Thinking,
		Vision:   req.Vision,
		System:   req.System,
		Messages: req.Messages,
		Client:   req.Client,
	}
}

// probeRewrite returns the post-serialization edits for this request, or nil
// when there are none.
func (req *E2ERequest) probeRewrite() *client.ProbeRewrite {
	if len(req.BodyOverrides) == 0 && len(req.Headers) == 0 {
		return nil
	}
	return &client.ProbeRewrite{Body: req.BodyOverrides, Headers: req.Headers}
}

// checkClientSimulation enforces what sending as a real client needs once the
// target is resolved: a loopback (TB must receive the request) speaking the
// Anthropic protocol (the only client implementation available, Claude Code).
func checkClientSimulation(req *E2ERequest, provider *typ.Provider, probeHeaders map[string]string) error {
	if req.Client == ClientNone {
		return nil
	}
	if len(probeHeaders) == 0 {
		return fmt.Errorf("sending as %s requires a through-TB probe (this target does not traverse TB)", req.Client)
	}
	if provider.APIStyle != protocol.APIStyleAnthropic {
		return fmt.Errorf("sending as %s requires the Anthropic protocol (the target speaks %s)", req.Client, provider.APIStyle)
	}
	return nil
}

// newClaudeCodeClient builds the loopback SDK client the way TB builds its
// own Claude Code OAuth client — same constructor, same headers, same request
// guard — pointed at the loopback provider. The http.Client is returned so
// the probe wrappers (probe headers, routing capture) can layer onto it.
func newClaudeCodeClient(ctx context.Context, provider *typ.Provider, model string, extra ...anthropicOption.RequestOption) (client.AnthropicClientInterface, *http.Client, error) {
	hc := &http.Client{Transport: http.DefaultTransport}
	opts := append([]anthropicOption.RequestOption{anthropicOption.WithHTTPClient(hc)}, extra...)
	cc, err := client.NewClaudeClient(ctx, provider, model, typ.GetSessionID(ctx), opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build Claude Code client for provider %s: %w", provider.Name, err)
	}
	return cc, hc, nil
}

// resolveTargetToProviderModel resolves an E2ERequest to a provider, model,
// and optional probe headers. Probe headers are injected into SDK HTTP calls
// via probeHeaderRoundTripper so that TB's own loopback endpoint can read them.
func (e *E2EProber) resolveTargetToProviderModel(ctx context.Context, req *E2ERequest) (*typ.Provider, string, map[string]string, error) {
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
	// The flag overlay rides on the probe-header family so TB's loopback
	// handler can fold it into flag resolution. It only makes sense where
	// those headers reach a TB handler; anything else has no middleware to
	// apply flags in.
	if len(req.Flags) > 0 {
		if len(probeHeaders) == 0 {
			return nil, "", nil, fmt.Errorf("flags require a through-TB probe (this target does not traverse TB)")
		}
		encoded, err := typ.EncodeFlagOverlay(req.Flags)
		if err != nil {
			return nil, "", nil, fmt.Errorf("encode flags: %w", err)
		}
		probeHeaders[typ.ProbeFlagsHeader] = encoded
	}
	return provider, model, probeHeaders, nil
}

func (e *E2EProber) resolveProviderTarget(ctx context.Context, req *E2ERequest) (*typ.Provider, string, map[string]string, error) {
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
		} else {
			return nil, "", nil, fmt.Errorf("no model to use: %s", req.ProviderUUID)
		}
	}

	// Direct probe: caller wants to test the upstream provider in isolation,
	// bypassing TB's middleware stack entirely. Useful for diagnosing whether
	// a failure is upstream vs TB-internal. A protocol override selects the
	// matching dual URL for dual-base providers (no-op for single-protocol).
	if req.Direct {
		logrus.Debugf("[probe-e2e] direct probe for provider %s (bypassing TB loopback)", provider.UUID)
		return provider.ResolveStyle(req.ResolveClientStyle(provider.APIStyle)), model, nil, nil
	}

	// Google providers don't have a matching /tingly/{scenario} endpoint;
	// probe them directly via SDK. They also have no protocol override path.
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

	// The client protocol the probe speaks: the provider's own by default,
	// or the requested override. With an override the loopback scenario is
	// the protocol family's canonical one (the SDK path and the URL must
	// agree); without one we prefer the scenario the caller is probing under
	// (the page's scenario, which may carry a profile suffix like
	// "claude_code:p4") so the loopback URL matches the page.
	clientStyle := req.ResolveClientStyle(provider.APIStyle)
	scenario := typ.RuleScenario(req.Scenario)
	if req.Protocol != "" {
		scenario, _ = defaultScenarioForAPIStyle(clientStyle)
	} else if scenario == "" {
		scenario, _ = defaultScenarioForAPIStyle(provider.APIStyle)
	}
	apiBase, _ := loopbackAPIBase(port, scenario)
	apiStyle := clientStyle
	probeHeaders := map[string]string{
		"X-Tingly-Probe-Service": req.ProviderUUID + ":" + model,
		"X-Tingly-Debug-Routing": "1",
	}
	logrus.Debugf("[probe-e2e] provider %s -> TB loopback %s (service pin=%s:%s)", provider.UUID, apiBase, req.ProviderUUID, model)

	loopbackProvider, loopbackModel, err := e.loopbackConfigTarget(ctx, provider.Name, apiBase, apiStyle, model)
	if err != nil {
		return nil, "", nil, err
	}
	return loopbackProvider, loopbackModel, probeHeaders, nil
}

// resolveOpenAIProbeEndpoint decides which OpenAI endpoint a probe should hit:
// an explicit override always wins; absent that, Codex OAuth providers only
// speak Responses, everything else defaults to Chat.
func resolveOpenAIProbeEndpoint(override string, provider *typ.Provider) string {
	switch override {
	case "chat", "responses":
		return override
	default:
		if provider.IsCodexProvider() {
			return "responses"
		}
		return "chat"
	}
}

// loopbackAPIBase returns the TB loopback base URL for the given scenario.
// TB registers both /tingly/:scenario and /tingly/:scenario/v1 with identical
// handlers, so the base URL needs no /v1 suffix — each SDK appends its own
// operation path (e.g. /chat/completions, /messages).
func loopbackAPIBase(port int, scenario typ.RuleScenario) (apiBase string, apiStyle protocol.APIStyle) {
	path, apiStyle := ScenarioEndpoint(string(scenario))
	return fmt.Sprintf("http://localhost:%d%s", port, path), apiStyle
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

func (e *E2EProber) resolveProviderConfigTarget(_ context.Context, req *E2ERequest) (*typ.Provider, string, error) {
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
		return nil, "", fmt.Errorf("no model specified for provider_config probe")
	}

	return provider, model, nil
}

// loopbackConfigTarget builds a provider_config target that points at TB's own
// loopback (using the in-process model token) instead of a real upstream. The
// three loopback paths — provider, rule, and vmodel — share this construction;
// only the name, apiStyle, and model differ.
func (e *E2EProber) loopbackConfigTarget(ctx context.Context, name, apiBase string, apiStyle protocol.APIStyle, model string) (*typ.Provider, string, error) {
	return e.resolveProviderConfigTarget(ctx, &E2ERequest{
		Name:     name,
		APIBase:  apiBase,
		APIStyle: string(apiStyle),
		Token:    e.config.GetModelToken(),
		Model:    model,
	})
}

func (e *E2EProber) resolveRuleTarget(ctx context.Context, req *E2ERequest) (*typ.Provider, string, map[string]string, error) {
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

	apiBase, apiStyle := loopbackAPIBase(port, scenario)

	logrus.Debugf("[probe-e2e] rule %s -> TB loopback %s (model=%s)", rule.UUID, apiBase, rule.RequestModel)

	probeHeaders := map[string]string{
		"X-Tingly-Debug-Routing": "1",
	}
	// Default (natural): no pin. The request carries only the rule's request
	// model and TB matches the rule exactly as it would for a real client;
	// the matched rule comes back in the routing trace, so a mismatch with
	// the rule the caller picked is visible rather than silently corrected.
	// Pinned: force this rule, skipping only the matching step.
	if req.Routing.Pinned() {
		probeHeaders["X-Tingly-Probe-Rule"] = rule.UUID
	}

	provider, model, err := e.loopbackConfigTarget(ctx, string(scenario), apiBase, apiStyle, rule.RequestModel)
	if err != nil {
		return nil, "", nil, err
	}
	return provider, model, probeHeaders, nil
}

// probeProviderWithSDK dispatches a minimal request through the provider's
// real-traffic client methods (the same methods production uses, so provider
// quirks cannot drift from the real path). The stream-vs-non-stream decision
// is made inside each per-provider helper from testMode.
//
// endpointOverride forces which OpenAI endpoint to hit ("chat"/"responses");
// pass "" for resolveOpenAIProbeEndpoint's default (Codex OAuth -> responses,
// everything else -> chat).
func (e *E2EProber) probeProviderWithSDK(ctx context.Context, provider *typ.Provider, params probeParams, endpointOverride string) (*E2EData, error) {
	_, wrapProbeHeaders := client.GetProbeHeaders(ctx)
	_, wrapRewrite := client.GetProbeRewrite(ctx)

	var result *E2EData
	var err error
	// maybeCapture wires probe-header + routing-capture round trippers onto a
	// client when this is a loopback probe, and returns a func that folds the
	// captured routing trace into the result once the call completes. For direct
	// probes (no probe headers) it returns a no-op. The body/header rewrite
	// (when present) goes on first so it sits innermost and has the last word.
	maybeCapture := func(c any) func(*E2EData) {
		if wrapRewrite {
			client.ApplyProbeRewriteToClient(c)
		}
		if !wrapProbeHeaders {
			return func(*E2EData) {}
		}
		client.ApplyProbeHeadersToClient(c)
		routing := client.ApplyRoutingCaptureToClient(c)
		return func(r *E2EData) {
			if r != nil {
				applyRoutingCapture(r, routing)
			}
		}
	}

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		oc := e.clientPool.GetOpenAIClient(ctx, provider, params.Model)
		if oc == nil {
			return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", provider.Name)
		}
		apply := maybeCapture(oc)
		switch ep := resolveOpenAIProbeEndpoint(endpointOverride, provider); ep {
		case "chat":
			result, err = probeOpenAIChat(ctx, oc, params)
		case "responses":
			result, err = probeOpenAIResponses(ctx, oc, params)
		default:
			// Unreachable: resolveOpenAIProbeEndpoint only returns "chat" or
			// "responses". Explicit so a future third value fails loudly
			// instead of silently returning (nil, nil).
			return nil, fmt.Errorf("probe: unhandled openai endpoint %q", ep)
		}
		if err == nil {
			apply(result)
		}

	case protocol.APIStyleAnthropic:
		var (
			ac    client.AnthropicClientInterface
			apply func(*E2EData)
		)
		if params.Client == ClientClaudeCode {
			// Send as Claude Code: TB's own Claude Code client emits the
			// loopback request, so what TB receives is what that client
			// sends — no separately maintained header list.
			cc, hc, cerr := newClaudeCodeClient(ctx, provider, params.Model)
			if cerr != nil {
				return nil, cerr
			}
			ac, apply = cc, maybeCapture(hc)
		} else {
			ac = e.clientPool.GetAnthropicClient(ctx, provider, params.Model)
			if ac == nil {
				return nil, fmt.Errorf("failed to get Anthropic client for provider: %s", provider.Name)
			}
			apply = maybeCapture(ac)
		}
		result, err = probeAnthropicMessages(ctx, ac, params)
		if err == nil {
			apply(result)
		}

	case protocol.APIStyleGoogle:
		gc := e.clientPool.GetGoogleClient(ctx, provider, params.Model)
		if gc == nil {
			return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
		}
		// Google probes are always direct (no loopback route) — no routing capture.
		result, err = probeGoogleGenerate(ctx, gc, params)

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
	if n, err := strconv.Atoi(cap.MatchedSmartRule); err == nil {
		result.MatchedSmartRule = &n
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
