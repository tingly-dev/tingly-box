package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	BuiltinWebtoolsSourceID   = "webtools"
	BuiltinWebtoolsSourceName = "Built-in Web Tools"
	BuiltinWebSearchToolName  = "mcp_web_search"
	BuiltinWebFetchToolName   = "mcp_web_fetch"
)

var builtinWebtoolDefaultNames = []string{
	BuiltinWebSearchToolName,
	BuiltinWebFetchToolName,
}

// DefaultBuiltinWebtoolNames returns a copy of default builtin webtools names.
func DefaultBuiltinWebtoolNames() []string {
	out := make([]string, len(builtinWebtoolDefaultNames))
	copy(out, builtinWebtoolDefaultNames)
	return out
}

// BuiltinWebtoolsSource defines the built-in webtools source configuration
var BuiltinWebtoolsSource = map[string]interface{}{
	"id":             BuiltinWebtoolsSourceID,
	"name":           BuiltinWebtoolsSourceName,
	"transport":      "builtin", // Special transport for built-in tools
	"enabled":        true,
	"is_client_tool": true, // Mark as client tool
	"tools":          DefaultBuiltinWebtoolNames(),
	"env": map[string]string{
		"SERPER_API_KEY": "${SERPER_API_KEY}", // User provides via UI
	},
}

// WebSearchTool implements mcp_web_search using Serper API
func WebSearchTool(ctx context.Context, args map[string]interface{}) (string, error) {
	// Validate required parameters
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required")
	}

	// Get API key from environment
	apiKey := os.Getenv("SERPER_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("SERPER_API_KEY is not set")
	}

	// Build search query with domain filters
	finalQuery := query
	if allowedDomains, ok := args["allowed_domains"].([]interface{}); ok && len(allowedDomains) > 0 {
		var siteExprs []string
		for _, d := range allowedDomains {
			if domain, ok := d.(string); ok {
				siteExprs = append(siteExprs, fmt.Sprintf("site:%s", domain))
			}
		}
		if len(siteExprs) > 0 {
			allowExpr := strings.Join(siteExprs, " OR ")
			finalQuery = fmt.Sprintf("%s (%s)", finalQuery, allowExpr)
		}
	}

	if blockedDomains, ok := args["blocked_domains"].([]interface{}); ok {
		for _, d := range blockedDomains {
			if domain, ok := d.(string); ok {
				finalQuery = fmt.Sprintf("%s -site:%s", finalQuery, domain)
			}
		}
	}

	// Build request payload
	payload := map[string]interface{}{
		"q":   finalQuery,
		"num": 5, // Limit to 5 results
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://google.serper.dev/search", bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search API returned status %d", resp.StatusCode)
	}

	// Parse response
	var serperResp struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"organic"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&serperResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Build results
	results := []map[string]interface{}{}
	for _, item := range serperResp.Organic {
		url := item.Link
		if url == "" {
			url = item.URL
		}
		results = append(results, map[string]interface{}{
			"title":   item.Title,
			"url":     url,
			"snippet": item.Snippet,
		})
	}

	// Build structured response
	structured := map[string]interface{}{
		"tool":            BuiltinWebSearchToolName,
		"query":           query,
		"effective_query": finalQuery,
		"result_count":    len(results),
		"results":         results,
	}

	if allowedDomains, ok := args["allowed_domains"].([]interface{}); ok && len(allowedDomains) > 0 {
		var domains []string
		for _, d := range allowedDomains {
			if domain, ok := d.(string); ok {
				domains = append(domains, domain)
			}
		}
		structured["allowed_domains"] = domains
	}

	if blockedDomains, ok := args["blocked_domains"].([]interface{}); ok && len(blockedDomains) > 0 {
		var domains []string
		for _, d := range blockedDomains {
			if domain, ok := d.(string); ok {
				domains = append(domains, domain)
			}
		}
		structured["blocked_domains"] = domains
	}

	// Return as MCP-formatted response
	response := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": func() string {
					b, _ := json.Marshal(structured)
					return string(b)
				}(),
			},
		},
		"structuredContent": structured,
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseBytes), nil
}

