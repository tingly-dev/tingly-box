package virtualserver_test

import (
	"bytes"
	"encoding/json"
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

// newInjectionServer wires a virtualserver with the supplied openai and
// anthropic mocks registered, and returns it as an httptest server under /v1.
func newInjectionServer(t *testing.T, openai openaivm.VirtualModel, anthropic anthropicvm.VirtualModel) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := virtualserver.NewService()
	if openai != nil {
		require.NoError(t, svc.GetOpenAIRegistry().Register(openai))
	}
	if anthropic != nil {
		require.NoError(t, svc.GetAnthropicRegistry().Register(anthropic))
	}

	engine := gin.New()
	svc.SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	return resp
}

// TestErrorInjection_PreContent_OpenAI verifies that a pre-content injection
// short-circuits ChatCompletions with the configured HTTP status and an
// OpenAI-shaped error envelope, regardless of whether the request asked
// for streaming.
func TestErrorInjection_PreContent_OpenAI(t *testing.T) {
	model := openaivm.NewMockModel(&openaivm.MockModelConfig{
		ID:      "inj-precontent-openai",
		Content: "should never be seen",
		Error: &vmodel.ErrorInjection{
			Stage:   vmodel.ErrorStagePreContent,
			Status:  http.StatusTooManyRequests,
			Message: "rate limited",
			Type:    "rate_limit_error",
		},
	})
	srv := newInjectionServer(t, model, nil)

	for _, streaming := range []bool{false, true} {
		streaming := streaming
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[streaming], func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
				"model":    "inj-precontent-openai",
				"stream":   streaming,
				"messages": []map[string]string{{"role": "user", "content": "hi"}},
			})
			defer resp.Body.Close()

			require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
			b, _ := io.ReadAll(resp.Body)
			var env struct {
				Error struct{ Message, Type string } `json:"error"`
			}
			require.NoError(t, json.Unmarshal(b, &env))
			require.Equal(t, "rate limited", env.Error.Message)
			require.Equal(t, "rate_limit_error", env.Error.Type)
		})
	}
}

// TestErrorInjection_PreContent_Anthropic mirrors the OpenAI test but
// confirms the Anthropic envelope shape ({"type":"error","error":{...}}).
func TestErrorInjection_PreContent_Anthropic(t *testing.T) {
	model := anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
		ID:      "inj-precontent-anthropic",
		Content: "should never be seen",
		Error: &vmodel.ErrorInjection{
			Stage:   vmodel.ErrorStagePreContent,
			Status:  http.StatusServiceUnavailable,
			Message: "overloaded",
			Type:    "overloaded_error",
		},
	})
	srv := newInjectionServer(t, nil, model)

	resp := postJSON(t, srv.URL+"/v1/messages?beta=true", map[string]any{
		"model":      "inj-precontent-anthropic",
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	b, _ := io.ReadAll(resp.Body)
	var env struct {
		Type  string `json:"type"`
		Error struct{ Message, Type string }
	}
	require.NoError(t, json.Unmarshal(b, &env))
	require.Equal(t, "error", env.Type)
	require.Equal(t, "overloaded", env.Error.Message)
	require.Equal(t, "overloaded_error", env.Error.Type)
}

// TestErrorInjection_PreContent_Defaults verifies the resolveErrorFields
// fallbacks: zero status → 500, empty type → "api_error", empty message →
// a stage-specific generic label.
func TestErrorInjection_PreContent_Defaults(t *testing.T) {
	model := openaivm.NewMockModel(&openaivm.MockModelConfig{
		ID:      "inj-defaults",
		Content: "irrelevant",
		Error:   &vmodel.ErrorInjection{Stage: vmodel.ErrorStagePreContent},
	})
	srv := newInjectionServer(t, model, nil)
	resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
		"model":    "inj-defaults",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	b, _ := io.ReadAll(resp.Body)
	var env struct {
		Error struct{ Message, Type string } `json:"error"`
	}
	require.NoError(t, json.Unmarshal(b, &env))
	require.Equal(t, "api_error", env.Error.Type)
	require.Equal(t, "simulated pre-content error", env.Error.Message)
}

// TestErrorInjection_MidStream_OpenAI_ErrorEvent verifies that an OpenAI
// stream emits AfterEvents real chunks, then a final SSE error frame
// containing the configured message — and no [DONE] terminator after.
func TestErrorInjection_MidStream_OpenAI_ErrorEvent(t *testing.T) {
	model := openaivm.NewMockModel(&openaivm.MockModelConfig{
		ID:           "inj-mid-openai-err",
		StreamChunks: []string{"a", "b", "c", "d"},
		Error: &vmodel.ErrorInjection{
			Stage:         vmodel.ErrorStageMidStream,
			AfterEvents:   2,
			MidStreamMode: vmodel.MidStreamModeErrorEvent,
			Message:       "boom",
		},
	})
	srv := newInjectionServer(t, model, nil)

	resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
		"model":    "inj-mid-openai-err",
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	b, _ := io.ReadAll(resp.Body)
	body := string(b)

	// Each delta event is emitted as one `data:` SSE line. Count them.
	// The mid-stream gate trips after 2, so we expect exactly 2 content chunks.
	contentChunks := strings.Count(body, `"delta":{"content":`)
	require.Equal(t, 2, contentChunks, "expected exactly 2 content chunks before mid-stream trip; body=%q", body)

	require.Contains(t, body, `"message":"boom"`)
	require.NotContains(t, body, "[DONE]", "no [DONE] terminator after mid-stream error")
}

// TestErrorInjection_MidStream_OpenAI_ConnectionClose verifies that a
// connection-close injection causes the client to receive a truncated body
// (no SSE error frame; the TCP connection is hijacked and closed).
func TestErrorInjection_MidStream_OpenAI_ConnectionClose(t *testing.T) {
	model := openaivm.NewMockModel(&openaivm.MockModelConfig{
		ID:           "inj-mid-openai-close",
		StreamChunks: []string{"x", "y", "z"},
		Error: &vmodel.ErrorInjection{
			Stage:         vmodel.ErrorStageMidStream,
			AfterEvents:   1,
			MidStreamMode: vmodel.MidStreamModeConnectionClose,
		},
	})
	srv := newInjectionServer(t, model, nil)

	resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
		"model":    "inj-mid-openai-close",
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	require.Equal(t, 1, strings.Count(body, `"delta":{"content":`), "expected exactly 1 chunk before close; body=%q", body)
	require.NotContains(t, body, "[DONE]")
	require.NotContains(t, body, `"error"`, "connection-close mode must not emit an SSE error frame")
}

// TestErrorInjection_MidStream_Anthropic_ErrorEvent verifies the Anthropic
// "event: error" frame shape after the configured number of real events.
func TestErrorInjection_MidStream_Anthropic_ErrorEvent(t *testing.T) {
	model := anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
		ID:           "inj-mid-anthropic-err",
		StreamChunks: []string{"alpha", "beta", "gamma"},
		Error: &vmodel.ErrorInjection{
			Stage:         vmodel.ErrorStageMidStream,
			AfterEvents:   1,
			MidStreamMode: vmodel.MidStreamModeErrorEvent,
			Message:       "anthropic-boom",
			Type:          "overloaded_error",
		},
	})
	srv := newInjectionServer(t, nil, model)

	resp := postJSON(t, srv.URL+"/v1/messages?beta=true", map[string]any{
		"model":      "inj-mid-anthropic-err",
		"stream":     true,
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	require.Contains(t, body, "event: error")
	require.Contains(t, body, `"message":"anthropic-boom"`)
	require.Contains(t, body, `"type":"overloaded_error"`)
	// message_stop is the normal terminator; mid-stream trip should skip it.
	require.NotContains(t, body, "event: message_stop")
}

// TestRegisterErrorMocks_OpenAI verifies that the opt-in registration helper
// wires all four error-injection variants into the OpenAI registry, and that
// each one trips its configured behavior end-to-end.
func TestRegisterErrorMocks_OpenAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := virtualserver.NewService()
	openaivm.RegisterErrorMocks(svc.GetOpenAIRegistry())
	engine := gin.New()
	svc.SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	t.Run("precontent-429", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
			"model":    "virtual-fail-precontent-429",
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		defer resp.Body.Close()
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	})

	t.Run("precontent-500", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
			"model":    "virtual-fail-precontent-500",
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		defer resp.Body.Close()
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("midstream-close", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
			"model":    "virtual-fail-midstream-close",
			"stream":   true,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		b, _ := io.ReadAll(resp.Body)
		body := string(b)
		require.NotContains(t, body, "[DONE]")
		require.NotContains(t, body, `"error"`)
	})

	t.Run("midstream-event", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/chat/completions", map[string]any{
			"model":    "virtual-fail-midstream-event",
			"stream":   true,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		b, _ := io.ReadAll(resp.Body)
		body := string(b)
		require.Contains(t, body, `"message":"simulated mid-stream error"`)
		require.NotContains(t, body, "[DONE]")
	})
}

