package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStream(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"object":"list","data":[{"id":"gpt-x","object":"model","created":0,"owned_by":"me"}]}`)
		case "/v1/chat/completions":
			auth = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.Header().Set("Content-Type", "text/event-stream")
			for _, d := range []string{
				`{"id":"1","object":"chat.completion.chunk","created":0,"model":"gpt-x","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
				`{"id":"1","object":"chat.completion.chunk","created":0,"model":"gpt-x","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":"stop"}]}`,
				`{"id":"1","object":"chat.completion.chunk","created":0,"model":"gpt-x","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}`,
			} {
				fmt.Fprintf(w, "data: %s\n\n", d)
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(Connection{APIStyle: StyleOpenAI, BaseURL: srv.URL + "/v1", APIKey: "k1"})
	require.NoError(t, err)

	var deltas []string
	res, err := c.Stream(context.Background(), Request{
		Model:    "gpt-x",
		System:   "sys",
		Messages: []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}, {Role: "user", Content: "again"}},
	}, func(s string) { deltas = append(deltas, s) })
	require.NoError(t, err)
	assert.Equal(t, "Hello", res.Text)
	assert.Equal(t, []string{"Hel", "lo"}, deltas)
	assert.Equal(t, "stop", res.StopReason)
	assert.Equal(t, Usage{InputTokens: 11, OutputTokens: 2}, res.Usage)
	assert.Equal(t, "Bearer k1", auth)

	msgs := got["messages"].([]any)
	require.Len(t, msgs, 4)
	assert.Equal(t, "system", msgs[0].(map[string]any)["role"])

	models, err := c.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Model{{ID: "gpt-x", DisplayName: "gpt-x"}}, models)
}

func TestEndpoint(t *testing.T) {
	assert.Equal(t, "http://h/tingly/anthropic/v1/messages", Endpoint(Connection{APIStyle: StyleAnthropic, BaseURL: "http://h/tingly/anthropic/"}))
	assert.Equal(t, "http://h/tingly/openai/v1/chat/completions", Endpoint(Connection{APIStyle: StyleOpenAI, BaseURL: "http://h/tingly/openai/v1"}))
}
