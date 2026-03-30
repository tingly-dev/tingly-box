package toolruntime

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
	searchTimeout     = 20 * time.Second
)

type builtinSearchHandler struct {
	config *builtinConfig
	cache  *builtinCache
	client *http.Client
}

type builtinSearchResponse struct {
	Tool        string                `json:"tool"`
	Query       string                `json:"query"`
	ResultCount int                   `json:"result_count"`
	Results     []builtinSearchResult `json:"results"`
}

func newBuiltinSearchHandler(config *builtinConfig, cache *builtinCache) *builtinSearchHandler {
	client := &http.Client{Timeout: searchTimeout}
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil {
			logrus.Warnf("Failed to parse proxy URL %s: %v", config.ProxyURL, err)
		} else {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}
	return &builtinSearchHandler{config: config, cache: cache, client: client}
}

func (h *builtinSearchHandler) SearchWithConfig(query string, count int, config *builtinConfig) ([]builtinSearchResult, error) {
	cacheKey := searchCacheKey(query)
	if cached, found := h.cache.Get(cacheKey); found {
		if results, ok := cached.([]builtinSearchResult); ok {
			return results, nil
		}
	}
	if count <= 0 || count > config.MaxResults {
		count = config.MaxResults
	}
	var (
		results []builtinSearchResult
		err     error
	)
	switch strings.ToLower(config.SearchAPI) {
	case "brave":
		results, err = h.searchBrave(query, count, config)
	case "google":
		err = fmt.Errorf("Google Search API not yet implemented")
	case "duckduckgo", "ddg":
		results, err = h.searchDuckDuckGo(query, count)
	default:
		err = fmt.Errorf("unsupported search API: %s (supported: brave, google, duckduckgo)", config.SearchAPI)
	}
	if err != nil {
		return nil, err
	}
	h.cache.Set(cacheKey, results, "search")
	return results, nil
}

type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func (h *builtinSearchHandler) searchBrave(query string, count int, config *builtinConfig) ([]builtinSearchResult, error) {
	if config.SearchKey == "" {
		return nil, fmt.Errorf("search API key is required for Brave Search")
	}
	apiURL, err := url.Parse(braveSearchAPIURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API URL: %w", err)
	}
	params := url.Values{}
	params.Add("q", query)
	params.Add("count", fmt.Sprintf("%d", count))
	apiURL.RawQuery = params.Encode()
	req, err := http.NewRequest(http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Add("X-Subscription-Token", config.SearchKey)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(body))
	}
	var braveResp braveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}
	results := make([]builtinSearchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, builtinSearchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return results, nil
}

type duckDuckGoResponse struct {
	AbstractText   string `json:"AbstractText"`
	AbstractURL    string `json:"AbstractURL"`
	AbstractSource string `json:"AbstractSource"`
	RelatedTopics  []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"RelatedTopics"`
	Results []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"Results"`
}

func (h *builtinSearchHandler) searchDuckDuckGo(query string, count int) ([]builtinSearchResult, error) {
	if count <= 0 || count > 20 {
		count = 10
	}
	apiURL, err := url.Parse(duckDuckGoAPIURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API URL: %w", err)
	}
	params := url.Values{}
	params.Add("q", query)
	params.Add("format", "json")
	apiURL.RawQuery = params.Encode()
	req, err := http.NewRequest(http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TinglyBox/1.0)")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, h.enrichSearchError(fmt.Errorf("search request failed: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search returned status %d: %s", resp.StatusCode, string(body))
	}
	var ddgResp duckDuckGoResponse
	if err := json.NewDecoder(resp.Body).Decode(&ddgResp); err != nil {
		return h.searchDuckDuckGoHTML(query, count)
	}
	results := make([]builtinSearchResult, 0, count)
	if ddgResp.AbstractURL != "" && ddgResp.AbstractText != "" {
		results = append(results, builtinSearchResult{
			Title:   ddgResp.AbstractSource,
			URL:     ddgResp.AbstractURL,
			Snippet: ddgResp.AbstractText,
		})
	}
	for _, topic := range ddgResp.RelatedTopics {
		if topic.FirstURL != "" && topic.Text != "" {
			title := topic.Text
			if idx := strings.Index(title, " - "); idx >= 0 {
				title = title[:idx]
			}
			results = append(results, builtinSearchResult{
				Title:   strings.TrimSpace(title),
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})
			if len(results) >= count {
				break
			}
		}
	}
	if len(results) < count {
		for _, r := range ddgResp.Results {
			if r.FirstURL != "" && r.Text != "" {
				results = append(results, builtinSearchResult{Title: r.Text, URL: r.FirstURL, Snippet: r.Text})
				if len(results) >= count {
					break
				}
			}
		}
	}
	if len(results) == 0 {
		return h.searchDuckDuckGoHTML(query, count)
	}
	return results, nil
}

