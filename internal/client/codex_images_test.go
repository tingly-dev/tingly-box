package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

var testPNGBytes = []byte("\x89PNG\r\n\x1a\nfake-image-data")

func newTestCodexImageClient(rt http.RoundTripper) *CodexClient {
	sdk := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL("https://chatgpt.com/backend-api/"),
		option.WithHTTPClient(&http.Client{Transport: &codexRoundTripper{RoundTripper: rt}}),
	)
	return &CodexClient{OpenAIClient: &OpenAIClient{client: sdk}}
}

func TestCodexImagesGenerate_NativeRequestPreservesMultipleImages(t *testing.T) {
	inner := &captureRoundTripper{resp: jsonHTTPResponse(`{
		"created":1778832973,
		"background":"opaque",
		"quality":"high",
		"size":"1024x1536",
		"data":[{"b64_json":"Zmlyc3Q="},{"b64_json":"c2Vjb25k"}]
	}`)}
	client := newTestCodexImageClient(inner)
	req := openai.ImageGenerateParams{
		Prompt:  "two friendly robots",
		Model:   "gpt-image-2",
		N:       param.NewOpt(int64(2)),
		Quality: openai.ImageGenerateParamsQualityStandard,
		Size:    openai.ImageGenerateParamsSize1024x1536,
	}

	resp, err := client.ImagesGenerate(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "/backend-api/codex/images/generations", inner.req.URL.Path)
	assert.NotEmpty(t, inner.req.Header.Get("x-codex-image-turn-id"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(inner.body, &body))
	assert.Equal(t, "two friendly robots", body["prompt"])
	assert.Equal(t, "gpt-image-2", body["model"])
	assert.Equal(t, "auto", body["background"])
	assert.Equal(t, float64(2), body["n"])
	assert.Equal(t, "medium", body["quality"])
	assert.Equal(t, "1024x1536", body["size"])
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "Zmlyc3Q=", resp.Data[0].B64JSON)
	assert.Equal(t, "c2Vjb25k", resp.Data[1].B64JSON)
	assert.Equal(t, int64(1778832973), resp.Created)
	assert.Equal(t, "opaque", string(resp.Background))
	assert.Equal(t, "high", string(resp.Quality))
	assert.Equal(t, "1024x1536", string(resp.Size))
}

func TestCodexImagesGenerate_OmitsUnsetNToMatchCodexDefault(t *testing.T) {
	inner := &captureRoundTripper{resp: jsonHTTPResponse(`{"data":[{"b64_json":"Zm9v"}]}`)}
	client := newTestCodexImageClient(inner)

	_, err := client.ImagesGenerate(context.Background(), openai.ImageGenerateParams{Prompt: "cat", Model: "gpt-image-2"})
	require.NoError(t, err)
	assert.False(t, gjson.GetBytes(inner.body, "n").Exists())
}

