package imagegen

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Image editing for vendors whose edit surface is not the OpenAI multipart
// /images/edits contract. client.OpenAIClient.ImagesEdit dispatches these
// vendors here and serves every other OpenAI-compatible provider through the
// SDK. See .design/image-edit-adapters.md.
//
//	xAI       POST {base}/images/edits            JSON, image(s) as data URLs, no mask
//	Qianfan   POST {base}/images/edits            JSON, mask as a white-means-edit image
//	DashScope wanx*imageedit  async image2image   mask as a white-means-edit image
//	          qwen-image*     sync multimodal-generation, no mask

// ErrEditUnsupported is returned by NewEditor for a vendor with no edit
// adapter here. The caller only dispatches edit-capable vendors, so seeing it
// means a routing bug rather than an expected condition.
var ErrEditUnsupported = errors.New("imagegen: provider does not support image editing here")

// Editor is the vendor-neutral image edit contract.
type Editor interface {
	Edit(ctx context.Context, req *EditRequest) (*Response, error)
	Close() error
}

// EditRequest is the normalized image edit request: the OpenAI edit fields
// with every input already read into memory, so adapters can inline them in
// whatever encoding their API wants (and re-send them) without single-use
// readers.
type EditRequest struct {
	Model  string
	Prompt string
	// Images are the reference images in request order; the mask, if any,
	// applies to the first.
	Images [][]byte
	// Mask is the OpenAI mask as sent: a PNG whose fully transparent pixels
	// mark the area to edit. Adapters convert it to their vendor's form.
	Mask []byte
	N    int
	Size string
	// ResponseFormat is "url" or "b64_json"; empty means b64_json, which is
	// what GPT image models return and what the gateway can persist.
	ResponseFormat string
	User           string
}

// WantsURL reports whether the caller explicitly asked for image URLs. Every
// other case gets base64, because the URLs these vendors return expire (24h
// for Qianfan and DashScope) and a URL cannot be saved to the image directory.
func (r *EditRequest) WantsURL() bool {
	return strings.EqualFold(r.ResponseFormat, "url")
}

// EditRequestFromOpenAI reads an OpenAI edit request into an EditRequest.
func EditRequestFromOpenAI(p *openai.ImageEditParams) (*EditRequest, error) {
	if p == nil {
		return nil, fmt.Errorf("imagegen: nil edit request")
	}
	req := &EditRequest{
		Model:          string(p.Model),
		Prompt:         p.Prompt,
		Size:           string(p.Size),
		ResponseFormat: string(p.ResponseFormat),
	}
	if p.N.Valid() {
		req.N = int(p.N.Value)
	}
	if p.User.Valid() {
		req.User = p.User.Value
	}

	var readers []io.Reader
	switch {
	case p.Image.OfFile != nil:
		readers = []io.Reader{p.Image.OfFile}
	case len(p.Image.OfFileArray) > 0:
		readers = p.Image.OfFileArray
	}
	for i, r := range readers {
		if r == nil {
			continue
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("imagegen: read input image %d: %w", i, err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("imagegen: input image %d is empty", i)
		}
		req.Images = append(req.Images, data)
	}
	if len(req.Images) == 0 {
		return nil, fmt.Errorf("imagegen: image edit request has no input image")
	}

	if p.Mask != nil {
		data, err := io.ReadAll(p.Mask)
		if err != nil {
			return nil, fmt.Errorf("imagegen: read mask: %w", err)
		}
		if len(data) > 0 {
			req.Mask = data
		}
	}
	return req, nil
}

// NewEditor builds the edit adapter for a provider, or ErrEditUnsupported.
func NewEditor(provider *typ.Provider, opts ...Option) (Editor, error) {
	if provider == nil {
		return nil, fmt.Errorf("imagegen: nil provider")
	}
	o := newOptions(opts)
	switch DetectVendor(provider) {
	case VendorXAI:
		return newXAIClient(provider, o.transport)
	case VendorQianfan:
		return newQianfanClient(provider, o.transport)
	case VendorDashScope:
		return newDashScopeClient(provider, o.transport)
	default:
		return nil, fmt.Errorf("%w: provider %s (api_base=%s)", ErrEditUnsupported, provider.Name, provider.APIBase)
	}
}

// dataURL encodes image bytes as a data URL, the inline form all three
// vendors accept. The media type is sniffed so callers need not track it.
func dataURL(data []byte) string {
	mediaType := http.DetectContentType(data)
	if !strings.HasPrefix(mediaType, "image/") {
		mediaType = "image/png"
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// maxFetchedImageBytes bounds a result image pulled back from a vendor URL.
const maxFetchedImageBytes = 32 << 20

// inlineResultURLs replaces URL-only results with their base64 content unless
// the caller asked for URLs. The vendors behind this only return short-lived
// URLs; handing those on would make every Playground result and every saved
// image depend on a link that dies within a day.
func inlineResultURLs(ctx context.Context, httpClient *http.Client, req *EditRequest, resp *Response) error {
	if req.WantsURL() {
		return nil
	}
	for i := range resp.Data {
		img := &resp.Data[i]
		if img.B64JSON != "" || img.URL == "" {
			continue
		}
		b64, err := fetchImageBase64(ctx, httpClient, img.URL)
		if err != nil {
			return fmt.Errorf("imagegen: fetch result image %d: %w", i, err)
		}
		img.B64JSON = b64
		img.URL = ""
	}
	return nil
}

func fetchImageBase64(ctx context.Context, httpClient *http.Client, url string) (string, error) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return "", fmt.Errorf("unsupported result URL scheme")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchedImageBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxFetchedImageBytes {
		return "", fmt.Errorf("result image exceeds %d bytes", maxFetchedImageBytes)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
