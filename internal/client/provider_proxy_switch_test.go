package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// The per-provider proxy switch must reach the wire: with the proxy on, a model
// listing goes through the proxy; with it off the stored URL is ignored and the
// request connects directly. Flipping it takes effect after the pool
// invalidation the provider-update hook performs, with no restart.
func TestProviderProxySwitch_ReachesRequestPath(t *testing.T) {
	var direct, proxied atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"m1","object":"model"}]}`))
	}))
	defer upstream.Close()
	// A forward proxy for plain HTTP receives the absolute-URI request itself.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"m1","object":"model"}]}`))
	}))
	defer proxy.Close()

	provider := &typ.Provider{
		UUID:     "proxy-switch-uuid",
		Name:     "switch",
		Token:    "sk-test",
		APIBase:  upstream.URL,
		ProxyURL: proxy.URL,
	}
	pool := NewClientPool()
	list := func() {
		t.Helper()
		_, err := pool.GetOpenAIClient(context.Background(), provider, "m1").(*OpenAIClient).ListModels(context.Background())
		require.NoError(t, err)
	}

	list()
	assert.EqualValues(t, 1, proxied.Load(), "proxy on: request must go through the proxy")
	assert.EqualValues(t, 0, direct.Load())

	provider.ProxyDisabled = true
	pool.InvalidateProvider(provider.UUID)
	list()
	assert.EqualValues(t, 1, proxied.Load(), "proxy off: the stored URL must not be used")
	assert.EqualValues(t, 1, direct.Load())

	provider.ProxyDisabled = false
	pool.InvalidateProvider(provider.UUID)
	list()
	assert.EqualValues(t, 2, proxied.Load(), "proxy back on without losing the URL")
	assert.EqualValues(t, 1, direct.Load())
}
