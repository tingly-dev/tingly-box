package toolinterceptor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	braveSearchAPIURL = "https://api.search.brave.com/res/v1/web/search"
	duckDuckGoAPIURL  = "https://api.duckduckgo.com/"
	duckDuckGoHTMLURL = "https://html.duckduckgo.com/html/"
	braveTimeout      = 20 * time.Second
	ddgTimeout        = 20 * time.Second
)

// SearchHandler handles web search requests
type SearchHandler struct {
	config *Config
	cache  *Cache
	client *http.Client
}

// NewSearchHandler creates a new search handler
func NewSearchHandler(config *Config, cache *Cache) *SearchHandler {
	client := &http.Client{
		Timeout: braveTimeout,
	}

	// Configure proxy if specified
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil {
			logrus.Warnf("Failed to parse proxy URL %s: %v", config.ProxyURL, err)
		} else {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		}
	}

	return &SearchHandler{
		config: config,
		cache:  cache,
		client: client,
	}
}

// BraveSearchResponse represents the Brave Search API response
type BraveSearchResponse struct {
	Query struct {
		Original string `json:"original"`
	} `json:"query"`
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// Search executes a web search query using the handler's configured API
func (h *SearchHandler) Search(query string, count int) ([]SearchResult, error) {
	return h.SearchWithConfig(query, count, h.config)
}

// SearchWithConfig executes a web search query using a specific config
// This allows provider-specific overrides at runtime
func (h *SearchHandler) SearchWithConfig(query string, count int, config *Config) ([]SearchResult, error) {
	// Check cache first
	cacheKey := SearchCacheKey(query)
	if cached, found := h.cache.Get(cacheKey); found {
		if results, ok := cached.([]SearchResult); ok {
			return results, nil
		}
	}

	// Apply count limits
	if count <= 0 || count > config.MaxResults {
		count = config.MaxResults
	}

	// Execute search based on configured API
	var results []SearchResult
	var err error

	switch strings.ToLower(config.SearchAPI) {
	case "brave":
		results, err = h.searchBraveWithConfig(query, count, config)
	case "google":
		results, err = h.searchGoogleWithConfig(query, count, config)
	case "duckduckgo", "ddg":
		results, err = h.searchDuckDuckGo(query, count)
	default:
		err = fmt.Errorf("unsupported search API: %s (supported: brave, google, duckduckgo)", config.SearchAPI)
	}

	if err != nil {
		return nil, err
	}

	// Cache the results
	h.cache.Set(cacheKey, results, "search")

	return results, nil
}

// searchBrave executes a search using Brave Search API
func (h *SearchHandler) searchBrave(query string, count int) ([]SearchResult, error) {
	return h.searchBraveWithConfig(query, count, h.config)
}

// searchBraveWithConfig executes a search using Brave Search API with a specific config
func (h *SearchHandler) searchBraveWithConfig(query string, count int, config *Config) ([]SearchResult, error) {
	if config.SearchKey == "" {
		return nil, fmt.Errorf("search API key is required for Brave Search")
	}

	// Build request URL
	apiURL, err := url.Parse(braveSearchAPIURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API URL: %w", err)
	}

	params := url.Values{}
	params.Add("q", query)
	params.Add("count", fmt.Sprintf("%d", count))
	apiURL.RawQuery = params.Encode()

	// Create request
	req, err := http.NewRequest("GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add API key header
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Add("X-Subscription-Token", config.SearchKey)

	// Execute request
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var braveResp BraveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Convert to search results
	results := make([]SearchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}

	return results, nil
}

// searchGoogle executes a search using Google Custom Search API
// Note: This requires a Google Custom Search API key and Search Engine ID
func (h *SearchHandler) searchGoogle(query string, count int) ([]SearchResult, error) {
	return h.searchGoogleWithConfig(query, count, h.config)
}

// searchGoogleWithConfig executes a search using Google Custom Search API with a specific config
// Note: This requires a Google Custom Search API key and Search Engine ID
func (h *SearchHandler) searchGoogleWithConfig(query string, count int, config *Config) ([]SearchResult, error) {
	// Google Custom Search API implementation
	// This is a placeholder - actual implementation would require:
	// - API key: https://developers.google.com/custom-search/v1/overview
	// - Search Engine ID (cx): created via Google Custom Search control panel

	return nil, fmt.Errorf("Google Search API not yet implemented")
}

// DuckDuckGoInstantAnswerAPIResponse represents the DuckDuckGo Instant Answer API response
type DuckDuckGoInstantAnswerAPIResponse struct {
	Abstract       string `json:"Abstract"`
	AbstractText   string `json:"AbstractText"`
	AbstractSource string `json:"AbstractSource"`
	AbstractURL    string `json:"AbstractURL"`
	Image          string `json:"Image"`
	Heading        string `json:"Heading"`
	Answer         string `json:"Answer"`
	AnswerType     string `json:"AnswerType"`
	Definition     string `json:"Definition"`
	DefinitionURL  string `json:"DefinitionURL"`
	RelatedTopics  []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"RelatedTopics"`
	Results []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"Results"`
	// Infobox can be either an object or empty string, so we use interface{}
	Infobox interface{} `json:"Infobox"`
}

// searchDuckDuckGo executes a search using DuckDuckGo Instant Answer API (no API key required)
func (h *SearchHandler) searchDuckDuckGo(query string, count int) ([]SearchResult, error) {
	// Apply count limits
	if count <= 0 || count > 20 {
		count = 10
	}

	// Build request URL
	apiURL, err := url.Parse(duckDuckGoAPIURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API URL: %w", err)
	}

	params := url.Values{}
	params.Add("q", query)
	params.Add("format", "json")
	apiURL.RawQuery = params.Encode()

	// Create request
	req, err := http.NewRequest("GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TinglyBox/1.0)")

	// Execute request using the handler's client (which may have proxy configured)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, h.enrichSearchError(fmt.Errorf("search request failed: %w", err))
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var ddgResp DuckDuckGoInstantAnswerAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&ddgResp); err != nil {
		// JSON parsing failed - DuckDuckGo might have changed format or returned error
		// Fall back to HTML parsing
		logrus.Debugf("DuckDuckGo JSON parsing failed: %v, falling back to HTML", err)
		return h.searchDuckDuckGoHTML(query, count)
	}

	// Convert to search results
	results := make([]SearchResult, 0, count)

	// Add main abstract/result if available
	if ddgResp.AbstractURL != "" && ddgResp.AbstractText != "" {
		results = append(results, SearchResult{
			Title:   ddgResp.AbstractSource,
			URL:     ddgResp.AbstractURL,
			Snippet: ddgResp.AbstractText,
		})
	}

	// Add RelatedTopics as results (these are usually external links)
	for _, topic := range ddgResp.RelatedTopics {
		if topic.FirstURL != "" && topic.Text != "" {
			// Extract title from topic text
			title := topic.Text
			if idx := strings.Index(title, " - "); idx >= 0 {
				title = title[:idx]
			}

			results = append(results, SearchResult{
				Title:   strings.TrimSpace(title),
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})

			if len(results) >= count {
				break
			}
		}
	}

	// Add Results (external search results) if we still need more
	if len(results) < count {
		for _, r := range ddgResp.Results {
			if r.FirstURL != "" && r.Text != "" {
				results = append(results, SearchResult{
					Title:   r.Text,
					URL:     r.FirstURL,
					Snippet: r.Text,
				})

				if len(results) >= count {
					break
				}
			}
		}
	}

	// If no results found, try HTML fallback
	if len(results) == 0 {
		return h.searchDuckDuckGoHTML(query, count)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no search results found from DuckDuckGo")
	}

	return results, nil
}

// searchDuckDuckGoHTML executes a search using DuckDuckGo HTML (fallback)
func (h *SearchHandler) searchDuckDuckGoHTML(query string, count int) ([]SearchResult, error) {
	// Apply count limits (DDG typically returns good results)
	if count <= 0 || count > 20 {
		count = 10
	}

	// Build request URL
	apiURL, err := url.Parse(duckDuckGoHTMLURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API URL: %w", err)
	}

	params := url.Values{}
	params.Add("q", query)
	apiURL.RawQuery = params.Encode()

	// Create request
	req, err := http.NewRequest("GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to appear as a normal browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	// Execute request using the handler's client (which may have proxy configured)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, h.enrichSearchError(fmt.Errorf("search request failed: %w", err))
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	// Parse HTML response
	return h.parseDuckDuckGoHTML(resp.Body, count)
}

// parseDuckDuckGoHTML parses DuckDuckGo HTML response to extract search results
func (h *SearchHandler) parseDuckDuckGoHTML(body io.Reader, maxCount int) ([]SearchResult, error) {
	// Read the HTML content
	htmlBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	html := string(htmlBytes)
	results := make([]SearchResult, 0, maxCount)

	// DuckDuckGo HTML structure: results are in <a> tags with class="result__a"
	// The structure is roughly:
	// <a class="result__a" href="https://example.com">Example Title</a>
	// <a class="result__snippet" href="https://example.com">Snippet text...</a>

	// Simple regex-based parsing (for production, consider using a proper HTML parser)
	lines := strings.Split(html, "\n")
	var currentURL, currentTitle, currentSnippet string
	foundResult := false

	for _, line := range lines {
		// Look for result link
		if strings.Contains(line, "class=\"result__a\"") {
			// Extract URL and title
			if idx := strings.Index(line, "href=\""); idx >= 0 {
				start := idx + 6
				if end := strings.Index(line[start:], "\""); end >= 0 {
					currentURL = line[start : start+end]
					// Sometimes URLs are prefixed with "//l.facebook.com/l.php?u=" etc.
					// For now, use as-is (would need proper URL parsing for production)
				}
			}
			// Extract title (text between > and <)
			if idx := strings.Index(line, ">"); idx >= 0 {
				start := idx + 1
				if end := strings.Index(line[start:], "<"); end >= 0 {
					currentTitle = strings.TrimSpace(line[start : start+end])
					// Decode HTML entities
					currentTitle = strings.ReplaceAll(currentTitle, "&amp;", "&")
					currentTitle = strings.ReplaceAll(currentTitle, "&quot;", "\"")
					currentTitle = strings.ReplaceAll(currentTitle, "&lt;", "<")
					currentTitle = strings.ReplaceAll(currentTitle, "&gt;", ">")
				}
			}
			foundResult = true
		}

		// Look for snippet (next line after the result link)
		if foundResult && strings.Contains(line, "result__snippet") {
			// Extract snippet text
			if idx := strings.Index(line, ">"); idx >= 0 {
				start := idx + 1
				if end := strings.Index(line[start:], "<"); end >= 0 {
					currentSnippet = strings.TrimSpace(line[start : start+end])
					// Decode HTML entities
					currentSnippet = strings.ReplaceAll(currentSnippet, "&amp;", "&")
					currentSnippet = strings.ReplaceAll(currentSnippet, "&quot;", "\"")
					currentSnippet = strings.ReplaceAll(currentSnippet, "&lt;", "<")
					currentSnippet = strings.ReplaceAll(currentSnippet, "&gt;", ">")
					currentSnippet = strings.ReplaceAll(currentSnippet, "<b>", "")
					currentSnippet = strings.ReplaceAll(currentSnippet, "</b>", "")
					currentSnippet = strings.ReplaceAll(currentSnippet, "<br/>", " ")
					// Limit snippet length
					if len(currentSnippet) > 500 {
						currentSnippet = currentSnippet[:500] + "..."
					}
				}
			}

			// Add result if we have all components
			if currentURL != "" && currentTitle != "" {
				// Clean up DuckDuckGo redirect URLs if present
				if strings.HasPrefix(currentURL, "//l.") || strings.HasPrefix(currentURL, "http://l.") {
					// These are redirect/tracking URLs, try to extract real URL
					if idx := strings.Index(currentURL, "u="); idx >= 0 {
						potentialURL := currentURL[idx+2:]
						if ampIdx := strings.Index(potentialURL, "&"); ampIdx >= 0 {
							currentURL = potentialURL[:ampIdx]
						}
					}
				}

				// Ensure URL has scheme
				if !strings.HasPrefix(currentURL, "http://") && !strings.HasPrefix(currentURL, "https://") {
					currentURL = "https://" + currentURL
				}

				results = append(results, SearchResult{
					Title:   currentTitle,
					URL:     currentURL,
					Snippet: currentSnippet,
				})

				// Reset for next result
				currentURL = ""
				currentTitle = ""
				currentSnippet = ""
				foundResult = false

				// Check if we have enough results
				if len(results) >= maxCount {
					break
				}
			}
		}
	}

	// If no results found with structured parsing, try a simpler approach
	if len(results) == 0 {
		// Look for any links that might be search results
		for _, line := range lines {
			if strings.Contains(line, "<a ") && strings.Contains(line, "href=") {
				// Very basic fallback - just look for URLs that might be results
				if idx := strings.Index(line, "href=\""); idx >= 0 {
					start := idx + 6
					if end := strings.Index(line[start:], "\""); end >= 0 {
						url := line[start : start+end]
						if strings.HasPrefix(url, "http") && !strings.Contains(url, "duckduckgo") && len(results) < maxCount {
							// Try to find title
							title := url
							if titleIdx := strings.Index(line, ">"); titleIdx >= 0 {
								titleStart := titleIdx + 1
								if titleEnd := strings.Index(line[titleStart:], "<"); titleEnd >= 0 {
									title = strings.TrimSpace(line[titleStart : titleStart+titleEnd])
								}
							}

							results = append(results, SearchResult{
								Title:   title,
								URL:     url,
								Snippet: "Search result from DuckDuckGo",
							})
						}
					}
				}
				if len(results) >= maxCount {
					break
				}
			}
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no search results found (DuckDuckGo may have blocked the request)")
	}

	return results, nil
}

// FormatResults formats search results as a JSON string for tool response
func FormatSearchResults(results []SearchResult) string {
	if len(results) == 0 {
		return `{"results": []}`
	}

	// Format as JSON
	jsonBytes, err := json.Marshal(map[string]interface{}{
		"results": results,
	})
	if err != nil {
		// Fallback to simple string format
		return fmt.Sprintf("Found %d results. Error formatting: %v", len(results), err)
	}

	return string(jsonBytes)
}

// enrichSearchError enriches search errors with helpful suggestions
func (h *SearchHandler) enrichSearchError(err error) error {
	// Check if this is a network error that might be solved with a proxy
	errStr := err.Error()

	// Common network error patterns
	isNetworkError := strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "Client.Timeout") ||
		strings.Contains(errStr, "i/o timeout")

	// Only suggest proxy if it's a network error and no proxy is configured
	if isNetworkError && h.config.ProxyURL == "" {
		logrus.Warnf("Search failed with network error: %v", err)
		logrus.Warn("DuckDuckGo search may be blocked or unavailable in your region")
		logrus.Warn("Consider configuring a proxy in tool_interceptor config:")
		logrus.Warn(`  "tool_interceptor": {
    "enabled": true,
    "search_api": "duckduckgo",
    "proxy_url": "http://127.0.0.1:7897",
    ...
  }`)
		return fmt.Errorf("search failed (network error): %w. Consider configuring a proxy in tool_interceptor.proxy_url", err)
	}

	return err
}
