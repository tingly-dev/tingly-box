package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestWorkBuddyMessageOrderRetryWire(t *testing.T) {
	for _, tc := range []struct {
		name, refusal string
		status, want  int
		success       bool
	}{
		{"specific ordering refusal", `{"code":11153,"msg":"the conversation must end with a user message"}`, 400, 2, true},
		{"still refused after repair", `{"code":11153,"msg":"the conversation must end with a user message"}`, 400, 2, false},
		{"channel refusal", `{"code":11128,"msg":"Illegal API invocation from an unapproved channel"}`, 400, 1, false},
		{"account restriction", `{"code":11140,"msg":"request illegal"}`, 403, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var headers []http.Header
			var bodies [][]byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				headers = append(headers, r.Header.Clone())
				bodies = append(bodies, body)
				if len(bodies) == 1 || !tc.success {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.refusal))
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"function\":{\"name\":\"fs_read\",\"arguments\":\"{}\"}}]}}]}\n\ndata: [DONE]\n\n"))
			}))
			defer server.Close()
			p := &typ.Provider{UUID: "wb", OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerWorkBuddy}}
			rt := newWorkBuddyRoundTripper(http.DefaultTransport, p, typ.SessionID{Source: typ.SessionSourceHeader, Value: "session"})
			var chat map[string]any
			require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(workbuddyToolUserChat, `"name":"read"`, `"name":"fs.read"`)), &chat))
			chat["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": "fs.read", "parameters": map[string]any{"type": "object"}}}}
			raw, _ := json.Marshal(chat)
			req, err := http.NewRequest("POST", server.URL+"/v2/chat/completions", strings.NewReader(string(raw)))
			require.NoError(t, err)
			resp, err := (&http.Client{Transport: rt}).Do(req)
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			require.Len(t, bodies, tc.want)
			require.Empty(t, req.Header.Get("X-Conversation-ID"), "transport must clone caller headers")
			if tc.want == 2 {
				require.Equal(t, headers[0].Get("X-Conversation-ID"), headers[1].Get("X-Conversation-ID"))
				require.Equal(t, headers[0].Get("X-Conversation-Request-ID"), headers[1].Get("X-Conversation-Request-ID"))
				require.NotEqual(t, headers[0].Get("X-Request-ID"), headers[1].Get("X-Request-ID"))
				_, _, messages := workbuddyChatMessages(bodies[1])
				require.Equal(t, "assistant", messages[len(messages)-2].Role)
				require.Equal(t, "user", messages[len(messages)-1].Role)
			}
			if tc.success {
				require.Equal(t, 200, resp.StatusCode)
				require.Contains(t, string(body), `"name":"fs.read"`)
			} else {
				require.Equal(t, tc.status, resp.StatusCode)
			}
		})
	}
}
