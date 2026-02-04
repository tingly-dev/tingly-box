package server

import (
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

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// PassthroughHandler handles pass-through requests
// It replaces the model name and proxies the request without any transformations
type PassthroughHandler struct {
	server *Server
}

// NewPassthroughHandler creates a new pass-through handler
func NewPassthroughHandler(server *Server) *PassthroughHandler {
	return &PassthroughHandler{
		server: server,
	}
}

// PassthroughOpenAI handles OpenAI-style pass-through requests
// Consolidates: PassthroughOpenAIChatCompletions, PassthroughOpenAIResponsesCreate, PassthroughOpenAIResponsesGet
func (s *Server) PassthroughOpenAI(c *gin.Context) {
	s.handlePassthroughRequest(c, "openai")
}

// PassthroughAnthropic handles Anthropic-style pass-through requests
// Consolidates: PassthroughAnthropicMessages, PassthroughAnthropicCountTokens
func (s *Server) PassthroughAnthropic(c *gin.Context) {
	s.handlePassthroughRequest(c, "anthropic")
}

// handlePassthroughRequest is the common handler for all pass-through requests
func (s *Server) handlePassthroughRequest(c *gin.Context, apiStyle string) {
	// Read the request body
	requestBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logrus.WithError(err).Error("Failed to read request body in pass-through handler")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read request body"})
		return
	}

	// Parse JSON to extract model
	var requestJSON map[string]interface{}
	if err := json.Unmarshal(requestBody, &requestJSON); err != nil {
		logrus.WithError(err).Error("Failed to parse request JSON in pass-through handler")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in request body"})
		return
	}

	// Get the model from request
	requestModel, ok := requestJSON["model"].(string)
	if !ok || requestModel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Model is required"})
		return
	}

	// Determine provider and service via load balancing
	provider, selectedService, err := s.DetermineProviderAndModel(requestModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set context for downstream middleware (stats tracking)
	c.Set("provider", provider.UUID)
	c.Set("model", selectedService.Model)
	c.Set("pass_through", true)

	// Replace the model name with the actual provider model
	requestJSON["model"] = selectedService.Model

	// Marshal back to JSON
	modifiedRequestBody, err := json.Marshal(requestJSON)
	if err != nil {
		logrus.WithError(err).Error("Failed to marshal modified request in pass-through handler")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
		return
	}

	logrus.WithFields(logrus.Fields{
		"provider":     provider.Name,
		"model":        selectedService.Model,
		"path":         c.Request.URL.Path,
		"method":       c.Request.Method,
		"pass_through": true,
	}).Debug("Pass-through handler proxying request")

	// Proxy the request
	if err := s.proxyPassthroughRequest(c, provider, modifiedRequestBody, apiStyle); err != nil {
		logrus.WithError(err).Error("Failed to proxy request in pass-through handler")
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to proxy request"})
	}
}

// proxyPassthroughRequest proxies the request to the provider
func (s *Server) proxyPassthroughRequest(c *gin.Context, provider *typ.Provider, body []byte, apiStyle string) error {
	// Build the target URL
	targetURL, err := s.buildPassthroughTargetURL(provider, c.Request.URL.Path, c.Request.URL.RawQuery, apiStyle)
	if err != nil {
		return fmt.Errorf("failed to build target URL: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"target_url": targetURL,
		"provider":   provider.Name,
		"base_url":   provider.APIBase,
	}).Debug("Passthrough target URL constructed")

	// Determine if this is a streaming request
	isStreaming := s.isPassthroughStreamingRequest(body)

	// Set timeout - use provider's timeout if configured
	timeout := time.Duration(provider.Timeout) * time.Second
	if timeout == 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}

	// For streaming, use 2x timeout to accommodate longer response times
	if isStreaming {
		timeout = timeout * 2
	}

	// Create HTTP client with timeout for passthrough
	// Note: We don't use the pooled client's HTTP client because it may have no timeout
	httpClient := &http.Client{
		Timeout: timeout,
	}

	// Create context with timeout for all requests
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create the proxy request
	proxyReq, err := http.NewRequestWithContext(ctx, c.Request.Method, targetURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	// Copy headers
	s.copyPassthroughHeaders(c.Request, proxyReq, provider)

	// Use the HTTP client from pool for the request
	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		return fmt.Errorf("failed to execute proxy request: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Handle streaming vs non-streaming
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return s.handlePassthroughStreamingResponse(c, resp)
	}

	return s.handlePassthroughNonStreamingResponse(c, resp)
}

