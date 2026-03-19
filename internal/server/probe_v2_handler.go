package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// HandleProbeV2 handles Probe V3 requests (unified endpoint for all test types)
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
		s.handleProbeV2Simple(c, &req)
	case ProbeV2ModeStreaming, ProbeV2ModeTool:
		s.handleProbeV2Streaming(c, &req)
	}
}

// handleProbeV2Simple handles simple (non-streaming) probe requests
func (s *Server) handleProbeV2Simple(c *gin.Context, req *ProbeV2Request) {
	ctx := c.Request.Context()
	startTime := time.Now()

	var data *ProbeV2Data
	var err error

	switch req.TargetType {
	case ProbeV2TargetRule:
		// Use original HTTP approach for rule-based probes
		data, err = s.probeV2RuleHTTP(ctx, req)
	case ProbeV2TargetProvider:
		// Use SDK for provider-based probes
		data, err = s.probeV2ProviderSDK(ctx, req)
	}

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

// handleProbeV2Streaming handles streaming probe requests
func (s *Server) handleProbeV2Streaming(c *gin.Context, req *ProbeV2Request) {
	ctx := c.Request.Context()
	startTime := time.Now()

	var data *ProbeV2Data
	var err error

	switch req.TargetType {
	case ProbeV2TargetRule:
		// Use original HTTP approach for rule-based probes (with chunk collection)
		data, err = s.probeV2RuleHTTPStreaming(ctx, req)
	case ProbeV2TargetProvider:
		// Use SDK for provider-based probes (with chunk collection)
		data, err = s.probeV2ProviderSDKStreaming(ctx, req)
	}

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

// probeV2ProviderSDK performs a provider-based probe using SDK (non-streaming)
func (s *Server) probeV2ProviderSDK(ctx context.Context, req *ProbeV2Request) (*ProbeV2Data, error) {
	provider, err := s.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		return nil, fmt.Errorf("provider not found: %s", req.ProviderUUID)
	}

	if !provider.Enabled {
		return nil, fmt.Errorf("provider is disabled: %s", req.ProviderUUID)
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

	// Get message
	message := getProbeMessage(req.TestMode, req.Message)

	return s.probeProviderWithSDK(ctx, provider, model, message, req.TestMode)
}

// probeV2ProviderSDKStreaming performs a streaming provider probe using SDK
func (s *Server) probeV2ProviderSDKStreaming(ctx context.Context, req *ProbeV2Request) (*ProbeV2Data, error) {
	provider, err := s.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		return nil, fmt.Errorf("provider not found: %s", req.ProviderUUID)
	}

	if !provider.Enabled {
		return nil, fmt.Errorf("provider is disabled: %s", req.ProviderUUID)
	}

	// Get model to use
	model := req.Model
	if model == "" {
		if len(provider.Models) > 0 {
			model = provider.Models[0]
		} else {
			if provider.APIStyle == protocol.APIStyleAnthropic {
				model = "claude-3-haiku-20240307"
			} else {
				model = "gpt-3.5-turbo"
			}
		}
	}

	// Get message
	message := getProbeMessage(req.TestMode, req.Message)

	return s.probeProviderWithSDKStreaming(ctx, provider, model, message, req.TestMode)
}

// probeV2RuleHTTP performs a rule-based probe using HTTP (original approach)
func (s *Server) probeV2RuleHTTP(ctx context.Context, req *ProbeV2Request) (*ProbeV2Data, error) {
	url, requestBody, apiStyle, err := s.buildProbeV2RuleRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.config.GetInternalAPIToken())

	// Execute request
	startTime := time.Now()
	httpClient := &http.Client{Timeout: 60 * time.Second}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	latencyMs := time.Since(startTime).Milliseconds()

	// Check for error status
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(body))
	}

	// Parse response
	return s.parseProbeV2Response(httpResp.Body, apiStyle, latencyMs, url)
}

