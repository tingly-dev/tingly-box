package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/vision/imagefmt"
	"github.com/tingly-dev/tingly-box/internal/vision/imagegen"
)

// Codex-native images protocol.
//
// The ChatGPT/Codex subscription backend exposes dedicated image endpoints
// alongside the Responses API:
//
//	POST {base}/codex/images/generations
//	POST {base}/codex/images/edits
//
// Unlike the public OpenAI Images API, the edit endpoint takes a JSON body
// with data-URL image references — NOT a multipart file upload:
//
//	{
//	  "images": [{"image_url": "data:image/png;base64,..."}],
//	  "prompt": "...",
//	  "model": "gpt-image-2",
//	  "background": "auto", "quality": "auto", "size": "auto"
//	}
//
// The response matches the OpenAI ImagesResponse shape (data[].b64_json plus
// usage), so it deserializes into openai.ImagesResponse directly. This mirrors
// what the Codex CLI's built-in image_gen/imagegen tool sends — verified
// against openai/codex source: codex-rs/codex-api/src/images.rs
// (ImageEditRequest / ImageResponse), codex-rs/codex-api/src/endpoint/images.rs
// (the images/edits POST), and codex-rs/ext/image-generation/src/{tool,backend}.rs
// (gpt-image-2 default model, auto defaults, 5-image cap, the
// x-codex-image-turn-id request header). See .design/image-edit.md.
//
// Image generation continues to ride the Responses API image_generation tool
// (codex_client.go); editing got its own endpoint because attaching reference
// images to that tool was assumed impossible for Codex. That assumption is now
// under test — codex itself sends input_image items to the same Responses
// endpoint — and masked edits, which this protocol cannot express at all, are
// routed to the tool instead (codex_images_responses.go).

// codexMaxReferenceImages is the reference-image cap enforced by Codex's
// built-in imagegen tool. The backend owns the hard limit; we only log when a
// request exceeds it so oversized requests fail loudly upstream, not silently
// truncated here.
const codexMaxReferenceImages = 5

type codexImageInput struct {
	ImageURL string `json:"image_url"`
}

// codexImageEditRequest mirrors codex-rs ImageEditRequest: images, prompt and
// model are required; background/quality/size are omitted when unset. The
// wire `n` is never sent (see ImagesEdit).
type codexImageEditRequest struct {
	Images     []codexImageInput `json:"images"`
	Prompt     string            `json:"prompt"`
	Model      string            `json:"model"`
	Background string            `json:"background,omitempty"`
	Quality    string            `json:"quality,omitempty"`
	Size       string            `json:"size,omitempty"`
	codexGIFFields
}

// codexGIFFields are the animated-output controls Codex added to both images
// endpoints (codex-rs GifOptions, openai/codex#59073). They are sent only for
// output_format "gif"; every other request omits all three, as the Codex CLI does.
type codexGIFFields struct {
	OutputFormat    string `json:"output_format,omitempty"`
	FrameCount      int    `json:"frame_count,omitempty"`
	FrameDurationMS int    `json:"frame_duration_ms,omitempty"`
}

// applyCodexGIFShape gives a GIF request the exact shape Codex's own imagegen
// tool sends (codex-rs ext/image-generation/src/tool.rs): quality and size
// "auto", and a background that is "transparent" only when asked for, "opaque"
// otherwise. Codex never sends "auto" or an explicit size for a GIF, so neither
// is forwarded; the backend chooses the sheet layout.
func applyCodexGIFShape(background, quality, size *string, requestedBackground string) {
	*background = "opaque"
	if requestedBackground == "transparent" {
		*background = "transparent"
	}
	*quality = "auto"
	*size = "auto"
}

// describeCodexImageResponse names what the upstream said when its error body
// carried no JSON the SDK could parse (an empty 400 reads as "body=" and nothing
// else): the content type and the request ids, so the failure can be traced.
func describeCodexImageResponse(err error) string {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) || apiErr.Response == nil {
		return ""
	}
	var parts []string
	for _, name := range []string{"Content-Type", "X-Codex-Imagegen-Request-Id", "X-Request-Id", "Cf-Ray"} {
		if v := apiErr.Response.Header.Get(name); v != "" {
			parts = append(parts, strings.ToLower(name)+"="+v)
		}
	}
	detail := strings.Join(parts, " ")
	if apiErr.RawJSON() == "" {
		return " (empty response body; " + detail + ")"
	}
	if detail == "" {
		return ""
	}
	return " (" + detail + ")"
}

