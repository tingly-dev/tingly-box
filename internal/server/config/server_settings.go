package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HTTPTransportConfig holds HTTP transport connection pool settings
// These settings control the connection pooling behavior for upstream API requests
// All fields use pointers so that omitting them means "use Go default" (backward compatible)
type HTTPTransportConfig struct {
	// MaxIdleConns is the maximum number of idle connections across all hosts
	// Default (nil): 100 (Go stdlib default)
	// Recommended for 200 concurrent users: 200-300
	MaxIdleConns *int `json:"max_idle_conns,omitempty" yaml:"max_idle_conns,omitempty"`

	// MaxIdleConnsPerHost is the maximum number of idle connections per host
	// Default (nil): 2 (Go stdlib default)
	// Recommended for 200 concurrent users: 20-50
	MaxIdleConnsPerHost *int `json:"max_idle_conns_per_host,omitempty" yaml:"max_idle_conns_per_host,omitempty"`

	// MaxConnsPerHost limits the total number of connections per host (active + idle)
	// Default (nil): 0 (no limit)
	// Set to control maximum concurrent connections to a single upstream host
	MaxConnsPerHost *int `json:"max_conns_per_host,omitempty" yaml:"max_conns_per_host,omitempty"`

	// DisableKeepAlives disables HTTP/1.1 keep-alive connections
	// Default (nil): false
	// WARNING: Setting this to true will significantly impact performance
	DisableKeepAlives *bool `json:"disable_keep_alives,omitempty" yaml:"disable_keep_alives,omitempty"`

	// RespectEnvProxy controls whether providers without explicit proxy configuration
	// should use environment/system proxy settings (HTTP_PROXY, HTTPS_PROXY, macOS system proxy, etc.)
	// Default (nil): false - providers without proxy_url connect directly
	// Set to true: providers without proxy_url will use system/environment proxy
	RespectEnvProxy *bool `json:"respect_env_proxy,omitempty" yaml:"respect_env_proxy,omitempty"`

	// GlobalProxyURL stores a shared proxy URL offered as a UI convenience default.
	// The backend does NOT apply it automatically; users must explicitly opt in per-provider/OAuth.
	GlobalProxyURL string `json:"global_proxy_url,omitempty" yaml:"global_proxy_url,omitempty"`
}

// GenericMCPConfig holds settings for the new generic MCP architecture
type GenericMCPConfig struct {
	// UseGenericAnthropicV1NonStream enables generic path for A→A V1 non-streaming
	// When false: uses existing dispatch implementation
	// When true: uses GenericLoopProcessor
	UseGenericAnthropicV1NonStream bool `json:"use_generic_anthropic_v1_non_stream,omitempty" yaml:"use_generic_anthropic_v1_non_stream,omitempty"`

	// UseGenericAnthropicV1Stream enables generic path for A→A V1 streaming
	// When false: uses existing dispatch implementation
	// When true: uses GenericStreamInterceptor
	UseGenericAnthropicV1Stream bool `json:"use_generic_anthropic_v1_stream,omitempty" yaml:"use_generic_anthropic_v1_stream,omitempty"`

	// UseGenericOpenAIChatNonStream enables generic path for O→O non-streaming
	UseGenericOpenAIChatNonStream bool `json:"use_generic_openai_chat_non_stream,omitempty" yaml:"use_generic_openai_chat_non_stream,omitempty"`

	// UseGenericOpenAIChatStream enables generic path for O→O streaming
	UseGenericOpenAIChatStream bool `json:"use_generic_openai_chat_stream,omitempty" yaml:"use_generic_openai_chat_stream,omitempty"`

	// ProviderLimits limits which providers use generic path
	// Empty means all providers can use generic path
	// Format: comma-separated provider names (e.g., "provider1,provider2")
	ProviderLimits string `json:"provider_limits,omitempty" yaml:"provider_limits,omitempty"`
}

// Server configuration methods (merged from AppConfig)

// GetServerPort returns the configured server port
func (c *Config) GetServerPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ServerPort
}

// SetServerPort updates the server port
func (c *Config) SetServerPort(port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ServerPort = port
	return c.Save()
}

// GetServerHost returns the configured server host
// Returns "localhost" if no host is configured
func (c *Config) GetServerHost() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.ServerHost == "" {
		return "localhost"
	}
	return c.ServerHost
}

// SetServerHost updates the server host
func (c *Config) SetServerHost(host string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ServerHost = host
	return c.Save()
}

// GetDefaultMaxTokens returns the configured default max_tokens
func (c *Config) GetDefaultMaxTokens() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.DefaultMaxTokens
}

