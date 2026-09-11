package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/sirupsen/logrus"
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
// x-codex-image-turn-id request header). See .design/imageedit.md.
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
// model are required; background/n/quality/size are omitted when unset.
type codexImageEditRequest struct {
	Images     []codexImageInput `json:"images"`
	Prompt     string            `json:"prompt"`
	Model      string            `json:"model"`
	Background string            `json:"background,omitempty"`
	N          *int64            `json:"n,omitempty"`
	Quality    string            `json:"quality,omitempty"`
	Size       string            `json:"size,omitempty"`
}

// ImagesEdit serves an OpenAI /images/edits request against the Codex-native
// images edit endpoint. The multipart-style file inputs are inlined as base64
// data URLs per the Codex wire protocol.
func (c *CodexClient) ImagesEdit(ctx context.Context, req openai.ImageEditParams) (*openai.ImagesResponse, error) {
	// A mask has no representation in this protocol, so it decides the surface
	// (codex_images_responses.go). Everything else stays on the proven endpoint.
	if resolveCodexImageEditRoute(req.Mask != nil) == codexImageEditRouteResponses {
		return c.imagesEditViaResponses(ctx, req)
	}

	logrus.WithContext(ctx).Debugf("[Codex] Using native images/edits endpoint for image edit, model: %s", req.Model)

	codexReq, err := buildCodexImageEditRequest(&req)
	if err != nil {
		return nil, err
	}

	var resp openai.ImagesResponse
	opts := []option.RequestOption{
		// Turn correlation id the Codex CLI sends on image requests
		// (codex-rs ext/image-generation/src/backend.rs); the gateway has no
		// Codex turn concept, so a fresh id per call stands in.
		option.WithHeader("x-codex-image-turn-id", uuid.NewString()),
	}
	if err := c.Client().Post(ctx, "images/edits", codexReq, &resp, opts...); err != nil {
		return nil, fmt.Errorf("codex image edit failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("codex image edit returned no image data")
	}

	logrus.WithContext(ctx).Infof("[Codex] Image edit succeeded, images: %d", len(resp.Data))
	return &resp, nil
}

// buildCodexImageEditRequest translates OpenAI ImageEditParams into the Codex
// JSON edit request. Parameters the Codex wire schema does not carry
// (response_format, output_format, output_compression, input_fidelity) are
// dropped, mirroring how ImagesGenerate treats n/style. A mask is the one
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

	if req.N.Valid() {
		n := req.N.Value
		out.N = &n
	}

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
