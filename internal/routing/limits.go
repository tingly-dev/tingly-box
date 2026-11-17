package routing

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AccountLimitKey is the limits-registry key of an upstream account.
func AccountLimitKey(providerUUID string) string { return "acct:" + providerUUID }

// ServiceLimitKey is the key of one rule's share of an account.
func ServiceLimitKey(ruleUUID string, svc *loadbalance.Service) string {
	return "svc:" + ruleUUID + "/" + svc.ServiceID()
}

// ModelLimitKey is the key of an upstream model's cross-account cap.
func ModelLimitKey(model string) string { return "model:" + model }

// UserLimitKey / TeamLimitKey are the keys of a person's and a team's caps.
func UserLimitKey(userID string) string { return "user:" + userID }
func TeamLimitKey(teamID string) string { return "team:" + teamID }

// EnterpriseUserRef is the policy ref of an enterprise-context identity.
func EnterpriseUserRef(id string) string { return "euser:" + id }

// LimitError is returned by selection when every candidate is at its limit and
// the rule's overflow policy does not allow routing anyway. Handlers map it to
// HTTP 429.
type LimitError struct {
	RuleUUID string
	Reasons  limits.Reason
	// Scopes names who is exhausted. Zero (or only account/service/model
	// scopes) means the upstream pool; ScopeUser/ScopeTeam mean the caller's
	// own cap, which no other account can fix.
	Scopes     limits.Scope
	RetryAfter time.Duration
	Waited     time.Duration
}

// Code is the stable machine-readable error code for the 429 body.
func (e *LimitError) Code() string {
	switch {
	case e.Scopes&limits.ScopeUser != 0:
		return "user_limit_exceeded"
	case e.Scopes&limits.ScopeTeam != 0:
		return "team_limit_exceeded"
	default:
		return "upstream_limit_exceeded"
	}
}

func (e *LimitError) Error() string {
	var msg string
	switch e.Code() {
	case "user_limit_exceeded":
		msg = fmt.Sprintf("your %s limit is reached; retry in %ds", e.Reasons, int(e.RetryAfter.Seconds()+0.5))
	case "team_limit_exceeded":
		msg = fmt.Sprintf("your team's %s limit is reached; retry in %ds", e.Reasons, int(e.RetryAfter.Seconds()+0.5))
	default:
		msg = fmt.Sprintf("all upstream accounts are at their %s limit; retry in %ds", e.Reasons, int(e.RetryAfter.Seconds()+0.5))
	}
	if e.Waited > 0 {
		msg += fmt.Sprintf(" (waited %s)", e.Waited.Round(time.Millisecond))
	}
	return msg
}

// LimitsGuard resolves which limit subjects apply to a candidate and exposes
// the registry operations the pipeline needs. A nil *LimitsGuard disables limits.
type LimitsGuard struct {
	reg       *limits.Registry
	providers ProviderResolver
	policies  PolicyResolver
}

// PolicyResolver looks up person / team / model limit policies
// (db.LimitPolicyStore satisfies it). Nil disables those scopes.
type PolicyResolver interface {
	GetLimitPolicy(kind ai.LimitPolicyKind, ref string) (ai.LimitPolicy, bool)
}

// SetPolicies enables person / team / model scopes.
func (g *LimitsGuard) SetPolicies(p PolicyResolver) { g.policies = p }

// NewLimitsGuard builds a guard over reg; providers resolves account-level limits.
func NewLimitsGuard(reg *limits.Registry, providers ProviderResolver) *LimitsGuard {
	return &LimitsGuard{reg: reg, providers: providers}
}

// Registry exposes the underlying registry (status APIs, tests).
func (g *LimitsGuard) Registry() *limits.Registry {
	if g == nil {
		return nil
	}
	return g.reg
}

// Subjects returns the limit subjects of a candidate: the account's limits
// (Provider.Limits) and the rule's share (Service.Limits). Empty = unlimited.
func (g *LimitsGuard) Subjects(ruleUUID string, svc *loadbalance.Service) []limits.Subject {
	if g == nil || svc == nil {
		return nil
	}
	var out []limits.Subject
	if g.providers != nil {
		if p, err := g.providers.GetProviderByUUID(svc.Provider); err == nil && p != nil {
			if l := p.Limits.Normalize(); l != nil {
				out = append(out, limits.Subject{Key: AccountLimitKey(svc.Provider), Scope: limits.ScopeAcct, Limits: *l, Quota: svc.Provider})
			}
		}
	}
	if l := svc.Limits.Normalize(); l != nil {
		l.QuotaPercent = 0 // quota is an account property
		if !l.IsZero() {
			out = append(out, limits.Subject{Key: ServiceLimitKey(ruleUUID, svc), Scope: limits.ScopeSvc, Limits: *l})
		}
	}
	if g.policies != nil && svc.Model != "" {
		if p, ok := g.policies.GetLimitPolicy(ai.LimitPolicyModel, svc.Model); ok {
			if l := p.Limits.Normalize(); l != nil {
				l.QuotaPercent = 0
				out = append(out, limits.Subject{Key: ModelLimitKey(svc.Model), Scope: limits.ScopeModel, Limits: *l})
			}
		}
	}
	return out
}

