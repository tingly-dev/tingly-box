package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"tingly-box/internal/typ"
)

// PassThroughHandler handles requests in pass-through mode
// In this mode, requests are forwarded as-is without any transformation
type PassThroughHandler struct {
	server *Server
}

// NewPassThroughHandler creates a new pass-through handler
func NewPassThroughHandler(server *Server) *PassThroughHandler {
	return &PassThroughHandler{
		server: server,
	}
}

// HandleRequest processes a request in pass-through mode
func (h *PassThroughHandler) HandleRequest(c *gin.Context, provider *typ.Provider, model string) {
	logrus.WithFields(logrus.Fields{
		"provider":     provider.Name,
		"model":        model,
		"path":         c.Request.URL.Path,
		"method":       c.Request.Method,
		"pass_through": true,
	}).Info("Processing request in pass-through mode")

	// Record that we're using pass-through mode
	h.recordPassThroughUsage(c, provider, model, "pass_through")

	// Read the original request body
	requestBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logrus.WithError(err).Error("Failed to read request body in pass-through mode")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read request body"})
		return
	}

	// Parse and modify the request to replace the model name
	var requestJSON map[string]interface{}
	if err := json.Unmarshal(requestBody, &requestJSON); err != nil {
		logrus.WithError(err).Error("Failed to parse request JSON in pass-through mode")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in request body"})
		return
	}

	// Replace the model name with the actual provider model
	requestJSON["model"] = model

	// Marshal back to JSON
	modifiedRequestBody, err := json.Marshal(requestJSON)
	if err != nil {
		logrus.WithError(err).Error("Failed to marshal modified request in pass-through mode")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
		return
	}

	// Restore the request body for potential middleware use
	c.Request.Body = io.NopCloser(bytes.NewBuffer(modifiedRequestBody))

	// Build the target URL
	targetURL, err := h.buildTargetURL(provider, c.Request.URL.Path, c.Request.URL.RawQuery)
	if err != nil {
		logrus.WithError(err).Error("Failed to build target URL in pass-through mode")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build target URL"})
		return
	}

	// Create the proxy request
	proxyReq, err := http.NewRequestWithContext(
		context.Background(),
		c.Request.Method,
		targetURL,
		bytes.NewBuffer(modifiedRequestBody),
	)
	if err != nil {
		logrus.WithError(err).Error("Failed to create proxy request in pass-through mode")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request"})
		return
	}

	// Copy headers from original request
	h.copyHeaders(c.Request, proxyReq, provider)

	// Set timeout
	timeout := time.Duration(provider.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	proxyReq = proxyReq.WithContext(ctx)

	// Execute the request
	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		logrus.WithError(err).Error("Failed to execute proxy request in pass-through mode")
		h.recordPassThroughUsage(c, provider, model, "error")
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to proxy request"})
		return
	}
	defer resp.Body.Close()

	// Copy response headers first
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Check if this is a streaming response
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		// Handle streaming response
		h.handleStreamingResponse(c, resp, provider, model)
	} else {
		// Handle non-streaming response
		h.handleNonStreamingResponse(c, resp, provider, model)
	}
}

// buildTargetURL constructs the target URL for the provider
func (h *PassThroughHandler) buildTargetURL(provider *typ.Provider, path, query string) (string, error) {
	baseURL := provider.APIBase
	if baseURL == "" {
		return "", fmt.Errorf("provider %s has no API base URL", provider.Name)
	}

	// Parse the base URL
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid API base URL for provider %s: %w", provider.Name, err)
	}

	// For pass-through mode, we need to map tingly-box paths to provider paths
	targetPath := h.mapPathToProvider(path, provider.APIStyle)

	// Construct the full URL
	targetURL := &url.URL{
		Scheme:   parsedBase.Scheme,
		Host:     parsedBase.Host,
		Path:     strings.TrimSuffix(parsedBase.Path, "/") + targetPath,
		RawQuery: query,
	}

	return targetURL.String(), nil
}

