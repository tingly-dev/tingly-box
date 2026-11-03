package protocoltest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestLimits_StreamCancelReleasesSlot: a streaming request holds its account
// slot for as long as the stream runs; when the client disconnects mid-stream
// the slot must come back (no counter leak), so the next request is served.
func TestLimits_StreamCancelReleasesSlot(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Close()

	chunk := func(delta map[string]any, finish any) string {
		return "data: " + string(mustMarshal(map[string]any{
			"id": "c", "object": "chat.completion.chunk", "created": 0, "model": "m",
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		})) + "\n\n"
	}
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, chunk(map[string]any{"role": "assistant", "content": "hi"}, nil))
		w.(http.Flusher).Flush()
		if n == 1 {
			<-r.Context().Done() // stream never finishes on its own
			return
		}
		_, _ = io.WriteString(w, chunk(map[string]any{}, "stop"))
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	registerOpenAIProvider(env, "lim-stream", srv.URL)

	const model = "pv-limit-stream-cancel"
	rule := newHarnessRule(model, typ.ScenarioOpenAI, model, "lim-model", limitedService("lim-stream", "lim-model", 0, 1))
	rule.Flags = typ.RuleFlags{LimitOverflow: typ.LimitOverflowReject}
	require.NoError(t, env.appConfig.GetGlobalConfig().AddRequestConfig(rule))

	path, body := flagBaseRequest(protocol.TypeOpenAIChat, model, true)
	newReq := func(ctx context.Context) *http.Request {
		req, err := http.NewRequestWithContext(ctx, "POST", env.gatewayServer.URL+path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+env.modelToken)
		return req
	}

	ctx, cancel := context.WithCancel(context.Background())
	resp, err := http.DefaultClient.Do(newReq(ctx))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	buf := make([]byte, 16)
	_, err = resp.Body.Read(buf) // first bytes arrived: stream is live
	require.NoError(t, err)

	// slot is held for the whole stream
	r2, err := http.DefaultClient.Do(newReq(context.Background()))
	require.NoError(t, err)
	b2, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, r2.StatusCode, string(b2))
	require.NotEmpty(t, r2.Header.Get("Retry-After"))
	require.True(t, strings.Contains(string(b2), "rate_limit_error"))

	// client goes away mid-stream
	cancel()
	resp.Body.Close()

	require.Eventually(t, func() bool {
		r3, err := http.DefaultClient.Do(newReq(context.Background()))
		if err != nil {
			return false
		}
		_, _ = io.Copy(io.Discard, r3.Body)
		r3.Body.Close()
		return r3.StatusCode == 200
	}, 5*time.Second, 50*time.Millisecond, "slot must be released after the client cancelled mid-stream")
}

// TestLimits_UsageWindowShiftsTraffic: a request budget on account A makes the
// gateway route to B once A's window is used up, without any in-flight request.
func TestLimits_UsageWindowShiftsTraffic(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Close()

	urlA, hitsA := newCountingChatServer(t, "from-A")
	urlB, hitsB := newCountingChatServer(t, "from-B")
	registerOpenAIProvider(env, "use-A", urlA)
	registerOpenAIProvider(env, "use-B", urlB)

	const model = "pv-limit-usage-window"
	a := tieredService("use-A", "lim-model", 0)
	a.Limits = &ai.Limits{MaxRequests: 2, WindowSeconds: 600}
	rule := newHarnessRule(model, typ.ScenarioOpenAI, model, "lim-model", a, tieredService("use-B", "lim-model", 1))
	rule.LBTactic = tierFailoverTactic()
	require.NoError(t, env.appConfig.GetGlobalConfig().AddRequestConfig(rule))

	for i := 0; i < 5; i++ {
		res := sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
		require.Equal(t, 200, res.HTTPStatus, string(res.RawBody))
	}
	require.Equal(t, int64(2), atomic.LoadInt64(hitsA), "A serves exactly its request budget")
	require.Equal(t, int64(3), atomic.LoadInt64(hitsB), "the rest shifts to B")
}
