package stage

import (
	"fmt"
	"strings"
)

// Capabilities is a deterministic bit set describing which semantic surfaces a
// Bridge preserves. Complete, stream, and error support are mandatory for every
// Bridge adapted as an Endpoint; a chain may require additional capabilities.
type Capabilities uint64

const (
	CapabilityComplete Capabilities = 1 << iota
	CapabilityStream
	CapabilityError
	CapabilityUsage
	CapabilityFinishReason
	CapabilityToolUse
	CapabilityToolResult
)

// CoreBridgeCapabilities are required for every Bridge used by Adapt.
const CoreBridgeCapabilities = CapabilityComplete | CapabilityStream | CapabilityError

// AllBridgeCapabilities contains every capability currently understood by the
// stage package. Identity bridges support this complete set.
const AllBridgeCapabilities = CoreBridgeCapabilities |
	CapabilityUsage |
	CapabilityFinishReason |
	CapabilityToolUse |
	CapabilityToolResult

var orderedCapabilities = []struct {
	capability Capabilities
	name       string
}{
	{CapabilityComplete, "complete"},
	{CapabilityStream, "stream"},
	{CapabilityError, "error"},
	{CapabilityUsage, "usage"},
	{CapabilityFinishReason, "finish_reason"},
	{CapabilityToolUse, "tool_use"},
	{CapabilityToolResult, "tool_result"},
}

// Supports reports whether c contains every required capability.
func (c Capabilities) Supports(required Capabilities) bool {
	return c&required == required
}

// Missing returns the required capabilities not present in c.
func (c Capabilities) Missing(required Capabilities) Capabilities {
	return required &^ c
}

// String returns stable, comma-separated concrete capability names.
func (c Capabilities) String() string {
	if c == 0 {
		return "none"
	}

	remaining := c
	names := make([]string, 0, len(orderedCapabilities)+1)
	for _, item := range orderedCapabilities {
		if c&item.capability == 0 {
			continue
		}
		names = append(names, item.name)
		remaining &^= item.capability
	}
	if remaining != 0 {
		names = append(names, fmt.Sprintf("unknown(%#x)", uint64(remaining)))
	}
	return strings.Join(names, ",")
}
