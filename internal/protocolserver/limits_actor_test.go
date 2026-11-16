package protocolserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func withPolicies(t *testing.T, guard *routing.LimitsGuard) *db.LimitPolicyStore {
	t.Helper()
	store, err := db.NewLimitPolicyStore(t.TempDir())
	require.NoError(t, err)
	guard.SetPolicies(store)
	return store
}

// A sharing-key request streams for a while. While the stream is open the
// person's concurrency slot is held (a second request of the same person gets
// 429 user_limit_exceeded + Retry-After, another person is served); when the
// client disconnects mid-stream the slot comes back.
func TestStreamingRequestHoldsPersonSlotUntilClientCancels(t *testing.T) {
	h, guard, _ := limitsTestHandler(t)
	store := withPolicies(t, guard)
	_, _, err := store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyUser, Ref: "alice", Limits: ai.Limits{MaxConcurrent: 1}, Overflow: ai.LimitOverflowReject})
	require.NoError(t, err)

	rule := &typ.Rule{UUID: "r1", Scenario: typ.ScenarioOpenAI, RequestModel: "m",
		Services: []*loadbalance.Service{{Provider: "acct-a", Model: "m", Active: true}}}

	engine := gin.New()
	engine.Use(func(c *gin.Context) { // stand-in for the sharing-key auth middleware
		c.Set(constant.CtxKeyUserID, c.GetHeader("X-Test-User"))
		c.Set(constant.CtxKeyTeamID, "t1")
		c.Next()
	}, h.limitsMiddleware)
	engine.POST("/chat", func(c *gin.Context) {
		if _, _, err := h.selectService(c, typ.ScenarioOpenAI, rule, nil); err != nil {
			writeSelectionError(c, err)
			return
		}
		c.Header("Content-Type", "text/event-stream")
		c.Status(200)
		flusher := c.Writer.(http.Flusher)
		for { // an upstream stream that never ends on its own
			_, _ = io.WriteString(c.Writer, "data: tick\n\n")
			flusher.Flush()
			select {
			case <-c.Request.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	})
	srv := httptest.NewServer(engine)
	defer srv.Close()

	do := func(ctx context.Context, user string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/chat", strings.NewReader("{}"))
		req.Header.Set("X-Test-User", user)
		return http.DefaultClient.Do(req)
	}
	inflight := func() int { return guard.Registry().Stat(routing.UserLimitKey("alice")).Inflight }

	ctx, cancel := context.WithCancel(context.Background())
	resp, err := do(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	buf := make([]byte, 12)
	_, err = io.ReadFull(resp.Body, buf)
	require.NoError(t, err)
	require.Equal(t, 1, inflight(), "slot held while the stream is live")

	r2, err := do(context.Background(), "alice")
	require.NoError(t, err)
	body, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, r2.StatusCode, string(body))
	require.NotEmpty(t, r2.Header.Get("Retry-After"))
	require.Contains(t, string(body), `"code":"user_limit_exceeded"`)

	bobCtx, bobCancel := context.WithCancel(context.Background())
	rb, err := do(bobCtx, "bob")
	require.NoError(t, err)
	require.Equal(t, 200, rb.StatusCode, "another person is not limited")
	bobCancel()
	rb.Body.Close()

	cancel() // alice's client goes away mid-stream
	resp.Body.Close()
	require.Eventually(t, func() bool { return inflight() == 0 }, 3*time.Second, 20*time.Millisecond,
		"the slot must be released after a mid-stream disconnect")

	r3, err := do(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, 200, r3.StatusCode, "…and she can start a new request")
	r3.Body.Close()
}

// Failover swaps the candidate permit of the failed attempt but keeps the
// person's permit: it is still one request by one person.
func TestFailoverKeepsActorPermit(t *testing.T) {
	loadbalance.DefaultBreakerStore().Reset()
	h, guard, _ := limitsTestHandler(t)
	store := withPolicies(t, guard)
	_, _, err := store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyUser, Ref: "alice", Limits: ai.Limits{MaxConcurrent: 1}})
	require.NoError(t, err)

	a := &loadbalance.Service{Provider: "acct-a", Model: "m", Active: true, Tier: 0, Limits: &ai.Limits{MaxConcurrent: 1}}
	b := &loadbalance.Service{Provider: "acct-b", Model: "m", Active: true, Tier: 1, Limits: &ai.Limits{MaxConcurrent: 1}}
	rule := &typ.Rule{UUID: "fo", Scenario: typ.ScenarioOpenAI, RequestModel: "m",
		LBTactic: typ.Tactic{Type: loadbalance.TacticTier, Params: &typ.TierParams{WithinTierTactic: loadbalance.TacticRandom}},
		Services: []*loadbalance.Service{a, b}}
	for _, s := range rule.Services {
		s.InitializeStats()
	}

	c := ginCtx()
	c.Set(constant.CtxKeyUserID, "alice")
	_, _, err = h.selectService(c, typ.ScenarioOpenAI, rule, nil) // gate 1 + gate 2 -> acct-a
	require.NoError(t, err)
	st := func(key string) int { return guard.Registry().Stat(key).Inflight }
	require.Equal(t, 1, st(routing.UserLimitKey("alice")))
	require.Equal(t, 1, st("svc:fo/acct-a/m"))

	tried := map[string]bool{a.ServiceID(): true}
	prov, svc, err := h.selectFallbackServiceLimited(c, rule, tried, a.ServiceID())
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.Equal(t, "acct-b", prov.UUID)
	require.Equal(t, 0, st("svc:fo/acct-a/m"), "failed attempt's account slot is freed")
	require.Equal(t, 1, st("svc:fo/acct-b/m"))
	require.Equal(t, 1, st(routing.UserLimitKey("alice")), "the person's permit survives failover, and is not taken twice")

	routing.ReleaseLease(c)
	require.Equal(t, 0, st(routing.UserLimitKey("alice")))
	require.Equal(t, 0, st("svc:fo/acct-b/m"))
}

