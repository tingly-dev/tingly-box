package webtools

import (
	"context"
	"fmt"
	"time"
)

// Tool 定义了一个可被 AI 调用的工具
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]Parameter
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// Parameter 定义工具参数
type Parameter struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// SearchResult 搜索结果
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// FetchResult 抓取结果
type FetchResult struct {
	Content string `json:"content"`
	Title   string `json:"title"`
	URL     string `json:"url"`
}

// Config 配置
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

// Option 配置选项
type Option func(*Config)

// WithTimeout 设置超时
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithMaxResults 设置最大结果数
func WithMaxResults(max int) Option {
	return func(c *Config) {
		c.MaxResults = max
	}
}

// WithHeadless 设置无头模式
func WithHeadless(headless bool) Option {
	return func(c *Config) {
		c.Headless = headless
	}
}

// NewConfig 创建配置
func NewConfig(opts ...Option) Config {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WebTools 工具集
type WebTools struct {
	config  Config
	browser Browser
}

// New 创建 WebTools 实例
func New(opts ...Option) (*WebTools, error) {
	cfg := NewConfig(opts...)
	
	// 注意: 实际需要初始化浏览器
	// 这里先创建基础结构
	
	wt := &WebTools{
		config: cfg,
	}
	
	return wt, nil
}

// Close 关闭浏览器
func (wt *WebTools) Close() error {
	if wt.browser != nil {
		return wt.browser.Close()
	}
	return nil
}

// Search 执行搜索
func (wt *WebTools) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	return nil, fmt.Errorf("not implemented: Search requires browser initialization")
}

// Fetch 抓取网页
func (wt *WebTools) Fetch(ctx context.Context, url string) (*FetchResult, error) {
	return nil, fmt.Errorf("not implemented: Fetch requires browser initialization")
}

// GetTools 返回工具列表 (MCP 格式)
func (wt *WebTools) GetTools() []Tool {
	return []Tool{
		&SearchTool{webtools: wt},
		&FetchTool{webtools: wt},
	}
}
