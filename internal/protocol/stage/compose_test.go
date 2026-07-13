package stage

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

func TestComposeCompleteOrder(t *testing.T) {
	t.Parallel()

	var calls []string
	terminal := &recordingEndpoint{
		protocol: protocol.TypeAnthropicBeta,
		calls:    &calls,
		response: &Response{
			Value: "terminal response",
			Usage: protocol.NewTokenUsage(7, 3),
			Model: "provider-model",
		},
	}

	composed, err := Compose(
		terminal,
		&recordingStage{name: "guardrails", protocol: protocol.TypeAnthropicBeta, calls: &calls},
		&recordingStage{name: "tool_loop", protocol: protocol.TypeAnthropicBeta, calls: &calls},
	)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("Compose() executed endpoint, calls = %v", calls)
	}

	call := Call{
		Request: "native request",
		Metadata: CallMetadata{
			RequestID: "req-1",
			Attempt:   2,
		},
	}
	response, err := composed.Complete(context.Background(), call)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	wantCalls := []string{
		"guardrails:request",
		"tool_loop:request",
		"terminal:request",
		"terminal:response",
		"tool_loop:response",
		"guardrails:response",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
	if response != terminal.response {
		t.Fatal("Complete() did not preserve terminal response")
	}
	if terminal.lastCall.Metadata != call.Metadata {
		t.Fatalf("metadata = %+v, want %+v", terminal.lastCall.Metadata, call.Metadata)
	}
}

