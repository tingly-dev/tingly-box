package stage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

func TestBuildTopologyCompleteAndStreamFlow(t *testing.T) {
	t.Parallel()

	var calls []string
	usage := protocol.NewTokenUsage(17, 9)
	terminalStream := &recordingEventStream{
		calls:  &calls,
		events: []Event{{Value: "terminal event"}},
		result: StreamResult{
			Usage:                usage,
			Model:                "provider-model",
			SideEffectsCommitted: true,
		},
	}
	terminal := &recordingEndpoint{
		protocol: protocol.TypeOpenAIResponses,
		calls:    &calls,
		response: &Response{
			Value:                "terminal response",
			Usage:                usage,
			Model:                "provider-model",
			SideEffectsCommitted: true,
		},
		stream: terminalStream,
	}
	providerBridge := &testingBridge{
		name:      "provider_bridge",
		source:    protocol.TypeAnthropicBeta,
		target:    protocol.TypeOpenAIResponses,
		caps:      AllBridgeCapabilities,
		calls:     &calls,
		dropFacts: true,
	}
	ingressBridge := &testingBridge{
		name:      "ingress_bridge",
		source:    protocol.TypeOpenAIChat,
		target:    protocol.TypeAnthropicBeta,
		caps:      AllBridgeCapabilities,
		calls:     &calls,
		dropFacts: true,
	}

	registry, err := NewBridgeRegistry(providerBridge, ingressBridge)
	if err != nil {
		t.Fatalf("NewBridgeRegistry() error = %v", err)
	}
	chain, err := BuildTopology(TopologyConfig{
		Terminal: terminal,
		Stages: []Stage{
			&recordingStage{name: "guardrails", protocol: protocol.TypeAnthropicBeta, calls: &calls},
			&recordingStage{name: "tool_loop", protocol: protocol.TypeAnthropicBeta, calls: &calls},
		},
		ClientProtocol: protocol.TypeOpenAIChat,
		Registry:       registry,
		RequiredCapabilities: CapabilityUsage |
			CapabilityToolUse |
			CapabilityToolResult,
	})
	if err != nil {
		t.Fatalf("BuildTopology() error = %v", err)
	}
	if chain.Protocol() != protocol.TypeOpenAIChat {
		t.Fatalf("chain.Protocol() = %q, want %q", chain.Protocol(), protocol.TypeOpenAIChat)
	}
	if len(calls) != 0 {
		t.Fatalf("BuildTopology() executed chain, calls = %v", calls)
	}

	call := Call{
		Request: "client request",
		Metadata: CallMetadata{
			RequestID: "req-chain",
			Attempt:   3,
		},
	}
	response, err := chain.Complete(context.Background(), call)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Value != "ingress_bridge(provider_bridge(terminal response))" {
		t.Fatalf("response.Value = %v", response.Value)
	}
	assertResponseFacts(t, response, usage, "provider-model", true)
	if terminal.lastCall.Metadata != call.Metadata {
		t.Fatalf("terminal metadata = %+v, want %+v", terminal.lastCall.Metadata, call.Metadata)
	}

	wantCompleteCalls := []string{
		"ingress_bridge:request",
		"guardrails:request",
		"tool_loop:request",
		"provider_bridge:request",
		"terminal:request",
		"terminal:response",
		"provider_bridge:response",
		"tool_loop:response",
		"guardrails:response",
		"ingress_bridge:response",
	}
	if !reflect.DeepEqual(calls, wantCompleteCalls) {
		t.Fatalf("complete calls = %v, want %v", calls, wantCompleteCalls)
	}

	calls = nil
	stream, err := chain.Stream(context.Background(), call)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	event, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if event.Value != "ingress_bridge(provider_bridge(terminal event))" {
		t.Fatalf("event.Value = %v", event.Value)
	}
	_, err = stream.Next(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("second Next() error = %v, want io.EOF", err)
	}
	result := stream.Result()
	if result.Usage != usage || result.Model != "provider-model" || !result.SideEffectsCommitted {
		t.Fatalf("Result() = %+v, want preserved terminal facts", result)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if terminalStream.closeCount != 1 {
		t.Fatalf("terminal close count = %d, want 1", terminalStream.closeCount)
	}

	wantStreamCalls := []string{
		"ingress_bridge:request",
		"guardrails:stream_request",
		"tool_loop:stream_request",
		"provider_bridge:request",
		"terminal:stream_request",
		"terminal:event",
		"provider_bridge:event",
		"tool_loop:event",
		"guardrails:event",
		"ingress_bridge:event",
		"terminal:eof",
		"provider_bridge:eof",
		"tool_loop:eof",
		"guardrails:eof",
		"ingress_bridge:eof",
		"ingress_bridge:close",
		"guardrails:close",
		"tool_loop:close",
		"provider_bridge:close",
		"terminal:close",
	}
	if !reflect.DeepEqual(calls, wantStreamCalls) {
		t.Fatalf("stream calls = %v, want %v", calls, wantStreamCalls)
	}

	if providerBridge.openCount != 2 || ingressBridge.openCount != 2 {
		t.Fatalf("bridge opens: provider=%d ingress=%d, want 2 each", providerBridge.openCount, ingressBridge.openCount)
	}
	wantOperations := []Operation{OperationComplete, OperationStream}
	if !reflect.DeepEqual(providerBridge.operations, wantOperations) || !reflect.DeepEqual(ingressBridge.operations, wantOperations) {
		t.Fatalf("bridge operations: provider=%v ingress=%v, want %v", providerBridge.operations, ingressBridge.operations, wantOperations)
	}
	if providerBridge.sessions[0] == providerBridge.sessions[1] || ingressBridge.sessions[0] == ingressBridge.sessions[1] {
		t.Fatal("Bridge.Open() reused a session across calls")
	}
}

func TestAdaptRejectsInvalidBoundary(t *testing.T) {
	t.Parallel()

	validEndpoint := &recordingEndpoint{protocol: protocol.TypeAnthropicBeta}
	validBridge := func() *testingBridge {
		return &testingBridge{
			name:   "bridge",
			source: protocol.TypeOpenAIChat,
			target: protocol.TypeAnthropicBeta,
			caps:   AllBridgeCapabilities,
		}
	}

	var typedNilEndpoint *recordingEndpoint
	var typedNilBridge *testingBridge
	tests := []struct {
		name   string
		next   Endpoint
		bridge Bridge
		want   string
	}{
		{name: "nil endpoint", bridge: validBridge(), want: "target endpoint is nil"},
		{name: "typed nil endpoint", next: typedNilEndpoint, bridge: validBridge(), want: "target endpoint is nil"},
		{name: "nil bridge", next: validEndpoint, want: "bridge is nil"},
		{name: "typed nil bridge", next: validEndpoint, bridge: typedNilBridge, want: "bridge is nil"},
		{
			name:   "empty source",
			next:   validEndpoint,
			bridge: &testingBridge{target: protocol.TypeAnthropicBeta, caps: AllBridgeCapabilities},
			want:   "empty source protocol",
		},
		{
			name:   "empty target",
			next:   validEndpoint,
			bridge: &testingBridge{source: protocol.TypeOpenAIChat, caps: AllBridgeCapabilities},
			want:   "empty target protocol",
		},
		{
			name:   "empty endpoint protocol",
			next:   &recordingEndpoint{},
			bridge: validBridge(),
			want:   "target endpoint has empty protocol",
		},
		{
			name: "target mismatch",
			next: &recordingEndpoint{protocol: protocol.TypeOpenAIResponses},
			bridge: &testingBridge{
				source: protocol.TypeOpenAIChat,
				target: protocol.TypeAnthropicBeta,
				caps:   AllBridgeCapabilities,
			},
			want: `cannot call endpoint speaking "openai_responses"`,
		},
		{
			name: "missing core capability",
			next: validEndpoint,
			bridge: &testingBridge{
				source: protocol.TypeOpenAIChat,
				target: protocol.TypeAnthropicBeta,
				caps:   CapabilityComplete | CapabilityError,
			},
			want: "missing core capabilities: stream",
		},
		{name: "valid baseline", next: validEndpoint, bridge: validBridge()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Adapt(tt.next, tt.bridge)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Adapt() error = %v", err)
				}
				if got == nil {
					t.Fatal("Adapt() returned nil endpoint")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Adapt() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAdaptRuntimeFailures(t *testing.T) {
	t.Parallel()

	upstreamErr := errors.New("upstream failed")
	conversionErr := errors.New("stream conversion failed")

	t.Run("request conversion error", func(t *testing.T) {
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta}
		bridge := validTestingBridge()
		bridge.openErr = errors.New("request conversion failed")
		adapted := mustAdapt(t, endpoint, bridge)

		_, err := adapted.Complete(context.Background(), Call{})
		if !errors.Is(err, bridge.openErr) {
			t.Fatalf("Complete() error = %v", err)
		}
		if endpoint.completeCalls != 0 {
			t.Fatal("target endpoint executed after request conversion error")
		}
	})

	t.Run("nil session", func(t *testing.T) {
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta}
		bridge := validTestingBridge()
		bridge.nilSession = true
		adapted := mustAdapt(t, endpoint, bridge)

		_, err := adapted.Complete(context.Background(), Call{})
		if err == nil || !strings.Contains(err.Error(), "Open returned a nil session") {
			t.Fatalf("Complete() error = %v", err)
		}
	})

	t.Run("target error converted", func(t *testing.T) {
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta, completeErr: upstreamErr}
		bridge := validTestingBridge()
		adapted := mustAdapt(t, endpoint, bridge)

		_, err := adapted.Complete(context.Background(), Call{})
		if !errors.Is(err, upstreamErr) || !strings.Contains(err.Error(), "bridge: ") {
			t.Fatalf("Complete() error = %v", err)
		}
	})

	t.Run("target error cannot be swallowed", func(t *testing.T) {
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta, completeErr: upstreamErr}
		bridge := validTestingBridge()
		bridge.swallowError = true
		adapted := mustAdapt(t, endpoint, bridge)

		_, err := adapted.Complete(context.Background(), Call{})
		if !errors.Is(err, upstreamErr) || !strings.Contains(err.Error(), "swallowed target error") {
			t.Fatalf("Complete() error = %v", err)
		}
	})

	t.Run("nil target response", func(t *testing.T) {
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta}
		adapted := mustAdapt(t, endpoint, validTestingBridge())

		_, err := adapted.Complete(context.Background(), Call{})
		if err == nil || !strings.Contains(err.Error(), "target endpoint returned a nil response") {
			t.Fatalf("Complete() error = %v", err)
		}
	})

	t.Run("nil converted response", func(t *testing.T) {
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta, response: &Response{Value: "ok"}}
		bridge := validTestingBridge()
		bridge.nilResponse = true
		adapted := mustAdapt(t, endpoint, bridge)

		_, err := adapted.Complete(context.Background(), Call{})
		if err == nil || !strings.Contains(err.Error(), "nil converted response") {
			t.Fatalf("Complete() error = %v", err)
		}
	})

	t.Run("target stream open error converted", func(t *testing.T) {
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta, streamErr: upstreamErr}
		adapted := mustAdapt(t, endpoint, validTestingBridge())

		_, err := adapted.Stream(context.Background(), Call{})
		if !errors.Is(err, upstreamErr) || !strings.Contains(err.Error(), "bridge: ") {
			t.Fatalf("Stream() error = %v", err)
		}
	})

	t.Run("nil target stream", func(t *testing.T) {
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta}
		adapted := mustAdapt(t, endpoint, validTestingBridge())

		_, err := adapted.Stream(context.Background(), Call{})
		if err == nil || !strings.Contains(err.Error(), "target endpoint returned a nil stream") {
			t.Fatalf("Stream() error = %v", err)
		}
	})

	t.Run("stream conversion error closes target", func(t *testing.T) {
		targetStream := &recordingEventStream{}
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta, stream: targetStream}
		bridge := validTestingBridge()
		bridge.streamConversionErr = conversionErr
		adapted := mustAdapt(t, endpoint, bridge)

		_, err := adapted.Stream(context.Background(), Call{})
		if !errors.Is(err, conversionErr) {
			t.Fatalf("Stream() error = %v", err)
		}
		if targetStream.closeCount != 1 {
			t.Fatalf("target close count = %d, want 1", targetStream.closeCount)
		}
	})

	t.Run("nil converted stream closes target", func(t *testing.T) {
		targetStream := &recordingEventStream{}
		endpoint := &failureEndpoint{protocol: protocol.TypeAnthropicBeta, stream: targetStream}
		bridge := validTestingBridge()
		bridge.nilStream = true
		adapted := mustAdapt(t, endpoint, bridge)

		_, err := adapted.Stream(context.Background(), Call{})
		if err == nil || !strings.Contains(err.Error(), "nil converted stream") {
			t.Fatalf("Stream() error = %v", err)
		}
		if targetStream.closeCount != 1 {
			t.Fatalf("target close count = %d, want 1", targetStream.closeCount)
		}
	})
}

