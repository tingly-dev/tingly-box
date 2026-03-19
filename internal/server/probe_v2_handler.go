package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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
func (s *Server) probe(ctx context.Context, req *ProbeV2Request) (*ProbeV2Data, error) {
	provider, model, err := s.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	message := getProbeMessage(req.TestMode, req.Message)
	return s.probeProviderWithSDK(ctx, provider, model, message, req.TestMode)
}

// probeStream performs a streaming probe using SDK for both rule and provider targets
func (s *Server) probeStream(ctx context.Context, req *ProbeV2Request) (*ProbeV2Data, error) {
	provider, model, err := s.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	message := getProbeMessage(req.TestMode, req.Message)
	return s.probeProviderStream(ctx, provider, model, message, req.TestMode)
}

// resolveTargetToProviderModel resolves a probe request (rule or provider) to a provider and model
func (s *Server) resolveTargetToProviderModel(ctx context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
	switch req.TargetType {
	case ProbeV2TargetProvider:
		return s.resolveProviderTarget(ctx, req)
	case ProbeV2TargetRule:
		return s.resolveRuleTarget(ctx, req)
	default:
		return nil, "", fmt.Errorf("invalid target type: %s", req.TargetType)
	}
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

// resolveRuleTarget resolves a rule target to provider and model
func (s *Server) resolveRuleTarget(_ context.Context, req *ProbeV2Request) (*typ.Provider, string, error) {
	rule := s.config.GetRuleByUUID(req.RuleUUID)
	if rule == nil {
		return nil, "", fmt.Errorf("rule not found: %s", req.RuleUUID)
	}

	// Get the first active service from the rule
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
