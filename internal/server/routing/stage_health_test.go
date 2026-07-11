package routing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestHealthStage_FiltersUnhealthy(t *testing.T) {
	// Create a health monitor that marks "provider-b/gpt-4" as unhealthy.
	// A 429 report marks a service unhealthy immediately (generic 5xx no
	// longer feeds the health monitor — the breaker owns that signal).
	monitor := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	monitor.ReportRateLimit("provider-b/gpt-4")

	filter := typ.NewHealthFilter(monitor)
	stage := NewHealthStage(filter)

	svcA := testService("provider-a", "gpt-4", true)
	svcB := testService("provider-b", "gpt-4", true)
	svcC := testService("provider-c", "gpt-4", true)

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svcA, svcB, svcC})
	ctx := testContext(rule, "")
	state := newSelectionState(ctx.Rule)

	result, handled := stage.Evaluate(ctx, state)
	require.False(t, handled, "should not select, just filter")
	require.NotNil(t, result)
	state.candidateServices = result.FilteredServices
	require.Len(t, state.candidateServices, 2, "unhealthy service should be filtered out")
	require.Equal(t, "provider-a", state.candidateServices[0].Provider)
	require.Equal(t, "provider-c", state.candidateServices[1].Provider)
}

func TestHealthStage_AllHealthy_NoFilter(t *testing.T) {
	monitor := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	filter := typ.NewHealthFilter(monitor)
	stage := NewHealthStage(filter)

	svcA := testService("provider-a", "gpt-4", true)
	svcB := testService("provider-b", "gpt-4", true)

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svcA, svcB})
	ctx := testContext(rule, "")
	state := newSelectionState(ctx.Rule)

	result, handled := stage.Evaluate(ctx, state)
	require.False(t, handled, "should not select, just filter")
	require.NotNil(t, result)
	state.candidateServices = result.FilteredServices
	require.Len(t, state.candidateServices, 2, "all services should remain")
}

func TestHealthStage_AllUnhealthy(t *testing.T) {
	monitor := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	monitor.ReportRateLimit("provider-a/gpt-4")
	monitor.ReportRateLimit("provider-b/gpt-4")

	filter := typ.NewHealthFilter(monitor)
	stage := NewHealthStage(filter)

	svcA := testService("provider-a", "gpt-4", true)
	svcB := testService("provider-b", "gpt-4", true)

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svcA, svcB})
	ctx := testContext(rule, "")
	state := newSelectionState(ctx.Rule)

	// Degrade, don't disappear: when every candidate is unhealthy, keep the full
	// set so a service (and the real upstream 429/auth) still reaches the client.
	result, handled := stage.Evaluate(ctx, state)
	require.False(t, handled, "should continue even when all unhealthy")
	require.NotNil(t, result)
	state.candidateServices = result.FilteredServices
	require.Len(t, state.candidateServices, 2, "all-unhealthy degrades to the full set, not empty")
}

func TestHealthStage_NilServices(t *testing.T) {
	filter := typ.NewHealthFilter(nil)
	stage := NewHealthStage(filter)

	rule := testRule("rule-1", "gpt-4", nil)
	ctx := testContext(rule, "")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when no services")
	// newSelectionState returns an empty (non-nil) candidate slice for a rule
	// with no Services. HealthStage filters that empty slice and returns a
	// filter result so downstream stages observe the narrowed state.
	require.NotNil(t, result)
	require.Empty(t, result.FilteredServices)
}

func TestHealthStage_NilFilter(t *testing.T) {
	stage := NewHealthStage(nil)

	svcA := testService("provider-a", "gpt-4", true)
	svcB := testService("provider-b", "gpt-4", true)

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svcA, svcB})
	ctx := testContext(rule, "")
	state := newSelectionState(ctx.Rule)

	result, handled := stage.Evaluate(ctx, state)
	require.False(t, handled, "should not select")
	require.NotNil(t, result)
	state.candidateServices = result.FilteredServices
	require.Len(t, state.candidateServices, 2, "all services should remain when filter is nil")
}

func TestHealthStage_ContinuesPipeline(t *testing.T) {
	// Test that health stage returns (nil, false) so pipeline continues
	filter := typ.NewHealthFilter(nil)
	stage := NewHealthStage(filter)

	svc := testService("provider-a", "gpt-4", true)
	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svc})
	ctx := testContext(rule, "")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should return handled=false to continue pipeline")
	require.NotNil(t, result, "should return filter result while continuing pipeline")
}

func TestHealthStage_Name(t *testing.T) {
	stage := NewHealthStage(nil)
	require.Equal(t, "health", stage.Name())
}
