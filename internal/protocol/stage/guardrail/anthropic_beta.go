package guardrail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailsruntime "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	guardrailspipeline "github.com/tingly-dev/tingly-box/internal/guardrails/pipeline"
	protocol "github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

// AnthropicBetaConfig supplies the existing Guardrails policy runtime and the
// request metadata envelope. The Stage owns a fresh credential/stream state for
// every Complete or Stream call.
type AnthropicBetaConfig struct {
	Name      string
	Runtime   *guardrailsruntime.Guardrails
	BaseInput guardrailscore.Input
	Observe   Observer
}

// NewAnthropicBeta constructs an authoritative Anthropic Beta Guardrail Stage.
// It preserves the legacy fail-open behavior for request and complete-response
// evaluation errors while keeping stream rewrite errors visible to the caller.
func NewAnthropicBeta(config AnthropicBetaConfig) (protocolstage.Stage, error) {
	name := config.Name
	if name == "" {
		name = "guardrail_anthropic_beta"
	}
	if config.Runtime == nil || config.Runtime.PolicyEngine() == nil {
		return nil, fmt.Errorf("construct Anthropic Beta Guardrail Stage %q: runtime is unavailable", name)
	}
	return &anthropicBetaStage{
		name:      name,
		runtime:   config.Runtime,
		baseInput: config.BaseInput,
		observe:   config.Observe,
	}, nil
}

type anthropicBetaStage struct {
	name      string
	runtime   *guardrailsruntime.Guardrails
	baseInput guardrailscore.Input
	observe   Observer
}

func (s *anthropicBetaStage) Name() string             { return s.name }
func (*anthropicBetaStage) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }
func (s *anthropicBetaStage) Wrap(next protocolstage.Endpoint) protocolstage.Endpoint {
	return &anthropicBetaEndpoint{stage: s, next: next}
}

type anthropicBetaEndpoint struct {
	stage *anthropicBetaStage
	next  protocolstage.Endpoint
}

func (*anthropicBetaEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }

func (e *anthropicBetaEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	session, prepared, err := e.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	response, err := e.next.Complete(ctx, prepared)
	if err != nil || response == nil {
		return response, err
	}
	message, ok := response.Value.(*anthropic.BetaMessage)
	if !ok || message == nil {
		return nil, fmt.Errorf("Anthropic Beta Guardrail Stage received response %T", response.Value)
	}

	input := session.responseInput()
	mutation, evaluationErr := guardrailspipeline.ProcessAnthropicV1BetaNonStreamResponse(ctx, e.stage.runtime, input, message)
	if evaluationErr != nil {
		e.report(PhaseResponse, Decision{}, evaluationErr)
		return response, nil
	}
	if !mutation.Changed {
		guardrailsmutate.RestoreAnthropicV1BetaResponseCredentials(session.mask, message)
	}
	e.report(PhaseResponse, decisionFromResult(mutation.Evaluation.Result), nil)
	return response, nil
}

func (e *anthropicBetaEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	session, prepared, err := e.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	stream, err := e.next.Stream(ctx, prepared)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, fmt.Errorf("Anthropic Beta Guardrail Stage received a nil stream")
	}
	baseInput := session.responseInput()
	streamState := newGuardrailsStreamState()
	onEvent, onError := guardrailspipeline.NewGuardrailsHooks(ctx, e.stage.runtime, baseInput, streamState)
	return &anthropicBetaGuardrailStream{
		parent:      e,
		stream:      stream,
		mask:        session.mask,
		streamState: streamState,
		onEvent:     onEvent,
		onError:     onError,
	}, nil
}

type anthropicBetaSession struct {
	request *anthropic.BetaMessageNewParams
	base    guardrailscore.Input
	mask    *guardrailscore.CredentialMaskState
}

func (e *anthropicBetaEndpoint) prepare(ctx context.Context, call protocolstage.Call) (*anthropicBetaSession, protocolstage.Call, error) {
	request, ok := call.Request.(*anthropic.BetaMessageNewParams)
	if !ok || request == nil {
		return nil, protocolstage.Call{}, fmt.Errorf("Anthropic Beta Guardrail Stage received request %T", call.Request)
	}
	mask := guardrailscore.NewCredentialMaskState()
	base := e.stage.baseInput
	base.Direction = guardrailscore.DirectionRequest
	base.State.CredentialMask = mask
	base.Payload.Protocol = "anthropic_beta"
	base.Payload.Request = request
	if base.Runtime.Context != nil {
		base.Runtime.Context.Set(guardrailscore.CredentialMaskStateContextKey, mask)
	}

	evaluationErr := guardrailspipeline.ProcessAnthropicBetaRequest(ctx, e.stage.runtime, base)
	if evaluationErr != nil {
		e.report(PhaseRequest, Decision{}, evaluationErr)
	} else {
		e.report(PhaseRequest, Decision{Verdict: VerdictAllow}, nil)
	}
	prepared := call
	prepared.Request = request
	return &anthropicBetaSession{request: request, base: base, mask: mask}, prepared, nil
}

