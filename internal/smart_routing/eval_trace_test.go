package smartrouting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func TestTraceEvaluation_MatchAndMiss(t *testing.T) {
	router, err := NewRouter([]SmartRouting{
		{
			Description: "haiku only",
			Ops: []SmartOp{
				{Position: PositionModel, Operation: OpModelContains, Value: "haiku"},
			},
			Services: []*loadbalance.Service{
				{Provider: "p1", Model: "claude-haiku", Weight: 1, Active: true},
			},
		},
		{
			Description: "long context",
			Ops: []SmartOp{
				{Position: PositionToken, Operation: OpTokenGe, Value: "1000",
					Meta: SmartOpMeta{Type: ValueTypeInt}},
			},
			Services: []*loadbalance.Service{
				{Provider: "p2", Model: "claude-opus", Weight: 1, Active: true},
			},
		},
	})
	require.NoError(t, err)

	ctx := &RequestContext{
		Model:           "claude-sonnet",
		EstimatedTokens: 5000,
	}
	trace := router.TraceEvaluation(ctx)
	require.Len(t, trace, 2, "second rule should still be evaluated since first failed")

	// Rule 0 — model contains haiku, should be a miss with explanation
	require.Equal(t, 0, trace[0].RuleIndex)
	require.False(t, trace[0].Matched)
	require.Equal(t, 1, trace[0].OpsTotal)
	require.Len(t, trace[0].Ops, 1)
	require.False(t, trace[0].Ops[0].Matched)
	require.NotEmpty(t, trace[0].Ops[0].Reason)
	require.Equal(t, "claude-sonnet", trace[0].Ops[0].Actual)

	// Rule 1 — token >= 1000 with 5000, should match and stop evaluation
	require.Equal(t, 1, trace[1].RuleIndex)
	require.True(t, trace[1].Matched)
	require.True(t, trace[1].Ops[0].Matched)
	require.Equal(t, "5000", trace[1].Ops[0].Actual)
}

func TestTraceEvaluation_SnippetWindowAroundMatch(t *testing.T) {
	// Build a long user message with the needle buried in the middle so the
	// trace has to extract a window around it instead of dumping the whole body.
	prefix := strings.Repeat("aaaaaaaaaa ", 200) // ~2200 chars
	suffix := strings.Repeat(" bbbbbbbbbb", 200)
	body := prefix + "NEEDLE" + suffix

	router, err := NewRouter([]SmartRouting{
		{
			Description: "user contains NEEDLE",
			Ops: []SmartOp{
				{Position: PositionContextUser, Operation: OpContextUserContains, Value: "NEEDLE"},
			},
			Services: []*loadbalance.Service{
				{Provider: "p", Model: "m", Weight: 1, Active: true},
			},
		},
	})
	require.NoError(t, err)

	trace := router.TraceEvaluation(&RequestContext{UserMessages: []string{body}})
	require.Len(t, trace, 1)
	require.True(t, trace[0].Matched)

	actual := trace[0].Ops[0].Actual
	require.Contains(t, actual, "NEEDLE", "snippet should contain the matched needle")
	require.True(t, strings.HasPrefix(actual, "…"), "long-prefix matches should be ellipsised on the left")
	require.True(t, strings.HasSuffix(actual, "…"), "long-suffix matches should be ellipsised on the right")
	require.Less(t, len(actual), 300, "snippet should be much smaller than the original body")
}

func TestTraceEvaluation_NoMatchKeepsHeadSnippet(t *testing.T) {
	body := strings.Repeat("xxxxxxxxxx", 500) // 5000 chars
	router, err := NewRouter([]SmartRouting{
		{
			Description: "user contains zzz (won't match)",
			Ops: []SmartOp{
				{Position: PositionContextUser, Operation: OpContextUserContains, Value: "zzz"},
			},
			Services: []*loadbalance.Service{
				{Provider: "p", Model: "m", Weight: 1, Active: true},
			},
		},
	})
	require.NoError(t, err)

	trace := router.TraceEvaluation(&RequestContext{UserMessages: []string{body}})
	require.Len(t, trace, 1)
	require.False(t, trace[0].Matched)

	actual := trace[0].Ops[0].Actual
	require.True(t, strings.HasSuffix(actual, "…"), "no-match should fall back to a head snippet with trailing ellipsis")
	require.LessOrEqual(t, len(actual), snippetHeadLen+5, "no-match head snippet should stay small")
}

func TestTraceEvaluation_ShortCircuitsAtFirstFailedOp(t *testing.T) {
	router, err := NewRouter([]SmartRouting{
		{
			Description: "two ops, second never reached when first fails",
			Ops: []SmartOp{
				{Position: PositionModel, Operation: OpModelEquals, Value: "no-such-model"},
				{Position: PositionToken, Operation: OpTokenGe, Value: "10",
					Meta: SmartOpMeta{Type: ValueTypeInt}},
			},
			Services: []*loadbalance.Service{
				{Provider: "p", Model: "m", Weight: 1, Active: true},
			},
		},
	})
	require.NoError(t, err)

	trace := router.TraceEvaluation(&RequestContext{Model: "actually-different", EstimatedTokens: 100})
	require.Len(t, trace, 1)
	require.False(t, trace[0].Matched)
	require.Equal(t, 1, trace[0].OpsEvaluated, "second op should not be evaluated after first miss")
	require.Equal(t, 2, trace[0].OpsTotal)
	require.Len(t, trace[0].Ops, 1)
}
