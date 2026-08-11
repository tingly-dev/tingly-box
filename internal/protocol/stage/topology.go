package stage

import (
	"fmt"
	"strings"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

// TopologyConfig describes an arbitrary protocol-stage path. Stages are listed
// in client request order, outermost to innermost. Adjacent stages may speak
// different protocols; BuildTopology resolves an explicit Bridge for each
// mismatch while constructing from Terminal outward.
type TopologyConfig struct {
	Terminal             Endpoint
	Stages               []Stage
	ClientProtocol       protocol.APIType
	Registry             *BridgeRegistry
	RequiredCapabilities Capabilities
}

// BuildTopology constructs a client-facing Endpoint without executing it. For
// a client A, outer stage B, inner stage C, and provider D, the result is:
//
//	bridge A->B (
//	  stage B (
//	    bridge B->C (
//	      stage C (
//	        bridge C->D (terminal D)))))
func BuildTopology(config TopologyConfig) (Endpoint, error) {
	if isNil(config.Terminal) {
		return nil, fmt.Errorf("build protocol stage topology: terminal endpoint is nil")
	}
	if config.Terminal.Protocol() == "" {
		return nil, fmt.Errorf("build protocol stage topology: terminal endpoint has empty protocol")
	}
	if config.ClientProtocol == "" {
		return nil, fmt.Errorf("build protocol stage topology: client protocol is empty")
	}
	if config.Registry == nil {
		return nil, fmt.Errorf("build protocol stage topology: bridge registry is nil")
	}

	required := config.RequiredCapabilities | CoreBridgeCapabilities
	current := config.Terminal
	for i := len(config.Stages) - 1; i >= 0; i-- {
		stage := config.Stages[i]
		if isNil(stage) {
			return nil, fmt.Errorf("build protocol stage topology: stage at index %d is nil", i)
		}
		name := strings.TrimSpace(stage.Name())
		if name == "" {
			return nil, fmt.Errorf("build protocol stage topology: stage at index %d has empty name", i)
		}
		stageProtocol := stage.Protocol()
		if stageProtocol == "" {
			return nil, fmt.Errorf("build protocol stage topology: stage %q has empty protocol", name)
		}

		if stageProtocol != current.Protocol() {
			bridge, err := config.Registry.Resolve(stageProtocol, current.Protocol(), required)
			if err != nil {
				return nil, fmt.Errorf("build protocol stage topology: bridge below stage %q: %w", name, err)
			}
			current, err = Adapt(current, bridge)
			if err != nil {
				return nil, fmt.Errorf("build protocol stage topology: adapt below stage %q: %w", name, err)
			}
		}

		var err error
		current, err = Compose(current, stage)
		if err != nil {
			return nil, fmt.Errorf("build protocol stage topology: compose stage %q: %w", name, err)
		}
	}

	if config.ClientProtocol == current.Protocol() {
		return current, nil
	}
	ingress, err := config.Registry.Resolve(config.ClientProtocol, current.Protocol(), required)
	if err != nil {
		return nil, fmt.Errorf("build protocol stage topology: ingress bridge: %w", err)
	}
	current, err = Adapt(current, ingress)
	if err != nil {
		return nil, fmt.Errorf("build protocol stage topology: adapt ingress bridge: %w", err)
	}
	return current, nil
}
