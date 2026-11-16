package ai

import "time"

// Limits caps what an upstream account (Provider.Limits) or one rule's share of
// it (loadbalance.Service.Limits) may do. All fields are optional; the zero
// value means "no limit". See .design/routing-limits.md.
type Limits struct {
	// MaxConcurrent caps in-flight requests. 0 = unlimited.
	MaxConcurrent int `json:"max_concurrent,omitempty" yaml:"max_concurrent,omitempty"`

	// WindowSeconds is the sliding window for MaxRequests / MaxTokens.
	// Defaults to DefaultLimitWindowSeconds when a budget is set.
	WindowSeconds int `json:"window_seconds,omitempty" yaml:"window_seconds,omitempty"`
	// MaxRequests caps requests admitted per window. 0 = unlimited.
	MaxRequests int64 `json:"max_requests,omitempty" yaml:"max_requests,omitempty"`
	// MaxTokens caps input+output tokens per window. 0 = unlimited.
	MaxTokens int64 `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`

	// SoftPercent (1-99): traffic is steered away from this subject once any
	// limit reaches this percentage, as long as an alternative below it
	// exists. 0 = off (only the hard limit applies).
	SoftPercent int `json:"soft_percent,omitempty" yaml:"soft_percent,omitempty"`

	// QuotaPercent (1-100, provider-level only): stop using the account once
	// its upstream ai/quota usage reaches this percentage. 0 = off.
	QuotaPercent int `json:"quota_percent,omitempty" yaml:"quota_percent,omitempty"`
}

// DefaultLimitWindowSeconds is the usage window used when a budget is set
// without an explicit WindowSeconds.
const DefaultLimitWindowSeconds = 3600

// IsZero reports whether no limit is configured.
func (l *Limits) IsZero() bool {
	return l == nil || *l == Limits{}
}

// HasBudget reports whether a request/token budget is configured.
func (l *Limits) HasBudget() bool {
	return l != nil && (l.MaxRequests > 0 || l.MaxTokens > 0)
}

// Window returns the effective usage window (0 when no budget is set).
func (l *Limits) Window() time.Duration {
	if !l.HasBudget() {
		return 0
	}
	if l.WindowSeconds > 0 {
		return time.Duration(l.WindowSeconds) * time.Second
	}
	return DefaultLimitWindowSeconds * time.Second
}

// Normalize clamps out-of-range values (negative numbers, percentages) so a
// hand-edited config can never produce nonsense. Returns a copy; nil stays nil
// and an all-zero result collapses to nil.
func (l *Limits) Normalize() *Limits {
	if l == nil {
		return nil
	}
	out := *l
	if out.MaxConcurrent < 0 {
		out.MaxConcurrent = 0
	}
	if out.WindowSeconds < 0 {
		out.WindowSeconds = 0
	}
	if out.MaxRequests < 0 {
		out.MaxRequests = 0
	}
	if out.MaxTokens < 0 {
		out.MaxTokens = 0
	}
	if out.SoftPercent < 0 || out.SoftPercent >= 100 {
		out.SoftPercent = 0
	}
	if out.QuotaPercent < 0 {
		out.QuotaPercent = 0
	}
	if out.QuotaPercent > 100 {
		out.QuotaPercent = 100
	}
	if !out.HasBudget() {
		out.WindowSeconds = 0
	}
	if out.IsZero() {
		return nil
	}
	return &out
}

// LimitPolicyKind selects what a LimitPolicy governs. See
// .design/routing-limits.md §10.
type LimitPolicyKind string

const (
	// LimitPolicyUser caps one person. Ref is the sharing key's user_id (or
	// "euser:<enterprise user id>" for enterprise-context identities).
	LimitPolicyUser LimitPolicyKind = "user"
	// LimitPolicyTeam caps a whole team together. Ref is the team id.
	LimitPolicyTeam LimitPolicyKind = "team"
	// LimitPolicyTeamMember is a per-member template: every distinct user in
	// the team gets its own counter governed by it. Ref is the team id. An
	// explicit LimitPolicyUser policy replaces it for that user.
	LimitPolicyTeamMember LimitPolicyKind = "team_member"
	// LimitPolicyModel caps an upstream model across all accounts and rules.
	// Ref is the upstream model name.
	LimitPolicyModel LimitPolicyKind = "model"
)

// Overflow behaviours for actor-scoped (user/team) concurrency blocks.
const (
	LimitOverflowQueue  = "queue"
	LimitOverflowReject = "reject"
)

// IsValid reports whether k is one of the known kinds.
func (k LimitPolicyKind) IsValid() bool {
	switch k {
	case LimitPolicyUser, LimitPolicyTeam, LimitPolicyTeamMember, LimitPolicyModel:
		return true
	}
	return false
}

// IsActor reports whether the kind is evaluated at request start (before
// routing) rather than per candidate account.
func (k LimitPolicyKind) IsActor() bool {
	return k == LimitPolicyUser || k == LimitPolicyTeam || k == LimitPolicyTeamMember
}

// LimitPolicy is one limit rule over a person, team or model.
type LimitPolicy struct {
	Kind LimitPolicyKind `json:"kind"`
	Ref  string          `json:"ref"`
	// Limits are the caps. soft_percent is only meaningful for model
	// policies (candidate steering) and quota_percent never is; both are
	// cleared for actor kinds by NormalizePolicy.
	Limits Limits `json:"limits"`
	// Overflow is what an actor-scoped concurrency block does: "queue"
	// (default) waits up to QueueTimeoutSeconds, "reject" answers 429 at
	// once. Usage-window blocks always reject. Ignored for model policies,
	// which follow the rule's limit_overflow flag.
	Overflow            string `json:"overflow,omitempty"`
	QueueTimeoutSeconds int    `json:"queue_timeout_seconds,omitempty"`
}

// NormalizePolicy clamps a policy into its valid shape: limits normalized,
// meaningless fields cleared, overflow defaulted. ok=false means the policy
// holds no limit and should be deleted.
func NormalizePolicy(p LimitPolicy) (out LimitPolicy, ok bool) {
	out = p
	l := p.Limits.Normalize()
	if l == nil {
		return out, false
	}
	out.Limits = *l
	out.Limits.QuotaPercent = 0
	if p.Kind.IsActor() {
		out.Limits.SoftPercent = 0
		if out.Overflow != LimitOverflowReject {
			out.Overflow = LimitOverflowQueue
		}
		if out.QueueTimeoutSeconds < 0 {
			out.QueueTimeoutSeconds = 0
		}
	} else {
		out.Overflow, out.QueueTimeoutSeconds = "", 0
	}
	if out.Limits.IsZero() {
		return out, false
	}
	return out, true
}
