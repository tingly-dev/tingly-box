package routing

import (
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type mockPolicies struct {
	mu sync.Mutex
	m  map[string]ai.LimitPolicy
}

func newMockPolicies() *mockPolicies { return &mockPolicies{m: map[string]ai.LimitPolicy{}} }

func (p *mockPolicies) set(kind ai.LimitPolicyKind, ref string, l ai.Limits, overflow string, timeoutSec int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m[string(kind)+"\x00"+ref] = ai.LimitPolicy{Kind: kind, Ref: ref, Limits: l, Overflow: overflow, QueueTimeoutSeconds: timeoutSec}
}

func (p *mockPolicies) GetLimitPolicy(kind ai.LimitPolicyKind, ref string) (ai.LimitPolicy, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.m[string(kind)+"\x00"+ref]
	return v, ok
}

// actorEnv is limitsEnv plus a policy store; selection goes through SimpleSelector
// with a gin context carrying the caller's identity, like the real middleware.
type actorEnv struct {
	*limitsEnv
	policies *mockPolicies
	simple   *SimpleSelector
	rule     *typ.Rule
}

func newActorEnv(t *testing.T, providerLimits map[string]*ai.Limits, uuids ...string) *actorEnv {
	e := newLimitsEnv(providerLimits, uuids...)
	pol := newMockPolicies()
	e.guard.SetPolicies(pol)
	svcs := make([]*loadbalance.Service, 0, len(uuids))
	for _, u := range uuids {
		s := testService(u, "m", true)
		s.InitializeStats()
		svcs = append(svcs, s)
	}
	return &actorEnv{limitsEnv: e, policies: pol, simple: NewSimpleSelector(e.sel), rule: testRule("r1", "m", svcs)}
}

func (e *actorEnv) ctxFor(t *testing.T, user, team string) *gin.Context {
	c := ginCtxWithHeader(t, "", "")
	if user != "" {
		c.Set(constant.CtxKeyUserID, user)
	}
	if team != "" {
		c.Set(constant.CtxKeyTeamID, team)
	}
	return c
}

func (e *actorEnv) selectFor(t *testing.T, c *gin.Context) error {
	_, _, err := e.simple.SelectService(c, typ.ScenarioOpenAI, e.rule, nil)
	return err
}

func TestActorGate_UserConcurrencyRejectIsPerPerson(t *testing.T) {
	e := newActorEnv(t, map[string]*ai.Limits{"a": {MaxConcurrent: 10}}, "a")
	e.policies.set(ai.LimitPolicyUser, "alice", ai.Limits{MaxConcurrent: 1}, ai.LimitOverflowReject, 0)

	c1 := e.ctxFor(t, "alice", "")
	require.NoError(t, e.selectFor(t, c1))
	require.Equal(t, 1, e.reg.Stat(UserLimitKey("alice")).Inflight)

	err := e.selectFor(t, e.ctxFor(t, "alice", ""))
	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t, "user_limit_exceeded", le.Code())
	require.Contains(t, le.Error(), "your concurrency limit")
	require.Equal(t, 1, e.reg.Stat(UserLimitKey("alice")).Inflight, "the rejected request holds nothing")
	require.Equal(t, 1, e.reg.Stat(AccountLimitKey("a")).Inflight, "...and took no account slot either (only alice's first request holds one)")

	require.NoError(t, e.selectFor(t, e.ctxFor(t, "bob", "")), "another person is unaffected")

	ReleaseLease(c1) // alice's first request ends
	require.Equal(t, 0, e.reg.Stat(UserLimitKey("alice")).Inflight)
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "alice", "")))
}

func TestActorGate_QueueWakesOnRelease(t *testing.T) {
	e := newActorEnv(t, nil, "a")
	e.policies.set(ai.LimitPolicyUser, "alice", ai.Limits{MaxConcurrent: 1}, ai.LimitOverflowQueue, 5)
	c1 := e.ctxFor(t, "alice", "")
	require.NoError(t, e.selectFor(t, c1))

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- e.selectFor(t, e.ctxFor(t, "alice", "")) }()
	time.Sleep(150 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("must wait while the person's slot is taken")
	default:
	}
	ReleaseLease(c1)
	require.NoError(t, <-done)
	require.Less(t, time.Since(start), 2*time.Second, "woken by the release, not by the timeout")
}

