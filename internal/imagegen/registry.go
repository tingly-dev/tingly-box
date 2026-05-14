package imagegen

import (
	"fmt"

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
func New(provider *typ.Provider, model string) (Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("imagegen: nil provider")
	}

	vendor := DetectVendor(provider)
	logrus.Debugf("[imagegen] provider %s (api_base=%s) detected vendor: %s", provider.Name, provider.APIBase, vendor)

	switch vendor {
	case VendorDashScope:
		return newDashScopeClient(provider)
	case VendorMinimax:
		return newMinimaxClient(provider)
	default:
		return nil, fmt.Errorf("%w: provider %s (api_base=%s)", ErrUnsupported, provider.Name, provider.APIBase)
	}
}