// probeV2RuleHTTPStreaming performs a streaming rule-based probe using HTTP (with chunk collection)
func (s *Server) probeV2RuleHTTPStreaming(ctx context.Context, req *ProbeV2Request) (*ProbeV2Data, error) {
	url, requestBody, apiStyle, err := s.buildProbeV2RuleRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	// Create HTTP request with streaming enabled
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.config.GetInternalAPIToken())

	// Execute request
	startTime := time.Now()
	httpClient := &http.Client{Timeout: 120 * time.Second}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	// Check for error status
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(body))
	}

	// Collect streaming response
	return s.collectStreamingResponse(httpResp.Body, apiStyle, time.Since(startTime).Milliseconds(), url)
}

// buildProbeV2RuleRequest builds request for rule-based probe
func (s *Server) buildProbeV2RuleRequest(ctx context.Context, req *ProbeV2Request) (string, []byte, protocol.APIStyle, error) {
	// Get rule config
	rule := s.config.GetRuleByUUID(req.RuleUUID)
	if rule == nil {
		return "", nil, "", fmt.Errorf("rule not found: %s", req.RuleUUID)
	}

	// Determine endpoint based on scenario
	endpoint, apiStyle := getScenarioEndpoint(req.Scenario)

	// Build URL
	baseURL := fmt.Sprintf("http://localhost:%d", s.config.GetServerPort())
	url := baseURL + endpoint

	// Build request body based on API style
	message := getProbeMessage(req.TestMode, req.Message)

	var requestBody []byte
	var err error

	switch apiStyle {
	case protocol.APIStyleAnthropic:
		requestBody, err = s.buildAnthropicRequestBody(rule.RequestModel, message, req.TestMode)
	default:
		requestBody, err = s.buildOpenAIRequestBody(rule.RequestModel, message, req.TestMode)
	}

	if err != nil {
		return "", nil, "", err
	}

	return url, requestBody, apiStyle, nil
}

// buildAnthropicRequestBody builds Anthropic-style request body
func (s *Server) buildAnthropicRequestBody(model, message string, testMode ProbeV2TestMode) ([]byte, error) {
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": message,
			},
		},
	}

	// Add tools for tool mode
	if testMode == ProbeV2ModeTool {
		tools := GetProbeToolsAnthropic()
		toolsJSON, err := json.Marshal(tools)
		if err != nil {
			return nil, err
		}

		var toolsArray []interface{}
		if err := json.Unmarshal(toolsJSON, &toolsArray); err != nil {
			return nil, err
		}
		body["tools"] = toolsArray
		body["tool_choice"] = map[string]interface{}{"type": "auto"}
	}

	return json.Marshal(body)
}

// buildOpenAIRequestBody builds OpenAI-style request body
func (s *Server) buildOpenAIRequestBody(model, message string, testMode ProbeV2TestMode) ([]byte, error) {
	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": message,
			},
		},
		"stream": false,
	}

	// Add tools for tool mode
	if testMode == ProbeV2ModeTool {
		tools := GetProbeToolsOpenAI()
		toolsJSON, err := json.Marshal(tools)
		if err != nil {
			return nil, err
		}

		var toolsArray []interface{}
		if err := json.Unmarshal(toolsJSON, &toolsArray); err != nil {
			return nil, err
		}
		body["tools"] = toolsArray
		body["tool_choice"] = map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "add_numbers"}}
	}

	return json.Marshal(body)
}

// parseProbeV2Response parses the probe response
func (s *Server) parseProbeV2Response(body io.Reader, apiStyle protocol.APIStyle, latencyMs int64, url string) (*ProbeV2Data, error) {
	data := &ProbeV2Data{
		LatencyMs:  latencyMs,
		RequestURL: url,
	}

	responseBody, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	switch apiStyle {
	case protocol.APIStyleAnthropic:
		return s.parseAnthropicProbeV2Response(responseBody, data)
	default:
		return s.parseOpenAIProbeV2Response(responseBody, data)
	}
}

