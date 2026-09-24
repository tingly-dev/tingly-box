package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

var testPNGBytes = []byte("\x89PNG\r\n\x1a\nfake-image-data")

func TestBuildCodexImageEditRequest_SingleImage(t *testing.T) {
	req := &openai.ImageEditParams{
		Prompt: "add a red hat",
		Model:  "gpt-image-2",
	}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")

	out, err := buildCodexImageEditRequest(t.Context(), req)
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

	out, err := buildCodexImageEditRequest(t.Context(), req)
	require.NoError(t, err)

	assert.Len(t, out.Images, 2)
	// "standard" is not part of the Codex quality enum (low/medium/high/auto);
	// it normalizes to "medium", matching the Responses-based generation path.
	assert.Equal(t, "medium", out.Quality)
	assert.Equal(t, "1024x1536", out.Size)
	assert.Equal(t, "opaque", out.Background)
	// n is served by fanning out one-image calls, never sent on the wire.
	body, err := json.Marshal(out)
	require.NoError(t, err)
	assert.False(t, gjson.GetBytes(body, "n").Exists())
}

func TestBuildCodexImageEditRequest_NoImage(t *testing.T) {
	req := &openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2"}
	_, err := buildCodexImageEditRequest(t.Context(), req)
	assert.Error(t, err)
}

func TestReaderToDataURL_EmptyContent(t *testing.T) {
	_, err := readerToDataURL(t.Context(), bytes.NewReader(nil))
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

	// Handed back as a response, so the SDK builds an *openai.Error that keeps
	// the status and body instead of a transport error it would retry.
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(respBody), "bad image")
}

// A policy block on the Codex images endpoint is a 400 with the reason in the
// body. It must reach the caller as that 400 with that reason, sent once — not
// retried as a dropped connection and reported as network_error / 502.
func TestCodexImagesEdit_UpstreamRejectionKeepsStatusAndMessage(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Your request was rejected by the safety system","code":"moderation_blocked"}}`))
	}))
	defer srv.Close()

	oc := openai.NewClient(
		option.WithBaseURL(srv.URL+"/backend-api/"),
		option.WithAPIKey("test"),
		option.WithHTTPClient(&http.Client{Transport: &codexRoundTripper{RoundTripper: http.DefaultTransport}}),
	)
	c := &CodexClient{OpenAIClient: &OpenAIClient{client: oc}}

	req := openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2"}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")
	_, err := c.ImagesEdit(context.Background(), req)
	require.Error(t, err)

	failure := protocol.ClassifyUpstreamFailure(err, http.StatusInternalServerError)
	assert.Equal(t, http.StatusBadRequest, failure.Status)
	assert.Contains(t, failure.Message, "rejected by the safety system")
	assert.EqualValues(t, 1, atomic.LoadInt32(&hits), "a rejected request must not be retried")
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