func TestComposeStreamOrderAndClose(t *testing.T) {
	t.Parallel()

	var calls []string
	terminalStream := &recordingEventStream{
		calls: &calls,
		events: []Event{
			{Value: "event-1"},
		},
		result: StreamResult{
			Usage:                protocol.NewTokenUsage(11, 5),
			Model:                "provider-model",
			SideEffectsCommitted: true,
		},
	}
	terminal := &recordingEndpoint{
		protocol: protocol.TypeAnthropicBeta,
		calls:    &calls,
		stream:   terminalStream,
	}

	composed, err := Compose(
		terminal,
		&recordingStage{name: "guardrails", protocol: protocol.TypeAnthropicBeta, calls: &calls},
		&recordingStage{name: "tool_loop", protocol: protocol.TypeAnthropicBeta, calls: &calls},
	)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}

	stream, err := composed.Stream(context.Background(), Call{Request: "native request"})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	event, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if event.Value != "event-1" {
		t.Fatalf("event.Value = %v, want event-1", event.Value)
	}
	if got := stream.Result(); !reflect.DeepEqual(got, terminalStream.result) {
		t.Fatalf("Result() = %+v, want %+v", got, terminalStream.result)
	}

	_, err = stream.Next(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("second Next() error = %v, want io.EOF", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if terminalStream.closeCount != 1 {
		t.Fatalf("terminal close count = %d, want 1", terminalStream.closeCount)
	}

	wantCalls := []string{
		"guardrails:stream_request",
		"tool_loop:stream_request",
		"terminal:stream_request",
		"terminal:event",
		"tool_loop:event",
		"guardrails:event",
		"terminal:eof",
		"tool_loop:eof",
		"guardrails:eof",
		"guardrails:close",
		"tool_loop:close",
		"terminal:close",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestComposeRejectsInvalidChain(t *testing.T) {
	t.Parallel()

	validEndpoint := &recordingEndpoint{protocol: protocol.TypeAnthropicBeta}
	validStage := func() *recordingStage {
		return &recordingStage{name: "guardrails", protocol: protocol.TypeAnthropicBeta}
	}

	var typedNilEndpoint *recordingEndpoint
	var typedNilStage *recordingStage

	tests := []struct {
		name     string
		terminal Endpoint
		stages   []Stage
		want     string
	}{
		{
			name: "nil terminal",
			want: "terminal endpoint is nil",
		},
		{
			name:     "typed nil terminal",
			terminal: typedNilEndpoint,
			want:     "terminal endpoint is nil",
		},
		{
			name:     "empty terminal protocol",
			terminal: &recordingEndpoint{},
			want:     "terminal endpoint has empty protocol",
		},
		{
			name:     "nil stage",
			terminal: validEndpoint,
			stages:   []Stage{nil},
			want:     "stage at index 0 is nil",
		},
		{
			name:     "typed nil stage",
			terminal: validEndpoint,
			stages:   []Stage{typedNilStage},
			want:     "stage at index 0 is nil",
		},
		{
			name:     "empty stage name",
			terminal: validEndpoint,
			stages:   []Stage{&recordingStage{protocol: protocol.TypeAnthropicBeta}},
			want:     "stage at index 0 has empty name",
		},
		{
			name:     "empty stage protocol",
			terminal: validEndpoint,
			stages:   []Stage{&recordingStage{name: "guardrails"}},
			want:     `stage "guardrails" has empty protocol`,
		},
		{
			name:     "protocol mismatch",
			terminal: validEndpoint,
			stages: []Stage{&recordingStage{
				name:     "guardrails",
				protocol: protocol.TypeOpenAIResponses,
			}},
			want: `stage "guardrails" speaks "openai_responses" and cannot wrap endpoint speaking "anthropic_beta"`,
		},
		{
			name:     "nil wrapped endpoint",
			terminal: validEndpoint,
			stages: []Stage{&recordingStage{
				name:      "guardrails",
				protocol:  protocol.TypeAnthropicBeta,
				returnNil: true,
			}},
			want: `stage "guardrails" returned a nil endpoint`,
		},
		{
			name:     "wrapped endpoint changed protocol",
			terminal: validEndpoint,
			stages: []Stage{&recordingStage{
				name:            "guardrails",
				protocol:        protocol.TypeAnthropicBeta,
				wrappedProtocol: protocol.TypeOpenAIChat,
			}},
			want: `stage "guardrails" returned endpoint speaking "openai_chat", want "anthropic_beta"`,
		},
		{
			name:     "valid baseline",
			terminal: validEndpoint,
			stages:   []Stage{validStage()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Compose(tt.terminal, tt.stages...)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Compose() error = %v", err)
				}
				if got == nil {
					t.Fatal("Compose() returned nil endpoint")
				}
				return
			}

			if err == nil {
				t.Fatalf("Compose() error = nil, want containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Compose() error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

type recordingEndpoint struct {
	protocol protocol.APIType
	calls    *[]string
	response *Response
	stream   EventStream
	lastCall Call
}

func (e *recordingEndpoint) Protocol() protocol.APIType {
	return e.protocol
}

func (e *recordingEndpoint) Complete(_ context.Context, call Call) (*Response, error) {
	e.lastCall = call
	e.append("terminal:request")
	e.append("terminal:response")
	return e.response, nil
}

func (e *recordingEndpoint) Stream(_ context.Context, call Call) (EventStream, error) {
	e.lastCall = call
	e.append("terminal:stream_request")
	return e.stream, nil
}

func (e *recordingEndpoint) append(value string) {
	if e.calls != nil {
		*e.calls = append(*e.calls, value)
	}
}

type recordingStage struct {
	name            string
	protocol        protocol.APIType
	calls           *[]string
	returnNil       bool
	wrappedProtocol protocol.APIType
}

func (s *recordingStage) Name() string {
	return s.name
}

func (s *recordingStage) Protocol() protocol.APIType {
	return s.protocol
}

func (s *recordingStage) Wrap(next Endpoint) Endpoint {
	if s.returnNil {
		return nil
	}
	wrappedProtocol := s.protocol
	if s.wrappedProtocol != "" {
		wrappedProtocol = s.wrappedProtocol
	}
	return &recordingStageEndpoint{
		name:     s.name,
		protocol: wrappedProtocol,
		calls:    s.calls,
		next:     next,
	}
}

type recordingStageEndpoint struct {
	name     string
	protocol protocol.APIType
	calls    *[]string
	next     Endpoint
}

func (e *recordingStageEndpoint) Protocol() protocol.APIType {
	return e.protocol
}

func (e *recordingStageEndpoint) Complete(ctx context.Context, call Call) (*Response, error) {
	e.append(e.name + ":request")
	response, err := e.next.Complete(ctx, call)
	if err != nil {
		return nil, err
	}
	e.append(e.name + ":response")
	return response, nil
}

func (e *recordingStageEndpoint) Stream(ctx context.Context, call Call) (EventStream, error) {
	e.append(e.name + ":stream_request")
	stream, err := e.next.Stream(ctx, call)
	if err != nil {
		return nil, err
	}
	return &recordingStageStream{name: e.name, calls: e.calls, next: stream}, nil
}

func (e *recordingStageEndpoint) append(value string) {
	if e.calls != nil {
		*e.calls = append(*e.calls, value)
	}
}

type recordingStageStream struct {
	name  string
	calls *[]string
	next  EventStream
}

func (s *recordingStageStream) Next(ctx context.Context) (Event, error) {
	event, err := s.next.Next(ctx)
	switch {
	case err == nil:
		s.append(s.name + ":event")
	case errors.Is(err, io.EOF):
		s.append(s.name + ":eof")
	}
	return event, err
}

func (s *recordingStageStream) Close() error {
	s.append(s.name + ":close")
	return s.next.Close()
}

func (s *recordingStageStream) Result() StreamResult {
	return s.next.Result()
}

func (s *recordingStageStream) append(value string) {
	if s.calls != nil {
		*s.calls = append(*s.calls, value)
	}
}

type recordingEventStream struct {
	calls      *[]string
	events     []Event
	result     StreamResult
	next       int
	closeCount int
}

func (s *recordingEventStream) Next(ctx context.Context) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}
	if s.next >= len(s.events) {
		s.append("terminal:eof")
		return Event{}, io.EOF
	}

	event := s.events[s.next]
	s.next++
	s.append("terminal:event")
	return event, nil
}

func (s *recordingEventStream) Close() error {
	s.closeCount++
	s.append("terminal:close")
	return nil
}

func (s *recordingEventStream) Result() StreamResult {
	return s.result
}

func (s *recordingEventStream) append(value string) {
	if s.calls != nil {
		*s.calls = append(*s.calls, value)
	}
}
