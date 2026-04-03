package routing

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HealthStage filters unhealthy services from the context.
// It runs first and narrows ctx.CandidateServices.
type HealthStage struct {
	filter *typ.HealthFilter
}

// NewHealthStage creates a new health stage with the given health filter
func NewHealthStage(filter *typ.HealthFilter) *HealthStage {
	return &HealthStage{filter: filter}
}

func (s *HealthStage) Name() string {
	return "health"
}

func (s *HealthStage) Evaluate(_ *SelectionContext, state *selectionState) (*SelectionResult, bool) {
	if state == nil || state.candidateServices == nil {
		return nil, false
	}

	// If no health filter configured, pass through unchanged
	if s.filter == nil {
		return NewFilterResult("health", state.candidateServices), false
	}

	before := len(state.candidateServices)
	healthy := s.filter.Filter(state.candidateServices)

	if len(healthy) < before {
		logrus.Debugf("[health] filtered %d unhealthy services, %d remaining",
			before-len(healthy), len(healthy))
	}

	// Continue pipeline (don't select, just filter)
	return NewFilterResult("health", healthy), false
}
