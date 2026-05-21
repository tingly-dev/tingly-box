// Package translate provides vendor adapters for text translation surfaces.
// It hides the fragmentation between dedicated translation APIs (HuggingFace
// Inference API, DeepL) and LLM-based translation (any OpenAI-compatible
// provider via chat completions) behind a single Client interface.
//
// Vendor landscape:
//
//	HuggingFace Inference API (POST {base}/models/{model}):
//	  Helsinki-NLP/opus-mt-*, NLLB-200, M2M-100, mBART, ...
//	  -> handled by huggingfaceClient.
//
//	DeepL API (POST api.deepl.com/v2/translate or api-free.deepl.com):
//	  -> handled by deeplClient.
//
//	LLM-based (any OpenAI-compatible provider via chat completions):
//	  OpenAI GPT-*, Anthropic Claude, Qwen, etc.
//	  -> handled by llmClient, which wraps an existing OpenAI chat client.
package translate

import (
	"context"
	"errors"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ErrUnsupported is returned by New when the provider has no known translate adapter.
var ErrUnsupported = errors.New("translate: provider does not support dedicated translation API")

// Client is the vendor-neutral translation contract.
type Client interface {
	// Translate translates the given request and returns the result.
	Translate(ctx context.Context, req *Request) (*Response, error)
	// Provider returns the upstream provider this client is bound to.
	Provider() *typ.Provider
	// Vendor returns the detected vendor family for diagnostics.
	Vendor() Vendor
	// Close releases any resources held by the client.
	Close() error
}

// Request is the normalized translation request.
type Request struct {
	// Model is the upstream model id (already resolved by routing).
	Model string
	// Input is the text to translate.
	Input string
	// SourceLang is the BCP-47 source language code (e.g. "en", "zh").
	// An empty string or "auto" means auto-detect.
	SourceLang string
	// TargetLang is the BCP-47 target language code (e.g. "zh", "en").
	TargetLang string
}

// Response is the normalized translation response.
type Response struct {
	// Model is the upstream model that served the request.
	Model string
	// Translation is the translated text.
	Translation string
	// DetectedSourceLang is the source language detected by the provider.
	// Empty when the provider does not report it.
	DetectedSourceLang string
	// Usage holds character accounting when the upstream reports it.
	Usage Usage
}

// Usage holds character-level accounting. Zero values mean the upstream did
// not report usage.
type Usage struct {
	InputCharacters  int
	OutputCharacters int
}

// APIRequest is the HTTP request body accepted by POST /tingly/translate/v1/translations.
type APIRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	SourceLang string `json:"source_lang,omitempty"`
	TargetLang string `json:"target_lang"`
}

// APIResponse is the HTTP response body for POST /tingly/translate/v1/translations.
type APIResponse struct {
	Model              string    `json:"model"`
	Translation        string    `json:"translation"`
	DetectedSourceLang string    `json:"detected_source_lang,omitempty"`
	Usage              APIUsage  `json:"usage,omitempty"`
}

// APIUsage is the usage section of APIResponse.
type APIUsage struct {
	InputCharacters  int `json:"input_characters"`
	OutputCharacters int `json:"output_characters"`
}

// ToAPIResponse converts the normalized Response to the HTTP response shape.
func (r *Response) ToAPIResponse() *APIResponse {
	if r == nil {
		return nil
	}
	return &APIResponse{
		Model:              r.Model,
		Translation:        r.Translation,
		DetectedSourceLang: r.DetectedSourceLang,
		Usage: APIUsage{
			InputCharacters:  r.Usage.InputCharacters,
			OutputCharacters: r.Usage.OutputCharacters,
		},
	}
}
