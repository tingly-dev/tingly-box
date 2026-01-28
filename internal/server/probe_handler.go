package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleProbeProvider tests a provider's API key and connectivity
func (s *Server) HandleProbeProvider(c *gin.Context) {
	var req ProbeProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ProbeProviderResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate required fields
	if req.Name == "" || req.APIBase == "" || req.APIStyle == "" || req.Token == "" {
		c.JSON(http.StatusBadRequest, ProbeProviderResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "All fields (name, api_base, api_style, token) are required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Start timing
	startTime := time.Now()

	// Test the provider by calling their models endpoint
	valid, message, modelsCount, err := s.testProviderConnectivity(&req)
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
				"provider": req.Name,
				"api_base": req.APIBase,
			}, false, err.Error())
		}

		c.JSON(http.StatusOK, ProbeProviderResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "provider_test_failed",
			},
		})
		return
	}

	// Log successful test
	if s.logger != nil {
		s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
			"provider":      req.Name,
			"api_base":      req.APIBase,
			"valid":         valid,
			"models_count":  modelsCount,
			"response_time": responseTime,
		}, true, message)
	}

	// Determine test result
	testResult := "models_endpoint_success"
	if !valid {
		testResult = "models_endpoint_invalid"
	}

	c.JSON(http.StatusOK, ProbeProviderResponse{
		Success: true,
		Data: &ProbeProviderResponseData{
			Provider:     req.Name,
			APIBase:      req.APIBase,
			APIStyle:     req.APIStyle,
			Valid:        valid,
			Message:      message,
			TestResult:   testResult,
			ResponseTime: responseTime,
			ModelsCount:  modelsCount,
		},
	})
}

// testProviderConnectivity tests if a provider's API key and connectivity are working using cascading validation
func (s *Server) testProviderConnectivity(req *ProbeProviderRequest) (bool, string, int, error) {
	// Create a temporary provider config
	provider := &typ.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: protocol.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get the appropriate client based on API style
	var prober client.Prober
	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		prober = s.clientPool.GetOpenAIClient(provider, "")
	case protocol.APIStyleAnthropic:
		prober = s.clientPool.GetAnthropicClient(provider, "")
	default:
		return false, "unsupported API style", 0, nil
	}

	if prober == nil {
		return false, "failed to create client for provider", 0, nil
	}

	// Tier 1: Try models list endpoint
	result := prober.ProbeModelsEndpoint(ctx)
	if result.Success {
		return true, result.Message, result.ModelsCount, nil
	}

	// Tier 2: Try chat/messages endpoint with minimal message
	defaultModel := s.getDefaultModelForAPIStyle(provider.APIStyle)
	result = prober.ProbeChatEndpoint(ctx, defaultModel)
	if result.Success {
		return true, result.Message, 0, nil
	}

	// Tier 3: Try OPTIONS request for basic connectivity
	result = prober.ProbeOptionsEndpoint(ctx)
	if result.Success {
		return true, "API key is valid and endpoint accessible", 0, nil
	}

	return false, "Failed to connect to provider: " + result.ErrorMessage, 0, nil
}

// getDefaultModelForAPIStyle returns a default model name for probing based on API style
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

// HandleProbeModelEndpoints handles adaptive probe for model endpoints (chat and responses)
func (s *Server) HandleProbeModelEndpoints(c *gin.Context) {
	var req ModelProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ModelProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate provider exists
	provider, err := s.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		c.JSON(http.StatusNotFound, ModelProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: fmt.Sprintf("Provider not found: %s", req.ProviderUUID),
				Type:    "provider_not_found",
			},
		})
		return
	}

	// Create adaptive probe instance
	adaptiveProbe := NewAdaptiveProbe(s)

	// Execute probe
	result, err := adaptiveProbe.ProbeModelEndpoints(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ModelProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "probe_failed",
			},
		})
		return
	}

	// Convert endpoint status to API response format
	chatStatus := EndpointProbeStatus{
		Available:    result.ChatEndpoint.Available,
		LatencyMs:    result.ChatEndpoint.LatencyMs,
		ErrorMessage: result.ChatEndpoint.ErrorMessage,
		LastChecked:  result.ChatEndpoint.LastChecked.Format(time.RFC3339),
	}

	responsesStatus := EndpointProbeStatus{
		Available:    result.ResponsesEndpoint.Available,
		LatencyMs:    result.ResponsesEndpoint.LatencyMs,
		ErrorMessage: result.ResponsesEndpoint.ErrorMessage,
		LastChecked:  result.ResponsesEndpoint.LastChecked.Format(time.RFC3339),
	}

	data := &ModelProbeData{
		ProviderUUID:      result.ProviderUUID,
		ModelID:           result.ModelID,
		ChatEndpoint:      chatStatus,
		ResponsesEndpoint: responsesStatus,
		PreferredEndpoint: result.PreferredEndpoint,
		LastUpdated:       result.LastUpdated.Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, ModelProbeResponse{
		Success: true,
		Data:    data,
	})
}

// InvalidateProviderCache invalidates cached capabilities for a provider
func (s *Server) InvalidateProviderCache(providerUUID string) {
	if s.probeCache != nil {
		s.probeCache.InvalidateProvider(providerUUID)
	}
}
