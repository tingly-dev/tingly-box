package routing

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// actorLeaseKey is the lease slot of the person/team permit. Service ids
// contain "/", so it can never collide with a candidate's slot, and failover
// (which swaps candidate permits by service id) never touches it.
const actorLeaseKey = "actor"

// Actor is who is calling, as far as limits are concerned.
type Actor struct {
	// UserID is the authenticated principal: one sharing key's user_id, or the
	// admin id for the global model token.
	UserID string
	// EnterpriseUserID is the end user asserted by an enterprise context JWT.
	EnterpriseUserID string
	// TeamID is the authorized team (sharing keys; the default team on the
	// public team surface).
	TeamID string
}

// ActorFromGin reads the identity the auth middleware stored on the request.
func ActorFromGin(c *gin.Context) Actor {
	return Actor{
		UserID:           c.GetString(constant.CtxKeyUserID),
		EnterpriseUserID: c.GetString(constant.CtxKeyEnterpriseUserID),
		TeamID:           c.GetString(constant.CtxKeyTeamID),
	}
}

// actorOverflow is the effective overflow behaviour for a blocked actor gate.
type actorOverflow struct {
	reject  bool
	timeout time.Duration
}

func (o *actorOverflow) merge(p ai.LimitPolicy) {
	if p.Overflow == ai.LimitOverflowReject {
		o.reject = true
	}
	t := typ.DefaultLimitQueueTimeout
	if p.QueueTimeoutSeconds > 0 {
		t = time.Duration(p.QueueTimeoutSeconds) * time.Second
	}
	if o.timeout == 0 || t < o.timeout {
		o.timeout = t
	}
}

// actorSubjects resolves the person and team subjects of a request, with the
// policy behind each scope (for overflow). A person's explicit `user` policy
// replaces the team's `team_member` template for the same counter.
func (g *LimitsGuard) actorSubjects(a Actor) (subs []limits.Subject, byScope map[limits.Scope]ai.LimitPolicy) {
	if g == nil || g.policies == nil {
		return nil, nil
	}
	byScope = map[limits.Scope]ai.LimitPolicy{}
	add := func(key string, scope limits.Scope, p ai.LimitPolicy) {
		l := p.Limits.Normalize()
		if l == nil {
			return
		}
		subs = append(subs, limits.Subject{Key: key, Scope: scope, Limits: *l})
		if prev, ok := byScope[scope]; ok { // two persons (key + enterprise id): the stricter overflow wins
			var o actorOverflow
			o.merge(prev)
			o.merge(p)
			if o.reject {
				p.Overflow = ai.LimitOverflowReject
			}
			if prev.QueueTimeoutSeconds > 0 && (p.QueueTimeoutSeconds == 0 || prev.QueueTimeoutSeconds < p.QueueTimeoutSeconds) {
				p.QueueTimeoutSeconds = prev.QueueTimeoutSeconds
			}
		}
		byScope[scope] = p
	}
	if a.TeamID != "" {
		if p, ok := g.policies.GetLimitPolicy(ai.LimitPolicyTeam, a.TeamID); ok {
			add(TeamLimitKey(a.TeamID), limits.ScopeTeam, p)
		}
	}
	if a.UserID != "" {
		p, ok := g.policies.GetLimitPolicy(ai.LimitPolicyUser, a.UserID)
		if !ok && a.TeamID != "" {
			p, ok = g.policies.GetLimitPolicy(ai.LimitPolicyTeamMember, a.TeamID)
		}
		if ok {
			add(UserLimitKey(a.UserID), limits.ScopeUser, p)
		}
	}
	if a.EnterpriseUserID != "" {
		ref := EnterpriseUserRef(a.EnterpriseUserID)
		if p, ok := g.policies.GetLimitPolicy(ai.LimitPolicyUser, ref); ok {
			add(UserLimitKey(ref), limits.ScopeUser, p)
		}
	}
	return subs, byScope
}

func overflowFor(blocked limits.Scope, byScope map[limits.Scope]ai.LimitPolicy) actorOverflow {
	var o actorOverflow
	for _, sc := range []limits.Scope{limits.ScopeUser, limits.ScopeTeam} {
		if blocked&sc != 0 {
			o.merge(byScope[sc])
		}
	}
	if o.timeout == 0 {
		o.timeout = typ.DefaultLimitQueueTimeout
	}
	return o
}

// AdmitActor is the first gate of a request (.design/routing-limits.md §10.4):
// the caller's own person and team limits, checked before routing because no
// choice of upstream can fix them. It takes one all-or-nothing permit over the
// person and team subjects into the request's lease; failover keeps it, and
// the limits middleware releases it when the handler returns — including when
// a later stage rejects the request, so a failed second gate leaks nothing.
//
// A concurrency-only block waits (queue) or fails at once (reject) per the
// blocking policy; an exhausted usage window always fails with Retry-After.
// The wait budget is shared with the candidate stage via ctx.limitStart.
// Idempotent per request.
func (g *LimitsGuard) AdmitActor(c *gin.Context, ctx *SelectionContext) error {
	if g == nil || g.reg == nil || g.policies == nil || c.GetBool(constant.CtxKeyLimitActorDone) {
		return nil
	}
	c.Set(constant.CtxKeyLimitActorDone, true)
	subs, byScope := g.actorSubjects(ActorFromGin(c))
	if len(subs) == 0 {
		return nil
	}
	if ctx.limitStart.IsZero() {
		ctx.limitStart = clock.Now()
	}
	reqCtx := selectionLogContext(ctx)
	for {
		changed := g.reg.Changed() // taken before the attempt so a release in between is not missed
		permit, v := g.reg.TryAcquire(subs, false)
		if permit != nil {
			LeaseFromGin(c).Hold(actorLeaseKey, permit)
			return nil
		}
		ov := overflowFor(v.Scopes, byScope)
		waited := clock.Now().Sub(ctx.limitStart)
		remaining := ov.timeout - waited
		if !v.ConcurrencyOnly() || ov.reject || remaining <= 0 {
			logrus.WithContext(reqCtx).WithFields(logrus.Fields{
				"stage": "routing_limit_actor_rejected", "scope": v.Scopes.String(), "reasons": v.Reasons.String(),
				"waited_ms": waited.Milliseconds(), "user_id": ActorFromGin(c).UserID, "team_id": ActorFromGin(c).TeamID,
			}).Warnf("[limits] %s limit reached (%s), rejecting after %s", v.Scopes, v.Reasons, waited.Round(time.Millisecond))
			return &LimitError{RuleUUID: selectionRuleUUID(ctx), Reasons: v.Reasons, Scopes: v.Scopes, RetryAfter: v.RetryAfter, Waited: waited}
		}
		timer := time.NewTimer(remaining)
		var done <-chan struct{}
		if c.Request != nil {
			done = c.Request.Context().Done()
		}
		select {
		case <-changed:
			timer.Stop()
		case <-timer.C:
		case <-done:
			timer.Stop()
			return &LimitError{RuleUUID: selectionRuleUUID(ctx), Reasons: v.Reasons, Scopes: v.Scopes, RetryAfter: v.RetryAfter, Waited: clock.Now().Sub(ctx.limitStart)}
		}
	}
}

// RecordActorTokens adds a finished request's tokens to the caller's person
// and team usage windows.
func (g *LimitsGuard) RecordActorTokens(c *gin.Context, tokens int64) {
	if g == nil || g.reg == nil || g.policies == nil {
		return
	}
	if subs, _ := g.actorSubjects(ActorFromGin(c)); len(subs) > 0 {
		g.reg.Record(subs, tokens)
	}
}
