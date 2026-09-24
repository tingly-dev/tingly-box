package imagegen

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// qianfanClient serves image edits against Baidu Qianfan v2. Generation is
// OpenAI-compatible and stays on the SDK; /v2/images/edits takes JSON rather
// than the OpenAI multipart form, and its mask is the opposite convention.
//
//	POST {base}/images/edits
//	{ "model", "prompt", "image": dataURL | [dataURL...], "mask": dataURL,
//	  "feature": "repaint" | "variation", "n", "size" }
//
// Models: ernie-irag-edit (one image; mask for erase/repaint) and
// qwen-image-edit (1-3 images, no mask, one output). The response only carries
// 24h URLs, which are pulled back into base64 (inlineResultURLs).
//
// Reference: https://cloud.baidu.com/doc/qianfan-api/s/Rm9m76ekf
type qianfanClient struct {
	provider   *typ.Provider
	httpClient *http.Client
	editURL    string
}

func newQianfanClient(provider *typ.Provider, transport http.RoundTripper) (*qianfanClient, error) {
	base := strings.TrimRight(strings.TrimSpace(provider.APIBase), "/")
	if base == "" {
		return nil, fmt.Errorf("imagegen: qianfan provider %q has no API base", provider.Name)
	}
	return &qianfanClient{
		provider:   provider,
		httpClient: &http.Client{Transport: transport},
		editURL:    base + "/images/edits",
	}, nil
}

func (c *qianfanClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

type qianfanEditBody struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt,omitempty"`
	Image   any    `json:"image"`
	Mask    string `json:"mask,omitempty"`
	Feature string `json:"feature,omitempty"`
	N       int    `json:"n,omitempty"`
	Size    string `json:"size,omitempty"`
}

type qianfanEditResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

func (c *qianfanClient) Edit(ctx context.Context, req *EditRequest) (*Response, error) {
	body, err := buildQianfanEditBody(req)
	if err != nil {
		return nil, err
	}
	var parsed qianfanEditResponse
	if err := postJSON(ctx, c.httpClient, c.editURL, c.provider.GetAccessToken(), nil, body, &parsed); err != nil {
		return nil, fmt.Errorf("imagegen: qianfan image edit: %w", err)
	}
	out := &Response{Created: parsed.Created, Model: req.Model}
	if out.Created == 0 {
		out.Created = time.Now().Unix()
	}
	for _, d := range parsed.Data {
		if d.URL == "" && d.B64JSON == "" {
			continue
		}
		out.Data = append(out.Data, Image{URL: d.URL, B64JSON: d.B64JSON})
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("imagegen: qianfan image edit returned no image data")
	}
	if err := inlineResultURLs(ctx, c.httpClient, req, out); err != nil {
		return nil, err
	}
	return out, nil
}

// qianfanMaskModel is the Qianfan edit model that takes a mask.
const qianfanMaskModel = "ernie-irag-edit"

func buildQianfanEditBody(req *EditRequest) (*qianfanEditBody, error) {
	ernie := strings.HasPrefix(strings.ToLower(req.Model), qianfanMaskModel)
	body := &qianfanEditBody{
		Model:  req.Model,
		Prompt: req.Prompt,
		N:      req.N,
		Size:   req.Size,
	}

	if ernie {
		if len(req.Images) > 1 {
			return nil, fmt.Errorf("imagegen: %s edits a single image; got %d", qianfanMaskModel, len(req.Images))
		}
		// With a mask the prompt says what goes in the painted area
		// (repaint); without one the only mask-free edit this model has is
		// a variation of the whole image.
		body.Feature = "variation"
		if len(req.Mask) > 0 {
			mask, err := whiteEditMask(req.Mask)
			if err != nil {
				return nil, err
			}
			body.Mask = dataURL(mask)
			body.Feature = "repaint"
		}
	} else if len(req.Mask) > 0 {
		return nil, fmt.Errorf("imagegen: Qianfan model %s has no mask; use %s for masked edits", req.Model, qianfanMaskModel)
	}

	if len(req.Images) == 1 {
		body.Image = dataURL(req.Images[0])
	} else {
		urls := make([]string, 0, len(req.Images))
		for _, img := range req.Images {
			urls = append(urls, dataURL(img))
		}
		body.Image = urls
	}
	return body, nil
}
