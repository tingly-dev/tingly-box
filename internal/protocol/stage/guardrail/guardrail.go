// Package guardrail implements a protocol-native, full-duplex Guardrail Stage.
//
// The foundation is deliberately observe-only: evaluators inspect requests,
// complete responses, and stream events without changing live traffic. Policy
// enforcement and mutation are separate adapters built on this lifecycle.
package guardrail

import (
	"context"
	"fmt"
	"strings"
	"sync"

	protocol "github.com/tingly-dev/tingly-box/ai"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

// Phase identifies the concrete lifecycle surface inspected by an Evaluator.
type Phase string

const (
	PhaseRequest  Phase = "request"
	PhaseResponse Phase = "response"
	PhaseEvent    Phase = "event"
)

// Verdict is an evaluator's protocol-neutral policy decision.
type Verdict string

const (
	VerdictAllow Verdict = "allow"
	VerdictBlock Verdict = "block"
)

// Decision is an immutable observation result. Reason is diagnostic context;
// it must not contain full request or response bodies.
type Decision struct {
	Verdict Verdict
	Reason  string
}

// Observation reports one dry-run evaluation without affecting live traffic.
// Err is evaluator failure, not downstream provider failure.
type Observation struct {
	Stage    string
	Protocol protocol.APIType
	Phase    Phase
	Decision Decision
	Err      error
}

// Evaluator inspects one concrete protocol. Implementations must be
// concurrency-safe and must not mutate Call, Response, Event, or their native
// protocol values. Mutable correlation belongs in the Session returned by
// Open, never on the shared Evaluator.
type Evaluator interface {
	Protocol() protocol.APIType
	Open(ctx context.Context, call protocolstage.Call) (Session, error)
}

// Session owns all mutable state for one Complete or Stream invocation.
type Session interface {
	EvaluateRequest(ctx context.Context, call protocolstage.Call) (Decision, error)
	EvaluateResponse(ctx context.Context, response *protocolstage.Response) (Decision, error)
	EvaluateEvent(ctx context.Context, event protocolstage.Event) (Decision, error)
}

// Observer receives dry-run facts. It must not retain native protocol values;
// Observation intentionally contains only bounded diagnostic metadata.
type Observer func(Observation)

// Config constructs one observe-only Guardrail Stage.
type Config struct {
	Name      string
	Evaluator Evaluator
	Observe   Observer
}

// New constructs a protocol-native Guardrail Stage. Evaluation failures are
// fail-open and reported to Observe; downstream failures retain their normal
// endpoint semantics.
func New(config Config) (protocolstage.Stage, error) {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		name = "guardrail"
	}
	if config.Evaluator == nil {
		return nil, fmt.Errorf("construct Guardrail Stage %q: evaluator is nil", name)
	}
	api := config.Evaluator.Protocol()
	if api == "" {
		return nil, fmt.Errorf("construct Guardrail Stage %q: evaluator protocol is empty", name)
	}
	return &guardrailStage{name: name, api: api, evaluator: config.Evaluator, observe: config.Observe}, nil
}

type guardrailStage struct {
	name      string
	api       protocol.APIType
	evaluator Evaluator
	observe   Observer
}

func (s *guardrailStage) Name() string               { return s.name }
func (s *guardrailStage) Protocol() protocol.APIType { return s.api }
func (s *guardrailStage) Wrap(next protocolstage.Endpoint) protocolstage.Endpoint {
	return &guardrailEndpoint{stage: s, next: next}
}

type guardrailEndpoint struct {
	stage *guardrailStage
	next  protocolstage.Endpoint
}

func (e *guardrailEndpoint) Protocol() protocol.APIType { return e.stage.api }

func (e *guardrailEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	session := e.open(ctx, call)
	e.evaluateRequest(ctx, session, call)
	response, err := e.next.Complete(ctx, call)
	if err != nil || response == nil {
		return response, err
	}
	e.evaluateResponse(ctx, session, response)
	return response, nil
}

func (e *guardrailEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	session := e.open(ctx, call)
	e.evaluateRequest(ctx, session, call)
	stream, err := e.next.Stream(ctx, call)
	if err != nil {
		return nil, err
	}
	return &observedStream{parent: e, session: session, stream: stream}, nil
}

func (e *guardrailEndpoint) open(ctx context.Context, call protocolstage.Call) Session {
	session, err := e.stage.evaluator.Open(ctx, call)
	if err != nil {
		e.report(PhaseRequest, Decision{}, err)
		return nil
	}
	if session == nil {
		e.report(PhaseRequest, Decision{}, fmt.Errorf("Guardrail Stage evaluator returned a nil session"))
	}
	return session
}

func (e *guardrailEndpoint) evaluateRequest(ctx context.Context, session Session, call protocolstage.Call) {
	if session == nil {
		return
	}
	decision, err := session.EvaluateRequest(ctx, call)
	e.report(PhaseRequest, decision, err)
}

func (e *guardrailEndpoint) evaluateResponse(ctx context.Context, session Session, response *protocolstage.Response) {
	if session == nil {
		return
	}
	decision, err := session.EvaluateResponse(ctx, response)
	e.report(PhaseResponse, decision, err)
}

func (e *guardrailEndpoint) evaluateEvent(ctx context.Context, session Session, event protocolstage.Event) {
	if session == nil {
		return
	}
	decision, err := session.EvaluateEvent(ctx, event)
	e.report(PhaseEvent, decision, err)
}

func (e *guardrailEndpoint) report(phase Phase, decision Decision, err error) {
	if e.stage.observe == nil {
		return
	}
	e.stage.observe(Observation{
		Stage:    e.stage.name,
		Protocol: e.stage.api,
		Phase:    phase,
		Decision: decision,
		Err:      err,
	})
}

type observedStream struct {
	parent  *guardrailEndpoint
	session Session
	stream  protocolstage.EventStream

	closeOnce sync.Once
	closeErr  error
}

func (s *observedStream) Next(ctx context.Context) (protocolstage.Event, error) {
	event, err := s.stream.Next(ctx)
	if err != nil {
		return event, err
	}
	s.parent.evaluateEvent(ctx, s.session, event)
	return event, nil
}

func (s *observedStream) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.stream.Close()
	})
	return s.closeErr
}

func (s *observedStream) Result() protocolstage.StreamResult { return s.stream.Result() }

var _ protocolstage.Stage = (*guardrailStage)(nil)
var _ protocolstage.Endpoint = (*guardrailEndpoint)(nil)
var _ protocolstage.EventStream = (*observedStream)(nil)
