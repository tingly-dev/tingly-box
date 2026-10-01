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
	// (device-code, polling, refresh, code exchange). Escape hatch for
	// per-flow header state like Kimi's X-Msh-Device-Id binding.
	ExtraHeaders http.Header

	// ZCodeAPIBase / ZCodeBizHost override the ZCode control plane and business
	// API hosts. Only tests set them; production always uses the constants, and
	// a server-poll flow has no callback whose BaseURL could stand in for them.
	ZCodeAPIBase string
	ZCodeBizHost string
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

// WithExtraHeader sets a header applied to every token-related OAuth
// request in this flow. Repeated calls accumulate; same-key values replace.
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

// WithZCodeEndpoints overrides the ZCode control-plane and business-API hosts.
// Test-only: it is what lets the whole login run against an httptest server.
func WithZCodeEndpoints(apiBase, bizHost string) Option {
	return func(o *Options) {
		o.ZCodeAPIBase = apiBase
		o.ZCodeBizHost = bizHost
	}
}

// kimiDeviceIDHeader is the HTTP header Kimi's auth and inference endpoints
// use to bind a credential to a specific device.
const kimiDeviceIDHeader = "X-Msh-Device-Id"

// WithKimiDeviceID pins X-Msh-Device-Id on every token-related request the
// manager makes during a flow. Callers that refresh later (the background
// token refresher) use it to rehydrate the same id.
func WithKimiDeviceID(deviceID string) Option {
	return WithExtraHeader(kimiDeviceIDHeader, deviceID)
}

// applyOptions creates an Options struct from variadic options
func applyOptions(opts ...Option) *Options {
	o := &Options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