// isPassthroughStreamingRequest checks if the request body indicates a streaming request
func (s *Server) isPassthroughStreamingRequest(body []byte) bool {
	var request map[string]interface{}
	if err := json.Unmarshal(body, &request); err != nil {
		return false
	}
	if stream, ok := request["stream"].(bool); ok {
		return stream
	}
	return false
}

// buildPassthroughTargetURL constructs the target URL for the provider
func (s *Server) buildPassthroughTargetURL(provider *typ.Provider, path, query, apiStyle string) (string, error) {
	baseURL := provider.APIBase
	if baseURL == "" {
		return "", fmt.Errorf("provider %s has no API base URL", provider.Name)
	}

	// Map passthrough paths to provider paths
	targetPath := s.mapPassthroughPathToProvider(path, apiStyle)

	baseURL = strings.TrimRight(baseURL, "/")
	targetPath = strings.TrimLeft(targetPath, "/")

	// Avoid duplicate version segments when provider base already includes /v1
	if strings.HasSuffix(baseURL, "/v1") {
		switch {
		case targetPath == "v1":
			targetPath = ""
		case strings.HasPrefix(targetPath, "v1/"):
			targetPath = targetPath[3:]
		}
	} else if apiStyle == "anthropic" && targetPath != "" && !strings.HasPrefix(targetPath, "v1/") {
		// For Anthropic, prepend /v1 if not already present and base URL doesn't include it
		targetPath = "v1/" + targetPath
	}

	finalURL := baseURL
	if targetPath != "" {
		finalURL = finalURL + "/" + targetPath
	}
	if query != "" {
		finalURL = finalURL + "?" + query
	}

	logrus.WithField("final_url", finalURL).Debug("Passthrough final URL")
	return finalURL, nil
}

// mapPassthroughPathToProvider maps passthrough API paths to provider-specific paths
func (s *Server) mapPassthroughPathToProvider(passthroughPath, apiStyle string) string {
	// Remove /passthrough/{apiStyle} prefix
	// e.g., /passthrough/openai/chat/completions -> /chat/completions
	// e.g., /passthrough/anthropic/messages -> /messages

	// For passthrough paths, we need to map them to the provider's expected path
	switch apiStyle {
	case "openai":
		return strings.TrimPrefix(passthroughPath, "/passthrough/openai/")
	case "anthropic":
		return strings.TrimPrefix(passthroughPath, "/passthrough/anthropic/")
	}
	return passthroughPath
}

// copyPassthroughHeaders copies relevant headers from source to destination request
func (s *Server) copyPassthroughHeaders(src *http.Request, dst *http.Request, provider *typ.Provider) {
	for key, values := range src.Header {
		switch key {
		case "Host", "Content-Length", "Connection":
			continue
		case "Authorization":
			if provider.Token != "" {
				dst.Header.Set("Authorization", "Bearer "+provider.Token)
			} else {
				for _, value := range values {
					dst.Header.Add(key, value)
				}
			}
		default:
			for _, value := range values {
				dst.Header.Add(key, value)
			}
		}
	}

	if provider.Token != "" && dst.Header.Get("Authorization") == "" {
		dst.Header.Set("Authorization", "Bearer "+provider.Token)
	}

	if dst.Header.Get("Content-Type") == "" {
		dst.Header.Set("Content-Type", "application/json")
	}
}

// handlePassthroughStreamingResponse handles streaming SSE responses
func (s *Server) handlePassthroughStreamingResponse(c *gin.Context, resp *http.Response) error {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	c.Status(resp.StatusCode)
	buffer := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			c.Writer.Write(buffer[:n])
			flusher.Flush()
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading streaming response: %w", err)
		}
	}
	return nil
}

// handlePassthroughNonStreamingResponse handles regular JSON responses
func (s *Server) handlePassthroughNonStreamingResponse(c *gin.Context, resp *http.Response) error {
	logrus.WithFields(logrus.Fields{
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
	}).Debug("Passthrough non-streaming response received")

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.WithError(err).Error("Failed to read proxy response body")
		return fmt.Errorf("failed to read proxy response: %w", err)
	}

	logrus.WithField("body_length", len(responseBody)).Debug("Passthrough response body read")
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), responseBody)
	logrus.Debug("Passthrough response sent")
	return nil
}
