package stage

import (
	"context"
	"reflect"
	"strings"
	"testing"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

func TestBuildTopologyMixedProtocols(t *testing.T) {
	t.Parallel()

	var calls []string
	usage := protocol.NewTokenUsage(23, 12)
	terminal := &recordingEndpoint{
		protocol: protocol.TypeAnthropicV1,
		calls:    &calls,
		response: &Response{
			Value:                "terminal response",
			Usage:                usage,
			Model:                "provider-model",
			SideEffectsCommitted: true,
		},
	}
	ingress := &testingBridge{
		name:   "bridge_chat_beta",
		source: protocol.TypeOpenAIChat,
		target: protocol.TypeAnthropicBeta,
		caps:   AllBridgeCapabilities,
		calls:  &calls,
	}
	between := &testingBridge{
		name:   "bridge_beta_responses",
		source: protocol.TypeAnthropicBeta,
		target: protocol.TypeOpenAIResponses,
		caps:   AllBridgeCapabilities,
		calls:  &calls,
	}
	provider := &testingBridge{
		name:   "bridge_responses_v1",
		source: protocol.TypeOpenAIResponses,
		target: protocol.TypeAnthropicV1,
		caps:   AllBridgeCapabilities,
		calls:  &calls,
	}
	registry, err := NewBridgeRegistry(ingress, between, provider)
	if err != nil {
		t.Fatalf("NewBridgeRegistry() error = %v", err)
	}

	topology, err := BuildTopology(TopologyConfig{
		Terminal: terminal,
		Stages: []Stage{
			&recordingStage{name: "guardrails", protocol: protocol.TypeAnthropicBeta, calls: &calls},
			&recordingStage{name: "tool_loop", protocol: protocol.TypeOpenAIResponses, calls: &calls},
		},
		ClientProtocol:       protocol.TypeOpenAIChat,
		Registry:             registry,
		RequiredCapabilities: CapabilityUsage | CapabilityToolUse,
	})
	if err != nil {
		t.Fatalf("BuildTopology() error = %v", err)
	}
	if topology.Protocol() != protocol.TypeOpenAIChat {
		t.Fatalf("topology.Protocol() = %q", topology.Protocol())
	}
	if len(calls) != 0 {
		t.Fatalf("BuildTopology() executed chain, calls = %v", calls)
	}

	call := Call{
		Request: "client request",
		Metadata: CallMetadata{
			RequestID: "req-topology",
			Attempt:   4,
		},
	}
	response, err := topology.Complete(context.Background(), call)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	wantValue := "bridge_chat_beta(bridge_beta_responses(bridge_responses_v1(terminal response)))"
	if response.Value != wantValue {
		t.Fatalf("response.Value = %v, want %v", response.Value, wantValue)
	}
	assertResponseFacts(t, response, usage, "provider-model", true)
	if terminal.lastCall.Metadata != call.Metadata {
		t.Fatalf("terminal metadata = %+v, want %+v", terminal.lastCall.Metadata, call.Metadata)
	}

	wantCalls := []string{
		"bridge_chat_beta:request",
		"guardrails:request",
		"bridge_beta_responses:request",
		"tool_loop:request",
		"bridge_responses_v1:request",
		"terminal:request",
		"terminal:response",
		"bridge_responses_v1:response",
		"tool_loop:response",
		"bridge_beta_responses:response",
		"guardrails:response",
		"bridge_chat_beta:response",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestBridgeRegistryResolution(t *testing.T) {
	t.Parallel()

	bridge := &testingBridge{
		name:   "chat_beta",
		source: protocol.TypeOpenAIChat,
		target: protocol.TypeAnthropicBeta,
		caps:   CoreBridgeCapabilities | CapabilityUsage,
	}
	registry, err := NewBridgeRegistry(bridge)
	if err != nil {
		t.Fatalf("NewBridgeRegistry() error = %v", err)
	}

	resolved, err := registry.Resolve(protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, CapabilityUsage)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved != bridge {
		t.Fatal("Resolve() did not return the registered bridge")
	}

	identity, err := registry.Resolve(protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, AllBridgeCapabilities)
	if err != nil {
		t.Fatalf("identity Resolve() error = %v", err)
	}
	if identity.Source() != protocol.TypeAnthropicBeta || identity.Target() != protocol.TypeAnthropicBeta {
		t.Fatalf("identity bridge = %q -> %q", identity.Source(), identity.Target())
	}

	_, err = registry.Resolve(protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, CapabilityToolUse)
	if err == nil || !strings.Contains(err.Error(), "missing capabilities: tool_use") {
		t.Fatalf("capability Resolve() error = %v", err)
	}
	_, err = registry.Resolve(protocol.TypeOpenAIResponses, protocol.TypeAnthropicBeta, 0)
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("missing Resolve() error = %v", err)
	}
}

func TestNewBridgeRegistryRejectsInvalidEntries(t *testing.T) {
	t.Parallel()

	valid := &testingBridge{
		source: protocol.TypeOpenAIChat,
		target: protocol.TypeAnthropicBeta,
		caps:   AllBridgeCapabilities,
	}
	tests := []struct {
		name    string
		bridges []Bridge
		want    string
	}{
		{name: "nil", bridges: []Bridge{nil}, want: "index 0 is nil"},
		{
			name:    "empty protocol",
			bridges: []Bridge{&testingBridge{caps: AllBridgeCapabilities}},
			want:    "has empty protocol",
		},
		{
			name: "missing core",
			bridges: []Bridge{&testingBridge{
				source: protocol.TypeOpenAIChat,
				target: protocol.TypeAnthropicBeta,
				caps:   CapabilityComplete,
			}},
			want: "missing core capabilities: stream,error",
		},
		{name: "duplicate", bridges: []Bridge{valid, valid}, want: "duplicate bridge"},
		{name: "valid", bridges: []Bridge{valid}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			registry, err := NewBridgeRegistry(tt.bridges...)
			if tt.want == "" {
				if err != nil || registry == nil {
					t.Fatalf("NewBridgeRegistry() = (%v, %v)", registry, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewBridgeRegistry() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBuildTopologyRejectsMissingBridgeBeforeExecution(t *testing.T) {
	t.Parallel()

	var calls []string
	registry, err := NewBridgeRegistry()
	if err != nil {
		t.Fatalf("NewBridgeRegistry() error = %v", err)
	}
	terminal := &recordingEndpoint{
		protocol: protocol.TypeAnthropicV1,
		calls:    &calls,
	}

	_, err = BuildTopology(TopologyConfig{
		Terminal: terminal,
		Stages: []Stage{
			&recordingStage{name: "guardrails", protocol: protocol.TypeAnthropicBeta, calls: &calls},
		},
		ClientProtocol: protocol.TypeOpenAIChat,
		Registry:       registry,
	})
	if err == nil || !strings.Contains(err.Error(), `"anthropic_beta" -> "anthropic_v1": not registered`) {
		t.Fatalf("BuildTopology() error = %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("BuildTopology() executed chain, calls = %v", calls)
	}
}
