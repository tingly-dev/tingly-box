package oauth

import (
	"github.com/google/uuid"
	"net/http"
)

// RequestHook defines preprocessing hooks for OAuth requests.
// Implementations can modify request parameters before they are sent.
type RequestHook interface {
	// BeforeAuth is called before building the authorization URL.
	// The params map contains URL query parameters that can be modified or extended.
	BeforeAuth(params map[string]string) error

	// BeforeToken is called before sending any token-related HTTP request.
	// This covers: token exchange, refresh token, device code request, and device token polling.
	// The body map contains request body parameters, header is the HTTP headers.
	BeforeToken(body map[string]string, header http.Header) error
}

// NoopHook is a default hook that does nothing.
// Used when no custom behavior is needed.
type NoopHook struct{}

func (h *NoopHook) BeforeAuth(params map[string]string) error {
	return nil
}

func (h *NoopHook) BeforeToken(body map[string]string, header http.Header) error {
	return nil
}

// AnthropicHook implements Anthropic Claude Code OAuth specific behavior.
type AnthropicHook struct{}

func (h *AnthropicHook) BeforeAuth(params map[string]string) error {
	params["code"] = "true"
	params["response_type"] = "code"
	return nil
}

func (h *AnthropicHook) BeforeToken(body map[string]string, header http.Header) error {
	header.Set("Content-Type", "application/json")
	header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	header.Set("Accept", "application/json, text/plain, */*")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Referer", "https://claude.ai/")
	header.Set("Origin", "https://claude.ai")
	return nil
}

// GeminiHook implements Gemini CLI OAuth specific behavior.
type GeminiHook struct{}

func (h *GeminiHook) BeforeAuth(params map[string]string) error {
	params["access_type"] = "offline"
	params["prompt"] = "consent"
	return nil
}

func (h *GeminiHook) BeforeToken(body map[string]string, header http.Header) error {
	// No special token handling for Gemini
	return nil
}

// AntigravityHook implements Antigravity OAuth specific behavior.
type AntigravityHook struct{}

func (h *AntigravityHook) BeforeAuth(params map[string]string) error {
	params["access_type"] = "offline"
	params["prompt"] = "consent"
	params["include_granted_scopes"] = "true"
	return nil
}

func (h *AntigravityHook) BeforeToken(body map[string]string, header http.Header) error {
	return nil
}

// QwenHook implements Qwen Device Code OAuth specific behavior.
type QwenHook struct{}

func (h *QwenHook) BeforeAuth(params map[string]string) error {
	// Qwen uses device code flow, no special auth params needed
	return nil
}

func (h *QwenHook) BeforeToken(body map[string]string, header http.Header) error {
	// Add dynamic x-request-id header for Qwen
	header.Set("x-request-id", uuid.New().String())
	return nil
}
