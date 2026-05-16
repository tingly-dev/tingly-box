package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleLightweightProbe handles lightweight probe requests for optional key validation
// This endpoint is used by the "Test Connection" button when adding API keys.
// It tests OPTIONS and models endpoints, but results are informational only - not blocking.
func (s *Server) HandleLightweightProbe(c *gin.Context) {
	var req LightweightProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LightweightProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate required fields
	if req.APIBase == "" || req.APIStyle == "" || req.Token == "" {
		c.JSON(http.StatusBadRequest, LightweightProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "api_base, api_style, and token are required",
				Type:    "validation_error",
			},
		})
		return
	}

	// Build a temporary provider for probing
	provider := &typ.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: protocol.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  true,
	}

	// Set auth type if provided
	if req.AuthType != "" {
		provider.AuthType = typ.AuthType(req.AuthType)
	}

	// Run lightweight probe
	data, err := s.lightweightProbe(c.Request.Context(), provider)
	if err != nil {
		c.JSON(http.StatusOK, LightweightProbeResponse{
			Success: true, // Always return success - this is informational only
			Data:    data,
		})
		return
	}

	c.JSON(http.StatusOK, LightweightProbeResponse{
		Success: true,
		Data:    data,
	})
}

// lightweightProbe performs a lightweight probe using all available endpoints
func (s *Server) lightweightProbe(ctx context.Context, provider *typ.Provider) (*LightweightProbeResponseData, error) {
	data := &LightweightProbeResponseData{
		Provider: provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
	}

	// Step 1: Probe OPTIONS endpoint (basic connectivity)
	optionsResult := s.probeOptionsEndpoint(ctx, provider)
	data.OptionsSuccess = optionsResult.Success
	data.OptionsMessage = optionsResult.Message
	data.OptionsResponseTime = optionsResult.ResponseTime

	// Step 2: Probe models endpoint (API access validation)
	modelsResult := s.probeModelsEndpoint(ctx, provider)
	data.ModelsSuccess = modelsResult.Success
	data.ModelsMessage = modelsResult.Message
	data.ModelsResponseTime = modelsResult.ResponseTime
	data.ModelsCount = modelsResult.ModelsCount
	data.Warning = modelsResult.Warning

	// Step 3: For OpenAI-style providers, probe Chat and Responses endpoints
	if provider.APIStyle == protocol.APIStyleOpenAI {
		chatResult := s.probeChatEndpoint(ctx, provider)
		data.ChatSuccess = chatResult.Success
		data.ChatMessage = chatResult.Message
		data.ChatResponseTime = chatResult.ResponseTime

		responsesResult := s.probeResponsesEndpoint(ctx, provider)
		data.ResponsesSuccess = responsesResult.Success
		data.ResponsesMessage = responsesResult.Message
		data.ResponsesResponseTime = responsesResult.ResponseTime
	}

	// Determine overall validity (best effort)
	// Consider it valid if at least one probe succeeded
	data.Valid = data.OptionsSuccess || data.ModelsSuccess || data.ChatSuccess || data.ResponsesSuccess

	if data.Valid {
		successCount := 0
		if data.OptionsSuccess {
			successCount++
		}
		if data.ModelsSuccess {
			successCount++
		}
		if data.ChatSuccess {
			successCount++
		}
		if data.ResponsesSuccess {
			successCount++
		}

		data.Message = fmt.Sprintf("Connection test completed - %d/%d endpoints accessible", successCount, 4)
	} else {
		data.Message = "Connection test failed - unable to reach any provider endpoint"
	}

	return data, nil
}

// probeOptionsEndpoint probes the OPTIONS endpoint for basic connectivity
func (s *Server) probeOptionsEndpoint(ctx context.Context, provider *typ.Provider) struct {
	Success      bool
	Message      string
	ResponseTime int64
} {
	startTime := time.Now()

	// Get appropriate client based on API style
	var result client.ProbeResult

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		c := s.clientPool.GetOpenAIClient(context.Background(), provider, "")
		if c == nil {
			return struct {
				Success      bool
				Message      string
				ResponseTime int64
			}{false, "Failed to create OpenAI client", 0}
		}
		if openaiClient, ok := c.(*client.OpenAIClient); ok {
			result = openaiClient.ProbeOptionsEndpoint(ctx)
		} else {
			// For CodexClient or other OpenAIClientInterface implementations
			// Try to use type assertion to ProbeOptionsEndpoint if available
			return struct {
				Success      bool
				Message      string
				ResponseTime int64
			}{false, "OPTIONS probe not implemented for this client type", 0}
		}
	case protocol.APIStyleAnthropic:
		c := s.clientPool.GetAnthropicClient(context.Background(), provider, "")
		if c == nil {
			return struct {
				Success      bool
				Message      string
				ResponseTime int64
			}{false, "Failed to create Anthropic client", 0}
		}
		if anthropicClient, ok := c.(*client.AnthropicClient); ok {
			result = anthropicClient.ProbeOptionsEndpoint(ctx)
		} else {
			return struct {
				Success      bool
				Message      string
				ResponseTime int64
			}{false, "OPTIONS probe not implemented for this client type", 0}
		}
	case protocol.APIStyleGoogle:
		c := s.clientPool.GetGoogleClient(context.Background(), provider, "")
		if c == nil {
			return struct {
				Success      bool
				Message      string
				ResponseTime int64
			}{false, "Failed to create Google client", 0}
		}
		// GoogleClient is a concrete type, can call directly
		result = c.ProbeOptionsEndpoint(ctx)
	default:
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
		}{false, fmt.Sprintf("Unsupported API style: %s", provider.APIStyle), 0}
	}

	responseTime := time.Since(startTime).Milliseconds()

	if result.Success {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
		}{true, "OPTIONS request successful", responseTime}
	}

	return struct {
		Success      bool
		Message      string
		ResponseTime int64
	}{false, fmt.Sprintf("OPTIONS failed: %s", result.ErrorMessage), responseTime}
}

