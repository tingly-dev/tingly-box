package smartrouting

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func limitOp(pos SmartOpPosition, op SmartOpOperation, value string) SmartOp {
	return SmartOp{Position: pos, Operation: op, Value: value, Meta: SmartOpMeta{Type: ValueTypeInt}}
}

func TestServiceConcurrencyOp(t *testing.T) {
	r := &Router{}
	ctx := &RequestContext{ServiceLimits: []ServiceLimitInfo{
		{ServiceID: "a/m", Concurrency: 25, HasConcurrency: true},
		{ServiceID: "b/m", Concurrency: 80, HasConcurrency: true}, // tightest
		{ServiceID: "c/m", Usage: 99, HasUsage: true},             // other dimension: ignored here
	}}
	ge := limitOp(PositionServiceConcurrency, OpServiceConcurrencyUtilGe, "75")
	res := r.evaluateOp(ctx, &ge)
	require.True(t, res.Matched, res.Reason)
	require.Equal(t, "80.0%", res.Actual)

	lt := limitOp(PositionServiceConcurrency, OpServiceConcurrencyUtilLt, "75")
	require.False(t, r.evaluateOp(ctx, &lt).Matched)
}

func TestServiceUsageOp(t *testing.T) {
	r := &Router{}
	ctx := &RequestContext{ServiceLimits: []ServiceLimitInfo{
		{ServiceID: "a/m", Usage: 10, HasUsage: true},
		{ServiceID: "b/m", Usage: 95, HasUsage: true},
		{ServiceID: "c/m", Concurrency: 100, HasConcurrency: true},
	}}
	gt := limitOp(PositionServiceUsage, OpServiceUsagePctGt, "90")
	require.True(t, r.evaluateOp(ctx, &gt).Matched)
	le := limitOp(PositionServiceUsage, OpServiceUsagePctLe, "50")
	require.False(t, r.evaluateOp(ctx, &le).Matched)
	bad := limitOp(PositionServiceUsage, OpServiceUsagePctGt, "x")
	require.False(t, r.evaluateOp(ctx, &bad).Matched)
}

func TestServiceLimitOps_PassWithoutLimits(t *testing.T) {
	r := &Router{}
	for _, pos := range []SmartOpPosition{PositionServiceConcurrency, PositionServiceUsage} {
		op := limitOp(pos, "pct_ge", "1")
		if pos == PositionServiceConcurrency {
			op.Operation = "util_ge"
		}
		res := r.evaluateOp(&RequestContext{}, &op)
		require.True(t, res.Matched, "unlimited services must read as 'no data', not idle/hot: %s", pos)
	}
}

func TestServiceLimitOps_ValidAndRegistered(t *testing.T) {
	require.True(t, PositionServiceConcurrency.IsValid())
	require.True(t, PositionServiceUsage.IsValid())
	found := map[SmartOpPosition]int{}
	for _, o := range Operations {
		found[o.Position]++
	}
	require.Equal(t, 4, found[PositionServiceConcurrency])
	require.Equal(t, 4, found[PositionServiceUsage])
}

func TestServiceLimitOps_ScopedPerRule(t *testing.T) {
	hot := &loadbalance.Service{Provider: "a", Model: "m", Active: true}
	cool := &loadbalance.Service{Provider: "b", Model: "m", Active: true}
	router, err := NewRouter([]SmartRouting{
		{Description: "overflow", Ops: []SmartOp{limitOp(PositionServiceConcurrency, OpServiceConcurrencyUtilGe, "80")}, Services: []*loadbalance.Service{cool}},
		{Description: "primary", Ops: []SmartOp{limitOp(PositionServiceConcurrency, OpServiceConcurrencyUtilLe, "100")}, Services: []*loadbalance.Service{hot}},
	})
	require.NoError(t, err)
	// the hot service is not part of rule 0, so rule 0 only sees `cool`'s load
	ctx := &RequestContext{ServiceLimits: []ServiceLimitInfo{
		{ServiceID: hot.ServiceID(), Concurrency: 100, HasConcurrency: true},
		{ServiceID: cool.ServiceID(), Concurrency: 10, HasConcurrency: true},
	}}
	_, idx, matched, _ := router.Evaluate(ctx)
	require.True(t, matched)
	require.Equal(t, 1, idx, "rule 0 must not read the other rule's service load")
}