// Classification of a candidate set.
type limitClass struct {
	ok, hot  []*loadbalance.Service
	blocked  []*loadbalance.Service
	verdicts map[string]limits.Verdict // by service ID, blocked only
}

// classify assesses every candidate. Services in excluded are treated as
// blocked by concurrency (a lost acquire race).
func (g *LimitsGuard) classify(ruleUUID string, candidates []*loadbalance.Service, excluded map[string]bool) (c limitClass, limited bool) {
	c.verdicts = map[string]limits.Verdict{}
	for _, svc := range candidates {
		if svc == nil {
			continue
		}
		id := svc.ServiceID()
		if excluded[id] {
			c.blocked = append(c.blocked, svc)
			c.verdicts[id] = limits.Verdict{State: limits.StateBlocked, Reasons: limits.ReasonConcurrency, RetryAfter: time.Second}
			limited = true
			continue
		}
		subs := g.Subjects(ruleUUID, svc)
		if len(subs) == 0 {
			c.ok = append(c.ok, svc)
			continue
		}
		limited = true
		v := g.reg.Assess(subs)
		switch v.State {
		case limits.StateBlocked:
			c.blocked = append(c.blocked, svc)
			c.verdicts[id] = v
		case limits.StateHot:
			c.hot = append(c.hot, svc)
		default:
			c.ok = append(c.ok, svc)
		}
	}
	return c, limited
}

// Filter narrows candidates for a path that cannot wait (failover): blocked
// services are dropped and cool ones are preferred over hot ones. With policy
// "ignore" blocked services stay as a last resort. Returns candidates
// unchanged when nothing is limited.
func (g *LimitsGuard) Filter(rule *typ.Rule, candidates []*loadbalance.Service, excluded map[string]bool) []*loadbalance.Service {
	if g == nil || g.reg == nil {
		return candidates
	}
	c, limited := g.classify(rule.UUID, candidates, excluded)
	if !limited {
		return candidates
	}
	switch {
	case len(c.ok) > 0:
		return c.ok
	case len(c.hot) > 0:
		return c.hot
	case rule.LimitOverflowPolicy() == typ.LimitOverflowIgnore:
		return candidates
	}
	return nil
}

// Acquire takes the permit for the chosen service. force skips the cap check
// (overflow policy "ignore"). A nil permit with ok=true means the service is
// unlimited and nothing needs releasing.
func (g *LimitsGuard) Acquire(ruleUUID string, svc *loadbalance.Service, force bool) (permit *limits.Permit, ok bool) {
	if g == nil || g.reg == nil {
		return nil, true
	}
	subs := g.Subjects(ruleUUID, svc)
	if len(subs) == 0 {
		return nil, true
	}
	p, _ := g.reg.TryAcquire(subs, force)
	return p, p != nil
}

// RecordTokens adds a finished request's tokens to the service's usage windows.
func (g *LimitsGuard) RecordTokens(ruleUUID string, svc *loadbalance.Service, tokens int64) {
	if g == nil || g.reg == nil || svc == nil {
		return
	}
	if subs := g.Subjects(ruleUUID, svc); len(subs) > 0 {
		g.reg.Record(subs, tokens)
	}
}

// LeaseFromGin returns the request's permit lease, creating it on first use.
// The limits middleware releases whatever the lease holds when the handler
// returns.
func LeaseFromGin(c *gin.Context) *limits.Lease {
	if v, ok := c.Get(constant.CtxKeyLimitLease); ok {
		if l, ok := v.(*limits.Lease); ok {
			return l
		}
	}
	l := limits.NewLease()
	c.Set(constant.CtxKeyLimitLease, l)
	return l
}

// ReleaseLease releases every permit the request still holds (no-op when none).
func ReleaseLease(c *gin.Context) {
	if v, ok := c.Get(constant.CtxKeyLimitLease); ok {
		if l, ok := v.(*limits.Lease); ok {
			l.ReleaseAll()
		}
	}
}
