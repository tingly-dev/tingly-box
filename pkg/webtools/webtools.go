package webtools

import (
	"context"
	"fmt"
	"time"
)

// Tool represents a callable tool that can be invoked by an AI.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]Parameter
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// Parameter defines a tool parameter.
type Parameter struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// SearchResult represents a search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// FetchResult represents a web page fetch result.
type FetchResult struct {
	Content string `json:"content"`
	Title   string `json:"title"`
	URL     string `json:"url"`
}

// Config holds the webtools configuration.
type Config struct {
	Timeout       time.Duration
	MaxResults    int
	BrowserPath   string
	UserAgent     string
	Headless     bool
}

var defaultConfig = Config{
	Timeout:    30 * time.Second,
	MaxResults: 10,
	Headless:   true,
	UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
}

// Option defines a configuration option.
type Option func(*Config)

// WithTimeout sets the request timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithMaxResults sets the maximum number of search results.
func WithMaxResults(max int) Option {
	return func(c *Config) {
		c.MaxResults = max
	}
}

// WithHeadless sets headless browser mode.
func WithHeadless(headless bool) Option {
	return func(c *Config) {
		c.Headless = headless
	}
}

// NewConfig creates a new configuration with the given options.
func NewConfig(opts ...Option) Config {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WebTools is a collection of web automation tools.
type WebTools struct {
	config  Config
	browser Browser
}

// New creates a new WebTools instance.
func New(opts ...Option) (*WebTools, error) {
	cfg := NewConfig(opts...)

	// Note: browser initialization is required for actual functionality.
	// For now, create the basic structure.

	wt := &WebTools{
		config: cfg,
	}

	return wt, nil
}

// Close closes the browser.
func (wt *WebTools) Close() error {
	if wt.browser != nil {
		return wt.browser.Close()
	}
	return nil
}

// Search performs a web search.
func (wt *WebTools) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	return nil, fmt.Errorf("not implemented: Search requires browser initialization")
}

// Fetch fetches a web page.
func (wt *WebTools) Fetch(ctx context.Context, url string) (*FetchResult, error) {
	return nil, fmt.Errorf("not implemented: Fetch requires browser initialization")
}

// GetTools returns available tools in MCP format.
func (wt *WebTools) GetTools() []Tool {
	return []Tool{
		&SearchTool{webtools: wt},
		&FetchTool{webtools: wt},
	}
}
