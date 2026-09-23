package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newCodexImagesTestClient builds a real *CodexClient (unmodified
// ImagesGenerate/parseImageGenerationStream) whose HTTP calls go to a local
// test server through the real codexRoundTripper, so the following tests
// exercise the whole production chain end-to-end rather than each hop in
// isolation.
func newCodexImagesTestClient(t *testing.T, handler http.HandlerFunc) *CodexClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	provider := &typ.Provider{APIBase: server.URL}
	base, err := NewOpenAIClient(provider, "gpt-image-2", typ.SessionID{}, option.WithHTTPClient(&http.Client{
		Transport: &codexRoundTripper{RoundTripper: http.DefaultTransport},
	}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = base.Close() })

	return &CodexClient{OpenAIClient: base}
}

// newCodexImagesTestClientAt is newCodexImagesTestClient but lets the caller
// pick the API base path. The native images/edits endpoint only classifies
// as codexProtocolPlainJSON (see rewriteCodexAPIPath) when the request path
// starts with "/backend-api/images/", so ImagesEdit needs APIBase to end in
// "/backend-api" the way the real Codex OAuth provider's does — plain
// server.URL (used for the Responses-API-based ImagesGenerate tests) isn't
// representative here.
func newCodexImagesTestClientAt(t *testing.T, apiBaseSuffix string, handler http.HandlerFunc) *CodexClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	provider := &typ.Provider{APIBase: server.URL + apiBaseSuffix}
	base, err := NewOpenAIClient(provider, "gpt-image-2", typ.SessionID{}, option.WithHTTPClient(&http.Client{
		Transport: &codexRoundTripper{RoundTripper: http.DefaultTransport},
	}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = base.Close() })

	return &CodexClient{OpenAIClient: base}
}

// TestCodexImagesEdit_E2E_NonOKStatusSurfacesRealCause is the end-to-end
// proof for the native Codex images/edits endpoint (codex_images.go), the
// other image-gen path besides the Responses-API-based ImagesGenerate.
// ImagesEdit wraps the SDK error with fmt.Errorf("codex image edit failed:
// %w", err), so this also confirms that extra wrap doesn't break errors.As's
// walk down to the underlying *openai.Error, mirroring
// TestCodexImagesGenerate_E2E_PreStreamErrorSurfacesRealCause for the other
// image-gen entry point.
func TestCodexImagesEdit_E2E_NonOKStatusSurfacesRealCause(t *testing.T) {
	var gotPath string
	client := newCodexImagesTestClientAt(t, "/backend-api", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Your request was rejected due to content policy.","code":"content_policy_violation"}}`))
	})

	req := &openai.ImageEditParams{Prompt: "add a red hat", Model: "gpt-image-2"}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")

	_, err := client.ImagesEdit(context.Background(), *req)
	require.Error(t, err)

	// Confirms the request actually took the native plain-JSON images route
	// (codexProtocolPlainJSON), not the Responses API path — the two are
	// classified by URL path in rewriteCodexAPIPath.
	assert.Equal(t, "/backend-api/codex/images/edits", gotPath)

	failure := protocol.ClassifyUpstreamFailure(err, http.StatusInternalServerError)
	assert.Equal(t, http.StatusBadRequest, failure.Status, "must be the real 400, not a flattened 500/502")
	assert.Contains(t, failure.Message, "content_policy_violation")
	assert.Contains(t, failure.Message, "Your request was rejected due to content policy.")
	assert.NotContains(t, failure.Message, "network_error")
}

// TestCodexImagesGenerate_E2E_PreStreamErrorSurfacesRealCause is the
// end-to-end proof for the pre-stream failure mode: Codex rejects the
// request outright (non-200) before any SSE stream opens. It drives the real,
// unmodified CodexClient.ImagesGenerate against a local server — no mocked
// stream, no isolated unit — and checks what a caller actually receives
// after classification, the same call error_response.go makes for every
// client-facing failure.
func TestCodexImagesGenerate_E2E_PreStreamErrorSurfacesRealCause(t *testing.T) {
	client := newCodexImagesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Your request was rejected due to content policy.","code":"content_policy_violation"}}`))
	})

	_, err := client.ImagesGenerate(context.Background(), openai.ImageGenerateParams{
		Model:  "gpt-image-2",
		Prompt: "a cat wearing a hat",
	})
	require.Error(t, err)

	failure := protocol.ClassifyUpstreamFailure(err, http.StatusInternalServerError)
	assert.Equal(t, http.StatusBadRequest, failure.Status, "must be the real 400, not a flattened 500/502")
	assert.Contains(t, failure.Message, "content_policy_violation")
	assert.Contains(t, failure.Message, "Your request was rejected due to content policy.")
	assert.NotContains(t, failure.Message, "network_error")
}