// WebFetchTool implements mcp_web_fetch using Jina Reader API
func WebFetchTool(ctx context.Context, args map[string]interface{}) (string, error) {
	// Validate required parameters
	url, ok := args["url"].(string)
	if !ok || strings.TrimSpace(url) == "" {
		return "", fmt.Errorf("url is required")
	}

	prompt, ok := args["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}

	// Validate URL scheme
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("url must use http or https scheme")
	}

	maxChars := 12000

	// Try Jina Reader first
	headers := map[string]string{
		"Content-Type":    "application/json",
		"X-Engine":        "direct",
		"X-Retain-Images": "none",
		"X-Return-Format": "markdown",
		"X-Timeout":       "60",
	}

	// Add optional Jina API key
	if jinaKey := os.Getenv("JINA_API_KEY"); strings.TrimSpace(jinaKey) != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", jinaKey)
	}

	// Build request payload
	payload := map[string]string{"url": url}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://r.jina.ai/", bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Execute request
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	var content string
	var source string
	var truncated bool

	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}
		content = string(body)
		source = "jina"
	} else {
		// Fallback to direct fetch
		if resp != nil {
			_ = resp.Body.Close()
		}
		content, err = fetchDirect(ctx, url)
		if err != nil {
			return "", fmt.Errorf("both jina and direct fetch failed: %w", err)
		}
		source = "direct"
	}

	// Truncate if necessary
	if len(content) > maxChars {
		content = content[:maxChars]
		truncated = true
	}

	// Extract relevant snippets based on prompt
	snippets := extractSnippets(content, prompt)

	// Build structured response
	structured := map[string]interface{}{
		"tool":      BuiltinWebFetchToolName,
		"url":       url,
		"source":    source,
		"prompt":    prompt,
		"truncated": truncated,
		"content":   content,
	}

	if len(snippets) > 0 {
		structured["snippets"] = snippets
	}

	// Return as MCP-formatted response
	response := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": func() string {
					b, _ := json.Marshal(structured)
					return string(b)
				}(),
			},
		},
		"structuredContent": structured,
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseBytes), nil
}

const maxDirectFetchBodySize = 10 * 1024 * 1024 // 10 MB

// fetchDirect performs a direct HTTP GET request as fallback.
// It blocks private/internal addresses to mitigate SSRF.
func fetchDirect(ctx context.Context, targetURL string) (string, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	if isPrivateOrInternalURL(u) {
		return "", fmt.Errorf("fetching private or internal urls is not allowed")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "tingly-box-mcp-web-tools/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("direct fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDirectFetchBodySize))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}

// isPrivateOrInternalURL reports whether u resolves to a private, loopback,
// link-local, or multicast address.
func isPrivateOrInternalURL(u *url.URL) bool {
	host := u.Hostname()

	// Block localhost by name.
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS lookup fails, conservatively block.
		return true
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return true
		}
	}

	return false
}

// extractSnippets extracts relevant lines from content based on prompt keywords
func extractSnippets(content, prompt string) []string {
	words := strings.Fields(prompt)
	var keywords []string
	for _, w := range words {
		w = strings.ToLower(strings.Trim(w, ".,;:!?()[]{}\"'"))
		if len(w) >= 2 {
			keywords = append(keywords, w)
		}
	}

	if len(keywords) == 0 {
		return nil
	}

	var snippets []string
	maxSnippets := 8

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lineLower := strings.ToLower(line)
		for _, keyword := range keywords {
			if strings.Contains(lineLower, keyword) {
				snippets = append(snippets, line)
				if len(snippets) >= maxSnippets {
					return snippets
				}
				break
			}
		}
	}

	return snippets
}
