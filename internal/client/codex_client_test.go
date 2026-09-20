package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
)

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