func TestCodexImagesGenerate_EmptyDataReturnsError(t *testing.T) {
	inner := &captureRoundTripper{resp: jsonHTTPResponse(`{"data":[]}`)}
	client := newTestCodexImageClient(inner)

	_, err := client.ImagesGenerate(context.Background(), openai.ImageGenerateParams{Prompt: "cat", Model: "gpt-image-2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "returned no image data")
}

func jsonHTTPResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestBuildCodexImageEditRequest_SingleImage(t *testing.T) {
	req := &openai.ImageEditParams{
		Prompt: "add a red hat",
		Model:  "gpt-image-2",
	}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")

	out, err := buildCodexImageEditRequest(req)
	require.NoError(t, err)

	require.Len(t, out.Images, 1)
	assert.True(t, strings.HasPrefix(out.Images[0].ImageURL, "data:image/png;base64,"),
		"image_url should be a png data URL, got prefix %q", out.Images[0].ImageURL[:32])

	payload := strings.TrimPrefix(out.Images[0].ImageURL, "data:image/png;base64,")
	decoded, decErr := base64.StdEncoding.DecodeString(payload)
	require.NoError(t, decErr)
	assert.Equal(t, testPNGBytes, decoded)

	assert.Equal(t, "add a red hat", out.Prompt)
	assert.Equal(t, "gpt-image-2", out.Model)
	assert.Equal(t, "auto", out.Background)
	assert.Equal(t, "auto", out.Quality)
	assert.Equal(t, "auto", out.Size)
	assert.Nil(t, out.N)
}

func TestBuildCodexImageEditRequest_MultipleImagesAndOptions(t *testing.T) {
	req := &openai.ImageEditParams{
		Prompt:     "merge these",
		Model:      "gpt-image-2",
		Quality:    openai.ImageEditParamsQualityStandard,
		Size:       openai.ImageEditParamsSize1024x1536,
		Background: openai.ImageEditParamsBackgroundOpaque,
		N:          param.NewOpt(int64(2)),
	}
	req.Image.OfFileArray = []io.Reader{
		bytes.NewReader(testPNGBytes),
		bytes.NewReader(testPNGBytes),
	}

	out, err := buildCodexImageEditRequest(req)
	require.NoError(t, err)

	assert.Len(t, out.Images, 2)
	// "standard" is not part of the Codex quality enum (low/medium/high/auto);
	// it normalizes to "medium", matching the Responses-based generation path.
	assert.Equal(t, "medium", out.Quality)
	assert.Equal(t, "1024x1536", out.Size)
	assert.Equal(t, "opaque", out.Background)
	require.NotNil(t, out.N)
	assert.Equal(t, int64(2), *out.N)
}

func TestBuildCodexImageEditRequest_NoImage(t *testing.T) {
	req := &openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2"}
	_, err := buildCodexImageEditRequest(req)
	assert.Error(t, err)
}

func TestReaderToDataURL_EmptyContent(t *testing.T) {
	_, err := readerToDataURL(bytes.NewReader(nil))
	assert.Error(t, err)
}

func TestNormalizeCodexImageQuality(t *testing.T) {
	assert.Equal(t, "auto", normalizeCodexImageQuality(""))
	assert.Equal(t, "medium", normalizeCodexImageQuality("standard"))
	assert.Equal(t, "high", normalizeCodexImageQuality("hd"))
	assert.Equal(t, "high", normalizeCodexImageQuality("high"))
	assert.Equal(t, "low", normalizeCodexImageQuality("low"))
	assert.Equal(t, "auto", normalizeCodexImageQuality("auto"))
}

// captureRoundTripper records the inner request and returns a canned response.
type captureRoundTripper struct {
	req  *http.Request
	body []byte
	resp *http.Response
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	if req.Body != nil {
		c.body, _ = io.ReadAll(req.Body)
	}
	return c.resp, nil
}

func TestCodexRoundTripper_ImagesEditPassthrough(t *testing.T) {
	jsonResp := `{"created":1778832973,"data":[{"b64_json":"Zm9v"}]}`
	inner := &captureRoundTripper{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(jsonResp)),
		},
	}
	rt := &codexRoundTripper{RoundTripper: inner}

	body := `{"images":[{"image_url":"data:image/png;base64,Zm9v"}],"prompt":"add a red hat","model":"gpt-image-2"}`
	req, err := http.NewRequest("POST", "https://chatgpt.com/backend-api/images/edits", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ChatGPT-Account-ID", "acc-1")

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)

	// Path rewritten to the Codex-native images endpoint.
	assert.Equal(t, "/backend-api/codex/images/edits", inner.req.URL.Path)
	// The Responses-only body rules must not touch the images JSON body.
	assert.Equal(t, body, string(inner.body))
	assert.False(t, gjson.GetBytes(inner.body, "stream").Exists(), "stream must not be injected")
	assert.False(t, gjson.GetBytes(inner.body, "store").Exists(), "store must not be injected")
	// Account header transform still applies; the responses beta header does not.
	assert.Equal(t, "acc-1", inner.req.Header.Get("ChatGPT-Account-ID"))
	assert.Empty(t, inner.req.Header.Get("X-ChatGPT-Account-ID"))
	assert.Empty(t, inner.req.Header.Get("OpenAI-Beta"))
	// JSON response passes through un-mangled (no SSE enforcement).
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, jsonResp, string(respBody))
}

func TestCodexRoundTripper_ImagesErrorStatusSurfaced(t *testing.T) {
	inner := &captureRoundTripper{
		resp: &http.Response{
			StatusCode: http.StatusBadRequest,
			Status:     "400 Bad Request",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"bad image"}`)),
		},
	}
	rt := &codexRoundTripper{RoundTripper: inner}

	req, err := http.NewRequest("POST", "https://chatgpt.com/backend-api/images/edits", strings.NewReader(`{}`))
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad image")
}

func TestRewriteCodexPath_Images(t *testing.T) {
	path, protocol := rewriteCodexPath("/backend-api/images/edits")
	assert.Equal(t, "/backend-api/codex/images/edits", path)
	assert.Equal(t, codexProtocolPlainJSON, protocol)

	path, protocol = rewriteCodexPath("/backend-api/images/generations")
	assert.Equal(t, "/backend-api/codex/images/generations", path)
	assert.Equal(t, codexProtocolPlainJSON, protocol)

	// Already-canonical paths are untouched but still classified as plain JSON.
	path, protocol = rewriteCodexPath("/backend-api/codex/images/edits")
	assert.Equal(t, "/backend-api/codex/images/edits", path)
	assert.Equal(t, codexProtocolPlainJSON, protocol)
}

func TestRewriteCodexPath_ResponsesProtocol(t *testing.T) {
	path, protocol := rewriteCodexPath("/backend-api/responses")
	assert.Equal(t, "/backend-api/codex/responses", path)
	assert.Equal(t, codexProtocolResponsesSSE, protocol)
}
