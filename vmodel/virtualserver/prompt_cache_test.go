package virtualserver_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// promptCacheUsage is the cache-relevant usage of one response, read from
// whichever wire shape the endpoint speaks.
type promptCacheUsage struct {
	prompt, cached int64 // prompt includes cached
	text           string
}

func newPromptCacheServer(t *testing.T) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	virtualserver.NewService().SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv.URL
}

// promptCacheBody builds turn n of a growing conversation in the endpoint's
// request shape: turn n+1 appends to turn n.
func promptCacheBody(endpoint string, n int, stream bool) []byte {
	var msgs []map[string]any
	for i := 0; i < n; i++ {
		if i > 0 {
			msgs = append(msgs, map[string]any{"role": "assistant", "content": fmt.Sprintf("answer %d %s", i-1, strings.Repeat("x", 200))})
		}
		msgs = append(msgs, map[string]any{"role": "user", "content": fmt.Sprintf("question %d %s", i, strings.Repeat("y", 200))})
	}
	body := map[string]any{"model": vmodel.PromptCacheModelID, "stream": stream}
	switch endpoint {
	case "messages":
		// Claude Code's shape: block-list content, a cached system prompt and
		// a rolling breakpoint on the last user block. (Mixing string and list
		// content across turns would itself break the prefix.)
		for _, m := range msgs {
			m["content"] = []map[string]any{{"type": "text", "text": m["content"]}}
		}
		last := msgs[len(msgs)-1]["content"].([]map[string]any)
		last[0]["cache_control"] = map[string]any{"type": "ephemeral"}
		body["max_tokens"] = 100
		body["system"] = []map[string]any{{"type": "text", "text": strings.Repeat("system ", 100), "cache_control": map[string]any{"type": "ephemeral"}}}
		body["messages"] = msgs
	case "chat/completions":
		body["messages"] = append([]map[string]any{{"role": "system", "content": strings.Repeat("system ", 100)}}, msgs...)
		if stream {
			body["stream_options"] = map[string]any{"include_usage": true}
		}
	case "responses":
		body["instructions"] = strings.Repeat("system ", 100)
		var input []map[string]any
		for _, m := range msgs {
			input = append(input, map[string]any{"type": "message", "role": m["role"], "content": m["content"]})
		}
		body["input"] = input
	}
	raw, _ := json.Marshal(body)
	return raw
}

func postPromptCache(t *testing.T, base, endpoint string, body []byte, stream bool) promptCacheUsage {
	t.Helper()
	resp, err := http.Post(base+"/v1/"+endpoint, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))

	var u promptCacheUsage
	apply := func(m map[string]any) {
		num := func(path ...string) int64 {
			var cur any = m
			for _, p := range path {
				next, _ := cur.(map[string]any)
				cur = next[p]
			}
			f, _ := cur.(float64)
			return int64(f)
		}
		switch endpoint {
		case "messages":
			u.prompt = num("input_tokens") + num("cache_read_input_tokens") + num("cache_creation_input_tokens")
			u.cached = num("cache_read_input_tokens")
		case "chat/completions":
			u.prompt, u.cached = num("prompt_tokens"), num("prompt_tokens_details", "cached_tokens")
		case "responses":
			u.prompt, u.cached = num("input_tokens"), num("input_tokens_details", "cached_tokens")
		}
	}
	if !stream {
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		usage, _ := m["usage"].(map[string]any)
		apply(usage)
		u.text = string(raw)
		return u
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "data:"))
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if usage, ok := ev["usage"].(map[string]any); ok {
			apply(usage) // message_delta / chat usage chunk: the final word
		}
		if r, ok := ev["response"].(map[string]any); ok && ev["type"] == "response.completed" {
			usage, _ := r["usage"].(map[string]any)
			apply(usage)
		}
	}
	u.text = string(raw)
	return u
}

func TestPromptCacheModel_GrowingConversationHits(t *testing.T) {
	for _, endpoint := range []string{"messages", "chat/completions", "responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", endpoint, stream), func(t *testing.T) {
				base := newPromptCacheServer(t)

				first := postPromptCache(t, base, endpoint, promptCacheBody(endpoint, 1, stream), stream)
				assert.Positive(t, first.prompt)
				assert.Zero(t, first.cached, "nothing cached before the first request")

				second := postPromptCache(t, base, endpoint, promptCacheBody(endpoint, 3, stream), stream)
				assert.Positive(t, second.cached, "turn N+1 must read turn N's prefix")
				assert.Less(t, second.cached, second.prompt)
				if !stream { // streamed text arrives in chunks
					assert.Contains(t, second.text, "Prompt cache simulation:")
				}

				third := postPromptCache(t, base, endpoint, promptCacheBody(endpoint, 3, stream), stream)
				assert.Equal(t, third.prompt, third.cached, "an identical request is fully cached")
			})
		}
	}
}

// Without breakpoints an Anthropic request is cached automatically — how the
// third-party providers the gateway strips cache_control for behave.
func TestPromptCacheModel_AnthropicWithoutBreakpointsCachesAutomatically(t *testing.T) {
	base := newPromptCacheServer(t)
	body := []byte(`{"model":"` + vmodel.PromptCacheModelID + `","max_tokens":10,"system":"stable system prompt","messages":[{"role":"user","content":"hi"}]}`)
	first := postPromptCache(t, base, "messages", body, false)
	again := postPromptCache(t, base, "messages", body, false)
	assert.Zero(t, first.cached)
	assert.Equal(t, again.prompt, again.cached)
}
