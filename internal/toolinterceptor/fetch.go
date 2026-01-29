package toolinterceptor

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-shiori/go-readability"
)

const (
	// Default values when config is not set
	defaultMaxFetchSize = 1 * 1024 * 1024 // 1MB
	defaultFetchTimeout = 30 * time.Second
	defaultMaxURLLength = 2000
)

// Private IP ranges to block for SSRF protection
var privateIPBlocks []*net.IPNet

func init() {
	// Initialize private IP blocks on package load
	privateIPBlocks = make([]*net.IPNet, 0)

	// Define private IP ranges
	cidrs := []string{
		"127.0.0.0/8",    // Loopback
		"10.0.0.0/8",     // Private Class A
		"172.16.0.0/12",  // Private Class B
		"192.168.0.0/16", // Private Class C
		"169.254.0.0/16", // Link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 private
	}

	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

// FetchHandler handles web content fetching and extraction
type FetchHandler struct {
	cache  *Cache
	client *http.Client
	config *Config
}

// NewFetchHandler creates a new fetch handler
func NewFetchHandler(cache *Cache) *FetchHandler {
	return &FetchHandler{
		cache: cache,
		client: &http.Client{
			Timeout: defaultFetchTimeout,
		},
		config: &Config{
			MaxFetchSize: int64(defaultMaxFetchSize),
			FetchTimeout: int64(defaultFetchTimeout.Seconds()),
			MaxURLLength: defaultMaxURLLength,
		},
	}
}

// NewFetchHandlerWithConfig creates a new fetch handler with custom configuration
func NewFetchHandlerWithConfig(cache *Cache, config *Config) *FetchHandler {
	timeout := defaultFetchTimeout
	if config.FetchTimeout > 0 {
		timeout = time.Duration(config.FetchTimeout) * time.Second
	}

	return &FetchHandler{
		cache: cache,
		client: &http.Client{
			Timeout: timeout,
		},
		config: config,
	}
}

// FetchAndExtract fetches a URL and extracts the main content
func (h *FetchHandler) FetchAndExtract(targetURL string) (string, error) {
	// Validate URL first
	maxURLLength := h.config.MaxURLLength
	if maxURLLength == 0 {
		maxURLLength = defaultMaxURLLength
	}
	if err := validateURL(targetURL, maxURLLength); err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Check cache first
	cacheKey := FetchCacheKey(targetURL)
	if cached, found := h.cache.Get(cacheKey); found {
		if content, ok := cached.(string); ok {
			return content, nil
		}
	}

	// Parse URL to get hostname for SSRF check
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// SSRF protection: check if hostname resolves to private IP
	if err := h.checkSSRF(parsedURL.Hostname()); err != nil {
		return "", err
	}

	// Fetch the content
	content, err := h.fetchURL(targetURL)
	if err != nil {
		return "", err
	}

	// Cache the result
	h.cache.Set(cacheKey, content, "fetch")

	return content, nil
}

// validateURL validates a URL for security and format
func validateURL(targetURL string, maxURLLength int) error {
	// Check length
	if len(targetURL) > maxURLLength {
		return fmt.Errorf("URL too long (max %d characters)", maxURLLength)
	}

	// Parse URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Check scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme (only http and https allowed)")
	}

	// Check hostname exists
	if parsedURL.Hostname() == "" {
		return fmt.Errorf("missing hostname in URL")
	}

	return nil
}

// checkSSRF checks if a hostname resolves to a private IP address
func (h *FetchHandler) checkSSRF(hostname string) error {
	// Resolve hostname to IP addresses
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		// If resolution fails, we'll still allow the fetch to proceed
		// The HTTP client will handle connection errors
		return nil
	}

	// Check if any resolved IP is in a private range
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		for _, block := range privateIPBlocks {
			if block.Contains(ip) {
				return fmt.Errorf("cannot fetch from private IP address: %s", ip.String())
			}
		}
	}

	return nil
}

// fetchURL fetches and extracts content from a URL
func (h *FetchHandler) fetchURL(targetURL string) (string, error) {
	// Get max fetch size from config
	maxFetchSize := h.config.MaxFetchSize
	if maxFetchSize == 0 {
		maxFetchSize = int64(defaultMaxFetchSize)
	}

	// Create request
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to appear as a normal browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TinglyBox/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml")

	// Execute request
	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch returned status %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return "", fmt.Errorf("unsupported content type: %s", contentType)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxFetchSize)
	html, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check if we hit the size limit
	if len(html) >= int(maxFetchSize) {
		return "", fmt.Errorf("content too large (max %d bytes)", maxFetchSize)
	}

	// Extract main content using readability
	parsedURL, _ := url.Parse(targetURL)
	article, err := readability.FromReader(strings.NewReader(string(html)), parsedURL)
	if err != nil {
		return "", fmt.Errorf("failed to extract content: %w", err)
	}

	// Return extracted content (plain text)
	return article.TextContent, nil
}
