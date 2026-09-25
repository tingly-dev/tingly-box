package stage

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
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
	if config.Terminal == nil {
		return nil, fmt.Errorf("build protocol stage topology: terminal endpoint is nil")
	}
	if err := checkChainProtocol(config.Terminal.Protocol()); err != nil {
		return nil, fmt.Errorf("build protocol stage topology: terminal endpoint: %w", err)
	}
	if err := checkChainProtocol(config.ClientProtocol); err != nil {
		return nil, fmt.Errorf("build protocol stage topology: client protocol: %w", err)
	}
	if config.Registry == nil {
		return nil, fmt.Errorf("build protocol stage topology: bridge registry is nil")
	}

	required := config.RequiredCapabilities | CoreBridgeCapabilities
	current := config.Terminal
	for i := len(config.Stages) - 1; i >= 0; i-- {
		stage := config.Stages[i]
		if stage == nil {
			return nil, fmt.Errorf("build protocol stage topology: stage at index %d is nil", i)
		}
		name := strings.TrimSpace(stage.Name())
		if name == "" {
			return nil, fmt.Errorf("build protocol stage topology: stage at index %d has empty name", i)
		}
		stageProtocol := stage.Protocol()
		if err := checkChainProtocol(stageProtocol); err != nil {
			return nil, fmt.Errorf("build protocol stage topology: stage %q: %w", name, err)
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
