package oauth

import (
	"net/http"
	"net/url"

	"github.com/sirupsen/logrus"
)

// Option is a functional option for OAuth operations
type Option func(*Options)

// Options holds optional parameters for OAuth operations
type Options struct {
	// ProxyURL overrides the default proxy for this request
	ProxyURL *url.URL

	// BaseURL overrides the default callback base URL for this request
	BaseURL string

	// HTTPClient allows passing a custom HTTP client
	HTTPClient *http.Client

	// ExtraHeaders are merged into every token-related outbound request
	// (device-code, polling, refresh, code exchange). It's a generic escape
	// hatch for providers that need per-flow header state — for example,
	// binding a stable X-Msh-Device-Id across all requests of a Kimi flow.
	// The OAuth manager itself stays oblivious to provider semantics; callers
	// own the per-provider semantics.
	ExtraHeaders http.Header
}

// WithProxyURL sets a proxy URL for the request
func WithProxyURL(proxyURL *url.URL) Option {
	return func(o *Options) {
		o.ProxyURL = proxyURL
	}
}

// WithProxyString sets a string proxy URL for the request
func WithProxyString(proxy string) Option {
	return func(o *Options) {
		if proxy != "" {
			proxyURL, err := url.Parse(proxy)
			if err != nil {
				logrus.Errorf("Failed to parse proxy URL: %v\n", err)
			} else {
				o.ProxyURL = proxyURL
			}
		}
	}
}

// WithProxyURLString sets a proxy URL from string
// Returns an option that does nothing if the URL is invalid
func WithProxyURLString(proxyURL string) Option {
	return func(o *Options) {
		if proxyURL == "" {
			return
		}
		if u, err := url.Parse(proxyURL); err == nil {
			o.ProxyURL = u
		}
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) Option {
	return func(o *Options) {
		o.HTTPClient = client
	}
}

// WithBaseURL sets a callback base URL for the request
func WithBaseURL(baseURL string) Option {
	return func(o *Options) {
		o.BaseURL = baseURL
	}
}

// WithExtraHeader appends a header to be applied to every token-related
// OAuth request in this flow. Use it for per-flow header state (e.g.
// pinning a device id binding for the entire authorize → poll → refresh
// lifecycle). Repeated calls accumulate; values for the same key replace.
func WithExtraHeader(key, value string) Option {
	return func(o *Options) {
		if key == "" {
			return
		}
		if o.ExtraHeaders == nil {
			o.ExtraHeaders = make(http.Header)
		}
		o.ExtraHeaders.Set(key, value)
	}
}

// applyOptions creates an Options struct from variadic options
func applyOptions(opts ...Option) *Options {
	o := &Options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
