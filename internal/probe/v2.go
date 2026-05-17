package probe

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SelectServiceFn picks one load-balanced service from a set of smart-routing
// matches. This is the single piece of behaviour V2Service borrows from the
// server-side load balancer; passed as a callback to avoid an
// internal/probe -> internal/server import cycle.
type SelectServiceFn func(matched []*loadbalance.Service, rule *typ.Rule) (*loadbalance.Service, error)

// V2Service runs SDK-level end-to-end probes against a rule, a saved
// provider, or an inline provider config. It is independent of *Server and
// is wired in NewServer.
type V2Service struct {
	config           *config.Config
	clientPool       *client.ClientPool
	selectFromRoutes SelectServiceFn
}

// NewV2Service constructs a V2Service. selectFromRoutes is required and
// receives the smart-routing decision when probing a rule target.
func NewV2Service(cfg *config.Config, pool *client.ClientPool, selectFromRoutes SelectServiceFn) *V2Service {
	return &V2Service{
		config:           cfg,
		clientPool:       pool,
		selectFromRoutes: selectFromRoutes,
	}
}

// Probe performs a non-streaming probe against the target described by req.
func (v *V2Service) Probe(ctx context.Context, req *ProbeV2Request) (*ProbeV2Data, error) {
	provider, model, err := v.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	message := ProbeMessage(req.TestMode, req.Message)
	return v.ProbeProviderWithSDK(ctx, provider, model, message, req.TestMode)
}

// ProbeStream performs a streaming probe against the target described by req.
func (v *V2Service) ProbeStream(ctx context.Context, req *ProbeV2Request) (*ProbeV2Data, error) {
	provider, model, err := v.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	message := ProbeMessage(req.TestMode, req.Message)
	return v.probeProviderStream(ctx, provider, model, message, req.TestMode)
}

func (v *V2Service) resolveTargetToProviderModel(ctx context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
	var (
		provider *typ.Provider
		model    string
		err      error
	)
	switch req.TargetType {
	case ProbeV2TargetProvider:
		provider, model, err = v.resolveProviderTarget(ctx, req)
	case ProbeV2TargetProviderConfig:
		provider, model, err = v.resolveProviderConfigTarget(ctx, req)
	case ProbeV2TargetRule:
		provider, model, err = v.resolveRuleTarget(ctx, req)
	default:
		return nil, "", fmt.Errorf("invalid target type: %s", req.TargetType)
	}
	if err != nil {
		return nil, "", err
	}
	if provider.IsVirtual() {
		// vmodel://local can't be dialed; reroute through loopback so the
		// probe exercises the in-process handler end-to-end without mutating
		// the stored provider record.
		return v.resolveVModelLoopbackTarget(ctx, provider, model)
	}
	return provider, model, nil
}

func (v *V2Service) resolveVModelLoopbackTarget(ctx context.Context, provider *typ.Provider, model string) (*typ.Provider, string, error) {
	port := v.config.GetServerPort()
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

	return v.resolveProviderConfigTarget(ctx, &ProbeV2Request{
		Name:     provider.Name,
		APIBase:  fmt.Sprintf("http://localhost:%d%s", port, path),
		APIStyle: string(provider.APIStyle),
		Token:    v.config.GetModelToken(),
		Model:    model,
	})
}

func (v *V2Service) resolveProviderTarget(_ context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
	provider, err := v.config.GetProviderByUUID(req.ProviderUUID)
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

	return provider, model, nil
}

