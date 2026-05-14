package server

import (
	"bytes"
	"context"
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

// GenericPassthrough handles generic pass-through requests for any API path.
// It proxies the request to the upstream provider without parsing the body,
// supporting any Content-Type and HTTP method.
func (s *Server) GenericPassthrough(c *gin.Context) {
	// Resolve which provider to forward to
	provider, err := s.resolveGenericProvider(c)
	if err != nil {
		logrus.WithError(err).Warn("Generic passthrough: failed to resolve provider")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Read raw body without parsing
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logrus.WithError(err).Error("Generic passthrough: failed to read request body")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read request body"})
		return
	}

	// Build the target URL from provider base + wildcard path
	targetPath := c.Param("path") // starts with "/"
	targetURL, err := buildGenericTargetURL(provider, targetPath, c.Request.URL.RawQuery)
	if err != nil {
		logrus.WithError(err).Error("Generic passthrough: failed to build target URL")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Set context for downstream middleware
	c.Set("provider", provider.UUID)
	c.Set("pass_through", true)
	c.Set("generic_passthrough", true)

	logrus.WithFields(logrus.Fields{
		"provider":   provider.Name,
		"target_url": targetURL,
		"method":     c.Request.Method,
		"path":       targetPath,
	}).Debug("Generic passthrough proxying request")

	// Proxy the request
	if err := s.proxyGenericRequest(c, provider, targetURL, body); err != nil {
		logrus.WithError(err).Error("Generic passthrough: failed to proxy request")
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to proxy request"})
	}
}

// resolveGenericProvider determines which provider to use for a generic passthrough request.
// It supports multiple resolution strategies:
//   - X-Tingly-Provider header (provider UUID)
//   - ?provider= query parameter (provider UUID)
func (s *Server) resolveGenericProvider(c *gin.Context) (*typ.Provider, error) {
	// Strategy 1: Header
	providerUUID := c.GetHeader("X-Tingly-Provider")

	// Strategy 2: Query param
	if providerUUID == "" {
		providerUUID = c.Query("provider")
	}

	if providerUUID == "" {
		return nil, fmt.Errorf("provider not specified: use X-Tingly-Provider header or ?provider= query param")
	}

	provider, err := s.config.GetProviderByUUID(providerUUID)
	if err != nil {
		return nil, fmt.Errorf("provider '%s' not found", providerUUID)
	}
	if !provider.Enabled {
		return nil, fmt.Errorf("provider '%s' is disabled", providerUUID)
	}

	return provider, nil
}

// buildGenericTargetURL constructs the upstream URL by concatenating the provider's
// API base with the wildcard path. No path rewriting or version deduplication.
func buildGenericTargetURL(provider *typ.Provider, path, query string) (string, error) {
	baseURL := strings.TrimRight(provider.APIBase, "/")
	if baseURL == "" {
		return "", fmt.Errorf("provider %s has no API base URL", provider.Name)
	}

	// path from gin wildcard already starts with "/"
	finalURL := baseURL + path
	if query != "" {
		finalURL += "?" + query
	}
	return finalURL, nil
}

// proxyGenericRequest forwards the request to the upstream provider and streams/copies the response back.
func (s *Server) proxyGenericRequest(c *gin.Context, provider *typ.Provider, targetURL string, body []byte) error {
	// Determine timeout
	timeout := time.Duration(provider.Timeout) * time.Second
	if timeout == 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}

	httpClient := &http.Client{
		Timeout: timeout,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	// Create proxy request preserving the original HTTP method
	proxyReq, err := http.NewRequestWithContext(ctx, c.Request.Method, targetURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	// Copy headers and inject credentials
	s.copyPassthroughHeaders(c.Request, proxyReq, provider)

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

	// Detect streaming by response Content-Type
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return s.handlePassthroughStreamingResponse(c, resp)
	}
	return s.handlePassthroughNonStreamingResponse(c, resp)
}
