package record

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

const jsonContentType = "application/json"

var ErrFinished = errors.New("request recorder is already finished")

// Config creates one request-scoped Recorder. Enabled must be explicitly true;
// the zero value keeps recording disabled.
type Config struct {
	Enabled       bool
	RequestID     string
	SessionID     string
	Scenario      string
	InputProtocol protocol.APIType
	Input         any
}

// Recorder incrementally builds one RequestRecord.
type Recorder struct {
	mu        sync.Mutex
	startedAt time.Time
	record    RequestRecord
	finished  *RequestRecord
}

// Exchange is the one-shot completion handle returned by BeginExchange.
type Exchange struct {
	recorder *Recorder
	index    int
	done     bool
}

// New creates a Recorder only when cfg.Enabled is true. The disabled path
// returns before validation or JSON serialization.
func New(cfg Config) (*Recorder, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	input, err := capturePayload(cfg.InputProtocol, cfg.Input)
	if err != nil {
		return nil, fmt.Errorf("capture input request: %w", err)
	}

	now := time.Now().UTC()
	return &Recorder{
		startedAt: now,
		record: RequestRecord{
			Timestamp:    now,
			RequestID:    cfg.RequestID,
			SessionID:    cfg.SessionID,
			Scenario:     cfg.Scenario,
			InputRequest: input,
			Outcome:      OutcomePending,
		},
	}, nil
}

// Enabled reports whether a non-nil recorder is active.
func (r *Recorder) Enabled() bool {
	return r != nil
}

// BeginExchange captures the provider-bound request and appends an ordered
// provider exchange. It is a no-op for a nil (disabled) Recorder.
func (r *Recorder) BeginExchange(meta ExchangeMetadata, request any) (*Exchange, error) {
	if r == nil {
		return nil, nil
	}

	payload, err := capturePayload(meta.Protocol, request)
	if err != nil {
		return nil, fmt.Errorf("capture provider request: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished != nil {
		return nil, ErrFinished
	}

	now := time.Now().UTC()
	r.record.ProviderExchanges = append(r.record.ProviderExchanges, ProviderExchange{
		Sequence:  len(r.record.ProviderExchanges) + 1,
		Attempt:   meta.Attempt,
		Provider:  meta.Provider,
		Model:     meta.Model,
		Protocol:  meta.Protocol,
		Request:   payload,
		Outcome:   OutcomePending,
		StartedAt: now,
	})
	return &Exchange{recorder: r, index: len(r.record.ProviderExchanges) - 1}, nil
}

// Finish completes one provider exchange. Repeated calls are idempotent.
func (e *Exchange) Finish(response any, callErr error) error {
	if e == nil || e.recorder == nil {
		return nil
	}

	var responsePayload *Payload
	if response != nil {
		payload, err := capturePayload(e.protocol(), response)
		if err != nil {
			return fmt.Errorf("capture provider response: %w", err)
		}
		responsePayload = &payload
	}

	r := e.recorder
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.done {
		return nil
	}
	if r.finished != nil {
		return ErrFinished
	}
	if e.index < 0 || e.index >= len(r.record.ProviderExchanges) {
		return fmt.Errorf("provider exchange index %d is invalid", e.index)
	}

	exchange := &r.record.ProviderExchanges[e.index]
	exchange.Response = responsePayload
	exchange.Outcome = outcomeForError(callErr)
	if callErr != nil {
		exchange.Error = callErr.Error()
	}
	exchange.Duration = time.Since(exchange.StartedAt)
	e.done = true
	return nil
}

// protocol returns the exchange protocol without exposing the mutable record.
func (e *Exchange) protocol() protocol.APIType {
	r := e.recorder
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.index < 0 || e.index >= len(r.record.ProviderExchanges) {
		return ""
	}
	return r.record.ProviderExchanges[e.index].Protocol
}

// SetFinalResponse captures the client-visible response after every outward
// transformation. It is a no-op for a nil (disabled) Recorder.
func (r *Recorder) SetFinalResponse(api protocol.APIType, response any) error {
	if r == nil {
		return nil
	}
	payload, err := capturePayload(api, response)
	if err != nil {
		return fmt.Errorf("capture final response: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished != nil {
		return ErrFinished
	}
	r.record.FinalResponse = &payload
	return nil
}

// Finish completes the request exactly once. The returned boolean is true only
// for the caller that performed the transition; later callers receive an
// immutable copy and false.
func (r *Recorder) Finish(requestErr error) (*RequestRecord, bool) {
	if r == nil {
		return nil, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished != nil {
		copy := cloneRequestRecord(*r.finished)
		return &copy, false
	}

	r.record.Outcome = outcomeForError(requestErr)
	if requestErr != nil {
		r.record.Error = requestErr.Error()
	}
	r.record.Duration = time.Since(r.startedAt)
	completed := cloneRequestRecord(r.record)
	r.finished = &completed
	copy := cloneRequestRecord(completed)
	return &copy, true
}

func capturePayload(api protocol.APIType, value any) (Payload, error) {
	if api == "" {
		return Payload{}, errors.New("protocol is empty")
	}
	body, err := json.Marshal(value)
	if err != nil {
		return Payload{}, err
	}
	return Payload{
		Protocol:    api,
		ContentType: jsonContentType,
		Body:        append(json.RawMessage(nil), body...),
	}, nil
}

func outcomeForError(err error) Outcome {
	if err == nil {
		return OutcomeSucceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return OutcomeCancelled
	}
	return OutcomeFailed
}
