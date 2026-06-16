package virtualserver_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/vmodel"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	virtualserver "github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// sequenceConfig is the canonical 200,200,429 program used across the tests.
func sequenceConfig(id string) *vmodel.SequenceConfig {
	return &vmodel.SequenceConfig{
		ID:             id,
		Name:           "Seq Test",
		DefaultContent: "sequenced ok",
		Steps: []vmodel.SequenceStep{
			{Status: 200},
			{Status: 200},
			{Status: 429},
		},
	}
}

// TestSequence_OpenAI_NonStreaming verifies a sequence model cycles
// 200,200,429,200 across successive non-streaming OpenAI requests.
func TestSequence_OpenAI_NonStreaming(t *testing.T) {
	model := openaivm.NewSequenceModel(sequenceConfig("seq-openai"))
	srv := newInjectionServer(t, model, nil)

	want := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests, http.StatusOK}
	for i, w := range want {
		resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
			"model":    "seq-openai",
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equalf(t, w, resp.StatusCode, "request %d: body=%s", i, body)
		if w == http.StatusOK {
			require.Contains(t, string(body), "sequenced ok")
		} else {
			require.Contains(t, string(body), "rate_limit_error")
		}
	}
}

// TestSequence_OpenAI_Streaming verifies the same cycle on the streaming path:
// success requests stream content + [DONE]; the 429 request short-circuits as a
// pre-content error with no stream.
func TestSequence_OpenAI_Streaming(t *testing.T) {
	model := openaivm.NewSequenceModel(sequenceConfig("seq-openai-stream"))
	srv := newInjectionServer(t, model, nil)

	want := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests}
	for i, w := range want {
		resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
			"model":    "seq-openai-stream",
			"stream":   true,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equalf(t, w, resp.StatusCode, "request %d: body=%s", i, body)
		if w == http.StatusOK {
			require.Contains(t, string(body), "[DONE]", "request %d should stream", i)
		} else {
			require.NotContains(t, string(body), "[DONE]")
			require.Contains(t, string(body), "rate_limit_error")
		}
	}
}

// TestSequence_Anthropic_NonStreaming mirrors the OpenAI non-streaming test.
func TestSequence_Anthropic_NonStreaming(t *testing.T) {
	model := anthropicvm.NewSequenceModel(sequenceConfig("seq-anthropic"))
	srv := newInjectionServer(t, nil, model)

	want := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests, http.StatusOK}
	for i, w := range want {
		resp := postJSON(t, srv.URL+"/v1/messages?beta=true", map[string]any{
			"model":      "seq-anthropic",
			"max_tokens": 16,
			"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equalf(t, w, resp.StatusCode, "request %d: body=%s", i, body)
		if w == http.StatusOK {
			require.Contains(t, string(body), "sequenced ok")
		} else {
			require.Contains(t, string(body), "rate_limit_error")
		}
	}
}

// TestSequence_StatusFactory verifies the NewStatusSequence shorthand wires a
// status-only program end-to-end (Anthropic side; OpenAI is symmetric).
func TestSequence_StatusFactory(t *testing.T) {
	model := anthropicvm.NewStatusSequence("seq-factory", "Seq Factory", 200, 503)
	require.Equal(t, vmodel.VirtualModelTypeSequence, model.GetType())
	srv := newInjectionServer(t, nil, model)

	want := []int{http.StatusOK, http.StatusServiceUnavailable, http.StatusOK}
	for i, w := range want {
		resp := postJSON(t, srv.URL+"/v1/messages?beta=true", map[string]any{
			"model":      "seq-factory",
			"max_tokens": 16,
			"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equalf(t, w, resp.StatusCode, "request %d: body=%s", i, body)
	}
}

// TestSequence_DefaultDemoRegistered verifies the user-facing demo sequence
// (virtual-sequence-429) ships in both default registries and behaves as
// advertised (200,200,429).
func TestSequence_DefaultDemoRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := virtualserver.NewService()
	openaivm.RegisterDefaults(svc.GetOpenAIRegistry())
	anthropicvm.RegisterDefaults(svc.GetAnthropicRegistry())
	engine := gin.New()
	svc.SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	// Present in the models list for both protocols.
	resp, err := http.Get(srv.URL + "/v1/models")
	require.NoError(t, err)
	listBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, 2, strings.Count(string(listBody), "virtual-sequence-429"),
		"demo sequence should appear in both protocol registries")

	want := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests}
	for i, w := range want {
		r := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
			"model":    "virtual-sequence-429",
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		r.Body.Close()
		require.Equalf(t, w, r.StatusCode, "demo request %d", i)
	}
}
