package protocoltest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newBlockingChatServer is a chat provider that holds every request until
// release() is called (or the client goes away), so a test can keep an account
// "in flight" while it sends further requests.
func newBlockingChatServer(t flagTB, content string) (baseURL string, hits *int64, release func()) {
	t.Helper()
	body := mustMarshal(map[string]any{
		"id": "chatcmpl-flag", "object": "chat.completion", "created": 0, "model": "m",
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
	gate := make(chan struct{})
	var once sync.Once
	var n int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&n, 1)
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	rel := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(func() { rel(); srv.Close() })
	return srv.URL, &n, rel
}

func waitHits(t flagTB, hits *int64, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt64(hits) < want {
		if time.Now().After(deadline) {
			t.Fatalf("upstream never reached %d in-flight request(s) (got %d)", want, atomic.LoadInt64(hits))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type asyncResult struct {
	status int
	body   string
	took   time.Duration
}

func sendFlagAsync(t flagTB, env *TestEnv, reqModel string) <-chan asyncResult {
	ch := make(chan asyncResult, 1)
	start := time.Now()
	go func() {
		path, body := flagBaseRequest(protocol.TypeOpenAIChat, reqModel, false)
		res, err := env.dispatch(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, "text", path, body, nil, false)
		if err != nil {
			ch <- asyncResult{status: -1, body: err.Error(), took: time.Since(start)}
			return
		}
		ch <- asyncResult{status: res.HTTPStatus, body: string(res.RawBody), took: time.Since(start)}
	}()
	return ch
}

func awaitResult(t flagTB, ch <-chan asyncResult, within time.Duration) asyncResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(within):
		t.Fatalf("request did not finish within %s", within)
		return asyncResult{}
	}
}

func limitedService(provider, model string, tier, maxConcurrent int) *loadbalance.Service {
	s := tieredService(provider, model, tier)
	s.Limits = &ai.Limits{MaxConcurrent: maxConcurrent}
	return s
}

// limitFlagCases cover the account-limits overflow policy flags end to end:
// real gateway, real handlers, two upstreams, one of them held in flight.
func limitFlagCases() []flagCase {
	return []flagCase{
		// limit_overflow: with A (tier 0) at its concurrency cap a second request
		// overflows to B; with a single saturated account, "reject" answers 429
		// at once and "queue" waits for the slot instead.
		{key: "limit_overflow", run: func(t flagTB, env *TestEnv) {
			// ── overflow to another account ──
			urlA, hitsA, releaseA := newBlockingChatServer(t, "from-A")
			urlB, hitsB := newCountingChatServer(t, "from-B")
			registerOpenAIProvider(env, "lim-A", urlA)
			registerOpenAIProvider(env, "lim-B", urlB)
			const ovModel = "pv-flag-limit-overflow"
			ov := newHarnessRule(ovModel, typ.ScenarioOpenAI, ovModel, "lim-model",
				limitedService("lim-A", "lim-model", 0, 1), tieredService("lim-B", "lim-model", 1))
			ov.LBTactic = tierFailoverTactic()
			_ = env.appConfig.GetGlobalConfig().AddRequestConfig(ov)

			first := sendFlagAsync(t, env, ovModel)
			waitHits(t, hitsA, 1) // A is now in flight, at its cap
			second := awaitResult(t, sendFlagAsync(t, env, ovModel), 5*time.Second)
			if second.status != 200 || !strings.Contains(second.body, "from-B") {
				t.Fatalf("second request should overflow to B: status=%d body=%s", second.status, truncate(second.body, 200))
			}
			if got := atomic.LoadInt64(hitsB); got != 1 {
				t.Errorf("expected exactly 1 hit on B, got %d", got)
			}
			releaseA()
			if r := awaitResult(t, first, 5*time.Second); r.status != 200 || !strings.Contains(r.body, "from-A") {
				t.Fatalf("first request should complete on A: status=%d body=%s", r.status, truncate(r.body, 200))
			}

			// ── slot released: A is routable again ──
			third := awaitResult(t, sendFlagAsync(t, env, ovModel), 5*time.Second)
			if third.status != 200 || !strings.Contains(third.body, "from-A") {
				t.Fatalf("after release traffic should return to A: status=%d body=%s", third.status, truncate(third.body, 200))
			}

			// ── reject: single account at its cap -> immediate 429 ──
			urlC, hitsC, releaseC := newBlockingChatServer(t, "from-C")
			registerOpenAIProvider(env, "lim-C", urlC)
			const rjModel = "pv-flag-limit-reject"
			rj := newHarnessRule(rjModel, typ.ScenarioOpenAI, rjModel, "lim-model", limitedService("lim-C", "lim-model", 0, 1))
			rj.Flags = typ.RuleFlags{LimitOverflow: typ.LimitOverflowReject}
			_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rj)
			held := sendFlagAsync(t, env, rjModel)
			waitHits(t, hitsC, 1)
			rejected := awaitResult(t, sendFlagAsync(t, env, rjModel), 3*time.Second)
			if rejected.status != http.StatusTooManyRequests || !strings.Contains(rejected.body, "rate_limit_error") {
				t.Fatalf("expected 429 rate_limit_error, got status=%d body=%s", rejected.status, truncate(rejected.body, 200))
			}
			if rejected.took > 2*time.Second {
				t.Errorf("reject must not wait; took %s", rejected.took)
			}
			releaseC()
			awaitResult(t, held, 5*time.Second)

			// ── queue (default): the second request waits and then succeeds ──
			urlD, hitsD, releaseD := newBlockingChatServer(t, "from-D")
			registerOpenAIProvider(env, "lim-D", urlD)
			const qModel = "pv-flag-limit-queue"
			q := newHarnessRule(qModel, typ.ScenarioOpenAI, qModel, "lim-model", limitedService("lim-D", "lim-model", 0, 1))
			q.Flags = typ.RuleFlags{LimitOverflow: typ.LimitOverflowQueue, LimitQueueTimeout: 10}
			_ = env.appConfig.GetGlobalConfig().AddRequestConfig(q)
			holder := sendFlagAsync(t, env, qModel)
			waitHits(t, hitsD, 1)
			waiter := sendFlagAsync(t, env, qModel)
			time.Sleep(200 * time.Millisecond)
			if atomic.LoadInt64(hitsD) != 1 {
				t.Fatalf("queued request must not reach the upstream before a slot frees (hits=%d)", atomic.LoadInt64(hitsD))
			}
			releaseD() // lets the holder finish -> slot frees -> waiter proceeds (its upstream call returns at once)
			if r := awaitResult(t, holder, 5*time.Second); r.status != 200 {
				t.Fatalf("holder failed: %d %s", r.status, truncate(r.body, 200))
			}
			if r := awaitResult(t, waiter, 5*time.Second); r.status != 200 {
				t.Fatalf("queued request should succeed once the slot frees: %d %s", r.status, truncate(r.body, 200))
			}
		}},

		// limit_queue_timeout: a queued request gives up with 429 after the
		// configured wait.
		{key: "limit_queue_timeout", run: func(t flagTB, env *TestEnv) {
			urlA, hitsA, releaseA := newBlockingChatServer(t, "from-A")
			registerOpenAIProvider(env, "lim-qt-A", urlA)
			const model = "pv-flag-limit-qtimeout"
			rule := newHarnessRule(model, typ.ScenarioOpenAI, model, "lim-model", limitedService("lim-qt-A", "lim-model", 0, 1))
			rule.Flags = typ.RuleFlags{LimitQueueTimeout: 1}
			_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

			held := sendFlagAsync(t, env, model)
			waitHits(t, hitsA, 1)
			r := awaitResult(t, sendFlagAsync(t, env, model), 5*time.Second)
			if r.status != http.StatusTooManyRequests {
				t.Fatalf("expected 429 after the queue wait, got %d %s", r.status, truncate(r.body, 200))
			}
			if r.took < 900*time.Millisecond || r.took > 4*time.Second {
				t.Errorf("expected ~1s queue wait, took %s", r.took)
			}
			releaseA()
			awaitResult(t, held, 5*time.Second)
		}},
	}
}
