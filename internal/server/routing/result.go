package routing

import (
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SelectionResult represents the output of service selection pipeline.
// It includes the selected service, provider, and metadata about the selection.
type SelectionResult struct {
	// Service is the selected load-balanced service
	Service *loadbalance.Service

	// Provider is the resolved provider for the service
	Provider *typ.Provider

	// Source indicates which stage selected this service
	// Values: "affinity", "smart_routing", "load_balancer"
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

// AddEvaluatedStage records that a stage was evaluated
func (r *SelectionResult) AddEvaluatedStage(stageName string) {
	r.EvaluatedStages = append(r.EvaluatedStages, stageName)
}
