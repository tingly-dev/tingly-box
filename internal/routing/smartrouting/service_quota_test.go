package smartrouting

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func quotaOp(operation SmartOpOperation, value string) SmartOp {
	return SmartOp{
		Position:  PositionServiceQuota,
		Operation: operation,
		Value:     value,
		Meta:      SmartOpMeta{Type: ValueTypeInt},
	}
}

func TestEvaluateServiceQuotaOp_PassesWhenNoData(t *testing.T) {
	r := &Router{}
	ctx := &RequestContext{}
	op := quotaOp(OpServiceQuotaPctGe, "85")

	res := r.evaluateServiceQuotaOp(ctx, &op)

	require.True(t, res.Matched, "no quota data must pass, not be read as 0%%")
}

func TestEvaluateServiceQuotaOp_TakesTightestNotAverage(t *testing.T) {
	r := &Router{}
	ctx := &RequestContext{
		ServiceQuota: []ServiceQuotaInfo{
			{ServiceID: "svc-a", Pct: 20},
			{ServiceID: "svc-b", Pct: 90}, // the tightest
		},
	}

	geOp := quotaOp(OpServiceQuotaPctGe, "85")
	res := r.evaluateServiceQuotaOp(ctx, &geOp)
	require.True(t, res.Matched, "max(20, 90)=90 >= 85 should match; an average (55) would not")
	require.Equal(t, "90.0%", res.Actual)

	leOp := quotaOp(OpServiceQuotaPctLe, "50")
	res = r.evaluateServiceQuotaOp(ctx, &leOp)
	require.False(t, res.Matched, "max(20, 90)=90 is not <= 50")
}

func TestEvaluateServiceQuotaOp_Thresholds(t *testing.T) {
	r := &Router{}
	ctx := &RequestContext{ServiceQuota: []ServiceQuotaInfo{{ServiceID: "svc-a", Pct: 85}}}

	cases := []struct {
		op      SmartOpOperation
		value   string
		matched bool
	}{
		{OpServiceQuotaPctGe, "85", true},
		{OpServiceQuotaPctGe, "86", false},
		{OpServiceQuotaPctGt, "85", false},
		{OpServiceQuotaPctGt, "84", true},
		{OpServiceQuotaPctLe, "85", true},
		{OpServiceQuotaPctLe, "84", false},
		{OpServiceQuotaPctLt, "85", false},
		{OpServiceQuotaPctLt, "86", true},
	}
	for _, c := range cases {
		op := quotaOp(c.op, c.value)
		res := r.evaluateServiceQuotaOp(ctx, &op)
		require.Equal(t, c.matched, res.Matched, "op=%s value=%s", c.op, c.value)
	}
}

func TestEvaluateServiceQuotaOp_InvalidValue(t *testing.T) {
	r := &Router{}
	ctx := &RequestContext{ServiceQuota: []ServiceQuotaInfo{{ServiceID: "svc-a", Pct: 85}}}
	op := quotaOp(OpServiceQuotaPctGe, "not-a-number")

	res := r.evaluateServiceQuotaOp(ctx, &op)

	require.False(t, res.Matched)
	require.Contains(t, res.Reason, "invalid int")
}

func TestFilterQuotaForRule(t *testing.T) {
	all := []ServiceQuotaInfo{
		{ServiceID: "svc-a", Pct: 10},
		{ServiceID: "svc-b", Pct: 90},
	}
	services := []*loadbalance.Service{
		{Provider: "provider-b", Model: "model-b"},
	}
	// ServiceID() combines provider+model; build a service whose ServiceID matches "svc-b".
	// Rather than depend on the exact ServiceID() format, filter using the same
	// service and assert only entries whose ID is present survive.
	svc := services[0]
	filtered := filterQuotaForRule(all, []*loadbalance.Service{svc})
	for _, q := range filtered {
		require.Equal(t, svc.ServiceID(), q.ServiceID)
	}

	require.Nil(t, filterQuotaForRule(nil, services))
}

// TestSmartRouting_ServiceQuota_RuleLevel exercises the op through a full
// Router.Evaluate pass, including evaluateRule's per-rule ServiceQuota
// filtering (a second rule with a different service must not see the first
// rule's quota data).
func TestSmartRouting_ServiceQuota_RuleLevel(t *testing.T) {
	hotService := &loadbalance.Service{Provider: "prov-hot", Model: "m", Weight: 1, Active: true}
	coldService := &loadbalance.Service{Provider: "prov-cold", Model: "m", Weight: 1, Active: true}

	rules := []SmartRouting{
		{
			Description: "hot pool has quota data and is over threshold",
			Ops: []SmartOp{
				{Position: PositionServiceQuota, Operation: OpServiceQuotaPctGe, Value: "85", Meta: SmartOpMeta{Type: ValueTypeInt}},
			},
			Services: []*loadbalance.Service{hotService},
		},
	}
	router, err := NewRouter(rules)
	require.NoError(t, err)

	ctx := &RequestContext{
		ServiceQuota: []ServiceQuotaInfo{
			{ServiceID: hotService.ServiceID(), Pct: 92},
			{ServiceID: coldService.ServiceID(), Pct: 5}, // belongs to a different rule/service
		},
	}

	services, matched := router.EvaluateRequest(ctx)
	require.True(t, matched)
	require.Equal(t, []*loadbalance.Service{hotService}, services)

	// evaluateRule must restore the full ServiceQuota slice after evaluation
	// so a later stage (or a subsequent rule) still sees every service.
	require.Len(t, ctx.ServiceQuota, 2)
}