// ToolTypeMCPRuntime is the ToolConfigs key for the MCP runtime tool config.
// Tool configs live in config.json (GetToolConfig/SetToolConfig); the constant
// used to live in internal/db next to a store that was never wired up.
const ToolTypeMCPRuntime = "mcp_runtime"

// GetMCPRuntimeConfig returns the global MCP runtime config.
func (c *Config) GetMCPRuntimeConfig() *typ.MCPRuntimeConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var config typ.MCPRuntimeConfig
	if c.ToolConfigs != nil {
		if data, ok := c.ToolConfigs[ToolTypeMCPRuntime]; ok {
			if err := json.Unmarshal(data, &config); err == nil {
				typ.ApplyMCPRuntimeDefaults(&config)
				return &config
			}
		}
	}

	return nil
}

// GetToolConfig returns the global config for a specific tool type
// target is a pointer to the config struct to unmarshal into
// Returns true if config was found and successfully unmarshaled
func (c *Config) GetToolConfig(toolType string, target interface{}) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.ToolConfigs == nil {
		return false
	}

	data, ok := c.ToolConfigs[toolType]
	if !ok {
		return false
	}

	if err := json.Unmarshal(data, target); err != nil {
		logrus.Warnf("Failed to unmarshal tool config for type %s: %v", toolType, err)
		return false
	}

	return true
}

// SetToolConfig sets the global config for a specific tool type
func (c *Config) SetToolConfig(toolType string, config interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ToolConfigs == nil {
		c.ToolConfigs = make(map[string]json.RawMessage)
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal tool config: %w", err)
	}

	c.ToolConfigs[toolType] = data
	return c.Save()
}

// SetDefaultMaxTokens updates the default max_tokens
func (c *Config) SetDefaultMaxTokens(maxTokens int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DefaultMaxTokens = maxTokens
	return c.Save()
}

// GetVerbose returns the verbose setting
func (c *Config) GetVerbose() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Verbose
}

// SetVerbose updates the verbose setting
func (c *Config) SetVerbose(verbose bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Verbose = verbose
	return c.Save()
}

// GetDebug returns the debug setting
func (c *Config) GetDebug() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Debug
}

// SetDebug updates the debug setting
func (c *Config) SetDebug(debug bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Debug = debug
	return c.Save()
}

// GetOpenBrowser returns the open browser setting
func (c *Config) GetOpenBrowser() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.OpenBrowser
}

// SetOpenBrowser updates the open browser setting (runtime only, not persisted)
func (c *Config) SetOpenBrowser(openBrowser bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.OpenBrowser = openBrowser
	return nil
}

// logProxyEnvironment logs proxy-related environment variables and the
// RespectEnvProxy config value so operators can diagnose unexpected proxy usage
// (e.g. when the process inherits HTTP_PROXY / HTTPS_PROXY from a shell or npx).
func (c *Config) logProxyEnvironment() {
	respectEnvProxy := false
	if c.HTTPTransport.RespectEnvProxy != nil {
		respectEnvProxy = *c.HTTPTransport.RespectEnvProxy
	}
	logrus.Debugf("proxy env: HTTP_PROXY=%q HTTPS_PROXY=%q NO_PROXY=%q http_proxy=%q https_proxy=%q no_proxy=%q respect_env_proxy=%v",
		os.Getenv("HTTP_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("NO_PROXY"),
		os.Getenv("http_proxy"), os.Getenv("https_proxy"), os.Getenv("no_proxy"),
		respectEnvProxy)
}

// ApplyHTTPTransportConfig applies the HTTP transport configuration to the global transport pool.
// Called at runtime when the operator updates transport settings via the config API.
// Default behavior (all fields nil): providers without proxy_url connect directly, ignoring env proxy.
func (c *Config) ApplyHTTPTransportConfig() {
	c.logProxyEnvironment()

	if c.HTTPTransport.MaxIdleConns == nil &&
		c.HTTPTransport.MaxIdleConnsPerHost == nil &&
		c.HTTPTransport.MaxConnsPerHost == nil &&
		c.HTTPTransport.DisableKeepAlives == nil &&
		c.HTTPTransport.RespectEnvProxy == nil {
		// No custom transport config, use Go defaults (backward compatible with TB)
		return
	}

	// Import client package to set transport config
	// We need to do this here to avoid circular dependency
	config := &client.TransportConfig{
		MaxIdleConns:        c.HTTPTransport.MaxIdleConns,
		MaxIdleConnsPerHost: c.HTTPTransport.MaxIdleConnsPerHost,
		MaxConnsPerHost:     c.HTTPTransport.MaxConnsPerHost,
		DisableKeepAlives:   c.HTTPTransport.DisableKeepAlives,
		RespectEnvProxy:     c.HTTPTransport.RespectEnvProxy,
	}
	client.SetTransportConfig(config)
}