func newCodexGIFFields(o imagegen.OutputOptions) codexGIFFields {
	if !o.IsGIF() {
		return codexGIFFields{}
	}
	return codexGIFFields{OutputFormat: string(imagefmt.GIF), FrameCount: o.FrameCount, FrameDurationMS: o.FrameDurationMS}
}

// codexImageGenerateRequest mirrors codex-rs ImageGenerationRequest.
type codexImageGenerateRequest struct {
	Prompt     string `json:"prompt"`
	Model      string `json:"model"`
	Background string `json:"background,omitempty"`
	Quality    string `json:"quality,omitempty"`
	Size       string `json:"size,omitempty"`
	codexGIFFields
}

// imagesGenerateNative serves an image generation request on the Codex-native
// images/generations endpoint. It is used only for animated GIF output, which
// the Responses image_generation tool cannot express; every other request
// keeps the Responses path (see .design/image-output-formats.md §3.4).
func (c *CodexClient) imagesGenerateNative(ctx context.Context, req openai.ImageGenerateParams, opts imagegen.OutputOptions) (*openai.ImagesResponse, error) {
	logrus.WithContext(ctx).Debugf("[Codex] Using native images/generations endpoint for GIF output, model: %s", req.Model)

	body := &codexImageGenerateRequest{
		Prompt:         req.Prompt,
		Model:          strings.TrimSuffix(string(req.Model), fastModelSuffix),
		codexGIFFields: newCodexGIFFields(opts),
	}
	applyCodexGIFShape(&body.Background, &body.Quality, &body.Size, string(req.Background))
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		if raw, err := json.Marshal(body); err == nil {
			logrus.WithContext(ctx).Debugf("[Codex] native images/generations request body: %s", raw)
		}
	}

	return fanOutCodexImages(ctx, codexImageCount(req.N), func(ctx context.Context) (*openai.ImagesResponse, error) {
		var resp openai.ImagesResponse
		reqOpts := []option.RequestOption{option.WithHeader("x-codex-image-turn-id", uuid.NewString())}
		if err := c.Client().Post(ctx, "images/generations", body, &resp, reqOpts...); err != nil {
			return nil, fmt.Errorf("codex image generation failed: %w%s", err, describeCodexImageResponse(err))
		}
		if len(resp.Data) == 0 {
			return nil, fmt.Errorf("codex image generation returned no image data")
		}
		logrus.WithContext(ctx).Infof("[Codex] Native image generation succeeded, images: %d", len(resp.Data))
		return &resp, nil
	})
}

// ImagesEdit serves an OpenAI /images/edits request against the Codex-native
// images edit endpoint. The multipart-style file inputs are inlined as base64
// data URLs per the Codex wire protocol.
func (c *CodexClient) ImagesEdit(ctx context.Context, req openai.ImageEditParams) (*openai.ImagesResponse, error) {
	// A mask has no representation in this protocol, so it decides the surface
	// (codex_images_responses.go). Everything else stays on the proven endpoint.
	opts := imagegen.OutputOptionsFrom(ctx)
	if resolveCodexImageEditRoute(req.Mask != nil) == codexImageEditRouteResponses {
		if opts.IsGIF() {
			return nil, fmt.Errorf("animated GIF output cannot be combined with a mask: Codex's native edit endpoint has no mask support")
		}
		return c.imagesEditViaResponses(ctx, req)
	}

	logrus.WithContext(ctx).Debugf("[Codex] Using native images/edits endpoint for image edit, model: %s", req.Model)

	codexReq, err := buildCodexImageEditRequest(&req)
	if err != nil {
		return nil, err
	}
	for i, in := range codexReq.Images {
		if strings.HasPrefix(in.ImageURL, "data:image/gif;") {
			return nil, fmt.Errorf("input image %d is a GIF: Codex cannot edit a GIF directly; pass a PNG spritesheet of its frames instead", i)
		}
	}
	codexReq.codexGIFFields = newCodexGIFFields(opts)
	if opts.IsGIF() {
		applyCodexGIFShape(&codexReq.Background, &codexReq.Quality, &codexReq.Size, codexReq.Background)
	}

	// One image per call, n calls in parallel (codex_images_fanout.go). The
	// body is built once — the reference readers are single-use — and reused.
	return fanOutCodexImages(ctx, codexImageCount(req.N), func(ctx context.Context) (*openai.ImagesResponse, error) {
		var resp openai.ImagesResponse
		opts := []option.RequestOption{
			// Turn correlation id the Codex CLI sends on image requests
			// (codex-rs ext/image-generation/src/backend.rs); the gateway has no
			// Codex turn concept, so a fresh id per call stands in.
			option.WithHeader("x-codex-image-turn-id", uuid.NewString()),
		}
		if err := c.Client().Post(ctx, "images/edits", codexReq, &resp, opts...); err != nil {
			return nil, fmt.Errorf("codex image edit failed: %w%s", err, describeCodexImageResponse(err))
		}
		if len(resp.Data) == 0 {
			return nil, fmt.Errorf("codex image edit returned no image data")
		}
		logrus.WithContext(ctx).Infof("[Codex] Image edit succeeded, images: %d", len(resp.Data))
		return &resp, nil
	})
}