// TestCodexImagesGenerate_E2E_MidStreamFailureSurfacesRealCause is the
// end-to-end proof for the other failure mode: the HTTP request succeeds
// (200, a real SSE stream opens) but the stream itself carries a terminal
// response.failed event — e.g. a moderation rejection discovered only after
// generation started.
func TestCodexImagesGenerate_E2E_MidStreamFailureSurfacesRealCause(t *testing.T) {
	client := newCodexImagesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: " + `{"type":"response.failed","sequence_number":1,"response":{"id":"resp_1","status":"failed","error":{"code":"content_policy_violation","message":"Your request was rejected by the safety system."}}}` + "\n\n"))
	})

	_, err := client.ImagesGenerate(context.Background(), openai.ImageGenerateParams{
		Model:  "gpt-image-2",
		Prompt: "a cat wearing a hat",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content_policy_violation")
	assert.Contains(t, err.Error(), "Your request was rejected by the safety system.")
}

// newImageGenSSEStream builds a stream from raw SSE event bodies, mirroring
// what a real Responses API image-generation stream would send.
func newImageGenSSEStream(t *testing.T, events ...string) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	t.Helper()
	var body strings.Builder
	for _, e := range events {
		body.WriteString("data: ")
		body.WriteString(e)
		body.WriteString("\n\n")
	}
	res := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body.String())),
	}
	return ssestream.NewStream[responses.ResponseStreamEventUnion](ssestream.NewDecoder(res), nil)
}

func TestParseImageGenerationStream_SurfacesResponseFailedError(t *testing.T) {
	stream := newImageGenSSEStream(t, `{"type":"response.failed","sequence_number":1,"response":{"id":"resp_1","status":"failed","error":{"code":"content_policy_violation","message":"Your request was rejected by the safety system."}}}`)

	c := &CodexClient{}
	_, err := c.parseImageGenerationStream(context.Background(), stream)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Your request was rejected by the safety system.") {
		t.Fatalf("expected error to surface upstream message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "content_policy_violation") {
		t.Fatalf("expected error to surface upstream code, got: %v", err)
	}
}

func TestParseImageGenerationStream_SurfacesTopLevelError(t *testing.T) {
	stream := newImageGenSSEStream(t, `{"type":"error","sequence_number":1,"code":"rate_limit_exceeded","message":"Too many requests.","param":""}`)

	c := &CodexClient{}
	_, err := c.parseImageGenerationStream(context.Background(), stream)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Too many requests.") {
		t.Fatalf("expected error to surface upstream message, got: %v", err)
	}
}

func TestParseImageGenerationStream_SurfacesFailedImageCallStatus(t *testing.T) {
	stream := newImageGenSSEStream(t, `{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"image_generation_call","id":"ig_1","status":"failed","result":""}}`)

	c := &CodexClient{}
	_, err := c.parseImageGenerationStream(context.Background(), stream)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ig_1") {
		t.Fatalf("expected error to reference image call id, got: %v", err)
	}
}

func TestApplyCodexDefaultsToParams_FastSuffix(t *testing.T) {
	req := responses.ResponseNewParams{Model: "gpt-5.6-sol:fast"}

	applyCodexDefaultsToParams(&req)

	if req.Model != "gpt-5.6-sol" {
		t.Fatalf("expected model to be stripped to %q, got %q", "gpt-5.6-sol", req.Model)
	}
	if req.ServiceTier != responses.ResponseNewParamsServiceTierPriority {
		t.Fatalf("expected service tier %q, got %q", responses.ResponseNewParamsServiceTierPriority, req.ServiceTier)
	}
}

func TestApplyCodexDefaultsToParams_NoFastSuffix(t *testing.T) {
	req := responses.ResponseNewParams{Model: "gpt-5.6-sol"}

	applyCodexDefaultsToParams(&req)

	if req.Model != "gpt-5.6-sol" {
		t.Fatalf("expected model to remain %q, got %q", "gpt-5.6-sol", req.Model)
	}
	if req.ServiceTier != "" {
		t.Fatalf("expected empty service tier, got %q", req.ServiceTier)
	}
}

// TestApplyCodexDefaultsToParams_ServiceTierSurvivesWireBody guards against the
// request body being silently stripped of "service_tier" by the second body-shaping
// pass in codexRoundTripper.filterField (a deny-list JSON filter applied to the
// already-marshaled body, separate from the SDK struct marshaling).
func TestApplyCodexDefaultsToParams_ServiceTierSurvivesWireBody(t *testing.T) {
	req := responses.ResponseNewParams{Model: "gpt-5.6-sol:fast"}
	applyCodexDefaultsToParams(&req)

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	rt := &codexRoundTripper{}
	filtered, err := rt.filterField(raw)
	if err != nil {
		t.Fatalf("filterField failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(filtered, &out); err != nil {
		t.Fatalf("unmarshal filtered body: %v", err)
	}

	if out["service_tier"] != "priority" {
		t.Fatalf("expected service_tier=priority in final wire body, got %v (full body: %s)", out["service_tier"], filtered)
	}
	if out["model"] != "gpt-5.6-sol" {
		t.Fatalf("expected model=gpt-5.6-sol in final wire body, got %v", out["model"])
	}
}
