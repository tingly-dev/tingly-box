package imagegen

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// New builds a native image generation Client for the given provider. It only
// serves vendors with a bespoke (non-OpenAI) image API — currently DashScope
// and MiniMax. The model argument is the already-routed upstream model id; it
// is not used for vendor selection (that is host-based) but adapters may read
// it.
//
// OpenAI-compatible and Codex providers are NOT served here: client.OpenAIClient
// and client.CodexClient handle those on their own native paths. New returns
// ErrUnsupported for them, which in practice signals a routing bug since the
// caller is expected to dispatch only DashScope / MiniMax here.
func New(ctx context.Context, provider *typ.Provider, model string, opts ...Option) (Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("imagegen: nil provider")
	}
	o := newOptions(opts)

	vendor := DetectVendor(provider)
	logrus.WithContext(ctx).Debugf("[imagegen] provider %s (api_base=%s) detected vendor: %s", provider.Name, provider.APIBase, vendor)

	switch vendor {
	case VendorDashScope:
		return newDashScopeClient(provider, o.transport)
	case VendorMinimax:
		return newMinimaxClient(provider, o.transport)
	default:
		return nil, fmt.Errorf("%w: provider %s (api_base=%s)", ErrUnsupported, provider.Name, provider.APIBase)
	}
}

// Option customizes an adapter built by New or NewEditor.
type Option func(*options)

type options struct {
	transport http.RoundTripper
}

func newOptions(opts []Option) options {
	o := options{transport: http.DefaultTransport}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithTransport sets the round-tripper the adapter's upstream calls go
// through. The client layer passes its logging transport here so a DashScope /
// MiniMax call lands in the request's timeline like every other provider call;
// this package cannot wrap it itself without importing internal/client.
func WithTransport(rt http.RoundTripper) Option {
	return func(o *options) {
		if rt != nil {
			o.transport = rt
		}
	}
}
