package quota

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	typ "github.com/tingly-dev/tingly-box/ai"
	"golang.org/x/net/proxy"
)

// Fetcher retrieves quota data for a provider type.
type Fetcher interface {
	// Name returns the fetcher name.
	Name() string

	// ProviderType returns the supported provider type.
	ProviderType() ProviderType

	// Fetch retrieves current quota data.
	Fetch(ctx context.Context, provider *typ.Provider) (*ProviderUsage, error)

	// Validate checks the provider configuration.
	Validate(provider *typ.Provider) error

	// RequiresAuth returns the required authentication type.
	RequiresAuth() typ.AuthType
}

// FetcherRegistrar is the interface for registering fetchers.
// Implemented by Manager; used by fetcher.RegisterAll to avoid import cycles.
type FetcherRegistrar interface {
	RegisterFetcher(fetcher Fetcher) error
}

// Registry stores fetchers by provider type.
type Registry struct {
	mu       sync.RWMutex
	fetchers map[ProviderType]Fetcher
}

// NewRegistry creates an empty fetcher registry.
func NewRegistry() *Registry {
	return &Registry{
		fetchers: make(map[ProviderType]Fetcher),
	}
}

// Register adds a fetcher to the registry.
func (r *Registry) Register(fetcher Fetcher) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := fetcher.ProviderType()
	if _, exists := r.fetchers[pt]; exists {
		return ErrFetcherAlreadyRegistered
	}
	r.fetchers[pt] = fetcher
	return nil
}

// Unregister removes a fetcher from the registry.
func (r *Registry) Unregister(pt ProviderType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.fetchers, pt)
}

// Get returns the fetcher registered for a provider type.
func (r *Registry) Get(pt ProviderType) (Fetcher, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.fetchers[pt]
	return f, ok
}

// List returns all registered fetchers.
func (r *Registry) List() map[ProviderType]Fetcher {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Return a copy so callers cannot mutate the registry.
	result := make(map[ProviderType]Fetcher, len(r.fetchers))
	for k, v := range r.fetchers {
		result[k] = v
	}
	return result
}

// ProviderTypes returns all registered provider types.
func (r *Registry) ProviderTypes() []ProviderType {
	r.mu.RLock()
	defer r.mu.RUnlock()
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

// NewHTTPClient creates an HTTP client with optional proxy support.
// proxyURL accepts formats such as "http://localhost:7890" or "socks5://localhost:1080".
func NewHTTPClient(proxyURL string, timeout time.Duration) *http.Client {
	client := &http.Client{
		Timeout: timeout,
	}

	if proxyURL == "" {
		return client
	}

	// Parse the proxy URL.
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		logrus.Warnf("Failed to parse proxy URL %s: %v, using direct connection", proxyURL, err)
		return client
	}

	// Configure a transport for the selected proxy scheme.
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
	logrus.Debugf("Using proxy: %s", proxyURL)
	return client
}