func TestActorGate_QueueTimeoutThenReject(t *testing.T) {
	e := newActorEnv(t, nil, "a")
	e.policies.set(ai.LimitPolicyUser, "alice", ai.Limits{MaxConcurrent: 1}, ai.LimitOverflowQueue, 1)
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "alice", "")))

	start := time.Now()
	err := e.selectFor(t, e.ctxFor(t, "alice", ""))
	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.GreaterOrEqual(t, time.Since(start), 900*time.Millisecond)
	require.GreaterOrEqual(t, le.Waited, 900*time.Millisecond)
}

func TestActorGate_UsageWindowNeverQueues(t *testing.T) {
	e := newActorEnv(t, nil, "a")
	e.policies.set(ai.LimitPolicyUser, "alice", ai.Limits{MaxTokens: 100, WindowSeconds: 600}, ai.LimitOverflowQueue, 30)
	c := e.ctxFor(t, "alice", "")
	require.NoError(t, e.selectFor(t, c))
	e.guard.RecordActorTokens(c, 100) // the request finished, the budget is spent
	ReleaseLease(c)

	start := time.Now()
	err := e.selectFor(t, e.ctxFor(t, "alice", ""))
	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Less(t, time.Since(start), time.Second, "an exhausted budget is a 429 now, not a 30 s wait")
	require.Greater(t, le.RetryAfter, time.Minute)
	require.Equal(t, "user_limit_exceeded", le.Code())

	require.NoError(t, e.selectFor(t, e.ctxFor(t, "bob", "")))
}

func TestActorGate_TeamAndUserStack(t *testing.T) {
	e := newActorEnv(t, nil, "a")
	e.policies.set(ai.LimitPolicyTeam, "t1", ai.Limits{MaxConcurrent: 2}, ai.LimitOverflowReject, 0)
	e.policies.set(ai.LimitPolicyTeamMember, "t1", ai.Limits{MaxConcurrent: 5}, ai.LimitOverflowReject, 0)

	var held []*gin.Context
	for _, u := range []string{"u1", "u2"} {
		c := e.ctxFor(t, u, "t1")
		require.NoError(t, e.selectFor(t, c))
		held = append(held, c)
	}
	// the team is full although u3 has plenty of personal headroom
	err := e.selectFor(t, e.ctxFor(t, "u3", "t1"))
	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t, "team_limit_exceeded", le.Code())
	require.Contains(t, le.Error(), "your team's")
	require.Equal(t, 0, e.reg.Stat(UserLimitKey("u3")).Inflight, "all-or-nothing: u3's own slot was not kept")

	// a different team is unaffected
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "u9", "t2")))
	for _, c := range held {
		ReleaseLease(c)
	}
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "u3", "t1")))
}

func TestActorGate_TemplateAppliesPerMemberAndExplicitOverrides(t *testing.T) {
	e := newActorEnv(t, nil, "a")
	e.policies.set(ai.LimitPolicyTeamMember, "t1", ai.Limits{MaxConcurrent: 1}, ai.LimitOverflowReject, 0)

	// every member gets their own counter from the template
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "u1", "t1")))
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "u2", "t1")))
	require.Error(t, e.selectFor(t, e.ctxFor(t, "u1", "t1")), "u1 is over the template's 1")
	require.Equal(t, 1, e.reg.Stat(UserLimitKey("u1")).Inflight)
	require.Equal(t, 1, e.reg.Stat(UserLimitKey("u2")).Inflight)

	// an explicit policy replaces the template for that person only
	e.policies.set(ai.LimitPolicyUser, "u1", ai.Limits{MaxConcurrent: 3}, ai.LimitOverflowReject, 0)
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "u1", "t1")))
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "u1", "t1")))
	require.Error(t, e.selectFor(t, e.ctxFor(t, "u1", "t1")), "now capped at 3")
	require.Error(t, e.selectFor(t, e.ctxFor(t, "u2", "t1")), "u2 still on the template")

	// no team / no policy: no gate at all
	require.NoError(t, e.selectFor(t, e.ctxFor(t, "nobody", "t9")))
}

