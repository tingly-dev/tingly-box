package quota

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Fetcher 配额获取器接口
type Fetcher interface {
	// Name 返回 fetcher 名称
	Name() string

	// ProviderType 返回支持的供应商类型
	ProviderType() ProviderType

	// Fetch 获取当前配额
	Fetch(ctx context.Context, provider *typ.Provider) (*ProviderUsage, error)

	// Validate 验证配置是否有效
	Validate(provider *typ.Provider) error

	// RequiresAuth 返回需要的认证类型
	RequiresAuth() typ.AuthType
}

// FetcherRegistrar is the interface for registering fetchers.
// Implemented by Manager; used by fetcher.RegisterAll to avoid import cycles.
type FetcherRegistrar interface {
	RegisterFetcher(fetcher Fetcher) error
}

// Registry Fetcher 注册表
type Registry struct {
	fetchers map[ProviderType]Fetcher
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	return &Registry{
		fetchers: make(map[ProviderType]Fetcher),
	}
}

// Register 注册 fetcher
func (r *Registry) Register(fetcher Fetcher) error {
	pt := fetcher.ProviderType()
	if _, exists := r.fetchers[pt]; exists {
		return ErrFetcherAlreadyRegistered
	}
	r.fetchers[pt] = fetcher
	return nil
}

// Unregister 注销 fetcher
func (r *Registry) Unregister(pt ProviderType) {
	delete(r.fetchers, pt)
}

// Get 获取指定类型的 fetcher
func (r *Registry) Get(pt ProviderType) (Fetcher, bool) {
	f, ok := r.fetchers[pt]
	return f, ok
}

// List 列出所有已注册的 fetcher
func (r *Registry) List() map[ProviderType]Fetcher {
	// 返回副本
	result := make(map[ProviderType]Fetcher, len(r.fetchers))
	for k, v := range r.fetchers {
		result[k] = v
	}
	return result
}

// ProviderTypes 返回所有支持的供应商类型
func (r *Registry) ProviderTypes() []ProviderType {
	types := make([]ProviderType, 0, len(r.fetchers))
	for pt := range r.fetchers {
		types = append(types, pt)
	}
	return types
}

var (
	ErrFetcherAlreadyRegistered = &quotaError{"fetcher already registered for this provider type"}
)

type quotaError struct {
	msg string
}

func (e *quotaError) Error() string {
	return e.msg
}

// NewHTTPClient 创建带 proxy 支持的 HTTP client
// proxyURL 格式: "http://127.0.0.1:7890" 或 "socks5://127.0.0.1:1080"
func NewHTTPClient(proxyURL string, timeout time.Duration) *http.Client {
	client := &http.Client{
		Timeout: timeout,
	}

	if proxyURL == "" {
		return client
	}

	// 解析 proxy URL
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		logrus.Warnf("Failed to parse proxy URL %s: %v, using direct connection", proxyURL, err)
		return client
	}

	// 创建 transport
	transport := &http.Transport{}

	switch parsedURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, nil, proxy.Direct)
		if err != nil {
			logrus.Warnf("Failed to create SOCKS5 proxy dialer: %v, using direct connection", err)
			return client
		}
		dialContext, ok := dialer.(proxy.ContextDialer)
		if ok {
			transport.DialContext = dialContext.DialContext
		} else {
			logrus.Warn("SOCKS5 dialer does not support context, using direct connection")
			return client
		}
	default:
		logrus.Warnf("Unsupported proxy scheme: %s, using direct connection", parsedURL.Scheme)
		return client
	}

	client.Transport = transport
	logrus.Infof("Using proxy: %s", proxyURL)
	return client
}
