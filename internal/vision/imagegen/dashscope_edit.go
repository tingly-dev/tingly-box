package imagegen

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Image editing on DashScope. Two model families, two services:
//
//	wanx*imageedit  POST {base}/services/aigc/image2image/image-synthesis (async, polled)
//	                input: {function, prompt, base_image_url, mask_image_url?}
//	                function: description_edit_with_mask (masked) | description_edit
//	                mask: white = edit, black = keep, same size as the image
//	qwen-image*     POST {base}/services/aigc/multimodal-generation/generation (sync)
//	                input.messages[0].content: [{image}..., {text}]; 1-3 images, no mask
//
// Both return URLs that expire in 24h; they are pulled back into base64
// (inlineResultURLs).
//
// References: https://help.aliyun.com/zh/model-studio/wanx-image-edit-api-reference ,
// https://help.aliyun.com/zh/model-studio/qwen-image-edit-api

type dashscopeImageEditInput struct {
	Function     string `json:"function"`
	Prompt       string `json:"prompt"`
	BaseImageURL string `json:"base_image_url"`
	MaskImageURL string `json:"mask_image_url,omitempty"`
}

type dashscopeImageEditBody struct {
	Model      string                  `json:"model"`
	Input      dashscopeImageEditInput `json:"input"`
	Parameters map[string]any          `json:"parameters,omitempty"`
}

type dashscopeMultimodalBody struct {
	Model string `json:"model"`
	Input struct {
		Messages []dashscopeMessage `json:"messages"`
	} `json:"input"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type dashscopeMessage struct {
	Role    string           `json:"role"`
	Content []map[string]any `json:"content"`
}

type dashscopeMultimodalResponse struct {
	Output struct {
		Choices []struct {
			Message struct {
				Content []struct {
					Image string `json:"image"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	} `json:"output"`
	Usage struct {
		ImageCount int64 `json:"image_count"`
	} `json:"usage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// isWanxImageEdit reports whether a model is on the Wanx image-editing
// service (wanx2.1-imageedit and its successors) rather than qwen-image's
// multimodal one.
func isWanxImageEdit(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "wan") && strings.Contains(m, "imageedit")
}

func (c *dashscopeClient) Edit(ctx context.Context, req *EditRequest) (*Response, error) {
	var (
		out *Response
		err error
	)
	if isWanxImageEdit(req.Model) {
		out, err = c.editWanx(ctx, req)
	} else {
		out, err = c.editQwenImage(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("imagegen: dashscope image edit returned no image data")
	}
	if err := inlineResultURLs(ctx, c.httpClient, req, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dashscopeClient) editWanx(ctx context.Context, req *EditRequest) (*Response, error) {
	body, err := buildWanxEditBody(req)
	if err != nil {
		return nil, err
	}
	taskID, err := c.submitTask(ctx, c.apiBase+"/services/aigc/image2image/image-synthesis", body)
	if err != nil {
		return nil, err
	}
	return c.poll(ctx, req.Model, taskID)
}

func buildWanxEditBody(req *EditRequest) (*dashscopeImageEditBody, error) {
	if len(req.Images) > 1 {
		return nil, fmt.Errorf("imagegen: %s edits a single image; got %d", req.Model, len(req.Images))
	}
	body := &dashscopeImageEditBody{
		Model: req.Model,
		Input: dashscopeImageEditInput{
			Function:     "description_edit",
			Prompt:       req.Prompt,
			BaseImageURL: dataURL(req.Images[0]),
		},
	}
	if len(req.Mask) > 0 {
		mask, err := whiteEditMask(req.Mask)
		if err != nil {
			return nil, err
		}
		body.Input.Function = "description_edit_with_mask"
		body.Input.MaskImageURL = dataURL(mask)
	}
	if req.N > 0 {
		body.Parameters = map[string]any{"n": req.N}
	}
	return body, nil
}

func (c *dashscopeClient) editQwenImage(ctx context.Context, req *EditRequest) (*Response, error) {
	body, err := buildQwenImageEditBody(req)
	if err != nil {
		return nil, err
	}
	var parsed dashscopeMultimodalResponse
	url := c.apiBase + "/services/aigc/multimodal-generation/generation"
	if err := postJSON(ctx, c.httpClient, url, c.provider.GetAccessToken(), nil, body, &parsed); err != nil {
		return nil, fmt.Errorf("imagegen: dashscope image edit: %w", err)
	}
	if parsed.Code != "" {
		return nil, fmt.Errorf("imagegen: dashscope image edit error %s: %s", parsed.Code, parsed.Message)
	}
	out := &Response{
		Created: time.Now().Unix(),
		Model:   req.Model,
		Usage:   Usage{OutputTokens: parsed.Usage.ImageCount},
	}
	for _, choice := range parsed.Output.Choices {
		for _, item := range choice.Message.Content {
			if item.Image != "" {
				out.Data = append(out.Data, Image{URL: item.Image})
			}
		}
	}
	return out, nil
}

func buildQwenImageEditBody(req *EditRequest) (*dashscopeMultimodalBody, error) {
	if len(req.Mask) > 0 {
		return nil, fmt.Errorf("imagegen: DashScope model %s has no mask; use wanx2.1-imageedit for masked edits", req.Model)
	}
	content := make([]map[string]any, 0, len(req.Images)+1)
	for _, img := range req.Images {
		content = append(content, map[string]any{"image": dataURL(img)})
	}
	content = append(content, map[string]any{"text": req.Prompt})

	body := &dashscopeMultimodalBody{Model: req.Model}
	body.Input.Messages = []dashscopeMessage{{Role: "user", Content: content}}
	params := map[string]any{}
	if req.N > 0 {
		params["n"] = req.N
	}
	if size := dashscopeSize(req.Size); size != "" {
		params["size"] = size
	}
	if len(params) > 0 {
		body.Parameters = params
	}
	return body, nil
}