func TestIdentityBridgePreservesValues(t *testing.T) {
	t.Parallel()

	usage := protocol.NewTokenUsage(5, 2)
	targetStream := &recordingEventStream{
		events: []Event{{Value: "event"}},
		result: StreamResult{Usage: usage, Model: "model", SideEffectsCommitted: true},
	}
	terminal := &recordingEndpoint{
		protocol: protocol.TypeAnthropicBeta,
		response: &Response{Value: "response", Usage: usage, Model: "model", SideEffectsCommitted: true},
		stream:   targetStream,
	}
	adapted := mustAdapt(t, terminal, NewIdentityBridge(protocol.TypeAnthropicBeta))

	config := &protocol.OpenAIConfig{HasThinking: true}
	call := Call{Request: "request", State: ProtocolState{OpenAIChat: config}}
	response, err := adapted.Complete(context.Background(), call)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Value != "response" {
		t.Fatalf("response.Value = %v", response.Value)
	}
	assertResponseFacts(t, response, usage, "model", true)

	if terminal.lastCall.State.OpenAIChat != config {
		t.Fatal("identity complete call did not preserve protocol state")
	}

	stream, err := adapted.Stream(context.Background(), call)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	event, err := stream.Next(context.Background())
	if err != nil || event.Value != "event" {
		t.Fatalf("Next() = (%+v, %v)", event, err)
	}
	if got := stream.Result(); got.Usage != usage || got.Model != "model" || !got.SideEffectsCommitted {
		t.Fatalf("Result() = %+v", got)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if targetStream.closeCount != 1 {
		t.Fatalf("target close count = %d, want 1", targetStream.closeCount)
	}
	if terminal.lastCall.State.OpenAIChat != config {
		t.Fatal("identity stream call did not preserve protocol state")
	}
}

func TestOperationString(t *testing.T) {
	t.Parallel()

	if OperationComplete.String() != "complete" || OperationStream.String() != "stream" {
		t.Fatalf("operation strings = %q, %q", OperationComplete, OperationStream)
	}
	if got := Operation(99).String(); got != "unknown(99)" {
		t.Fatalf("unknown operation string = %q", got)
	}
}

func TestCapabilities(t *testing.T) {
	t.Parallel()

	got := CapabilityComplete | CapabilityUsage | CapabilityToolUse
	if !got.Supports(CapabilityComplete | CapabilityUsage) {
		t.Fatal("Capabilities.Supports() = false for contained set")
	}
	if got.Supports(CapabilityStream) {
		t.Fatal("Capabilities.Supports() = true for missing capability")
	}
	if missing := got.Missing(CapabilityComplete | CapabilityStream | CapabilityToolResult); missing != CapabilityStream|CapabilityToolResult {
		t.Fatalf("Missing() = %v", missing)
	}
	if got.String() != "complete,usage,tool_use" {
		t.Fatalf("String() = %q", got)
	}
	if Capabilities(0).String() != "none" {
		t.Fatalf("zero String() = %q", Capabilities(0))
	}
	unknown := Capabilities(1 << 20)
	if unknown.String() != "unknown(0x100000)" {
		t.Fatalf("unknown String() = %q", unknown)
	}
}

func TestBuildTopologyRejectsMissingSemanticCapability(t *testing.T) {
	t.Parallel()

	terminal := &recordingEndpoint{protocol: protocol.TypeOpenAIResponses}
	provider := &testingBridge{
		name:   "provider",
		source: protocol.TypeAnthropicBeta,
		target: protocol.TypeOpenAIResponses,
		caps:   CoreBridgeCapabilities | CapabilityUsage,
	}
	ingress := &testingBridge{
		name:   "ingress",
		source: protocol.TypeOpenAIChat,
		target: protocol.TypeAnthropicBeta,
		caps:   AllBridgeCapabilities,
	}

	registry, err := NewBridgeRegistry(provider, ingress)
	if err != nil {
		t.Fatalf("NewBridgeRegistry() error = %v", err)
	}
	_, err = BuildTopology(TopologyConfig{
		Terminal: terminal,
		Stages: []Stage{
			&recordingStage{name: "guardrails", protocol: protocol.TypeAnthropicBeta},
		},
		ClientProtocol:       protocol.TypeOpenAIChat,
		Registry:             registry,
		RequiredCapabilities: CapabilityToolUse,
	})
	if err == nil || !strings.Contains(err.Error(), "bridge below stage") || !strings.Contains(err.Error(), "tool_use") {
		t.Fatalf("BuildTopology() error = %v", err)
	}
}

func assertResponseFacts(t *testing.T, response *Response, usage *protocol.TokenUsage, model string, committed bool) {
	t.Helper()
	if response.Usage != usage || response.Model != model || response.SideEffectsCommitted != committed {
		t.Fatalf("response facts = %+v, want usage=%p model=%q committed=%v", response, usage, model, committed)
	}
}

func mustAdapt(t *testing.T, endpoint Endpoint, bridge Bridge) Endpoint {
	t.Helper()
	adapted, err := Adapt(endpoint, bridge)
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	return adapted
}

func validTestingBridge() *testingBridge {
	return &testingBridge{
		name:   "bridge",
		source: protocol.TypeOpenAIChat,
		target: protocol.TypeAnthropicBeta,
		caps:   AllBridgeCapabilities,
	}
}

type testingBridge struct {
	name                string
	source              protocol.APIType
	target              protocol.APIType
	caps                Capabilities
	calls               *[]string
	dropFacts           bool
	openErr             error
	nilSession          bool
	swallowError        bool
	nilResponse         bool
	streamConversionErr error
	nilStream           bool
	openCount           int
	sessions            []*testingBridgeSession
	operations          []Operation
}

func (b *testingBridge) Source() protocol.APIType {
	return b.source
}

func (b *testingBridge) Target() protocol.APIType {
	return b.target
}

func (b *testingBridge) Capabilities() Capabilities {
	return b.caps
}

func (b *testingBridge) Open(_ context.Context, call Call, operation Operation) (BridgeSession, error) {
	if b.openErr != nil {
		return nil, b.openErr
	}
	b.append(b.name + ":request")
	b.operations = append(b.operations, operation)
	b.openCount++
	if b.nilSession {
		return nil, nil
	}

	session := &testingBridgeSession{
		bridge: b,
		call: Call{
			Request: fmt.Sprintf("%s(%v)", b.name, call.Request),
			// Metadata is intentionally omitted. Adapt must restore it.
		},
	}
	b.sessions = append(b.sessions, session)
	return session, nil
}

func (b *testingBridge) append(value string) {
	if b.calls != nil {
		*b.calls = append(*b.calls, value)
	}
}

type testingBridgeSession struct {
	bridge *testingBridge
	call   Call
}

func (s *testingBridgeSession) TargetCall() Call {
	return s.call
}

func (s *testingBridgeSession) ConvertComplete(_ context.Context, response *Response) (*Response, error) {
	s.bridge.append(s.bridge.name + ":response")
	if s.bridge.nilResponse {
		return nil, nil
	}
	converted := &Response{Value: fmt.Sprintf("%s(%v)", s.bridge.name, response.Value)}
	if !s.bridge.dropFacts {
		converted.Usage = response.Usage
		converted.Model = response.Model
		converted.SideEffectsCommitted = response.SideEffectsCommitted
	}
	return converted, nil
}

func (s *testingBridgeSession) ConvertStream(_ context.Context, stream EventStream) (EventStream, error) {
	if s.bridge.streamConversionErr != nil {
		return nil, s.bridge.streamConversionErr
	}
	if s.bridge.nilStream {
		return nil, nil
	}
	return &testingBridgeStream{bridge: s.bridge, target: stream}, nil
}

func (s *testingBridgeSession) ConvertError(_ context.Context, err error) error {
	if s.bridge.swallowError {
		return nil
	}
	return fmt.Errorf("%s: %w", s.bridge.name, err)
}

type testingBridgeStream struct {
	bridge *testingBridge
	target EventStream
}

func (s *testingBridgeStream) Next(ctx context.Context) (Event, error) {
	event, err := s.target.Next(ctx)
	switch {
	case err == nil:
		s.bridge.append(s.bridge.name + ":event")
		event.Value = fmt.Sprintf("%s(%v)", s.bridge.name, event.Value)
	case errors.Is(err, io.EOF):
		s.bridge.append(s.bridge.name + ":eof")
	default:
		err = fmt.Errorf("%s: %w", s.bridge.name, err)
	}
	return event, err
}

func (s *testingBridgeStream) Close() error {
	s.bridge.append(s.bridge.name + ":close")
	return s.target.Close()
}

func (s *testingBridgeStream) Result() StreamResult {
	if s.bridge.dropFacts {
		return StreamResult{}
	}
	return s.target.Result()
}

type failureEndpoint struct {
	protocol      protocol.APIType
	response      *Response
	completeErr   error
	stream        EventStream
	streamErr     error
	completeCalls int
	streamCalls   int
}

func (e *failureEndpoint) Protocol() protocol.APIType {
	return e.protocol
}

func (e *failureEndpoint) Complete(_ context.Context, _ Call) (*Response, error) {
	e.completeCalls++
	return e.response, e.completeErr
}

func (e *failureEndpoint) Stream(_ context.Context, _ Call) (EventStream, error) {
	e.streamCalls++
	return e.stream, e.streamErr
}