func (h *builtinSearchHandler) searchDuckDuckGoHTML(query string, count int) ([]builtinSearchResult, error) {
	if count <= 0 || count > 20 {
		count = 10
	}
	apiURL, err := url.Parse(duckDuckGoHTMLURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API URL: %w", err)
	}
	params := url.Values{}
	params.Add("q", query)
	apiURL.RawQuery = params.Encode()
	req, err := http.NewRequest(http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, h.enrichSearchError(fmt.Errorf("search request failed: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}
	return parseDuckDuckGoHTML(resp.Body, count)
}

func parseDuckDuckGoHTML(body io.Reader, maxCount int) ([]builtinSearchResult, error) {
	htmlBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	html := string(htmlBytes)
	results := make([]builtinSearchResult, 0, maxCount)
	lines := strings.Split(html, "\n")
	var currentURL, currentTitle, currentSnippet string
	foundResult := false
	for _, line := range lines {
		if strings.Contains(line, "class=\"result__a\"") {
			if idx := strings.Index(line, "href=\""); idx >= 0 {
				start := idx + 6
				if end := strings.Index(line[start:], "\""); end >= 0 {
					currentURL = line[start : start+end]
				}
			}
			if idx := strings.Index(line, ">"); idx >= 0 {
				start := idx + 1
				if end := strings.Index(line[start:], "<"); end >= 0 {
					currentTitle = decodeHTMLText(strings.TrimSpace(line[start : start+end]))
				}
			}
			foundResult = true
		}
		if foundResult && strings.Contains(line, "result__snippet") {
			if idx := strings.Index(line, ">"); idx >= 0 {
				start := idx + 1
				if end := strings.Index(line[start:], "<"); end >= 0 {
					currentSnippet = decodeHTMLText(strings.TrimSpace(line[start : start+end]))
					currentSnippet = strings.ReplaceAll(currentSnippet, "<b>", "")
					currentSnippet = strings.ReplaceAll(currentSnippet, "</b>", "")
					currentSnippet = strings.ReplaceAll(currentSnippet, "<br/>", " ")
					if len(currentSnippet) > 500 {
						currentSnippet = currentSnippet[:500] + "..."
					}
				}
			}
			if currentURL != "" && currentTitle != "" {
				if strings.HasPrefix(currentURL, "//l.") || strings.HasPrefix(currentURL, "http://l.") {
					if idx := strings.Index(currentURL, "u="); idx >= 0 {
						potentialURL := currentURL[idx+2:]
						if ampIdx := strings.Index(potentialURL, "&"); ampIdx >= 0 {
							currentURL = potentialURL[:ampIdx]
						}
					}
				}
				if !strings.HasPrefix(currentURL, "http://") && !strings.HasPrefix(currentURL, "https://") {
					currentURL = "https://" + currentURL
				}
				results = append(results, builtinSearchResult{Title: currentTitle, URL: currentURL, Snippet: currentSnippet})
				currentURL, currentTitle, currentSnippet = "", "", ""
				foundResult = false
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

func decodeHTMLText(value string) string {
	value = strings.ReplaceAll(value, "&amp;", "&")
	value = strings.ReplaceAll(value, "&quot;", "\"")
	value = strings.ReplaceAll(value, "&lt;", "<")
	value = strings.ReplaceAll(value, "&gt;", ">")
	return value
}

func formatBuiltinSearchResults(query string, results []builtinSearchResult) string {
	payload := builtinSearchResponse{
		Tool:        BuiltinToolSearch,
		Query:       query,
		ResultCount: len(results),
		Results:     results,
	}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("Found %d results. Error formatting: %v", len(results), err)
	}
	return string(jsonBytes)
}

func (h *builtinSearchHandler) enrichSearchError(err error) error {
	errStr := err.Error()
	isNetworkError := strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "Client.Timeout") ||
		strings.Contains(errStr, "i/o timeout")
	if isNetworkError && h.config.ProxyURL == "" {
		return fmt.Errorf("search failed (network error): %w. Consider configuring a proxy in tool_runtime builtin proxy_url", err)
	}
	return err
}
