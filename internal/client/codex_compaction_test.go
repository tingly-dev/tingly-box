package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the actual SDK -> Codex defaults -> HTTP filter -> SSE decoder path.
// A mock upstream checks fidelity; it cannot validate or decrypt real ciphertext.
func TestCodexCompactionReplay(t *testing.T) {
	for _, outcome := range []string{"completed", "http_error", "stream_error"} {
		t.Run(outcome, func(t *testing.T) {
			calls := 0
			transport := &codexRoundTripper{RoundTripper: rtPtr(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "/backend-api/codex/responses", req.URL.Path)
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				input := gjson.GetBytes(body, "input").Array()
				require.Len(t, input, 4)
				require.Equal(t, "opaque-reasoning", input[0].Get("encrypted_content").String())
				require.False(t, input[0].Get("id").Exists())
				require.Equal(t, "web_search_call", input[1].Get("type").String())
				require.Equal(t, "opaque-prior-compaction", input[2].Get("encrypted_content").String())
				require.Equal(t, "compaction_trigger", input[3].Get("type").String())
				require.Equal(t, "web_search", gjson.GetBytes(body, "tools.0.type").String())
				require.False(t, gjson.GetBytes(body, "store").Bool())
				require.True(t, gjson.GetBytes(body, "stream").Bool())

				status, contentType := http.StatusOK, "text/event-stream"
				payload := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_compacted\",\"status\":\"completed\",\"output\":[{\"type\":\"compaction\",\"encrypted_content\":\"opaque-new-compaction\"}]}}\n\n"
				if outcome == "http_error" {
					status, contentType = http.StatusBadGateway, "application/json"
					payload = `{"error":{"message":"response protection is unavailable","type":"internal_error","param":null,"code":null}}`
				} else if outcome == "stream_error" {
					payload = "event: error\ndata: {\"error\":{\"message\":\"response protection is unavailable\",\"type\":\"internal_error\",\"param\":null,\"code\":null}}\n\n"
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
			})}
			c := &CodexClient{OpenAIClient: &OpenAIClient{client: openai.NewClient(
				option.WithBaseURL("https://codex.test/backend-api/"),
				option.WithAPIKey("test"), option.WithMaxRetries(0),
				option.WithHTTPClient(&http.Client{Transport: transport}),
			)}}
			var params responses.ResponseNewParams
			require.NoError(t, json.Unmarshal([]byte(`{"model":"gpt-5-codex","tools":[{"type":"web_search"}],"input":[{"type":"reasoning","summary":[],"encrypted_content":"opaque-reasoning"},{"type":"web_search_call","id":"ws_valid","status":"completed","action":{"type":"search","query":"test"}},{"type":"compaction","encrypted_content":"opaque-prior-compaction"},{"type":"compaction_trigger"}]}`), &params))
			stream := c.ResponsesNewStreaming(context.Background(), params)
			defer stream.Close()
			completed := false
			for stream.Next() {
				event := stream.Current()
				if event.Type == "response.completed" {
					completed = true
					require.Equal(t, "opaque-new-compaction", gjson.Get(event.RawJSON(), "response.output.0.encrypted_content").String())
				}
			}
			if outcome == "completed" {
				require.NoError(t, stream.Err())
				require.True(t, completed)
			} else {
				require.Error(t, stream.Err())
				require.False(t, completed)
				if outcome == "http_error" {
					var apiErr *openai.Error
					require.ErrorAs(t, stream.Err(), &apiErr)
					require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
					require.Equal(t, "response protection is unavailable", apiErr.Message)
					require.Equal(t, "internal_error", apiErr.Type)
				} else {
					require.ErrorContains(t, stream.Err(), "response protection is unavailable")
				}
			}
			require.Equal(t, 1, calls)
		})
	}
}

// Codex 0.163.0 compact.rs builds a local summarization Prompt with default
// (empty) tools, retaining hosted history. This reconstructs that request shape;
// the injected upstream failure verifies its diagnostic path, not its cause.
func TestCodexLocalCompactionErrorSignature(t *testing.T) {
	const upstreamError = `{"error":{"message":"response protection is unavailable","type":"internal_error","param":null,"code":null}}`
	transport := &codexRoundTripper{RoundTripper: rtPtr(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/backend-api/codex/responses", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		input := gjson.GetBytes(body, "input").Array()
		require.Len(t, input, 3)
		require.Equal(t, "rs_valid", input[0].Get("id").String())
		require.Equal(t, "opaque-reasoning", input[0].Get("encrypted_content").String())
		require.Equal(t, "web_search_call", input[1].Get("type").String())
		require.Equal(t, "message", input[2].Get("type").String())
		require.Empty(t, gjson.GetBytes(body, "tools").Array())
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + upstreamError + "\n\n")), Request: req}, nil
	})}
	c := &CodexClient{OpenAIClient: &OpenAIClient{client: openai.NewClient(
		option.WithBaseURL("https://codex.test/backend-api/"), option.WithAPIKey("test"),
		option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transport}),
	)}}
	var params responses.ResponseNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gpt-5-codex","input":[{"type":"reasoning","id":"rs_valid","summary":[],"encrypted_content":"opaque-reasoning"},{"type":"web_search_call","id":"ws_valid","status":"completed","action":{"type":"search","query":"test"}},{"type":"message","role":"user","content":"Summarize the conversation."}]}`), &params))
	stream := c.ResponsesNewStreaming(context.Background(), params)
	defer stream.Close()
	require.False(t, stream.Next())
	require.EqualError(t, stream.Err(), `received error while streaming: {"message":"response protection is unavailable","type":"internal_error","param":null,"code":null}`)
}
