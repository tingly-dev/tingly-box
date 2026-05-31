package routing

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
)

// ---------------------------------------------------------------------------
// Harness — pipeline driver
// ---------------------------------------------------------------------------

// runStagePipeline mimics the body of ServiceSelector.Select's stage loop for
// in-package assertions. Returns the first result a stage produced, the index
// of the producing stage, and a slice naming every stage that was evaluated.
func runStagePipeline(t *testing.T, stages []SelectionStage, ctx *SelectionContext, state *selectionState) (*SelectionResult, int, []string) {
	t.Helper()
	var evaluated []string
	for i, stage := range stages {
		evaluated = append(evaluated, stage.Name())
		result, handled := stage.Evaluate(ctx, state)
		if handled {
			return result, i, evaluated
		}
	}
	return nil, -1, evaluated
}

// recordingStage records every Evaluate call (with the request pointer it
// observed) and returns a canned result. Used as a stand-in for the
// LoadBalancerStage so tests can verify the request handed downstream after
// a bypass is the MUTATED one.
type recordingStage struct {
	name    string
	calls   []recordedCall
	result  *SelectionResult
	handled bool
}

type recordedCall struct {
	Request any
}

func (s *recordingStage) Name() string { return s.name }

func (s *recordingStage) Evaluate(ctx *SelectionContext, _ *selectionState) (*SelectionResult, bool) {
	s.calls = append(s.calls, recordedCall{Request: ctx.Request})
	return s.result, s.handled
}

// betaReqWithImage builds a Beta request carrying one image — local copy to
// keep this harness file self-contained.
func betaReqWithImage(prompt string) *anthropic.BetaMessageNewParams {
	const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: prompt}},
					anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
						Data:      tinyPNG,
						MediaType: anthropic.BetaBase64ImageSourceMediaType("image/png"),
					}),
				},
			},
		},
	}
}

// bypassOpPosition / bypassOpEnabled are synthetic position/op values used to
// register a processor-bearing op for the generic bypass tests below. They do
// not need a real evaluator: the tests register a matching processor directly
// and drive the stage with a rule whose model op also matches, so the rule is
// selected and the processor runs. (The vision-proxy op these tests used to
// piggy-back on was removed in favor of the rule/scenario vision proxy flags.)
const (
	bypassOpPosition smartrouting.SmartOpPosition  = smartrouting.PositionModel
	bypassOpEnabled  smartrouting.SmartOpOperation = smartrouting.OpModelContains
)

// bypassOp builds a model-contains op that both matches the test request and
// carries a registered processor, exercising the implicit-bypass path.
func bypassOp() smartrouting.SmartOp {
	return smartrouting.SmartOp{
		UUID:      "bypass-op",
		Position:  bypassOpPosition,
		Operation: bypassOpEnabled,
		Value:     "claude",
	}
}

// ---------------------------------------------------------------------------
// Tests — bypass behavior (Phase B makes these green)
// ---------------------------------------------------------------------------

func TestSmartRoutingStage_ProcessorBypass_RunsProcessorAndContinues(t *testing.T) {
	// A processor registered for a matched op must run, and the smart-routing
	// stage must return (nil, false) so the pipeline continues to the next
	// stage (implicit bypass).
	called := 0
	smartrouting.RegisterProcessor(bypassOpPosition, bypassOpEnabled,
		processorFunc(func(_ *smartrouting.ProcessorContext) error {
			called++
			return nil
		}))
	t.Cleanup(func() {
		smartrouting.UnregisterProcessor(bypassOpPosition, bypassOpEnabled)
	})

	services := []*loadbalance.Service{testService("provider-a", "vision-model", true)}
	rule := testSmartRule("rule-1", "any-model", services, bypassOp())
	ctx := testContext(rule, "")
	ctx.Request = betaReqWithImage("describe")

	stage := NewSmartRoutingStage(&mockLoadBalancer{service: services[0]}, newMockAffinityStore())
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))

	require.False(t, handled, "stage must not terminate when a processor is present (implicit bypass)")
	require.Equal(t, 1, called, "registered processor must be invoked")
}

