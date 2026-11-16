package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestResponsesCompactOpaqueWindow(t *testing.T) {
	const window = `{"object":"response.compaction", "output":[{"type":"message","role":"user","content":"retained"},{"type":"compaction","encrypted_content":"opaque-window","future":{"keep":true}}],"usage":{"input_tokens":10,"output_tokens":2}}`
	for _, codex := range []bool{false, true} {
		t.Run(map[bool]string{false: "openai", true: "codex"}[codex], func(t *testing.T) {
			baseURL, wantPath, model := "https://compact.test/v1/", "/v1/responses/compact", "native-model"
			if codex {
				baseURL, wantPath, model = "https://compact.test/backend-api/", "/backend-api/codex/responses/compact", "native-model:fast"
			}
			body := []byte(`{"model":"` + model + `", "input":[{"type":"reasoning","encrypted_content":"opaque-reasoning","future":7}],"instructions":"keep facts"}`)
			var transport http.RoundTripper = rtPtr(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, wantPath, req.URL.Path)
				wire, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, "opaque-reasoning", gjson.GetBytes(wire, "input.0.encrypted_content").String())
				require.Equal(t, int64(7), gjson.GetBytes(wire, "input.0.future").Int())
				require.False(t, gjson.GetBytes(wire, "input.0.id").Exists())
				require.False(t, gjson.GetBytes(wire, "stream").Exists())
				require.False(t, gjson.GetBytes(wire, "store").Exists())
				if codex {
					require.Equal(t, "native-model", gjson.GetBytes(wire, "model").String())
					require.Equal(t, "priority", gjson.GetBytes(wire, "service_tier").String())
				} else {
					require.Equal(t, string(body), string(wire))
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(window)), Request: req}, nil
			})
			if codex {
				transport = &codexRoundTripper{RoundTripper: transport}
			}
			base := &OpenAIClient{client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(baseURL), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transport}))}
			var out []byte
			var err error
			if codex {
				out, err = (&CodexClient{OpenAIClient: base}).ResponsesCompact(context.Background(), body)
			} else {
				out, err = base.ResponsesCompact(context.Background(), body)
			}
			require.NoError(t, err)
			require.Equal(t, window, string(out), "the canonical window must not be pruned or rebuilt")
		})
	}
}

func TestCodexCompactPathAndJSONProtocol(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/responses/compact":                "/codex/responses/compact",
		"/backend-api/responses/compact":       "/backend-api/codex/responses/compact",
		"/backend-api/v1/responses/compact":    "/backend-api/codex/responses/compact",
		"/backend-api/codex/responses/compact": "/backend-api/codex/responses/compact",
		"/responses/compact":                   "/responses/compact",
	} {
		t.Run(path, func(t *testing.T) {
			const body = `{"model":"native-model","prompt_cache_key":"8d964ec5-610c-4d45-83f4-418a23624c05","input":[{"type":"reasoning","id":"","encrypted_content":"opaque"}]}`
			inner := &captureRoundTripper{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"output":[]}`))}}
			req, err := http.NewRequest(http.MethodPost, "https://compact.test"+path, strings.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("X-ChatGPT-Account-ID", "test-account")
			resp, err := (&codexRoundTripper{RoundTripper: inner}).RoundTrip(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, want, inner.req.URL.Path)
			require.Equal(t, body, string(inner.body))
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, "test-account", inner.req.Header.Get("ChatGPT-Account-ID"))
			require.Equal(t, "8d964ec5-610c-4d45-83f4-418a23624c05", inner.req.Header.Get("session-id"))
			replay, err := req.GetBody()
			require.NoError(t, err)
			defer replay.Close()
			b, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.Equal(t, body, string(b))
		})
	}
}

func TestResponsesCompactRejectsInvalidUpstreamSuccess(t *testing.T) {
	for _, body := range []string{`data: {"output":[]}`, `{"error":{"message":"blocked"}}`, `{"output":null}`, `{"output":{}}`, `{"output":[],"error":{"message":"blocked"}}`} {
		transport := rtPtr(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		})
		c := &OpenAIClient{client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL("https://compact.test/v1/"), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transport}))}
		_, err := c.ResponsesCompact(context.Background(), []byte(`{"model":"m","input":[]}`))
		require.Error(t, err)
		require.Equal(t, http.StatusBadGateway, protocol.UpstreamStatus(err, 500))
	}
}

func TestResponsesCompactPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls int
	transport := rtPtr(func(req *http.Request) (*http.Response, error) { calls++; return nil, req.Context().Err() })
	c := &OpenAIClient{client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL("https://compact.test/v1/"), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transport}))}
	_, err := c.ResponsesCompact(ctx, []byte(`{"model":"m","input":"history"}`))
	require.ErrorIs(t, err, context.Canceled)
	require.LessOrEqual(t, calls, 1)
}
