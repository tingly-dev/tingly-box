// Package imagegen provides a vendor-neutral abstraction for text-to-image
// generation. Image generation is one of the most fragmented surfaces across
// AI providers: some expose an OpenAI-compatible POST /images/generations,
// some require the OpenAI Responses API with an image_generation tool, and
// others ship entirely bespoke schemas (async task polling, custom request
// bodies). This package hides that fragmentation behind a single Client
// interface so the gateway can route any provider through one code path.
//
// Vendor landscape (derived from internal/data/providers.json):
//
//	OpenAI-compatible (POST {base}/images/generations, OpenAI request/response):
//	  openai-com, x-ai, volces-com(+coding), z-ai(+coding)/bigmodel-cn(+coding),
//	  siliconflow-cn/com, stepfun-com/ai, together-xyz, modelscope-cn,
//	  googleapis-com (Gemini OpenAI-compat layer), baidubce-com (v2),
//	  deepinfra-com, novita-ai, fireworks-ai, openrouter-ai, ...
//	  -> New returns ErrDelegateRequired; the caller injects the pool's existing
//	     OpenAI client via NewDelegate to avoid duplicating client construction.
//
//	OpenAI Responses API (image_generation tool, no /images/generations):
//	  codex (ChatGPT OAuth).
//	  -> New returns ErrResponsesAPIRequired; the caller forwards through the
//	     existing OpenAIClientInterface (client.CodexClient) instead.
//
//	Native async task API (submit -> poll task_id):
//	  dashscope-cn, dashscope-intl (Alibaba Wan / Tongyi Wanxiang, qwen-image).
//	  -> handled by dashscopeClient.
//
//	Native sync custom schema:
//	  minimaxi-com, minimax-io (MiniMax image-01, POST /v1/image_generation).
//	  -> handled by minimaxClient.
package imagegen

