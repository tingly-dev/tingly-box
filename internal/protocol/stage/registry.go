package stage

import (
	"fmt"

	protocol "github.com/tingly-dev/tingly-box/ai"
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
		if isNil(bridge) {
			return nil, fmt.Errorf("create bridge registry: bridge at index %d is nil", i)
		}
		source := bridge.Source()
		target := bridge.Target()
		if source == "" || target == "" {
			return nil, fmt.Errorf(
				"create bridge registry: bridge at index %d has empty protocol (%q -> %q)",
				i,
				source,
				target,
			)
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

// Resolve returns the exact source/target Bridge with the required semantic
// capabilities. Same-protocol pairs use an identity bridge unless an explicit
// same-protocol bridge was registered.
func (r *BridgeRegistry) Resolve(source, target protocol.APIType, required Capabilities) (Bridge, error) {
	return r.resolve(source, target, required, true)
}

// ResolveRegistered returns only a Bridge explicitly registered for the exact
// source/target pair. Unlike Resolve, it does not synthesize an identity Bridge
// for same-protocol pairs. Runtime selectors use this method so enabling a
// production route always requires an intentional registry entry.
func (r *BridgeRegistry) ResolveRegistered(source, target protocol.APIType, required Capabilities) (Bridge, error) {
	return r.resolve(source, target, required, false)
}

func (r *BridgeRegistry) resolve(source, target protocol.APIType, required Capabilities, allowIdentity bool) (Bridge, error) {
	if r == nil {
		return nil, fmt.Errorf("resolve protocol bridge %q -> %q: registry is nil", source, target)
	}
	if source == "" || target == "" {
		return nil, fmt.Errorf("resolve protocol bridge: protocols must be concrete (%q -> %q)", source, target)
	}

	bridge, exists := r.bridges[bridgeKey{source: source, target: target}]
	if !exists && allowIdentity && source == target {
		bridge = NewIdentityBridge(source)
		exists = true
	}
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
