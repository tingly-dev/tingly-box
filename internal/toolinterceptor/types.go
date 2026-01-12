package toolinterceptor

import "time"

// SearchResult represents a single search result from the search API
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolCallID string // OpenAI: tool_call_id, Anthropic: tool_use_id
	Content    string // JSON string or plain text result
	Error      string // Error message if execution failed
	IsError    bool   // True if the result is an error
}

// SearchRequest represents a search tool call parameters
type SearchRequest struct {
	Query string `json:"query"`
	Count int    `json:"count,omitempty"` // Number of results to return
}

// FetchRequest represents a fetch tool call parameters
type FetchRequest struct {
	URL string `json:"url"`
}

// Config represents the tool interceptor configuration
type Config struct {
	Enabled    bool
	SearchAPI  string // "brave", "google", or "duckduckgo"
	SearchKey  string // API key for search service (not needed for duckduckgo)
	MaxResults int    // Max search results (default: 10)

	// Proxy configuration
	ProxyURL string // HTTP proxy URL (e.g., "http://127.0.0.1:7897")

	// Fetch configuration
	MaxFetchSize int64 // Max content size for fetch in bytes (default: 1MB)
	FetchTimeout int64 // Fetch timeout in seconds (default: 30)
	MaxURLLength int   // Max URL length (default: 2000)
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		SearchAPI:    "duckduckgo",
		MaxResults:   10,
		MaxFetchSize: 1 * 1024 * 1024, // 1MB
		FetchTimeout: 30,              // 30 seconds
		MaxURLLength: 2000,
	}
}

// CacheEntry represents a cached search or fetch result
type CacheEntry struct {
	Result      interface{} // SearchResult[] or string (for fetch)
	ExpiresAt   time.Time
	ContentType string // "search" or "fetch"
}
