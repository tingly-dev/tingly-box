package routing

import (
	"context"

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
	return SourceHealth
}

func (s *HealthStage) Evaluate(ctx *SelectionContext, state *selectionState) (*SelectionResult, bool) {
	if state == nil || state.candidateServices == nil {
		return nil, false
	}

	// If no health filter configured, pass through unchanged
	if s.filter == nil {
		return NewFilterResult(SourceHealth, state.candidateServices), false
	}

	before := len(state.candidateServices)
	healthy := s.filter.Filter(state.candidateServices)

	// Degrade, don't disappear: if every candidate is unhealthy, keep the full
	// set rather than emptying it — mirrors LoadBalancer.SelectService, so the
	// caller still gets a service (and the real upstream 429/auth) instead of a
	// "no service available" routing error.
	if before > 0 && len(healthy) == 0 {
		logrus.WithContext(selectionLogContext(ctx)).WithFields(logrus.Fields{
			"stage":      "routing_health_degrade",
			"rule_uuid":  selectionRuleUUID(ctx),
			"candidates": before,
		}).Warnf("[health] all %d candidates unhealthy; keeping the full set (degrade)", before)
		return NewFilterResult(SourceHealth, state.candidateServices), false
	}

	filteredCount := before - len(healthy)

	if filteredCount > 0 {
		logrus.WithContext(selectionLogContext(ctx)).WithFields(logrus.Fields{
			"stage":           "routing_health_filtered",
			"rule_uuid":       selectionRuleUUID(ctx),
			"filtered_count":  filteredCount,
			"remaining_count": len(healthy),
			"candidate_count": before,
		}).Warnf("[health] Filtered %d unhealthy services, %d remaining (of %d total)",
			filteredCount, len(healthy), before)
		// Log each filtered service for debugging
		for _, svc := range state.candidateServices {
			if !s.filter.IsHealthy(svc.ServiceID()) {
				logrus.WithContext(selectionLogContext(ctx)).WithFields(logrus.Fields{
					"stage":         "routing_health_filtered_service",
					"rule_uuid":     selectionRuleUUID(ctx),
					"service":       svc.ServiceID(),
					"provider_uuid": svc.Provider,
					"model":         svc.Model,
					"reason":        "rate_limited_or_auth_error",
				}).Warnf("[health] Service %s:%s is unhealthy (rate limited/auth error)",
					svc.Provider, svc.Model)
			}
		}
	}

	// Continue pipeline (don't select, just filter)
	return NewFilterResult(SourceHealth, healthy), false
}

func selectionLogContext(ctx *SelectionContext) context.Context {
	if ctx != nil && ctx.GinContext != nil && ctx.GinContext.Request != nil {
		return ctx.GinContext.Request.Context()
	}
	return context.Background()
}

func selectionRuleUUID(ctx *SelectionContext) string {
	if ctx != nil && ctx.Rule != nil {
		return ctx.Rule.UUID
	}
	return ""
}
