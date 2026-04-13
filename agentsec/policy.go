package agentsec

// Decision represents the outcome of policy evaluation.
type Decision int

const (
	// DecisionAllow means the tool invocation is permitted by a matching rule.
	DecisionAllow Decision = iota

	// DecisionDeny means the tool invocation is denied and cannot be approved.
	DecisionDeny

	// DecisionRequireApproval means no rule matched; user approval is required.
	DecisionRequireApproval
)

// String returns the string representation of the decision.
func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "Allow"
	case DecisionDeny:
		return "Deny"
	case DecisionRequireApproval:
		return "RequireApproval"
	default:
		return "Unknown"
	}
}

// Policy evaluates rules against tool invocations.
// Policy is stateless and safe for concurrent use.
// Rules are evaluated in order; the first matching rule determines the decision.
type Policy struct {
	rules           []Rule
	defaultDecision Decision
}

// NewPolicy creates a Policy with the given rules.
// The rules slice is copied to prevent external modification.
// The default decision is DecisionRequireApproval.
func NewPolicy(rules []Rule) *Policy {
	copied := make([]Rule, len(rules))
	copy(copied, rules)
	return &Policy{
		rules:           copied,
		defaultDecision: DecisionRequireApproval,
	}
}

// NewPolicyWithDefault creates a Policy with the given rules and default decision.
func NewPolicyWithDefault(rules []Rule, defaultDecision Decision) *Policy {
	copied := make([]Rule, len(rules))
	copy(copied, rules)
	return &Policy{
		rules:           copied,
		defaultDecision: defaultDecision,
	}
}

// Evaluate returns the decision for the given tool invocation.
// If any rule matches, returns DecisionAllow.
// Otherwise, returns the configured default decision.
func (p *Policy) Evaluate(tool, input string) Decision {
	for _, rule := range p.rules {
		if rule.Matches(tool, input) {
			return DecisionAllow
		}
	}
	return p.defaultDecision
}

// SetDefaultDecision sets the decision returned when no rules match.
func (p *Policy) SetDefaultDecision(d Decision) {
	p.defaultDecision = d
}

// AddRule returns a new Policy with the additional rule appended.
// The original Policy is unchanged (immutable pattern).
func (p *Policy) AddRule(rule Rule) *Policy {
	newRules := make([]Rule, len(p.rules)+1)
	copy(newRules, p.rules)
	newRules[len(p.rules)] = rule
	return &Policy{
		rules:           newRules,
		defaultDecision: p.defaultDecision,
	}
}

// Rules returns a copy of the rules slice (read-only access).
func (p *Policy) Rules() []Rule {
	copied := make([]Rule, len(p.rules))
	copy(copied, p.rules)
	return copied
}

// DefaultDecision returns the current default decision.
func (p *Policy) DefaultDecision() Decision {
	return p.defaultDecision
}
