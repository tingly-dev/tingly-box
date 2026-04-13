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
	filteredCount := before - len(healthy)

	if filteredCount > 0 {
		logrus.Warnf("[health] Filtered %d unhealthy services, %d remaining (of %d total)",
			filteredCount, len(healthy), before)
		// Log each filtered service for debugging
		for _, svc := range state.candidateServices {
			if !s.filter.IsHealthy(svc.ServiceID()) {
				logrus.Warnf("[health] Service %s:%s is unhealthy (rate limited/auth error)",
					svc.Provider, svc.Model)
			}
		}
	}

	// Continue pipeline (don't select, just filter)
	return NewFilterResult("health", healthy), false
}