func TestSmartRoutingStage_NoProcessor_TerminalSelectionUnchanged(t *testing.T) {
	// Rules without processor-bearing ops keep current terminal behavior.
	services := []*loadbalance.Service{testService("provider-a", "gpt-4", true)}
	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(&mockLoadBalancer{service: services[0]}, newMockAffinityStore())
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))

	require.True(t, handled, "no processor → terminal selection")
	require.NotNil(t, result)
	require.Equal(t, "gpt-4", result.Service.Model)
}

func TestSmartRoutingStage_BypassedRule_NotReentered(t *testing.T) {
	called := 0
	smartrouting.RegisterProcessor(bypassOpPosition, bypassOpEnabled,
		processorFunc(func(_ *smartrouting.ProcessorContext) error {
			called++
			return nil
		}))
	t.Cleanup(func() {
		smartrouting.UnregisterProcessor(bypassOpPosition, bypassOpEnabled)
	})

	services := []*loadbalance.Service{testService("provider-a", "vision-model", true)}
	rule := testSmartRule("rule-1", "any-model", services, bypassOp())
	ctx := testContext(rule, "")
	ctx.Request = betaReqWithImage("describe")
	// Pre-mark rule 0 as already bypassed.
	ctx.BypassedSmartRules = map[int]struct{}{0: {}}

	stage := NewSmartRoutingStage(&mockLoadBalancer{service: services[0]}, newMockAffinityStore())
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))

	require.False(t, handled, "stage must not terminate; pipeline continues")
	require.Equal(t, 0, called, "processor must NOT be re-invoked for an already-bypassed rule")
}

func TestSmartRoutingStage_ProcessorMutatesRequest_LoadBalancerSeesMutation(t *testing.T) {
	// Processor mutates ctx.Request; the next stage in the pipeline must see
	// the mutated request.
	smartrouting.RegisterProcessor(bypassOpPosition, bypassOpEnabled,
		processorFunc(func(pctx *smartrouting.ProcessorContext) error {
			// Simulate vision-proxy mutation: drop image content blocks entirely.
			if r, ok := pctx.Request.(*anthropic.BetaMessageNewParams); ok {
				for i := range r.Messages {
					filtered := r.Messages[i].Content[:0]
					for _, b := range r.Messages[i].Content {
						if b.OfImage == nil {
							filtered = append(filtered, b)
						}
					}
					r.Messages[i].Content = filtered
				}
			}
			return nil
		}))
	t.Cleanup(func() {
		smartrouting.UnregisterProcessor(bypassOpPosition, bypassOpEnabled)
	})

	services := []*loadbalance.Service{testService("provider-a", "vision-model", true)}
	rule := testSmartRule("rule-1", "any-model", services, bypassOp())
	ctx := testContext(rule, "")
	ctx.Request = betaReqWithImage("describe")

	smart := NewSmartRoutingStage(&mockLoadBalancer{service: services[0]}, newMockAffinityStore())
	rec := &recordingStage{name: "recording", result: NewResult(services[0], "recording"), handled: true}

	_, idx, evaluated := runStagePipeline(t, []SelectionStage{smart, rec}, ctx, newSelectionState(ctx.Rule))

	require.Equal(t, 1, idx, "recording stage produced the result (smart bypassed)")
	require.Equal(t, []string{"smart_routing", "recording"}, evaluated)
	require.Len(t, rec.calls, 1, "recording stage saw the request once")

	betaReq, ok := rec.calls[0].Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)
	for _, m := range betaReq.Messages {
		for _, b := range m.Content {
			require.Nil(t, b.OfImage, "no image should remain in the request seen by the LB stage")
		}
	}
}

// processorFunc is an inline OpProcessor adapter used by the tests above.
type processorFunc func(*smartrouting.ProcessorContext) error

func (f processorFunc) Process(p *smartrouting.ProcessorContext) error { return f(p) }
