package benchmark

import (
	"testing"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
	"github.com/tingly-dev/tingly-box/vmodel/vmodeltest"
)

// toRoundTrip adapts a vmodeltest.ParsedResponse into a check.RoundTripResult so
// the reusable assertion library can run against it.
func toRoundTrip(resp *vmodeltest.ParsedResponse) *check.RoundTripResult {
	r := &check.RoundTripResult{
		IsStreaming:  resp.IsStreaming,
		HTTPStatus:   resp.HTTPStatus,
		RawBody:      resp.RawBody,
		StreamEvents: resp.StreamEvents,
		Content:      resp.Content,
		Role:         resp.Role,
		Model:        resp.Model,
		FinishReason: resp.FinishReason,
	}
	for _, tc := range resp.ToolCalls {
		r.ToolCalls = append(r.ToolCalls, check.ToolCallResult{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	if resp.Usage != nil {
		r.Usage = &check.TokenUsage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	}
	return r
}

// TestProductionServer_RealModels verifies the production responder returns
// wire-correct OpenAI and Anthropic responses and that the capture layer counts
// hits and records the forwarded request.
func TestProductionServer_RealModels(t *testing.T) {
	srv := NewProductionServer()
	url := srv.InProcess()
	defer srv.Close()

	client := vmodeltest.NewClient(url)

	chat := client.SendOpenAIChatModel(t, "echo-model", false)
	if chat.HTTPStatus != 200 {
		t.Fatalf("openai chat status: got %d, body=%s", chat.HTTPStatus, chat.RawBody)
	}
	if err := check.AssertContentNonEmpty().Check(toRoundTrip(chat)); err != nil {
		t.Fatalf("openai chat content: %v", err)
	}

	msg := client.SendAnthropicV1Model(t, "echo-model", false)
	if msg.HTTPStatus != 200 {
		t.Fatalf("anthropic status: got %d, body=%s", msg.HTTPStatus, msg.RawBody)
	}
	if err := check.AssertContentNonEmpty().Check(toRoundTrip(msg)); err != nil {
		t.Fatalf("anthropic content: %v", err)
	}

	// Observability: two calls, one per endpoint, last chat request carries the model.
	if got := srv.CallCount(); got != 2 {
		t.Fatalf("call count: got %d, want 2", got)
	}
	if got := srv.EndpointHits(EndpointChat); got != 1 {
		t.Fatalf("chat hits: got %d, want 1", got)
	}
	if got := srv.EndpointHits(EndpointAnthropic); got != 1 {
		t.Fatalf("anthropic hits: got %d, want 1", got)
	}
	if last := srv.LastRequest(EndpointChat); last == nil || last.JSON()["model"] != "echo-model" {
		t.Fatalf("last chat request did not capture model=echo-model: %+v", last)
	}
}

// TestScenarioServer_Fixtures verifies the scenario responder serves a registered
// scenario's fixture and that the scenario's own assertions pass against it.
func TestScenarioServer_Fixtures(t *testing.T) {
	srv := NewScenarioServer()
	url := srv.InProcess()
	defer srv.Close()

	sc := scenario.TextScenario()
	srv.RegisterScenario(sc)

	client := vmodeltest.NewClient(url)
	// Raw scenario name as the model is accepted by the scenario responder.
	resp := client.SendOpenAIChatModel(t, sc.Name, false)

	rt := toRoundTrip(resp)
	for _, a := range sc.Assertions {
		if err := a.Check(rt); err != nil {
			t.Fatalf("scenario %q assertion %q failed: %v", sc.Name, a.Name, err)
		}
	}
	if got := srv.EndpointHits(EndpointChat); got != 1 {
		t.Fatalf("chat hits: got %d, want 1", got)
	}
}

// TestListenTransport verifies the real-TCP transport path works end to end.
func TestListenTransport(t *testing.T) {
	srv := NewProductionServer()
	url, err := srv.Listen(":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer srv.Close()

	if srv.Port() == 0 {
		t.Fatal("expected a non-zero ephemeral port")
	}

	resp := vmodeltest.NewClient(url).SendOpenAIChatModel(t, "echo-model", false)
	if resp.HTTPStatus != 200 {
		t.Fatalf("status over TCP: got %d", resp.HTTPStatus)
	}
	if srv.CallCount() != 1 {
		t.Fatalf("call count: got %d, want 1", srv.CallCount())
	}
}
