package toolinterceptor

import (
	"strings"
	"testing"
)

func TestMatchToolAlias(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		expectMatch bool
		expectType  HandlerType
	}{
		{
			name:        "web_search alias",
			toolName:    "web_search",
			expectMatch: true,
			expectType:  HandlerTypeSearch,
		},
		{
			name:        "Google Search alias",
			toolName:    "Google Search",
			expectMatch: true,
			expectType:  HandlerTypeSearch,
		},
		{
			name:        "search alias",
			toolName:    "search",
			expectMatch: true,
			expectType:  HandlerTypeSearch,
		},
		{
			name:        "bing_search alias",
			toolName:    "bing_search",
			expectMatch: true,
			expectType:  HandlerTypeSearch,
		},
		{
			name:        "web_fetch alias",
			toolName:    "web_fetch",
			expectMatch: true,
			expectType:  HandlerTypeFetch,
		},
		{
			name:        "browse alias",
			toolName:    "browse",
			expectMatch: true,
			expectType:  HandlerTypeFetch,
		},
		{
			name:        "read_url alias",
			toolName:    "read_url",
			expectMatch: true,
			expectType:  HandlerTypeFetch,
		},
		{
			name:        "get_page_content alias",
			toolName:    "get_page_content",
			expectMatch: true,
			expectType:  HandlerTypeFetch,
		},
		{
			name:        "unknown tool",
			toolName:    "unknown_tool",
			expectMatch: false,
			expectType:  HandlerTypeNone,
		},
		{
			name:        "empty tool name",
			toolName:    "",
			expectMatch: false,
			expectType:  HandlerTypeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerType, matched := MatchToolAlias(tt.toolName)
			if matched != tt.expectMatch {
				t.Errorf("MatchToolAlias(%q) matched = %v, want %v", tt.toolName, matched, tt.expectMatch)
			}
			if matched && handlerType != tt.expectType {
				t.Errorf("MatchToolAlias(%q) handlerType = %v, want %v", tt.toolName, handlerType, tt.expectType)
			}
		})
	}
}

func TestIsSearchTool(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		expected bool
	}{
		{"web_search", "web_search", true},
		{"Google Search", "Google Search", true},
		{"search", "search", true},
		{"bing_search", "bing_search", true},
		{"web_fetch", "web_fetch", false},
		{"unknown", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSearchTool(tt.toolName)
			if result != tt.expected {
				t.Errorf("IsSearchTool(%q) = %v, want %v", tt.toolName, result, tt.expected)
			}
		})
	}
}

func TestIsFetchTool(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		expected bool
	}{
		{"web_fetch", "web_fetch", true},
		{"browse", "browse", true},
		{"read_url", "read_url", true},
		{"get_page_content", "get_page_content", true},
		{"web_search", "web_search", false},
		{"unknown", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsFetchTool(tt.toolName)
			if result != tt.expected {
				t.Errorf("IsFetchTool(%q) = %v, want %v", tt.toolName, result, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.SearchAPI != "duckduckgo" {
		t.Errorf("DefaultConfig().SearchAPI = %q, want \"duckduckgo\"", config.SearchAPI)
	}
	if config.MaxResults != 10 {
		t.Errorf("DefaultConfig().MaxResults = %d, want 10", config.MaxResults)
	}
	if config.MaxFetchSize != 1*1024*1024 {
		t.Errorf("DefaultConfig().MaxFetchSize = %d, want 1MB", config.MaxFetchSize)
	}
	if config.FetchTimeout != 30 {
		t.Errorf("DefaultConfig().FetchTimeout = %d, want 30", config.FetchTimeout)
	}
	if config.MaxURLLength != 2000 {
		t.Errorf("DefaultConfig().MaxURLLength = %d, want 2000", config.MaxURLLength)
	}
}

func TestFormatSearchResults(t *testing.T) {
	tests := []struct {
		name     string
		results  []SearchResult
		contains []string // substrings that should be in the JSON output
	}{
		{
			name:     "empty results",
			results:  []SearchResult{},
			contains: []string{`"results":`, `[]`},
		},
		{
			name: "single result",
			results: []SearchResult{
				{Title: "Test Title", URL: "https://example.com", Snippet: "Test snippet"},
			},
			contains: []string{`"title":"Test Title"`, `"url":"https://example.com"`, `"snippet":"Test snippet"`},
		},
		{
			name: "multiple results",
			results: []SearchResult{
				{Title: "First", URL: "https://first.com", Snippet: "First snippet"},
				{Title: "Second", URL: "https://second.com", Snippet: "Second snippet"},
			},
			contains: []string{`"title":"First"`, `"title":"Second"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := FormatSearchResults(tt.results)
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("FormatSearchResults() output does not contain %q\nGot: %s", expected, output)
				}
			}
		})
	}
}

func TestSearchCacheKey(t *testing.T) {
	key1 := SearchCacheKey("test query")
	key2 := SearchCacheKey("test query")
	key3 := SearchCacheKey("different query")

	if key1 != key2 {
		t.Errorf("SearchCacheKey() not deterministic for same input")
	}
	if key1 == key3 {
		t.Errorf("SearchCacheKey() produces same key for different inputs")
	}
	// Cache keys should be hashes (hex strings)
	if len(key1) != 64 { // SHA256 = 64 hex chars
		t.Errorf("SearchCacheKey() length = %d, want 64", len(key1))
	}
}

func TestFetchCacheKey(t *testing.T) {
	key1 := FetchCacheKey("https://example.com")
	key2 := FetchCacheKey("https://example.com")
	key3 := FetchCacheKey("https://different.com")

	if key1 != key2 {
		t.Errorf("FetchCacheKey() not deterministic for same input")
	}
	if key1 == key3 {
		t.Errorf("FetchCacheKey() produces same key for different inputs")
	}
	// Cache keys should be hashes (hex strings)
	if len(key1) != 64 { // SHA256 = 64 hex chars
		t.Errorf("FetchCacheKey() length = %d, want 64", len(key1))
	}
}
