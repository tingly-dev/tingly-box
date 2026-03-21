package guardrails

// PolicyKind is the user-facing policy classification.
type PolicyKind string

const (
	PolicyKindResourceAccess   PolicyKind = "resource_access"
	PolicyKindCommandExecution PolicyKind = "command_execution"
	PolicyKindContent          PolicyKind = "content"
	PolicyKindOperationLegacy  PolicyKind = "operation"
)

// PolicyGroup provides shared defaults and risk grouping for policies.
type PolicyGroup struct {
	ID             string  `json:"id" yaml:"id"`
	Name           string  `json:"name,omitempty" yaml:"name,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Severity       string  `json:"severity,omitempty" yaml:"severity,omitempty"`
	DefaultVerdict Verdict `json:"default_verdict,omitempty" yaml:"default_verdict,omitempty"`
	DefaultScope   Scope   `json:"default_scope,omitempty" yaml:"default_scope,omitempty"`
}

// Policy is the top-level user-facing policy definition.
type Policy struct {
	ID      string      `json:"id" yaml:"id"`
	Name    string      `json:"name,omitempty" yaml:"name,omitempty"`
	Group   string      `json:"group,omitempty" yaml:"group,omitempty"`
	Kind    PolicyKind  `json:"kind" yaml:"kind"`
	Enabled *bool       `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Scope   Scope       `json:"scope,omitempty" yaml:"scope,omitempty"`
	Match   PolicyMatch `json:"match" yaml:"match"`
	Verdict Verdict     `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	Reason  string      `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// PolicyMatch keeps operation and content selectors in one shape so the UI can
// stay policy-oriented while the backend compiles into internal evaluators.
type PolicyMatch struct {
	ToolNames      []string         `json:"tool_names,omitempty" yaml:"tool_names,omitempty"`
	Actions        *ActionSelector  `json:"actions,omitempty" yaml:"actions,omitempty"`
	Resources      *ResourceMatcher `json:"resources,omitempty" yaml:"resources,omitempty"`
	Terms          []string         `json:"terms,omitempty" yaml:"terms,omitempty"`
	CredentialRefs []string         `json:"credential_refs,omitempty" yaml:"credential_refs,omitempty"`
	Patterns       []string         `json:"patterns,omitempty" yaml:"patterns,omitempty"`
	PatternMode    string           `json:"pattern_mode,omitempty" yaml:"pattern_mode,omitempty"`
	MatchMode      string           `json:"match_mode,omitempty" yaml:"match_mode,omitempty"`
	MinMatches     int              `json:"min_matches,omitempty" yaml:"min_matches,omitempty"`
	CaseSensitive  bool             `json:"case_sensitive,omitempty" yaml:"case_sensitive,omitempty"`
}

// ActionSelector narrows operation policies to semantic action labels.
type ActionSelector struct {
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

// ResourceMatcher selects the protected resource set for an operation policy.
type ResourceMatcher struct {
	Type   string   `json:"type,omitempty" yaml:"type,omitempty"`
	Mode   string   `json:"mode,omitempty" yaml:"mode,omitempty"`
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
}
