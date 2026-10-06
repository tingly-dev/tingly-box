package client

import (
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/recording/capture"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// wireRecord mounts the recording tap directly on a wire base — the pooled
// *http.Transport or the in-process vmodel transport (SessionBoundTransport
// taps itself in RoundTrip) —
// so it sits inside every header/body-rewriting round tripper (rule flags,
// vendor adapters, SDK middleware such as Bedrock signing) and records the
// request that actually leaves the gateway. It is read-only and a plain
// pass-through unless the request context carries an enabled Trace
// (.design/recording.md §4.1).
func wireRecord(base http.RoundTripper, provider *typ.Provider) http.RoundTripper {
	if base == nil || provider == nil {
		return base
	}
	return capture.WrapTransport(base, recordProvider(provider))
}

func recordProvider(provider *typ.Provider) capture.ProviderInfo {
	if provider == nil {
		return capture.ProviderInfo{}
	}
	return capture.ProviderInfo{
		Name:     provider.Name,
		UUID:     provider.UUID,
		APIStyle: string(provider.APIStyle),
	}
}
