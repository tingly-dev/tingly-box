package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleProbeV2 handles Probe V2 requests (unified endpoint for all test types)
func (s *Server) HandleProbeV2(c *gin.Context) {
	var req ProbeV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ProbeV2Response{
			Success: false,
			Error: &ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate request
	if err := validateProbeV2Request(&req); err != nil {
		c.JSON(http.StatusBadRequest, ProbeV2Response{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "validation_error",
			},
		})
		return
	}

	// Route to appropriate handler based on test mode
	switch req.TestMode {
	case ProbeV2ModeSimple:
		s.handleProbe(c, &req)
	case ProbeV2ModeStreaming, ProbeV2ModeTool:
		s.handleProbeStream(c, &req)
	}
}

// handleProbe handles simple (non-streaming) probe requests
func (s *Server) handleProbe(c *gin.Context, req *ProbeV2Request) {
	ctx := c.Request.Context()
	startTime := time.Now()

	// Both rule and provider probes use SDK
	data, err := s.probe(ctx, req)

	if err != nil {
		c.JSON(http.StatusOK, ProbeV2Response{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "probe_error",
			},
		})
		return
	}

	data.LatencyMs = time.Since(startTime).Milliseconds()

	c.JSON(http.StatusOK, ProbeV2Response{
		Success: true,
		Data:    data,
	})
}

// handleProbeStream handles streaming probe requests
func (s *Server) handleProbeStream(c *gin.Context, req *ProbeV2Request) {
	ctx := c.Request.Context()
	startTime := time.Now()

	// Both rule and provider probes use SDK
	data, err := s.probeStream(ctx, req)

	if err != nil {
		c.JSON(http.StatusOK, ProbeV2Response{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "probe_error",
			},
		})
		return
	}

	data.LatencyMs = time.Since(startTime).Milliseconds()

	c.JSON(http.StatusOK, ProbeV2Response{
		Success: true,
		Data:    data,
	})
}

// probe performs a probe using SDK for both rule and provider targets
func (s *Server) probe(ctx context.Context, req *ProbeV2Request) (*client.ProbeResult, error) {
	provider, model, err := s.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	message := getProbeMessage(req.TestMode, req.Message)
	return s.probeProviderWithSDK(ctx, provider, model, message, req.TestMode)
}

// probeStream performs a streaming probe using SDK for both rule and provider targets
func (s *Server) probeStream(ctx context.Context, req *ProbeV2Request) (*client.ProbeResult, error) {
	provider, model, err := s.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	message := getProbeMessage(req.TestMode, req.Message)
	return s.probeProviderStream(ctx, provider, model, message, req.TestMode)
}

// resolveTargetToProviderModel resolves a probe request (rule or provider) to a provider and model
func (s *Server) resolveTargetToProviderModel(ctx context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
	var (
		provider *typ.Provider
		model    string
		err      error
	)
	switch req.TargetType {
	case ProbeV2TargetProvider:
		provider, model, err = s.resolveProviderTarget(ctx, req)
	case ProbeV2TargetProviderConfig:
		provider, model, err = s.resolveProviderConfigTarget(ctx, req)
	case ProbeV2TargetRule:
		provider, model, err = s.resolveRuleTarget(ctx, req)
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
		return s.resolveVModelLoopbackTarget(ctx, provider, model)
	}
	return provider, model, nil
}

// resolveVModelLoopbackTarget synthesizes an inline provider config pointing
// at this server's own /virtual/<style> loopback route, then delegates to
// resolveProviderConfigTarget for the rest of the probe pipeline.
func (s *Server) resolveVModelLoopbackTarget(ctx context.Context, provider *typ.Provider, model string) (*typ.Provider, string, error) {
	port := s.config.GetServerPort()
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

	return s.resolveProviderConfigTarget(ctx, &ProbeV2Request{
		Name:     provider.Name,
		APIBase:  fmt.Sprintf("http://localhost:%d%s", port, path),
		APIStyle: string(provider.APIStyle),
		Token:    s.config.GetModelToken(),
		Model:    model,
	})
}

// resolveProviderTarget resolves a provider target to provider and model
func (s *Server) resolveProviderTarget(_ context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
	provider, err := s.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		return nil, "", fmt.Errorf("provider not found: %s", req.ProviderUUID)
	}

	if !provider.Enabled {
		return nil, "", fmt.Errorf("provider is disabled: %s", req.ProviderUUID)
	}

	// Get model to use
	model := req.Model
	if model == "" {
		// Use first available model from provider
		if len(provider.Models) > 0 {
			model = provider.Models[0]
		} else {
			// Fallback defaults
			if provider.APIStyle == protocol.APIStyleAnthropic {
				model = "claude-3-haiku-20240307"
			} else {
				model = "gpt-3.5-turbo"
			}
		}
	}

	return provider, model, nil
}

// resolveProviderConfigTarget builds a temporary provider from inline config
func (s *Server) resolveProviderConfigTarget(_ context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
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
		// Choose default model based on API style
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

// resolveRuleTarget resolves a rule target to provider and model.
// When smart routing is enabled, it evaluates smart routing rules first
// instead of falling back to the base service list.
func (s *Server) resolveRuleTarget(_ context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
	rule := s.config.GetRuleByUUID(req.RuleUUID)
	if rule == nil {
		return nil, "", fmt.Errorf("rule not found: %s", req.RuleUUID)
	}

	// Try smart routing first if enabled
	if rule.SmartEnabled && len(rule.SmartRouting) > 0 {
		selectedService, err := s.resolveSmartRoutingForProbe(rule)
		if err == nil && selectedService != nil {
			provider, err := s.config.GetProviderByUUID(selectedService.Provider)
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

	// Fallback: get the first active service from the rule's base services
	services := rule.GetServices()
	if len(services) == 0 {
		return nil, "", fmt.Errorf("rule has no services: %s", req.RuleUUID)
	}

	// Find first active service
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

	// Resolve provider from service
	provider, err := s.config.GetProviderByUUID(selectedService.Provider)
	if err != nil || provider == nil {
		return nil, "", fmt.Errorf("provider not found for service: %s", selectedService.Provider)
	}

	if !provider.Enabled {
		return nil, "", fmt.Errorf("provider is disabled: %s", provider.Name)
	}

	// Use the model from the service or the rule's request model
	model := selectedService.Model
	if model == "" {
		model = rule.RequestModel
	}

	return provider, model, nil
}

// resolveSmartRoutingForProbe evaluates smart routing rules for a probe request.
// It builds a minimal request based on the rule's scenario to extract context
// and then runs the smart routing evaluator.
func (s *Server) resolveSmartRoutingForProbe(rule *typ.Rule) (*loadbalance.Service, error) {
	_, apiStyle := getScenarioEndpoint(string(rule.Scenario))

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

	selectedService, err := s.SelectServiceFromSmartRouting(matchedServices, rule)
	if err != nil {
		return nil, fmt.Errorf("failed to select service from smart routing matches: %w", err)
	}
	if selectedService == nil {
		return nil, fmt.Errorf("smart routing returned no selectable service")
	}

	return selectedService, nil
}
