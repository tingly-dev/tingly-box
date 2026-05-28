package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// minimaxClient implements image generation against MiniMax (image-01 family).
// MiniMax exposes a synchronous, bespoke endpoint — NOT the OpenAI
// /images/generations contract — at POST {APIBase}/image_generation, keyed by
// aspect_ratio rather than pixel size.
//
// Reference: https://platform.minimax.io/docs/guides/image-generation
type minimaxClient struct {
	UnsupportedEdit
	provider    *typ.Provider
	httpClient  *http.Client
	endpointURL string
}

func newMinimaxClient(provider *typ.Provider) (*minimaxClient, error) {
	base := strings.TrimRight(provider.APIBase, "/")
	if base == "" {
		return nil, fmt.Errorf("imagegen: minimax provider %q has no API base", provider.Name)
	}
	return &minimaxClient{
		provider:    provider,
		httpClient:  &http.Client{Transport: http.DefaultTransport},
		endpointURL: base + "/image_generation",
	}, nil
}

func (c *minimaxClient) Provider() *typ.Provider { return c.provider }

func (c *minimaxClient) Vendor() Vendor { return VendorMinimax }

func (c *minimaxClient) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

type minimaxRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	N              int    `json:"n,omitempty"`
}

type minimaxResponse struct {
	ID   string `json:"id"`
	Data struct {
		ImageURLs   []string `json:"image_urls"`
		ImageBase64 []string `json:"image_base64"`
	} `json:"data"`
	Metadata struct {
		SuccessCount json.Number `json:"success_count"`
	} `json:"metadata"`
	BaseResp struct {
		StatusCode int64  `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func (c *minimaxClient) Generate(ctx context.Context, req *Request) (*Response, error) {
	body := minimaxRequest{
		Model:          req.Model,
		Prompt:         req.Prompt,
		N:              req.N,
		ResponseFormat: minimaxResponseFormat(req.ResponseFormat),
		AspectRatio:    minimaxAspectRatio(req),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("imagegen: minimax marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.provider.GetAccessToken())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("imagegen: minimax request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imagegen: minimax returned %d: %s", resp.StatusCode, string(raw))
	}

	var parsed minimaxResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("imagegen: minimax parse response: %w", err)
	}
	// MiniMax reports business errors inside base_resp with HTTP 200.
	if parsed.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("imagegen: minimax error %d: %s", parsed.BaseResp.StatusCode, parsed.BaseResp.StatusMsg)
	}

	out := &Response{Created: time.Now().Unix(), Model: req.Model}
	for _, u := range parsed.Data.ImageURLs {
		out.Data = append(out.Data, Image{URL: u})
	}
	for _, b := range parsed.Data.ImageBase64 {
		out.Data = append(out.Data, Image{B64JSON: b})
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("imagegen: minimax returned no images (id: %s)", parsed.ID)
	}
	return out, nil
}

func minimaxResponseFormat(format string) string {
	switch strings.ToLower(format) {
	case "b64_json", "base64":
		return "base64"
	case "url", "":
		return "url"
	default:
		return format
	}
}

// minimaxSupportedRatios is the fixed set of aspect ratios MiniMax accepts.
var minimaxSupportedRatios = map[string]bool{
	"1:1": true, "16:9": true, "4:3": true, "3:2": true,
	"2:3": true, "3:4": true, "9:16": true, "21:9": true,
}

// minimaxStandardSizes maps the common OpenAI / DALL-E pixel sizes (which do
// not reduce cleanly to a MiniMax ratio) onto the closest supported ratio.
var minimaxStandardSizes = map[string]string{
	"1024x1024": "1:1",
	"1792x1024": "16:9",
	"1024x1792": "9:16",
	"1536x1024": "3:2",
	"1024x1536": "2:3",
}

// minimaxAspectRatio resolves the aspect_ratio MiniMax expects. An explicit
// Extra["aspect_ratio"] wins; otherwise it is derived from the normalized
// "WIDTHxHEIGHT" size — first via the standard-size table, then via a reduced
// ratio — falling back to the upstream default for anything unsupported.
func minimaxAspectRatio(req *Request) string {
	if req.Extra != nil {
		if ar, ok := req.Extra["aspect_ratio"].(string); ok && ar != "" {
			return ar
		}
	}
	size := strings.ToLower(strings.TrimSpace(req.Size))
	if ar, ok := minimaxStandardSizes[size]; ok {
		return ar
	}
	w, h, ok := parseSize(size)
	if !ok || w == 0 || h == 0 {
		return ""
	}
	g := gcd(w, h)
	ar := fmt.Sprintf("%d:%d", w/g, h/g)
	if minimaxSupportedRatios[ar] {
		return ar
	}
	logrus.Warnf("[MiniMax] size %q maps to unsupported aspect ratio %q, using upstream default", req.Size, ar)
	return ""
}

func parseSize(size string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return w, h, true
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}
