package guardrail

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	protocol "github.com/tingly-dev/tingly-box/ai"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

func TestGuardrailStageObservesCompleteLifecycleWithoutMutation(t *testing.T) {
	t.Parallel()

	request := &struct{ Text string }{Text: "request"}
	responseValue := &struct{ Text string }{Text: "response"}
	evaluator := &fakeEvaluator{api: protocol.TypeAnthropicBeta}
	var observations []Observation
	guardrail := mustGuardrail(t, Config{
		Name:      "guardrail_beta",
		Evaluator: evaluator,
		Observe:   func(observation Observation) { observations = append(observations, observation) },
	})
	terminal := &fakeEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(_ context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
			if call.Request != request {
				t.Fatalf("terminal request = %p, want %p", call.Request, request)
			}
			return &protocolstage.Response{Value: responseValue, Model: "provider", SideEffectsCommitted: true}, nil
		},
	}
	endpoint, err := protocolstage.Compose(terminal, guardrail)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Value != responseValue || response.Model != "provider" || !response.SideEffectsCommitted {
		t.Fatalf("response changed = %+v", response)
	}
	if got := evaluator.sessions; got != 1 {
		t.Fatalf("sessions = %d, want 1", got)
	}
	wantPhases := []Phase{PhaseRequest, PhaseResponse}
	if got := observationPhases(observations); !reflect.DeepEqual(got, wantPhases) {
		t.Fatalf("phases = %v, want %v", got, wantPhases)
	}
}

func TestGuardrailStageObservesStreamAndPreservesOwnership(t *testing.T) {
	t.Parallel()

	evaluator := &fakeEvaluator{api: protocol.TypeAnthropicBeta}
	var observations []Observation
	guardrail := mustGuardrail(t, Config{
		Evaluator: evaluator,
		Observe:   func(observation Observation) { observations = append(observations, observation) },
	})
	target := &fakeStream{
		events: []protocolstage.Event{{Value: "first"}, {Value: "second"}},
		result: protocolstage.StreamResult{
			Usage:                protocol.NewTokenUsage(3, 2),
			Model:                "stream-model",
			SideEffectsCommitted: true,
		},
	}
	terminal := &fakeEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(context.Context, protocolstage.Call) (protocolstage.EventStream, error) {
			return target, nil
		},
	}
	endpoint, err := protocolstage.Compose(terminal, guardrail)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	stream, err := endpoint.Stream(context.Background(), protocolstage.Call{Request: "request"})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	for index, want := range []string{"first", "second"} {
		event, nextErr := stream.Next(context.Background())
		if nextErr != nil || event.Value != want {
			t.Fatalf("Next(%d) = (%v, %v), want %q", index, event.Value, nextErr, want)
		}
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final Next() error = %v, want io.EOF", err)
	}
	if got := stream.Result(); got.Model != "stream-model" || got.Usage == nil || !got.SideEffectsCommitted {
		t.Fatalf("Result() = %+v", got)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if target.closeCount != 1 {
		t.Fatalf("target close count = %d, want 1", target.closeCount)
	}
	wantPhases := []Phase{PhaseRequest, PhaseEvent, PhaseEvent}
	if got := observationPhases(observations); !reflect.DeepEqual(got, wantPhases) {
		t.Fatalf("phases = %v, want %v", got, wantPhases)
	}
}

func TestGuardrailStageFailsOpenOnEvaluatorErrors(t *testing.T) {
	t.Parallel()

	evaluationErr := errors.New("evaluation unavailable")
	evaluator := &fakeEvaluator{api: protocol.TypeAnthropicBeta, openErr: evaluationErr}
	var observations []Observation
	guardrail := mustGuardrail(t, Config{
		Evaluator: evaluator,
		Observe:   func(observation Observation) { observations = append(observations, observation) },
	})
	terminal := &fakeEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(context.Context, protocolstage.Call) (*protocolstage.Response, error) {
			return &protocolstage.Response{Value: "ok"}, nil
		},
	}
	endpoint, err := protocolstage.Compose(terminal, guardrail)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: "request"})
	if err != nil || response.Value != "ok" {
		t.Fatalf("Complete() = (%+v, %v)", response, err)
	}
	if len(observations) != 1 || !errors.Is(observations[0].Err, evaluationErr) {
		t.Fatalf("observations = %+v", observations)
	}
}

func TestGuardrailStageRejectsInvalidConstruction(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{}); err == nil {
		t.Fatal("New() error = nil, want evaluator error")
	}
	if _, err := New(Config{Evaluator: &fakeEvaluator{}}); err == nil {
		t.Fatal("New() error = nil, want protocol error")
	}
}

type fakeEvaluator struct {
	api      protocol.APIType
	openErr  error
	sessions int
}

func (e *fakeEvaluator) Protocol() protocol.APIType { return e.api }
func (e *fakeEvaluator) Open(context.Context, protocolstage.Call) (Session, error) {
	if e.openErr != nil {
		return nil, e.openErr
	}
	e.sessions++
	return fakeSession{}, nil
}

type fakeSession struct{}

func (fakeSession) EvaluateRequest(context.Context, protocolstage.Call) (Decision, error) {
	return Decision{Verdict: VerdictAllow, Reason: "request observed"}, nil
}
func (fakeSession) EvaluateResponse(context.Context, *protocolstage.Response) (Decision, error) {
	return Decision{Verdict: VerdictAllow, Reason: "response observed"}, nil
}
func (fakeSession) EvaluateEvent(context.Context, protocolstage.Event) (Decision, error) {
	return Decision{Verdict: VerdictAllow, Reason: "event observed"}, nil
}

type fakeEndpoint struct {
	api      protocol.APIType
	complete func(context.Context, protocolstage.Call) (*protocolstage.Response, error)
	stream   func(context.Context, protocolstage.Call) (protocolstage.EventStream, error)
}

func (e *fakeEndpoint) Protocol() protocol.APIType { return e.api }
func (e *fakeEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	return e.complete(ctx, call)
}
func (e *fakeEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	return e.stream(ctx, call)
}

type fakeStream struct {
	events     []protocolstage.Event
	index      int
	result     protocolstage.StreamResult
	closeCount int
}

func (s *fakeStream) Next(context.Context) (protocolstage.Event, error) {
	if s.index >= len(s.events) {
		return protocolstage.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
func (s *fakeStream) Close() error {
	s.closeCount++
	return nil
}
func (s *fakeStream) Result() protocolstage.StreamResult { return s.result }

func mustGuardrail(t *testing.T, config Config) protocolstage.Stage {
	t.Helper()
	guardrail, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return guardrail
}

func observationPhases(observations []Observation) []Phase {
	phases := make([]Phase, 0, len(observations))
	for _, observation := range observations {
		phases = append(phases, observation.Phase)
	}
	return phases
}
