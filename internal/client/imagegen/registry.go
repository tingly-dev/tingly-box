package imagegen

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// New builds a vendor-neutral image generation Client for the given provider.
// The model argument is the already-routed upstream model id; it is not used
// for vendor selection (that is host-based) but adapters may read it.
//
// For Codex / ChatGPT OAuth providers, image generation is only reachable via
// the OpenAI Responses API, which is already implemented by
// client.OpenAIClientInterface (CodexClient). New therefore returns
// ErrResponsesAPIRequired so the caller can route through that existing path
// instead of duplicating the Responses-API transformation here.
func New(provider *typ.Provider, model string) (Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("imagegen: nil provider")
	}

	vendor := DetectVendor(provider)
	logrus.Debugf("[imagegen] provider %s (api_base=%s) detected vendor: %s", provider.Name, provider.APIBase, vendor)

	switch vendor {
	case VendorCodex:
		return nil, ErrResponsesAPIRequired
	case VendorDashScope:
		return newDashScopeClient(provider)
	case VendorMinimax:
		return newMinimaxClient(provider)
	case VendorOpenAICompat:
		return newOpenAICompatClient(provider)
	default:
		return nil, fmt.Errorf("%w: provider %s (api_base=%s)", ErrUnsupported, provider.Name, provider.APIBase)
	}
}
