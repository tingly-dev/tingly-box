package smartrouting

import (
	"tingly-box/internal/loadbalance"
)

// SmartOpPosition represents the field to check in a request for smart routing
type SmartOpPosition string

const (
	PositionModel    SmartOpPosition = "model"    // Request model name
	PositionThinking SmartOpPosition = "thinking" // Thinking mode enabled
	PositionSystem   SmartOpPosition = "system"   // System message content
	PositionUser     SmartOpPosition = "user"     // User message content
	PositionToolUse  SmartOpPosition = "tool_use" // Tool use/name
	PositionToken    SmartOpPosition = "token"    // Token count
)

// SmartOpOperation represents the operation to perform on a position
type SmartOpOperation string

const (
	// Model operations
	OpModelContains SmartOpOperation = "contains" // Model name contains the value
	OpModelGlob     SmartOpOperation = "glob"     // Model name matches glob pattern
	OpModelEquals   SmartOpOperation = "equals"   // Model name equals the value

	// Thinking operations
	OpThinkingEnabled  SmartOpOperation = "enabled"  // Thinking mode is enabled
	OpThinkingDisabled SmartOpOperation = "disabled" // Thinking mode is disabled

	// System message operations
	OpSystemAnyContains SmartOpOperation = "any_contains" // Any system messages contain the value
	OpSystemRegex       SmartOpOperation = "regex"        // Any system messages match regex pattern

	// User message operations
	OpUserAnyContains SmartOpOperation = "any_contains" // Any user messages contain the value
	OpUserContains    SmartOpOperation = "contains"     // Latest message is `user` role and it contains the value
	OpUserRegex       SmartOpOperation = "regex"        // Combined user messages match regex pattern
	OpUserRequestType SmartOpOperation = "type"         // Latest message is `user` role and check its content type

	// Tool use operations
	OpToolUseIs       SmartOpOperation = "is"       // Latest message is `tool use` and its name is the value
	OpToolUseContains SmartOpOperation = "contains" // Latest message is `tool use` and its name or arguments contains the value

	// Token operations
	OpTokenGe SmartOpOperation = "ge" // Token count greater than or equal to value
	OpTokenGt SmartOpOperation = "gt" // Token count greater than value
	OpTokenLe SmartOpOperation = "le" // Token count less than or equal to value
	OpTokenLt SmartOpOperation = "lt" // Token count less than value
)

// SmartOpValueType represents the type of value in a smart routing operation
type SmartOpValueType string

const (
	ValueTypeString SmartOpValueType = "string" // String value
	ValueTypeInt    SmartOpValueType = "int"    // Integer value
	ValueTypeBool   SmartOpValueType = "bool"   // Boolean value
	ValueTypeFloat  SmartOpValueType = "float"  // Float value
)

// SmartOpMeta contains metadata for a smart routing operation
type SmartOpMeta struct {
	Description string           `json:"description,omitempty" yaml:"description,omitempty"` // Human-readable description of the operation
	Type        SmartOpValueType `json:"type,omitempty" yaml:"type,omitempty"`               // Type of the value field
}

// SmartOp represents a single operation for smart routing
// Each operation has 4 parts: position, operation, value, meta
type SmartOp struct {
	Position  SmartOpPosition  `json:"position" yaml:"position"`
	Operation SmartOpOperation `json:"operation" yaml:"operation"`
	Value     string           `json:"value,omitempty" yaml:"value,omitempty"`
	Meta      SmartOpMeta      `json:"meta,omitempty" yaml:"meta,omitempty"`
}

// SmartRouting represents a smart routing rule block
type SmartRouting struct {
	Description string                `json:"description" yaml:"description"`
	Ops         []SmartOp             `json:"ops" yaml:"ops"`
	Services    []loadbalance.Service `json:"services" yaml:"services"`
}

// IsValid checks if the position is valid
func (p SmartOpPosition) IsValid() bool {
	switch p {
	case PositionModel, PositionThinking, PositionSystem, PositionUser, PositionToolUse, PositionToken:
		return true
	default:
		return false
	}
}
