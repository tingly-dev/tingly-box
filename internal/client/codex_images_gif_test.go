package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/vision/imagefmt"
	"github.com/tingly-dev/tingly-box/internal/vision/imagegen"
)

const gifB64 = "R0lGODlhAQABAAAAACw=" // "GIF89a" + payload

type recordedRequest struct {
	path    string
	body    []byte
	headers http.Header
}

// fakeCodexBackend answers the Codex-native images endpoints with a canned GIF
// and records what it was sent. Anything else is a test failure: a PNG request
// must not touch it, and the GIF path must not fall back to the Responses tool.
func fakeCodexBackend(t *testing.T) (*CodexClient, func() []recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var got []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, recordedRequest{path: r.URL.Path, body: body, headers: r.Header.Clone()})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"` + gifB64 + `","generation_id":"g1"}]}`))
	}))
	t.Cleanup(srv.Close)

	oc := openai.NewClient(
		option.WithBaseURL(srv.URL+"/backend-api/"),
		option.WithAPIKey("test"),
		option.WithHTTPClient(&http.Client{Transport: &codexRoundTripper{RoundTripper: http.DefaultTransport}}),
	)
	c := &CodexClient{OpenAIClient: &OpenAIClient{client: oc}}
	return c, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), got...)
	}
}

func TestCodexImagesGenerate_GIFUsesNativeEndpoint(t *testing.T) {
	c, requests := fakeCodexBackend(t)
	ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF, FrameCount: 6, FrameDurationMS: 150})

	resp, err := c.ImagesGenerate(ctx, openai.ImageGenerateParams{
		Prompt:  "a dancing cat",
		Model:   "gpt-image-2",
		Size:    openai.ImageGenerateParamsSize1024x1024,
		Quality: openai.ImageGenerateParamsQualityHigh,
	})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, gifB64, resp.Data[0].B64JSON)

	reqs := requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/backend-api/codex/images/generations", reqs[0].path)
	assert.NotEmpty(t, reqs[0].headers.Get("x-codex-image-turn-id"))
	b := reqs[0].body
	assert.Equal(t, "a dancing cat", gjson.GetBytes(b, "prompt").String())
	assert.Equal(t, "gpt-image-2", gjson.GetBytes(b, "model").String())
	assert.Equal(t, "gif", gjson.GetBytes(b, "output_format").String())
	assert.EqualValues(t, 6, gjson.GetBytes(b, "frame_count").Int())
	assert.EqualValues(t, 150, gjson.GetBytes(b, "frame_duration_ms").Int())
	// Codex's own GIF request always carries size/quality "auto" and an opaque
	// or transparent background, never an explicit size or "auto" background.
	assert.Equal(t, "auto", gjson.GetBytes(b, "size").String())
	assert.Equal(t, "auto", gjson.GetBytes(b, "quality").String())
	assert.Equal(t, "opaque", gjson.GetBytes(b, "background").String())
	assert.False(t, gjson.GetBytes(b, "stream").Exists(), "the native endpoint is plain JSON")
	assert.False(t, gjson.GetBytes(b, "tools").Exists())
}

func TestCodexImagesGenerate_GIFDefaultsOmitFrameFields(t *testing.T) {
	c, requests := fakeCodexBackend(t)
	ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF})

	_, err := c.ImagesGenerate(ctx, openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2"})
	require.NoError(t, err)

	b := requests()[0].body
	assert.Equal(t, "gif", gjson.GetBytes(b, "output_format").String())
	assert.False(t, gjson.GetBytes(b, "frame_count").Exists(), "unset means the upstream's defaults (9 frames)")
	assert.False(t, gjson.GetBytes(b, "frame_duration_ms").Exists())
	assert.Equal(t, "auto", gjson.GetBytes(b, "size").String())
	assert.Equal(t, "auto", gjson.GetBytes(b, "quality").String())
	assert.Equal(t, "opaque", gjson.GetBytes(b, "background").String())
}

func TestCodexImagesGenerate_GIFRequestMatchesCodexsOwnShape(t *testing.T) {
	for _, tc := range []struct{ name, background, want string }{
		{"unset is opaque", "", "opaque"},
		{"auto is opaque", "auto", "opaque"},
		{"opaque", "opaque", "opaque"},
		{"transparent is kept", "transparent", "transparent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, requests := fakeCodexBackend(t)
			ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF})
			_, err := c.ImagesGenerate(ctx, openai.ImageGenerateParams{
				Prompt: "x", Model: "gpt-image-2",
				Size: openai.ImageGenerateParamsSize1536x1024, Quality: openai.ImageGenerateParamsQualityLow,
				Background: openai.ImageGenerateParamsBackground(tc.background),
			})
			require.NoError(t, err)
			b := requests()[0].body
			assert.Equal(t, tc.want, gjson.GetBytes(b, "background").String())
			assert.Equal(t, "auto", gjson.GetBytes(b, "size").String(), "an explicit size is not forwarded for GIF")
			assert.Equal(t, "auto", gjson.GetBytes(b, "quality").String())
		})
	}
}

// An upstream 400 with no usable JSON body must still leave the operator
// something to go on: the request ids and content type from the response.
func TestCodexImagesGenerate_GIFEmptyBodyRejectionCarriesResponseDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-codex-imagegen-request-id", "req-abc123")
		w.Header().Set("x-request-id", "gw-77")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	oc := openai.NewClient(
		option.WithBaseURL(srv.URL+"/backend-api/"),
		option.WithAPIKey("test"),
		option.WithHTTPClient(&http.Client{Transport: &codexRoundTripper{RoundTripper: http.DefaultTransport}}),
	)
	c := &CodexClient{OpenAIClient: &OpenAIClient{client: oc}}

	ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF})
	_, err := c.ImagesGenerate(ctx, openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "req-abc123")
	assert.Contains(t, err.Error(), "gw-77")
	assert.Contains(t, err.Error(), "empty response body")
	assert.Equal(t, http.StatusBadRequest, protocol.ClassifyUpstreamFailure(err, http.StatusInternalServerError).Status, "classification by status is unchanged")
}

func TestCodexImagesGenerate_GIFFansOutForN(t *testing.T) {
	c, requests := fakeCodexBackend(t)
	ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF})
	req := openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2", N: param.NewOpt[int64](3)}

	resp, err := c.ImagesGenerate(ctx, req)
	require.NoError(t, err)
	assert.Len(t, resp.Data, 3)
	assert.Len(t, requests(), 3)
	for _, r := range requests() {
		assert.False(t, gjson.GetBytes(r.body, "n").Exists(), "one image per native call")
	}
}

func TestCodexImagesGenerate_GIFUpstreamRejectionIsClear(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"output_format gif is not enabled for this account"}}`))
	}))
	defer srv.Close()
	oc := openai.NewClient(
		option.WithBaseURL(srv.URL+"/backend-api/"),
		option.WithAPIKey("test"),
		option.WithHTTPClient(&http.Client{Transport: &codexRoundTripper{RoundTripper: http.DefaultTransport}}),
	)
	c := &CodexClient{OpenAIClient: &OpenAIClient{client: oc}}

	ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF})
	_, err := c.ImagesGenerate(ctx, openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2"})
	require.Error(t, err)
	failure := protocol.ClassifyUpstreamFailure(err, http.StatusInternalServerError)
	assert.Equal(t, http.StatusBadRequest, failure.Status)
	assert.Contains(t, failure.Message, "not enabled for this account", "the upstream's own reason must reach the caller")
}

