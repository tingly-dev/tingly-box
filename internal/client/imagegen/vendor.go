package imagegen

import (
	"net/url"
	"strings"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Vendor identifies the image generation surface a provider exposes. It is
// derived from the provider's API base host (and OAuth issuer), not from a
// user-chosen provider name, so it stays stable regardless of how the user
// labelled the provider.
type Vendor string

const (
	// VendorOpenAICompat covers every provider that speaks the OpenAI
	// /images/generations contract. This is the large majority.
	VendorOpenAICompat Vendor = "openai_compat"
	// VendorCodex is ChatGPT OAuth: image generation only via Responses API.
	VendorCodex Vendor = "codex"
	// VendorDashScope is Alibaba Model Studio / DashScope (Wan, qwen-image):
	// native async task-submit-then-poll API.
	VendorDashScope Vendor = "dashscope"
	// VendorMinimax is MiniMax: native POST /v1/image_generation.
	VendorMinimax Vendor = "minimax"
	// VendorUnknown is a provider with no known image surface.
	VendorUnknown Vendor = "unknown"
)

// DetectVendor inspects a provider and returns the image generation vendor
// family it belongs to. Detection is host-based so it works for both the
// canonical providers in internal/data and user-defined clones that point at
// the same hosts.
func DetectVendor(provider *typ.Provider) Vendor {
	if provider == nil {
		return VendorUnknown
	}

	// Codex / ChatGPT OAuth: image generation rides the Responses API.
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil &&
		provider.OAuthDetail.GetIssuer() == ai.IssuerCodex {
		return VendorCodex
	}
	if provider.APIBase == protocol.CodexAPIBase {
		return VendorCodex
	}

	host := apiHost(provider.APIBase)
	switch {
	case strings.Contains(host, "dashscope") && strings.Contains(host, "aliyuncs.com"):
		// Matches both dashscope.aliyuncs.com (Beijing) and
		// dashscope-intl.aliyuncs.com (Singapore).
		return VendorDashScope
	case strings.Contains(host, "api.minimax.io"), strings.Contains(host, "api.minimaxi.com"):
		return VendorMinimax
	}

	// Anything else with an OpenAI-style base is assumed OpenAI-compatible:
	// this is the documented contract for x-ai, volcengine ark, zhipu/z-ai,
	// siliconflow, stepfun, together, modelscope, gemini's compat layer,
	// baidu qianfan v2, and the various aggregators.
	if provider.APIStyle == protocol.APIStyleOpenAI || provider.APIStyle == "" {
		return VendorOpenAICompat
	}
	return VendorUnknown
}

// apiHost extracts the lowercased host from an API base URL. Falls back to the
// raw string when parsing fails so substring matching still has something to
// work with.
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

// apiScheme returns the scheme of the API base, defaulting to https.
func apiScheme(apiBase string) string {
	if u, err := url.Parse(strings.TrimSpace(apiBase)); err == nil && u.Scheme != "" {
		return u.Scheme
	}
	return "https"
}
