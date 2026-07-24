package routing

import (
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Selection sources: which stage produced a SelectionResult. Also used as the
// stage names in EvaluatedStages and observability output.
const (
	SourceHealth       = "health"
	SourceAffinity     = "affinity"
	SourceSmartRouting = "smart_routing"
	SourceLoadBalancer = "load_balancer"
	// SourceProbePin marks the X-Tingly-Probe-Service bypass, which pins a
	// specific service without running the pipeline.
	SourceProbePin = "probe_pin"
)

// SelectionResult represents the output of service selection pipeline.
// It includes the selected service, provider, and metadata about the selection.
type SelectionResult struct {
	// Service is the selected load-balanced service
	Service *loadbalance.Service

	// FilteredServices contains a narrowed candidate set produced by filter stages.
	// When set, selector updates SelectionContext.CandidateServices with this value.
	FilteredServices []*loadbalance.Service

	// Provider is the resolved provider for the service
	Provider *typ.Provider

	// Source indicates which stage selected this service (one of the
	// Source* constants above).
	Source string

	// EvaluatedStages tracks which stages were evaluated (for observability)
	EvaluatedStages []string

	// MatchedSmartRuleIndex is the index of the matched smart routing rule
	// -1 if no smart routing rule matched
	MatchedSmartRuleIndex int
}

// NewResult creates a new selection result with the given service and source
func NewResult(service *loadbalance.Service, source string) *SelectionResult {
	return &SelectionResult{
		Service:               service,
		Source:                source,
		EvaluatedStages:       []string{},
		MatchedSmartRuleIndex: -1,
	}
}

// NewFilterResult creates a non-terminal result for filtering stages.
func NewFilterResult(source string, services []*loadbalance.Service) *SelectionResult {
	return &SelectionResult{
		FilteredServices:      services,
		Source:                source,
		EvaluatedStages:       []string{},
		MatchedSmartRuleIndex: -1,
	}
}