func TestRecordLimitUsageFeedsPersonAndTeamWindows(t *testing.T) {
	h, guard, _ := limitsTestHandler(t)
	store := withPolicies(t, guard)
	_, _, err := store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyUser, Ref: "alice", Limits: ai.Limits{MaxTokens: 1000, WindowSeconds: 600}})
	require.NoError(t, err)
	_, _, err = store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyTeam, Ref: "t1", Limits: ai.Limits{MaxTokens: 5000, WindowSeconds: 600}})
	require.NoError(t, err)

	svc := &loadbalance.Service{Provider: "acct-a", Model: "m", Active: true}
	rule := &typ.Rule{UUID: "r", Services: []*loadbalance.Service{svc}}
	prov, _ := h.deps.Config.GetProviderByUUID("acct-a")

	c := ginCtx()
	c.Set(constant.CtxKeyUserID, "alice")
	c.Set(constant.CtxKeyTeamID, "t1")
	h.recordLimitUsage(c, rule, prov, "m", 400)
	h.recordLimitUsage(c, rule, prov, "m", 700)
	require.Equal(t, int64(1100), guard.Registry().Stat(routing.UserLimitKey("alice")).Tokens)
	require.Equal(t, int64(1100), guard.Registry().Stat(routing.TeamLimitKey("t1")).Tokens)

	// alice is now over her token budget -> next request is a user 429; a teammate is fine
	c2 := ginCtx()
	c2.Set(constant.CtxKeyUserID, "alice")
	c2.Set(constant.CtxKeyTeamID, "t1")
	_, _, err = h.selectService(c2, typ.ScenarioOpenAI, rule, nil)
	var le *routing.LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t, "user_limit_exceeded", le.Code())
	require.Equal(t, limits.ReasonUsage, le.Reasons)
	c3 := ginCtx()
	c3.Set(constant.CtxKeyUserID, "bob")
	c3.Set(constant.CtxKeyTeamID, "t1")
	_, _, err = h.selectService(c3, typ.ScenarioOpenAI, rule, nil)
	require.NoError(t, err)
}

func TestCountTokensDoesNotConsumePersonBudget(t *testing.T) {
	h, guard, _ := limitsTestHandler(t)
	store := withPolicies(t, guard)
	_, _, err := store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyUser, Ref: "alice", Limits: ai.Limits{MaxRequests: 1, WindowSeconds: 600}})
	require.NoError(t, err)
	rule := &typ.Rule{UUID: "r", Scenario: typ.ScenarioOpenAI, RequestModel: "m",
		Services: []*loadbalance.Service{{Provider: "acct-a", Model: "m", Active: true}}}

	c := ginCtx()
	c.Set(constant.CtxKeyUserID, "alice")
	c.Set(constant.CtxKeyLimitActorDone, true) // what the count_tokens handler does
	_, _, err = h.selectService(c, typ.ScenarioOpenAI, rule, nil)
	require.NoError(t, err)
	require.Equal(t, int64(0), guard.Registry().Stat(routing.UserLimitKey("alice")).Requests)
}