// buildCodexImageEditRequest translates OpenAI ImageEditParams into the Codex
// JSON edit request. Parameters the Codex wire schema does not carry
// (response_format, output_format, output_compression, input_fidelity) are
// dropped, mirroring how ImagesGenerate treats style. A mask is the one
// exception: it changes what the result must be, so it errors rather than
// being dropped.
func buildCodexImageEditRequest(req *openai.ImageEditParams) (*codexImageEditRequest, error) {
	readers := imageEditInputReaders(req)
	if len(readers) == 0 {
		return nil, fmt.Errorf("image edit request has no input image")
	}
	if len(readers) > codexMaxReferenceImages {
		logrus.Debugf("[Codex] %d input images exceeds the Codex reference cap of %d; the backend may reject the request",
			len(readers), codexMaxReferenceImages)
	}

	images := make([]codexImageInput, 0, len(readers))
	for i, r := range readers {
		dataURL, err := readerToDataURL(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read input image %d: %w", i, err)
		}
		images = append(images, codexImageInput{ImageURL: dataURL})
	}

	out := &codexImageEditRequest{
		Images:     images,
		Prompt:     req.Prompt,
		Model:      string(req.Model),
		Background: defaultCodexImageOption(string(req.Background)),
		Quality:    normalizeCodexImageQuality(string(req.Quality)),
		Size:       defaultCodexImageOption(string(req.Size)),
	}

	// N stays unset: the wire field exists, but the Codex CLI never sends it
	// and one image per call is the only shape known to work, so ImagesEdit
	// serves n > 1 by fanning out instead.

	if req.Mask != nil {
		// Reachable only when the Responses route is pinned off. Dropping the
		// mask would hand back a full repaint the caller never asked for, so
		// this fails instead of silently widening the edit.
		return nil, fmt.Errorf("mask is not supported by the Codex native images/edits protocol (see .design/image-mask.md)")
	}

	return out, nil
}

// imageEditInputReaders flattens the ImageEditParams image union into a
// reader slice, preserving order.
func imageEditInputReaders(req *openai.ImageEditParams) []io.Reader {
	if req.Image.OfFile != nil {
		return []io.Reader{req.Image.OfFile}
	}
	if len(req.Image.OfFileArray) > 0 {
		readers := make([]io.Reader, 0, len(req.Image.OfFileArray))
		for _, r := range req.Image.OfFileArray {
			if r != nil {
				readers = append(readers, r)
			}
		}
		return readers
	}
	return nil
}

// readerToDataURL drains an input image reader into a base64 data URL, the
// reference-image form the Codex edit endpoint expects. The media type is
// sniffed from the content so callers don't have to thread filenames through.
func readerToDataURL(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty image content")
	}
	mediaType := http.DetectContentType(data)
	if !strings.HasPrefix(mediaType, "image/") {
		// Keep going: the backend validates actual decodability, and PNG/JPEG/
		// WebP all sniff correctly — this only fires for exotic inputs.
		logrus.Debugf("[Codex] Input image sniffed as %q, sending anyway", mediaType)
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// normalizeCodexImageQuality maps OpenAI quality values ("standard"/"hd", the
// dall-e vocabulary) onto what the Codex endpoint accepts ("low"/"medium"/
// "high"/"auto"), defaulting to "auto". Shared by the edit path here and the
// Responses-based generation path in buildImageGenerationResponsesRequest
// (codex_client.go) — keep both quality mappings in this one place.
func normalizeCodexImageQuality(quality string) string {
	switch quality {
	case "":
		return "auto"
	case "standard":
		return "medium"
	case "hd":
		return "high"
	default:
		return quality
	}
}

// defaultCodexImageOption substitutes "auto" for unset size/background values,
// matching the defaults the Codex CLI fills in.
func defaultCodexImageOption(v string) string {
	if v == "" {
		return "auto"
	}
	return v
}