func (v *V2Service) resolveProviderConfigTarget(_ context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
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

func (v *V2Service) resolveRuleTarget(_ context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
	rule := v.config.GetRuleByUUID(req.RuleUUID)
	if rule == nil {
		return nil, "", fmt.Errorf("rule not found: %s", req.RuleUUID)
	}

	if rule.SmartEnabled && len(rule.SmartRouting) > 0 {
		selectedService, err := v.resolveSmartRoutingForProbe(rule)
		if err == nil && selectedService != nil {
			provider, err := v.config.GetProviderByUUID(selectedService.Provider)
			if err == nil && provider != nil && provider.Enabled {
				model := selectedService.Model
				if model == "" {
					model = rule.RequestModel
				}
				logrus.Debugf("[probe_v2] smart routing selected service: %s -> %s", provider.Name, model)
				return provider, model, nil
			}
		}
		logrus.Debugf("[probe_v2] smart routing evaluation failed: %v, falling back to base services", err)
	}

	services := rule.GetServices()
	if len(services) == 0 {
		return nil, "", fmt.Errorf("rule has no services: %s", req.RuleUUID)
	}

	var selectedService *loadbalance.Service
	for _, svc := range services {
		if svc.Active {
			selectedService = svc
			break
		}
	}
	if selectedService == nil {
		selectedService = services[0]
	}

	provider, err := v.config.GetProviderByUUID(selectedService.Provider)
	if err != nil || provider == nil {
		return nil, "", fmt.Errorf("provider not found for service: %s", selectedService.Provider)
	}

	if !provider.Enabled {
		return nil, "", fmt.Errorf("provider is disabled: %s", provider.Name)
	}

	model := selectedService.Model
	if model == "" {
		model = rule.RequestModel
	}

	return provider, model, nil
}

func (v *V2Service) resolveSmartRoutingForProbe(rule *typ.Rule) (*loadbalance.Service, error) {
	_, apiStyle := ScenarioEndpoint(string(rule.Scenario))

	var reqCtx *smartrouting.RequestContext
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		probeReq := &anthropic.MessageNewParams{
			Model:     anthropic.Model(rule.RequestModel),
			MaxTokens: 5,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
			},
		}
		reqCtx = smartrouting.ExtractContext(probeReq)
	default:
		probeReq := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel(rule.RequestModel),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hi"),
			},
		}
		reqCtx = smartrouting.ExtractContext(probeReq)
	}

	router, err := smartrouting.NewRouter(rule.SmartRouting)
	if err != nil {
		return nil, fmt.Errorf("failed to create smart routing router: %w", err)
	}

	matchedServices, matched := router.EvaluateRequest(reqCtx)
	if !matched || len(matchedServices) == 0 {
		return nil, fmt.Errorf("no smart routing rule matched")
	}

	selectedService, err := v.selectFromRoutes(matchedServices, rule)
	if err != nil {
		return nil, fmt.Errorf("failed to select service from smart routing matches: %w", err)
	}
	if selectedService == nil {
		return nil, fmt.Errorf("smart routing returned no selectable service")
	}

	return selectedService, nil
}

// getClientForProvider returns a Prober for the given provider via the client pool.
func (v *V2Service) getClientForProvider(provider *typ.Provider, model string) (client.Prober, error) {
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		c := v.clientPool.GetAnthropicClient(context.Background(), provider, model)
		if c == nil {
			return nil, fmt.Errorf("failed to get Anthropic client for provider: %s", provider.Name)
		}
		return c, nil
	case protocol.APIStyleOpenAI:
		c := v.clientPool.GetOpenAIClient(context.Background(), provider, model)
		if c == nil {
			return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", provider.Name)
		}
		return c, nil
	case protocol.APIStyleGoogle:
		c := v.clientPool.GetGoogleClient(context.Background(), provider, model)
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
func (v *V2Service) ProbeProviderWithSDK(ctx context.Context, provider *typ.Provider, model, message string, testMode ProbeMode) (*ProbeV2Data, error) {
	prober, err := v.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}
	clientMode := client.ProbeMode(testMode)
	return prober.ProbeStream(ctx, model, message, clientMode)
}

func (v *V2Service) probeProviderStream(ctx context.Context, provider *typ.Provider, model, message string, testMode ProbeMode) (*ProbeV2Data, error) {
	prober, err := v.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}
	clientMode := client.ProbeMode(testMode)
	return prober.ProbeStream(ctx, model, message, clientMode)
}
