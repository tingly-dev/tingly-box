package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// dashscopeClient implements image generation against Alibaba Model Studio /
// DashScope (Tongyi Wanxiang "Wan" models and qwen-image). DashScope's
// text-to-image API is asynchronous: a submit call returns a task_id, and the
// caller polls a task endpoint until the task reaches a terminal state.
//
// Endpoints (host derived from the provider APIBase, so both the Beijing
// dashscope.aliyuncs.com and the Singapore dashscope-intl.aliyuncs.com sites
// are supported):
//
//	POST {scheme}://{host}/api/v1/services/aigc/text2image/image-synthesis
//	     header: X-DashScope-Async: enable
//	GET  {scheme}://{host}/api/v1/tasks/{task_id}
//
// Reference: https://www.alibabacloud.com/help/en/model-studio/text-to-image
type dashscopeClient struct {
	provider     *typ.Provider
	httpClient   *http.Client
	submitURL    string
	taskBaseURL  string
	pollInterval time.Duration
	pollTimeout  time.Duration
}

func newDashScopeClient(provider *typ.Provider) (*dashscopeClient, error) {
	host := apiHost(provider.APIBase)
	if host == "" {
		return nil, fmt.Errorf("imagegen: dashscope provider %q has no API base host", provider.Name)
	}
	scheme := apiScheme(provider.APIBase)
	base := fmt.Sprintf("%s://%s/api/v1", scheme, host)

	timeout := time.Duration(provider.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &dashscopeClient{
		provider:     provider,
		httpClient:   &http.Client{Transport: http.DefaultTransport},
		submitURL:    base + "/services/aigc/text2image/image-synthesis",
		taskBaseURL:  base + "/tasks/",
		pollInterval: 2 * time.Second,
		pollTimeout:  timeout,
	}, nil
}

func (c *dashscopeClient) Provider() *typ.Provider { return c.provider }

func (c *dashscopeClient) Vendor() Vendor { return VendorDashScope }

func (c *dashscopeClient) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

// dashscopeSubmitBody is the async task submission payload.
type dashscopeSubmitBody struct {
	Model      string                 `json:"model"`
	Input      dashscopeInput         `json:"input"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type dashscopeInput struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
}

// dashscopeTaskResponse covers both the submit response and the poll response.
type dashscopeTaskResponse struct {
	RequestID string `json:"request_id"`
	Output    struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Results    []struct {
			URL      string `json:"url"`
			B64Image string `json:"b64_image"`
			Code     string `json:"code"`
			Message  string `json:"message"`
		} `json:"results"`
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"output"`
	Usage struct {
		ImageCount int64 `json:"image_count"`
	} `json:"usage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (c *dashscopeClient) Generate(ctx context.Context, req *Request) (*Response, error) {
	taskID, err := c.submit(ctx, req)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("[DashScope] image task submitted: %s", taskID)
	return c.poll(ctx, req.Model, taskID)
}

func (c *dashscopeClient) submit(ctx context.Context, req *Request) (string, error) {
	params := map[string]interface{}{}
	if n := req.N; n > 0 {
		params["n"] = n
	}
	if size := dashscopeSize(req.Size); size != "" {
		params["size"] = size
	}
	// Allow callers to pass DashScope-native knobs (seed, prompt_extend,
	// watermark, ...) straight through.
	for k, v := range req.Extra {
		params[k] = v
	}

	body := dashscopeSubmitBody{
		Model:      req.Model,
		Input:      dashscopeInput{Prompt: req.Prompt},
		Parameters: params,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("imagegen: dashscope marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.submitURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.provider.GetAccessToken())
	httpReq.Header.Set("X-DashScope-Async", "enable")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("imagegen: dashscope submit: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imagegen: dashscope submit returned %d: %s", resp.StatusCode, string(raw))
	}

	var parsed dashscopeTaskResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("imagegen: dashscope parse submit response: %w", err)
	}
	if parsed.Code != "" {
		return "", fmt.Errorf("imagegen: dashscope submit error %s: %s", parsed.Code, parsed.Message)
	}
	if parsed.Output.TaskID == "" {
		return "", fmt.Errorf("imagegen: dashscope submit returned no task_id")
	}
	return parsed.Output.TaskID, nil
}

func (c *dashscopeClient) poll(ctx context.Context, model, taskID string) (*Response, error) {
	deadline := time.Now().Add(c.pollTimeout)
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		parsed, err := c.fetchTask(ctx, taskID)
		if err != nil {
			return nil, err
		}

		switch strings.ToUpper(parsed.Output.TaskStatus) {
		case "SUCCEEDED":
			return c.toResponse(model, parsed), nil
		case "FAILED", "CANCELED", "UNKNOWN":
			msg := parsed.Output.Message
			if msg == "" {
				msg = parsed.Message
			}
			return nil, fmt.Errorf("imagegen: dashscope task %s %s: %s", taskID, parsed.Output.TaskStatus, msg)
		}

		// PENDING / RUNNING — keep polling.
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("imagegen: dashscope task %s timed out after %s", taskID, c.pollTimeout)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *dashscopeClient) fetchTask(ctx context.Context, taskID string) (*dashscopeTaskResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.taskBaseURL+taskID, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.provider.GetAccessToken())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("imagegen: dashscope poll: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imagegen: dashscope poll returned %d: %s", resp.StatusCode, string(raw))
	}
	var parsed dashscopeTaskResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("imagegen: dashscope parse poll response: %w", err)
	}
	return &parsed, nil
}

func (c *dashscopeClient) toResponse(model string, parsed *dashscopeTaskResponse) *Response {
	out := &Response{
		Created: time.Now().Unix(),
		Model:   model,
		Usage:   Usage{OutputTokens: parsed.Usage.ImageCount},
	}
	for _, r := range parsed.Output.Results {
		if r.URL == "" && r.B64Image == "" {
			continue
		}
		out.Data = append(out.Data, Image{URL: r.URL, B64JSON: r.B64Image})
	}
	return out
}

// dashscopeSize converts a normalized "WIDTHxHEIGHT" size into DashScope's
// "WIDTH*HEIGHT" form. Empty / non-conforming values are passed through so the
// upstream can apply its own default.
func dashscopeSize(size string) string {
	size = strings.TrimSpace(size)
	if size == "" {
		return ""
	}
	return strings.ReplaceAll(strings.ToLower(size), "x", "*")
}
