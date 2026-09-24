package imagegen

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// xaiClient serves image edits against xAI (Grok Imagine). Generation is
// OpenAI-compatible and stays on the SDK; editing is not — xAI's
// /v1/images/edits takes a JSON body, and the OpenAI SDK's multipart upload
// is explicitly unsupported there.
//
//	POST {base}/images/edits
//	{ "model", "prompt", "image": {"url": dataURL} | "images": [{"url"}...],
//	  "n", "response_format", "aspect_ratio", "user" }
//
// There is no mask field: a masked request is refused rather than silently
// turned into a whole-image edit.
//
// Reference: https://docs.x.ai/developers/rest-api-reference/inference/images
type xaiClient struct {
	provider   *typ.Provider
	httpClient *http.Client
	editURL    string
}

func newXAIClient(provider *typ.Provider, transport http.RoundTripper) (*xaiClient, error) {
	base := strings.TrimRight(strings.TrimSpace(provider.APIBase), "/")
	if base == "" {
		return nil, fmt.Errorf("imagegen: xai provider %q has no API base", provider.Name)
	}
	return &xaiClient{
		provider:   provider,
		httpClient: &http.Client{Transport: transport},
		editURL:    base + "/images/edits",
	}, nil
}

func (c *xaiClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

type xaiImageRef struct {
	URL string `json:"url"`
}

type xaiEditBody struct {
	Model          string        `json:"model,omitempty"`
	Prompt         string        `json:"prompt"`
	Image          *xaiImageRef  `json:"image,omitempty"`
	Images         []xaiImageRef `json:"images,omitempty"`
	N              int           `json:"n,omitempty"`
	ResponseFormat string        `json:"response_format,omitempty"`
	AspectRatio    string        `json:"aspect_ratio,omitempty"`
	User           string        `json:"user,omitempty"`
}

type xaiEditResponse struct {
	Data []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Usage struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
		TotalTokens  int64 `json:"total_tokens"`
	} `json:"usage"`
}

func (c *xaiClient) Edit(ctx context.Context, req *EditRequest) (*Response, error) {
	body, err := buildXAIEditBody(req)
	if err != nil {
		return nil, err
	}
	var parsed xaiEditResponse
	if err := postJSON(ctx, c.httpClient, c.editURL, c.provider.GetAccessToken(), nil, body, &parsed); err != nil {
		return nil, fmt.Errorf("imagegen: xai image edit: %w", err)
	}
	out := &Response{
		Created: time.Now().Unix(),
		Model:   req.Model,
		Usage: Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		},
	}
	for _, d := range parsed.Data {
		if d.URL == "" && d.B64JSON == "" {
			continue
		}
		out.Data = append(out.Data, Image{URL: d.URL, B64JSON: d.B64JSON})
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("imagegen: xai image edit returned no image data")
	}
	return out, nil
}

func buildXAIEditBody(req *EditRequest) (*xaiEditBody, error) {
	if len(req.Mask) > 0 {
		return nil, fmt.Errorf("imagegen: xAI image editing has no mask parameter; remove the mask or use a provider that supports inpainting")
	}
	body := &xaiEditBody{
		Model:       req.Model,
		Prompt:      req.Prompt,
		N:           req.N,
		AspectRatio: xaiAspectRatio(req.Size),
		User:        req.User,
		// Asked for explicitly: xAI defaults to URLs, which the gateway cannot
		// persist and which expire.
		ResponseFormat: "b64_json",
	}
	if req.WantsURL() {
		body.ResponseFormat = "url"
	}
	if len(req.Images) == 1 {
		body.Image = &xaiImageRef{URL: dataURL(req.Images[0])}
	} else {
		for _, img := range req.Images {
			body.Images = append(body.Images, xaiImageRef{URL: dataURL(img)})
		}
	}
	return body, nil
}

// xaiAspectRatios is the aspect_ratio enum /v1/images/edits accepts.
var xaiAspectRatios = map[string]bool{
	"1:1": true, "3:4": true, "4:3": true, "9:16": true, "16:9": true, "2:3": true, "3:2": true,
	"9:20": true, "20:9": true, "1:2": true, "2:1": true, "21:9": true, "5:2": true,
}

// xaiAspectRatio turns an OpenAI "WIDTHxHEIGHT" size into xAI's aspect_ratio
// when it reduces to one xAI knows. Anything else is left unset, so the output
// follows the first input image — xAI's own default, and the sensible one for
// an edit.
func xaiAspectRatio(size string) string {
	// parseSize / gcd are shared with the MiniMax adapter (minimax.go).
	w, h, ok := parseSize(size)
	if !ok || w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	ratio := strconv.Itoa(w/g) + ":" + strconv.Itoa(h/g)
	if xaiAspectRatios[ratio] {
		return ratio
	}
	return ""
}
