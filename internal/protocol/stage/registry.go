package stage

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

type bridgeKey struct {
	source protocol.APIType
	target protocol.APIType
}

// BridgeRegistry is an immutable exact-pair registry. Build a new registry for
// configuration reloads rather than mutating a registry used by active calls.
type BridgeRegistry struct {
	bridges map[bridgeKey]Bridge
}

// NewBridgeRegistry validates and registers exact source/target pairs. Duplicate
// pairs are rejected so bridge selection never depends on registration order.
func NewBridgeRegistry(bridges ...Bridge) (*BridgeRegistry, error) {
	registry := &BridgeRegistry{bridges: make(map[bridgeKey]Bridge, len(bridges))}
	for i, bridge := range bridges {
		if bridge == nil {
			return nil, fmt.Errorf("create bridge registry: bridge at index %d is nil", i)
		}
		source := bridge.Source()
		target := bridge.Target()
		for _, api := range []protocol.APIType{source, target} {
			if err := checkChainProtocol(api); err != nil {
				return nil, fmt.Errorf("create bridge registry: bridge at index %d (%q -> %q): %w", i, source, target, err)
			}
		}
		if missing := bridge.Capabilities().Missing(CoreBridgeCapabilities); missing != 0 {
			return nil, fmt.Errorf(
				"create bridge registry: bridge %q -> %q missing core capabilities: %s",
				source,
				target,
				missing,
			)
		}

		key := bridgeKey{source: source, target: target}
		if _, exists := registry.bridges[key]; exists {
			return nil, fmt.Errorf("create bridge registry: duplicate bridge %q -> %q", source, target)
		}
		registry.bridges[key] = bridge
	}
	return registry, nil
}

// Resolve returns the Bridge registered for the exact source/target pair,
// provided it preserves the required capabilities. There is no implicit
// fallback: a pair is usable only through an intentional registry entry, and
// same-protocol neighbours need no Bridge at all (BuildTopology skips them).
func (r *BridgeRegistry) Resolve(source, target protocol.APIType, required Capabilities) (Bridge, error) {
	if r == nil {
		return nil, fmt.Errorf("resolve protocol bridge %q -> %q: registry is nil", source, target)
	}
	bridge, exists := r.bridges[bridgeKey{source: source, target: target}]
	if !exists {
		return nil, fmt.Errorf("resolve protocol bridge %q -> %q: not registered", source, target)
	}

	required |= CoreBridgeCapabilities
	if missing := bridge.Capabilities().Missing(required); missing != 0 {
		return nil, fmt.Errorf(
			"resolve protocol bridge %q -> %q: missing capabilities: %s",
			source,
			target,
			missing,
		)
	}
	return bridge, nil
}