import (
	"context"
	"errors"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ErrResponsesAPIRequired is returned by New for providers whose image
// generation can only be reached through the OpenAI Responses API (currently
// Codex / ChatGPT OAuth). Callers should fall back to forwarding the request
// through client.OpenAIClientInterface.ImagesGenerate, which already contains
// the Responses-API transformation.
var ErrResponsesAPIRequired = errors.New("imagegen: provider requires the OpenAI Responses API for image generation")

// ErrDelegateRequired is returned by New for VendorOpenAICompat providers.
// The caller must construct a delegate via NewDelegate, injecting the existing
// pool OpenAI client, so imagegen does not duplicate client construction.
var ErrDelegateRequired = errors.New("imagegen: provider requires an injected delegate client")

// ErrUnsupported is returned by New when a provider has no known image
// generation surface wired up yet.
var ErrUnsupported = errors.New("imagegen: provider does not support image generation")

// Client is the vendor-neutral image generation contract. Every vendor adapter
// implements it, so the gateway forwards image requests through a single path
// regardless of the upstream's native API shape.
type Client interface {
	// Generate produces one or more images for the given request.
	Generate(ctx context.Context, req *Request) (*Response, error)
	// Provider returns the upstream provider this client is bound to.
	Provider() *typ.Provider
	// Vendor returns the detected vendor family for diagnostics.
	Vendor() Vendor
	// Close releases any resources held by the client.
	Close() error
}

// Request is the normalized image generation request. It mirrors the common
// subset of the OpenAI Images API; vendor adapters translate it into their
// native schema and ignore fields they do not support (logging a warning).
type Request struct {
	// Model is the upstream model id (already resolved by routing).
	Model string
	// Prompt is the text description of the desired image(s).
	Prompt string
	// N is the number of images to generate (default 1).
	N int
	// Size is "WIDTHxHEIGHT" (e.g. "1024x1024"). Adapters that expect an
	// aspect ratio or a "W*H" form convert from this value.
	Size string
	// Quality is one of standard|hd|low|medium|high|auto (vendor-dependent).
	Quality string
	// Style is one of vivid|natural (OpenAI dall-e-3 only).
	Style string
	// ResponseFormat is "url" or "b64_json". Not every vendor honors both.
	ResponseFormat string
	// Background is transparent|opaque|auto (GPT image models).
	Background string
	// OutputFormat is png|jpeg|webp (GPT image models).
	OutputFormat string
	// User is an opaque end-user identifier for abuse monitoring.
	User string
	// Extra carries vendor-specific passthrough parameters that have no
	// normalized field. Adapters merge these into their native request body.
	Extra map[string]any
}

// Image is a single generated image. Exactly one of URL or B64JSON is set,
// depending on the upstream and the requested ResponseFormat.
type Image struct {
	URL           string
	B64JSON       string
	RevisedPrompt string
}

// Usage holds token accounting when the upstream reports it (GPT image models
// and a few others). Zero values mean the upstream did not report usage.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// Response is the normalized image generation response.
type Response struct {
	Created int64
	Model   string
	Data    []Image
	Usage   Usage
}

// RequestFromOpenAI converts an OpenAI Images API request into the normalized
// Request. This lets the OpenAI-compatible inbound surface feed the gateway
// without each handler re-deriving the mapping.
func RequestFromOpenAI(p *openai.ImageGenerateParams) *Request {
	if p == nil {
		return nil
	}
	req := &Request{
		Model:          string(p.Model),
		Prompt:         p.Prompt,
		Size:           string(p.Size),
		Quality:        string(p.Quality),
		Style:          string(p.Style),
		ResponseFormat: string(p.ResponseFormat),
		Background:     string(p.Background),
		OutputFormat:   string(p.OutputFormat),
	}
	if p.N.Valid() {
		req.N = int(p.N.Value)
	}
	if p.User.Valid() {
		req.User = p.User.Value
	}
	return req
}

// ToOpenAI converts the normalized Response back into the OpenAI Images API
// response shape, so OpenAI-compatible inbound clients see a familiar payload
// no matter which vendor served the request.
func (r *Response) ToOpenAI() *openai.ImagesResponse {
	if r == nil {
		return nil
	}
	out := &openai.ImagesResponse{Created: r.Created}
	out.Data = make([]openai.Image, 0, len(r.Data))
	for _, img := range r.Data {
		out.Data = append(out.Data, openai.Image{
			URL:           img.URL,
			B64JSON:       img.B64JSON,
			RevisedPrompt: img.RevisedPrompt,
		})
	}
	out.Usage = openai.ImagesResponseUsage{
		InputTokens:  r.Usage.InputTokens,
		OutputTokens: r.Usage.OutputTokens,
		TotalTokens:  r.Usage.TotalTokens,
	}
	return out
}

// ToOpenAIParams converts the normalized Request into OpenAI SDK params. It is
// used by the OpenAI-compatible adapter.
func (req *Request) ToOpenAIParams() openai.ImageGenerateParams {
	p := openai.ImageGenerateParams{
		Model:  openai.ImageModel(req.Model),
		Prompt: req.Prompt,
	}
	if req.N > 0 {
		p.N = param.NewOpt(int64(req.N))
	}
	if req.Size != "" {
		p.Size = openai.ImageGenerateParamsSize(req.Size)
	}
	if req.Quality != "" {
		p.Quality = openai.ImageGenerateParamsQuality(req.Quality)
	}
	if req.Style != "" {
		p.Style = openai.ImageGenerateParamsStyle(req.Style)
	}
	if req.ResponseFormat != "" {
		p.ResponseFormat = openai.ImageGenerateParamsResponseFormat(req.ResponseFormat)
	}
	if req.Background != "" {
		p.Background = openai.ImageGenerateParamsBackground(req.Background)
	}
	if req.OutputFormat != "" {
		p.OutputFormat = openai.ImageGenerateParamsOutputFormat(req.OutputFormat)
	}
	if req.User != "" {
		p.User = param.NewOpt(req.User)
	}
	return p
}