func (s *anthropicBetaSession) responseInput() guardrailscore.Input {
	input := s.base
	input.Direction = guardrailscore.DirectionResponse
	input.State.CredentialMask = s.mask
	input.Content = guardrailscore.Content{
		Messages: guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(s.request.System, s.request.Messages),
	}
	input.Payload.Request = s.request
	return input
}

func (e *anthropicBetaEndpoint) report(phase Phase, decision Decision, err error) {
	if e.stage.observe == nil {
		return
	}
	e.stage.observe(Observation{
		Stage:    e.stage.name,
		Protocol: protocol.TypeAnthropicBeta,
		Phase:    phase,
		Decision: decision,
		Err:      err,
	})
}

func decisionFromResult(result guardrailscore.Result) Decision {
	decision := Decision{Verdict: VerdictAllow}
	if result.Verdict == guardrailscore.VerdictBlock {
		decision.Verdict = VerdictBlock
	}
	if len(result.Reasons) > 0 {
		decision.Reason = result.Reasons[0].Reason
	}
	return decision
}

func newGuardrailsStreamState() *protocol.GuardrailsStreamState {
	return &protocol.GuardrailsStreamState{
		PendingBlockMessages: make(map[string]string),
		PendingBlockedIndex:  make(map[int]string),
		AnthropicToolEvents:  make(map[int][]protocol.GuardrailsBufferedEvent),
		AnthropicToolIDs:     make(map[int]string),
	}
}

type anthropicBetaGuardrailStream struct {
	parent      *anthropicBetaEndpoint
	stream      protocolstage.EventStream
	mask        *guardrailscore.CredentialMaskState
	streamState *protocol.GuardrailsStreamState
	onEvent     func(event interface{}) error
	onError     func(error)
	pending     []protocolstage.Event

	closeOnce sync.Once
	closeErr  error
}

func (s *anthropicBetaGuardrailStream) Next(ctx context.Context) (protocolstage.Event, error) {
	for {
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			return event, nil
		}

		event, err := s.stream.Next(ctx)
		if err != nil {
			if !errors.Is(err, io.EOF) && s.onError != nil {
				s.onError(err)
			}
			return event, err
		}
		betaEvent, normalizeErr := normalizeAnthropicBetaEvent(event.Value)
		if normalizeErr != nil {
			return protocolstage.Event{}, normalizeErr
		}
		if s.onEvent != nil {
			if hookErr := s.onEvent(betaEvent); hookErr != nil {
				if s.onError != nil {
					s.onError(hookErr)
				}
				return protocolstage.Event{}, hookErr
			}
		}

		handled, rewritten, rewriteErr := guardrailsmutate.RewriteAnthropicToolUseEvent(s.mask, s.streamState, betaEvent)
		if rewriteErr != nil {
			if s.onError != nil {
				s.onError(rewriteErr)
			}
			return protocolstage.Event{}, rewriteErr
		}
		if !handled {
			s.parent.report(PhaseEvent, Decision{Verdict: VerdictAllow}, nil)
			return event, nil
		}
		if len(rewritten) == 0 {
			continue
		}
		for _, value := range rewritten {
			s.pending = append(s.pending, protocolstage.Event{Value: protocolstream.AnthropicEvent{
				Type: value.EventType,
				Data: value.Payload,
			}})
		}
		s.parent.report(PhaseEvent, Decision{Verdict: VerdictBlock, Reason: "stream tool use rewritten"}, nil)
	}
}

func (s *anthropicBetaGuardrailStream) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.stream.Close()
	})
	return s.closeErr
}

func (s *anthropicBetaGuardrailStream) Result() protocolstage.StreamResult { return s.stream.Result() }

func normalizeAnthropicBetaEvent(value any) (*anthropic.BetaRawMessageStreamEventUnion, error) {
	switch event := value.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		copy := event
		return &copy, nil
	case *anthropic.BetaRawMessageStreamEventUnion:
		if event == nil {
			return nil, fmt.Errorf("Anthropic Beta Guardrail Stage received a nil stream event")
		}
		return event, nil
	case protocolstream.AnthropicEvent:
		payload, err := json.Marshal(event.Data)
		if err != nil {
			return nil, fmt.Errorf("marshal converted Anthropic Beta Guardrail event: %w", err)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(payload, &object); err != nil {
			return nil, fmt.Errorf("decode converted Anthropic Beta Guardrail event: %w", err)
		}
		eventType, err := json.Marshal(event.Type)
		if err != nil {
			return nil, fmt.Errorf("marshal converted Anthropic Beta Guardrail event type: %w", err)
		}
		object["type"] = eventType
		payload, err = json.Marshal(object)
		if err != nil {
			return nil, fmt.Errorf("encode converted Anthropic Beta Guardrail event: %w", err)
		}
		var decoded anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return nil, fmt.Errorf("unmarshal converted Anthropic Beta Guardrail event: %w", err)
		}
		return &decoded, nil
	default:
		return nil, fmt.Errorf("Anthropic Beta Guardrail Stage received stream event %T", value)
	}
}

var _ protocolstage.Stage = (*anthropicBetaStage)(nil)
var _ protocolstage.EventStream = (*anthropicBetaGuardrailStream)(nil)