func TestActorGate_EnterpriseIdentity(t *testing.T) {
	e := newActorEnv(t, nil, "a")
	e.policies.set(ai.LimitPolicyUser, EnterpriseUserRef("emp-7"), ai.Limits{MaxConcurrent: 1}, ai.LimitOverflowReject, 0)
	mk := func(emp string) *gin.Context {
		c := e.ctxFor(t, "admin", "")
		c.Set(constant.CtxKeyEnterpriseUserID, emp)
		return c
	}
	require.NoError(t, e.selectFor(t, mk("emp-7")))
	require.Error(t, e.selectFor(t, mk("emp-7")))
	require.NoError(t, e.selectFor(t, mk("emp-8")))
}

func TestActorGate_RunsOncePerRequest(t *testing.T) {
	e := newActorEnv(t, nil, "a")
	e.policies.set(ai.LimitPolicyUser, "alice", ai.Limits{MaxConcurrent: 1, MaxRequests: 1, WindowSeconds: 60}, ai.LimitOverflowReject, 0)
	c := e.ctxFor(t, "alice", "")
	require.NoError(t, e.selectFor(t, c))
	require.NoError(t, e.selectFor(t, c), "a second selection inside the same request (e.g. a helper call) is not a second request")
	st := e.reg.Stat(UserLimitKey("alice"))
	require.Equal(t, 1, st.Inflight)
	require.Equal(t, int64(1), st.Requests)
}

// Gate 1 passes, gate 2 fails (every account at its cap, reject): the request
// ends with 429 and the person's permit is released with the lease.
func TestActorGate_SecondGateFailureReleasesFirst(t *testing.T) {
	e := newActorEnv(t, map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a")
	e.rule.Flags.LimitOverflow = typ.LimitOverflowReject
	e.policies.set(ai.LimitPolicyUser, "alice", ai.Limits{MaxConcurrent: 5}, ai.LimitOverflowReject, 0)
	_, ok := e.guard.Acquire(e.rule.UUID, e.rule.Services[0], false) // someone else fills the account
	require.True(t, ok)

	c := e.ctxFor(t, "alice", "")
	err := e.selectFor(t, c)
	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t, "upstream_limit_exceeded", le.Code(), "the account is full, not alice")
	require.Equal(t, 1, e.reg.Stat(UserLimitKey("alice")).Inflight, "held until the handler returns...")
	ReleaseLease(c)
	require.Equal(t, 0, e.reg.Stat(UserLimitKey("alice")).Inflight, "...then nothing leaks")
	require.Equal(t, 1, e.reg.Stat(AccountLimitKey("a")).Inflight, "the other request's slot is untouched")
}

func TestModelPolicy_CapsModelAcrossAccounts(t *testing.T) {
	e := newActorEnv(t, nil, "a", "b")
	opus := testService("a", "opus", true)
	haiku := testService("b", "haiku", true)
	rule := testRule("r2", "m", []*loadbalance.Service{opus, haiku})
	e.policies.set(ai.LimitPolicyModel, "opus", ai.Limits{MaxConcurrent: 1}, "", 0)

	pick := func() string {
		ctx := testContext(rule, "")
		res, err := e.sel.Select(ctx)
		require.NoError(t, err)
		return res.Service.Model
	}
	require.Equal(t, "opus", pick())
	p, ok := e.guard.Acquire(rule.UUID, opus, false)
	require.True(t, ok)
	require.Equal(t, "haiku", pick(), "opus is at its cross-account cap: traffic shifts to the other model")
	require.Equal(t, 1, e.reg.Stat(ModelLimitKey("opus")).Inflight)
	p.Release()
	require.Equal(t, "opus", pick())
}

func TestActorGate_NoPoliciesIsTransparent(t *testing.T) {
	e := newActorEnv(t, nil, "a")
	for i := 0; i < 10; i++ {
		c := e.ctxFor(t, "alice", "t1")
		require.NoError(t, e.selectFor(t, c))
	}
	require.Empty(t, e.reg.Keys("user:"))
	require.Empty(t, e.reg.Keys("team:"))
}
