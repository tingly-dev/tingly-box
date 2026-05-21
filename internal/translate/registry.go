package translate

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// New builds a dedicated translation Client for providers that expose a
// non-LLM translation API (currently HuggingFace and DeepL). For providers
// that should be driven as LLMs, callers must use the llmClient path directly
// via NewLLMClient — this function returns ErrUnsupported for those.
func New(provider *typ.Provider, model string) (Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("translate: nil provider")
	}

	vendor := DetectVendor(provider)
	logrus.Debugf("[translate] provider %s (api_base=%s) detected vendor: %s", provider.Name, provider.APIBase, vendor)

	switch vendor {
	case VendorHuggingFace:
		return newHuggingFaceClient(provider, model)
	case VendorDeepL:
		return newDeepLClient(provider, model)
	default:
		return nil, fmt.Errorf("%w: provider %s (api_base=%s)", ErrUnsupported, provider.Name, provider.APIBase)
	}
}
