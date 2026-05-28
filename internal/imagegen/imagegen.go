// Package imagegen provides vendor adapters for the text-to-image surfaces
// that do NOT speak the OpenAI /images/generations contract. Image generation
// is one of the most fragmented surfaces across AI providers: some ship
// entirely bespoke schemas (async task polling, custom request bodies). This
// package hides that fragmentation behind a single Client interface.
//
// It is an implementation detail of client.OpenAIClient: that client's
// ImagesGenerate and ImagesEdit dispatch DashScope / MiniMax providers here and
// serve every OpenAI-compatible provider through its own native path. The
// package is intentionally a leaf (it does not import internal/client) so the
// client layer can depend on it without an import cycle.
//
// Vendor landscape (derived from internal/data/providers.json):
//
//	OpenAI-compatible (POST {base}/images/generations, OpenAI request/response):
//	  openai-com, x-ai, volces-com, z-ai/bigmodel-cn, siliconflow, stepfun,
//	  together-xyz, modelscope-cn, googleapis-com (Gemini compat), baidubce-com,
//	  deepinfra-com, novita-ai, fireworks-ai, openrouter-ai, ...
//	  -> NOT handled here; client.OpenAIClient serves these natively.
//
//	OpenAI Responses API (image_generation tool, no /images/generations):
//	  codex (ChatGPT OAuth).
//	  -> NOT handled here; client.CodexClient handles these.
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
	"fmt"
	"io"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ErrUnsupported is returned by New when a provider has no native image
// generation adapter in this package. The caller (client.OpenAIClient) only
// dispatches DashScope / MiniMax providers here, so in practice this signals a
// detection/routing bug rather than an expected condition.
var (
	ErrUnsupported     = errors.New("imagegen: provider does not support image generation")
	ErrEditUnsupported = errors.New("imagegen: provider does not support image editing")
)

// Client is the vendor-neutral image generation contract. Every vendor adapter
// implements it, so the gateway forwards image requests through a single path
// regardless of the upstream's native API shape.
type Client interface {
	// Generate produces one or more images for the given request.
	Generate(ctx context.Context, req *Request) (*Response, error)
	// Edit edits or inpaints one or more source images guided by a prompt.
	// Vendors that do not support editing embed UnsupportedEdit and return
	// ErrEditUnsupported.
	Edit(ctx context.Context, req *Request) (*Response, error)
	// Provider returns the upstream provider this client is bound to.
	Provider() *typ.Provider
	// Vendor returns the detected vendor family for diagnostics.
	Vendor() Vendor
	// Close releases any resources held by the client.
	Close() error
}

// UnsupportedEdit is an embed that satisfies the Edit method for vendors that
// only support generation. Adapters embed this rather than writing boilerplate.
type UnsupportedEdit struct{}

func (UnsupportedEdit) Edit(_ context.Context, _ *Request) (*Response, error) {
	return nil, ErrEditUnsupported
}

// ImageInput carries a single image (source or mask) for edit requests.
// Exactly one of Data or URL is expected to be non-zero.
type ImageInput struct {
	// Data holds the raw image bytes (PNG / JPEG / WebP). When set, URL is
	// ignored for wire purposes but may be kept for logging.
	Data []byte
	// MimeType is the MIME type of Data (e.g. "image/png").
	MimeType string
	// URL is an https:// or data: URI. Used when Data is empty.
	URL string
	// Filename is the filename hint used when building multipart uploads.
	Filename string
}

// Request is the normalized image request. It mirrors the common
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

	// --- Edit-specific fields ---

	// Images holds the source image(s) for edit requests. For dall-e-2 exactly
	// one image is required; GPT image models accept up to 16.
	Images []ImageInput
	// Mask is the inpainting mask. Black (alpha=0) regions are edited; white
	// (alpha=1) regions are preserved. Required for dall-e-2; optional for GPT
	// image models which infer the edit region from the prompt.
	Mask *ImageInput
	// InputFidelity controls how closely the model preserves facial features
	// and other distinctive details in source images. "high"|"low" (default
	// "low"). Supported on gpt-image-1 and later; ignored by dall-e-2.
	InputFidelity string
	// Moderation controls content-filter sensitivity. "auto"|"low". GPT image
	// models only.
	Moderation string
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

// RequestFromOpenAIEdit converts an OpenAI ImageEditParams into a normalized
// Request with Action="edit". It reads all io.Reader image and mask fields
// eagerly; callers must not close the readers before this returns.
// On read error the function returns a partial request and a non-nil error.
func RequestFromOpenAIEdit(p *openai.ImageEditParams) (*Request, error) {
	if p == nil {
		return nil, nil
	}
	req := &Request{
		Model:         string(p.Model),
		Prompt:        p.Prompt,
		Size:          string(p.Size),
		Quality:       string(p.Quality),
		ResponseFormat: string(p.ResponseFormat),
		Background:    string(p.Background),
		OutputFormat:  string(p.OutputFormat),
		InputFidelity: string(p.InputFidelity),
	}
	if p.N.Valid() {
		req.N = int(p.N.Value)
	}
	if p.User.Valid() {
		req.User = p.User.Value
	}

	// Read source images.
	readImage := func(r io.Reader, filename string) (ImageInput, error) {
		data, err := io.ReadAll(r)
		if err != nil {
			return ImageInput{}, err
		}
		mime := "image/png"
		if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 {
			mime = "image/jpeg"
		} else if len(data) >= 4 && string(data[:4]) == "RIFF" {
			mime = "image/webp"
		}
		return ImageInput{Data: data, MimeType: mime, Filename: filename}, nil
	}

	if r := p.Image.OfFile; r != nil {
		img, err := readImage(r, "image.png")
		if err != nil {
			return req, fmt.Errorf("imagegen: reading image: %w", err)
		}
		req.Images = []ImageInput{img}
	} else if len(p.Image.OfFileArray) > 0 {
		req.Images = make([]ImageInput, 0, len(p.Image.OfFileArray))
		for i, r := range p.Image.OfFileArray {
			img, err := readImage(r, fmt.Sprintf("image%d.png", i))
			if err != nil {
				return req, fmt.Errorf("imagegen: reading image[%d]: %w", i, err)
			}
			req.Images = append(req.Images, img)
		}
	}

	// Read optional mask.
	if p.Mask != nil {
		mask, err := readImage(p.Mask, "mask.png")
		if err != nil {
			return req, fmt.Errorf("imagegen: reading mask: %w", err)
		}
		req.Mask = &mask
	}

	return req, nil
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
