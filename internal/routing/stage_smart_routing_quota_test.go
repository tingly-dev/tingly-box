package routing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
)

func testServiceQuotaOp(operation smartrouting.SmartOpOperation, value string) smartrouting.SmartOp {
	return smartrouting.SmartOp{
		Position:  smartrouting.PositionServiceQuota,
		Operation: operation,
		Value:     value,
		Meta:      smartrouting.SmartOpMeta{Type: smartrouting.ValueTypeInt},
	}
}

// TestSmartRouting_ServiceQuota_Match verifies the end-to-end wiring: a
// QuotaProvider set via SetQuotaProvider is consulted per-request, and a
// rule whose service is over the configured threshold matches.
func TestSmartRouting_ServiceQuota_Match(t *testing.T) {
	services := []*loadbalance.Service{testService("prov-hot", "gpt-4", true)}
	rule := testSmartRule("rule-1", "gpt-4", services, testServiceQuotaOp(smartrouting.OpServiceQuotaPctGe, "85"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4")

	qp := newMockQuotaProvider()
	qp.setPct("prov-hot", 92)

	stage := NewSmartRoutingStage(newMockAffinityStore())
	stage.SetQuotaProvider(qp)

	narrowed, final, err := stage.Evaluate(ctx, initialCandidateServices(ctx.Rule))
	require.NoError(t, err)
	require.Nil(t, final)
	require.Len(t, narrowed, 1)
	require.Equal(t, "prov-hot", narrowed[0].Provider)
}

// TestSmartRouting_ServiceQuota_BelowThreshold_NoMatch verifies the rule
// falls through to the base pool when quota is under the threshold.
func TestSmartRouting_ServiceQuota_BelowThreshold_NoMatch(t *testing.T) {
	services := []*loadbalance.Service{testService("prov-cool", "gpt-4", true)}
	rule := testSmartRule("rule-1", "gpt-4", services, testServiceQuotaOp(smartrouting.OpServiceQuotaPctGe, "85"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4")

	qp := newMockQuotaProvider()
	qp.setPct("prov-cool", 10)

	stage := NewSmartRoutingStage(newMockAffinityStore())
	stage.SetQuotaProvider(qp)

	narrowed, final, err := stage.Evaluate(ctx, initialCandidateServices(ctx.Rule))
	require.NoError(t, err)
	require.Nil(t, final)
	require.Equal(t, services, narrowed, "under threshold must fall back to the rule's base pool")
}

// TestSmartRouting_ServiceQuota_IgnoresResourceKindWindows proves the
// service_quota op does not trigger avoidance off a standing balance/credit
// (Kind == WindowKindResource) even though it is fully countable — only
// self-healing quota (Kind == WindowKindLimit) should drive this op. See
// Pct's kinds filter in ai/quota/semantic.go and .design/quota-semantics.md §8.1.
func TestSmartRouting_ServiceQuota_IgnoresResourceKindWindows(t *testing.T) {
	services := []*loadbalance.Service{testService("prov-balance", "gpt-4", true)}
	rule := testSmartRule("rule-1", "gpt-4", services, testServiceQuotaOp(smartrouting.OpServiceQuotaPctGe, "85"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4")

	qp := newMockQuotaProvider()
	qp.setResourcePct("prov-balance", 95) // a near-exhausted balance, not standard quota

	stage := NewSmartRoutingStage(newMockAffinityStore())
	stage.SetQuotaProvider(qp)

	narrowed, final, err := stage.Evaluate(ctx, initialCandidateServices(ctx.Rule))
	require.NoError(t, err)
	require.Nil(t, final)
	require.Equal(t, services, narrowed, "resource-kind usage must not trigger avoidance")
}

// TestSmartRouting_ServiceQuota_NoProviderWired_Passes ensures a
// service_quota op does not block routing when no QuotaProvider was ever
// set (e.g. quota manager failed to initialize) — the op is optional.
func TestSmartRouting_ServiceQuota_NoProviderWired_Passes(t *testing.T) {
	services := []*loadbalance.Service{testService("prov-unknown", "gpt-4", true)}
	rule := testSmartRule("rule-1", "gpt-4", services, testServiceQuotaOp(smartrouting.OpServiceQuotaPctGe, "85"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4")

	stage := NewSmartRoutingStage(newMockAffinityStore()) // no SetQuotaProvider call

	narrowed, final, err := stage.Evaluate(ctx, initialCandidateServices(ctx.Rule))
	require.NoError(t, err)
	require.Nil(t, final)
	require.Len(t, narrowed, 1, "op must pass through (match) when quota is unwired, not block routing")
	require.Equal(t, "prov-unknown", narrowed[0].Provider)
}
