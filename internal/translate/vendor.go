package translate

import (
	"net/url"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Vendor identifies the translation surface a provider exposes. Derived from
// the provider's API base host, not from the user-chosen provider name.
type Vendor string

const (
	// VendorHuggingFace covers providers backed by the HuggingFace Inference API.
	// Models: Helsinki-NLP/opus-mt-*, NLLB-200, M2M-100, mBART, Madlad-400, ...
	VendorHuggingFace Vendor = "huggingface"
	// VendorDeepL covers the DeepL translation API (api.deepl.com and the free tier).
	VendorDeepL Vendor = "deepl"
	// VendorLLM covers any OpenAI-compatible provider used as an LLM translator
	// via chat completions with a translation system prompt.
	VendorLLM Vendor = "llm"
	// VendorUnknown is a provider with no recognised translation surface.
	VendorUnknown Vendor = "unknown"
)

// DetectVendor inspects a provider and returns the translation vendor family.
func DetectVendor(provider *typ.Provider) Vendor {
	if provider == nil {
		return VendorUnknown
	}
	host := apiHost(provider.APIBase)
	switch {
	case strings.Contains(host, "huggingface.co"):
		return VendorHuggingFace
	case strings.Contains(host, "deepl.com"):
		return VendorDeepL
	}
	// Anything OpenAI-compatible falls back to LLM-based translation.
	return VendorLLM
}

func apiHost(apiBase string) string {
	apiBase = strings.TrimSpace(apiBase)
	if apiBase == "" {
		return ""
	}
	if u, err := url.Parse(apiBase); err == nil && u.Host != "" {
		return strings.ToLower(u.Host)
	}
	return strings.ToLower(apiBase)
}