// mapPathToProvider maps tingly-box API paths to provider-specific paths
func (h *PassThroughHandler) mapPathToProvider(tinglyPath string, apiStyle typ.APIStyle) string {
	// Remove tingly-box specific prefixes and map to provider paths
	switch {
	case strings.HasPrefix(tinglyPath, "/tingly/openai/v1/"):
		// /tingly/openai/v1/chat/completions -> /v1/chat/completions
		return strings.TrimPrefix(tinglyPath, "/tingly/openai")
	case strings.HasPrefix(tinglyPath, "/tingly/anthropic/v1/"):
		// /tingly/anthropic/v1/messages -> /v1/messages
		return strings.TrimPrefix(tinglyPath, "/tingly/anthropic")
	case strings.HasPrefix(tinglyPath, "/tingly/claude_code/"):
		// /tingly/claude_code/v1/messages -> /v1/messages
		return strings.TrimPrefix(tinglyPath, "/tingly/claude_code")
	case strings.HasPrefix(tinglyPath, "/api/"):
		// Direct API calls - map based on API style
		switch apiStyle {
		case typ.APIStyleOpenAI:
			return strings.TrimPrefix(tinglyPath, "/api")
		case typ.APIStyleAnthropic:
			return strings.TrimPrefix(tinglyPath, "/api")
		default:
			return strings.TrimPrefix(tinglyPath, "/api")
		}
	default:
		// Default: pass through as-is
		return tinglyPath
	}
}

// copyHeaders copies relevant headers from source to destination request
func (h *PassThroughHandler) copyHeaders(src *http.Request, dst *http.Request, provider *typ.Provider) {
	// Copy most headers, but handle authentication specially
	for key, values := range src.Header {
		switch strings.ToLower(key) {
		case "host", "content-length", "connection":
			// Skip these headers
			continue
		case "authorization":
			// Handle authentication based on provider configuration
			if provider.Token != "" {
				// Use provider's token instead of client's
				dst.Header.Set("Authorization", "Bearer "+provider.Token)
			} else {
				// Pass through client's authorization
				for _, value := range values {
					dst.Header.Add(key, value)
				}
			}
		default:
			// Copy other headers as-is
			for _, value := range values {
				dst.Header.Add(key, value)
			}
		}
	}

	// Set provider-specific headers
	if provider.Token != "" && dst.Header.Get("Authorization") == "" {
		dst.Header.Set("Authorization", "Bearer "+provider.Token)
	}

	// Set content type if not present
	if dst.Header.Get("Content-Type") == "" {
		dst.Header.Set("Content-Type", "application/json")
	}
}

// recordPassThroughUsage records usage statistics for pass-through requests
func (h *PassThroughHandler) recordPassThroughUsage(c *gin.Context, provider *typ.Provider, model, status string) {
	// Set context for middleware to track this as pass-through
	c.Set("provider", provider.UUID)
	c.Set("model", model)
	c.Set("pass_through", true)
	c.Set("transformation_mode", "pass_through")

	// Log the pass-through usage
	logrus.WithFields(logrus.Fields{
		"provider":       provider.Name,
		"model":          model,
		"status":         status,
		"transformation": "pass_through",
		"path":           c.Request.URL.Path,
		"method":         c.Request.Method,
	}).Debug("Recorded pass-through usage")
}

// handleStreamingResponse handles streaming SSE responses
func (h *PassThroughHandler) handleStreamingResponse(c *gin.Context, resp *http.Response, provider *typ.Provider, model string) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		logrus.Error("Streaming not supported by this connection")
		h.recordPassThroughUsage(c, provider, model, "error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
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
			logrus.WithError(err).Error("Error reading streaming response")
			break
		}
	}

	h.recordPassThroughUsage(c, provider, model, "success")
}

// handleNonStreamingResponse handles regular JSON responses
func (h *PassThroughHandler) handleNonStreamingResponse(c *gin.Context, resp *http.Response, provider *typ.Provider, model string) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.WithError(err).Error("Failed to read proxy response in pass-through mode")
		h.recordPassThroughUsage(c, provider, model, "error")
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to read proxy response"})
		return
	}

	h.recordPassThroughUsage(c, provider, model, "success")
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), responseBody)
}