// probeModelsEndpoint probes the models endpoint for API access validation
func (s *Server) probeModelsEndpoint(ctx context.Context, provider *typ.Provider) struct {
	Success      bool
	Message      string
	ResponseTime int64
	ModelsCount  int
	Warning      string
} {
	startTime := time.Now()

	// Get appropriate client based on API style
	var lister client.ModelLister
	var err error

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		c := s.clientPool.GetOpenAIClient(context.Background(), provider, "")
		if c == nil {
			return struct {
				Success      bool
				Message      string
				ResponseTime int64
				ModelsCount  int
				Warning      string
			}{false, "Failed to create OpenAI client", 0, 0, ""}
		}
		lister = c
	case protocol.APIStyleAnthropic:
		c := s.clientPool.GetAnthropicClient(context.Background(), provider, "")
		if c == nil {
			return struct {
				Success      bool
				Message      string
				ResponseTime int64
				ModelsCount  int
				Warning      string
			}{false, "Failed to create Anthropic client", 0, 0, ""}
		}
		lister = c
	case protocol.APIStyleGoogle:
		c := s.clientPool.GetGoogleClient(context.Background(), provider, "")
		if c == nil {
			return struct {
				Success      bool
				Message      string
				ResponseTime int64
				ModelsCount  int
				Warning      string
			}{false, "Failed to create Google client", 0, 0, ""}
		}
		lister = c
	default:
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
			ModelsCount  int
			Warning      string
		}{false, fmt.Sprintf("Unsupported API style: %s", provider.APIStyle), 0, 0, ""}
	}

	// Run models list probe with timeout
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	models, err := lister.ListModels(probeCtx)
	responseTime := time.Since(startTime).Milliseconds()

	// Check if models endpoint is not supported (e.g., Codex OAuth)
	if client.IsModelsEndpointNotSupported(err) {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
			ModelsCount  int
			Warning      string
		}{
			false,
			"Models endpoint not supported for this provider type",
			responseTime,
			0,
			"This provider does not support the models list endpoint (e.g., OAuth-based providers)",
		}
	}

	if err != nil {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
			ModelsCount  int
			Warning      string
		}{false, fmt.Sprintf("Models endpoint failed: %v", err), responseTime, 0, ""}
	}

	if len(models) == 0 {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
			ModelsCount  int
			Warning      string
		}{false, "Models endpoint returned no models", responseTime, 0, ""}
	}

	return struct {
		Success      bool
		Message      string
		ResponseTime int64
		ModelsCount  int
		Warning      string
	}{
		true,
		fmt.Sprintf("Models endpoint accessible - %d models found", len(models)),
		responseTime,
		len(models),
		"",
	}
}

// probeChatEndpoint probes the Chat Completions endpoint (for OpenAI-style providers)
func (s *Server) probeChatEndpoint(ctx context.Context, provider *typ.Provider) struct {
	Success      bool
	Message      string
	ResponseTime int64
} {
	startTime := time.Now()

	c := s.clientPool.GetOpenAIClient(context.Background(), provider, "")
	if c == nil {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
		}{false, "Failed to create OpenAI client", 0}
	}

	// Create a short timeout for the probe
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use ProbeChatEndpoint with simple options
	result, err := c.ProbeChatEndpoint(probeCtx, "gpt-3.5-turbo", client.ProbeEndpointOptions{
		Message: "Hi",
		Stream:  false,
		Mode:    client.ProbeModeSimple,
	})
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
		}{false, fmt.Sprintf("Chat endpoint failed: %v", err), responseTime}
	}

	if result != nil && result.Content != "" {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
		}{true, "Chat endpoint accessible", responseTime}
	}

	return struct {
		Success      bool
		Message      string
		ResponseTime int64
	}{false, "Chat endpoint returned no content", responseTime}
}

// probeResponsesEndpoint probes the Responses API endpoint (for OpenAI-style providers)
func (s *Server) probeResponsesEndpoint(ctx context.Context, provider *typ.Provider) struct {
	Success      bool
	Message      string
	ResponseTime int64
} {
	startTime := time.Now()

	c := s.clientPool.GetOpenAIClient(context.Background(), provider, "")
	if c == nil {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
		}{false, "Failed to create OpenAI client", 0}
	}

	// Create a short timeout for the probe
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use ProbeResponsesEndpoint with simple options
	result, err := c.ProbeResponsesEndpoint(probeCtx, "gpt-4o", client.ProbeEndpointOptions{
		Message: "Hi",
		Stream:  false,
		Mode:    client.ProbeModeSimple,
	})
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
		}{false, fmt.Sprintf("Responses endpoint failed: %v", err), responseTime}
	}

	if result != nil && result.Content != "" {
		return struct {
			Success      bool
			Message      string
			ResponseTime int64
		}{true, "Responses API endpoint accessible", responseTime}
	}

	return struct {
		Success      bool
		Message      string
		ResponseTime int64
	}{false, "Responses endpoint returned no content", responseTime}
}