// collectStreamingResponse collects a streaming response and returns as complete data
func (s *Server) collectStreamingResponse(body io.Reader, apiStyle protocol.APIStyle, latencyMs int64, url string) (*ProbeV2Data, error) {
	data := &ProbeV2Data{
		LatencyMs:  latencyMs,
		RequestURL: url,
	}

	reader := bufio.NewReader(body)
	var content strings.Builder
	var toolCalls []ProbeV2ToolCall
	var usage *ProbeV2Usage

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse SSE data
		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			if dataStr == "[DONE]" {
				break
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil {
				// Extract content
				extractedContent := s.extractProbeV2ContentFromChunk(chunk, apiStyle)
				if extractedContent != "" {
					content.WriteString(extractedContent)
				}

				// Extract usage
				if apiStyle == protocol.APIStyleAnthropic {
					if u, ok := chunk["usage"].(map[string]interface{}); ok {
						usage = &ProbeV2Usage{}
						if inputTokens, ok := u["input_tokens"].(float64); ok {
							usage.PromptTokens = int(inputTokens)
						}
						if outputTokens, ok := u["output_tokens"].(float64); ok {
							usage.CompletionTokens = int(outputTokens)
						}
						usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
					}
				} else {
					// OpenAI
					if u, ok := chunk["usage"].(map[string]interface{}); ok {
						usage = &ProbeV2Usage{}
						if promptTokens, ok := u["prompt_tokens"].(float64); ok {
							usage.PromptTokens = int(promptTokens)
						}
						if completionTokens, ok := u["completion_tokens"].(float64); ok {
							usage.CompletionTokens = int(completionTokens)
						}
						if totalTokens, ok := u["total_tokens"].(float64); ok {
							usage.TotalTokens = int(totalTokens)
						}
					}
				}
			}
		}
	}

	data.Content = content.String()
	data.ToolCalls = toolCalls
	data.Usage = usage

	return data, nil
}

// extractProbeV2ContentFromChunk extracts content from a streaming chunk
func (s *Server) extractProbeV2ContentFromChunk(chunk map[string]interface{}, apiStyle protocol.APIStyle) string {
	if apiStyle == protocol.APIStyleAnthropic {
		if content, ok := chunk["content"].([]interface{}); ok && len(content) > 0 {
			if item, ok := content[0].(map[string]interface{}); ok {
				if text, ok := item["text"].(string); ok {
					return text
				}
			}
		}
	} else {
		// OpenAI format
		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if content, ok := delta["content"].(string); ok {
						return content
					}
				}
			}
		}
	}
	return ""
}

// parseAnthropicProbeV2Response parses Anthropic-style response
func (s *Server) parseAnthropicProbeV2Response(body []byte, data *ProbeV2Data) (*ProbeV2Data, error) {
	var resp struct {
		Content []struct {
			Type  string                 `json:"type"`
			Text  string                 `json:"text"`
			ID    string                 `json:"id"`
			Name  string                 `json:"name"`
			Input map[string]interface{} `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	for _, content := range resp.Content {
		switch content.Type {
		case "text":
			data.Content += content.Text
		case "tool_use":
			data.ToolCalls = append(data.ToolCalls, ProbeV2ToolCall{
				ID:        content.ID,
				Name:      content.Name,
				Arguments: content.Input,
			})
		}
	}

	data.Usage = &ProbeV2Usage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}

	return data, nil
}

// parseOpenAIProbeV2Response parses OpenAI-style response
func (s *Server) parseOpenAIProbeV2Response(body []byte, data *ProbeV2Data) (*ProbeV2Data, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls,omitempty"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	if len(resp.Choices) > 0 {
		data.Content = resp.Choices[0].Message.Content

		for _, tc := range resp.Choices[0].Message.ToolCalls {
			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Function.Arguments), &args)

			data.ToolCalls = append(data.ToolCalls, ProbeV2ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
	}

	data.Usage = &ProbeV2Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}

	return data, nil
}

// Log initialization
func init() {
	logrus.Debug("Probe V3 handler initialized")
}