func TestCodexResponsesToolSendsTheRequestedOutputFormat(t *testing.T) {
	c := &CodexClient{}
	for _, tc := range []struct {
		name string
		req  openai.ImageGenerateParams
		want string
	}{
		{"default is png", openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2"}, "png"},
		{"jpeg", openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2", OutputFormat: openai.ImageGenerateParamsOutputFormatJPEG}, "jpeg"},
		{"webp", openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2", OutputFormat: openai.ImageGenerateParamsOutputFormatWebP}, "webp"},
		{"response_format is not an output format", openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2", ResponseFormat: openai.ImageGenerateParamsResponseFormatB64JSON}, "png"},
		{"url response_format is not an output format", openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2", ResponseFormat: openai.ImageGenerateParamsResponseFormatURL, OutputFormat: openai.ImageGenerateParamsOutputFormatWebP}, "webp"},
		{"gif never goes to the tool", openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2", OutputFormat: "gif"}, "png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := c.buildImageGenerationResponsesRequest(tc.req)
			require.Len(t, params.Tools, 1)
			require.NotNil(t, params.Tools[0].OfImageGeneration)
			assert.Equal(t, tc.want, params.Tools[0].OfImageGeneration.OutputFormat)
		})
	}
}

func TestCodexImagesEdit_GIFOutputGoesToTheNativeEndpoint(t *testing.T) {
	c, requests := fakeCodexBackend(t)
	ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF, FrameCount: 8, FrameDurationMS: 100})

	req := openai.ImageEditParams{Prompt: "animate the sprites", Model: "gpt-image-2"}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "sheet.png", "image/png")
	resp, err := c.ImagesEdit(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, gifB64, resp.Data[0].B64JSON)

	reqs := requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/backend-api/codex/images/edits", reqs[0].path)
	b := reqs[0].body
	assert.Equal(t, "gif", gjson.GetBytes(b, "output_format").String())
	assert.EqualValues(t, 8, gjson.GetBytes(b, "frame_count").Int())
	assert.EqualValues(t, 100, gjson.GetBytes(b, "frame_duration_ms").Int())
	assert.Equal(t, "opaque", gjson.GetBytes(b, "background").String(), "Codex never sends an auto background for GIF")
	assert.Equal(t, "auto", gjson.GetBytes(b, "size").String())
	assert.Equal(t, "auto", gjson.GetBytes(b, "quality").String())
}

func TestCodexImagesEdit_PNGRequestsStayUnchanged(t *testing.T) {
	c, requests := fakeCodexBackend(t)
	req := openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2"}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "in.png", "image/png")
	_, err := c.ImagesEdit(context.Background(), req)
	require.NoError(t, err)
	b := requests()[0].body
	assert.False(t, gjson.GetBytes(b, "output_format").Exists())
	assert.False(t, gjson.GetBytes(b, "frame_count").Exists())
}

func TestCodexImagesEdit_RejectsGIFInput(t *testing.T) {
	c, requests := fakeCodexBackend(t)
	req := openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2"}
	req.Image.OfFile = openai.File(bytes.NewReader([]byte("GIF89a\x01\x00\x01\x00")), "anim.gif", "image/gif")
	_, err := c.ImagesEdit(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spritesheet")
	assert.Empty(t, requests(), "nothing may be sent upstream")
}

func TestCodexImagesEdit_GIFOutputWithMaskIsRefused(t *testing.T) {
	c, requests := fakeCodexBackend(t)
	ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF})
	req := openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2"}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "in.png", "image/png")
	req.Mask = openai.File(bytes.NewReader(testPNGBytes), "mask.png", "image/png")
	_, err := c.ImagesEdit(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mask")
	assert.Empty(t, requests())
}