// TestRegisterErrorMocks_Anthropic mirrors the OpenAI variant for the
// Anthropic registry.
func TestRegisterErrorMocks_Anthropic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := virtualserver.NewService()
	anthropicvm.RegisterErrorMocks(svc.GetAnthropicRegistry())
	engine := gin.New()
	svc.SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	body := func(model string, stream bool) map[string]any {
		return map[string]any{
			"model":      model,
			"stream":     stream,
			"max_tokens": 16,
			"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		}
	}

	t.Run("precontent-429", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/messages?beta=true", body("virtual-fail-precontent-429", false))
		defer resp.Body.Close()
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	})

	t.Run("precontent-500", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/messages?beta=true", body("virtual-fail-precontent-500", false))
		defer resp.Body.Close()
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("midstream-close", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/messages?beta=true", body("virtual-fail-midstream-close", true))
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		b, _ := io.ReadAll(resp.Body)
		txt := string(b)
		require.NotContains(t, txt, "event: message_stop")
		require.NotContains(t, txt, "event: error")
	})

	t.Run("midstream-event", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/v1/messages?beta=true", body("virtual-fail-midstream-event", true))
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		b, _ := io.ReadAll(resp.Body)
		txt := string(b)
		require.Contains(t, txt, "event: error")
		require.Contains(t, txt, `"message":"simulated mid-stream error"`)
		require.NotContains(t, txt, "event: message_stop")
	})
}

// TestErrorInjection_MidStream_Anthropic_ConnectionClose verifies that the
// Anthropic streaming path also supports TCP-level close.
func TestErrorInjection_MidStream_Anthropic_ConnectionClose(t *testing.T) {
	model := anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
		ID:           "inj-mid-anthropic-close",
		StreamChunks: []string{"one", "two", "three"},
		Error: &vmodel.ErrorInjection{
			Stage:         vmodel.ErrorStageMidStream,
			AfterEvents:   2,
			MidStreamMode: vmodel.MidStreamModeConnectionClose,
		},
	})
	srv := newInjectionServer(t, nil, model)

	resp := postJSON(t, srv.URL+"/v1/messages?beta=true", map[string]any{
		"model":      "inj-mid-anthropic-close",
		"stream":     true,
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	require.NotContains(t, body, "event: message_stop")
	require.NotContains(t, body, "event: error")
}
