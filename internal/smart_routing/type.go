package smartrouting

import (
	"tingly-box/internal/loadbalance"
)

// AllOperations is a comprehensive list of all available operations for smart routing.
// This registry defines all operations across all positions for documentation,
// UI rendering, and future API integrations.
var AllOperations = []SmartOpDefinition{
	// Model operations
	{Position: PositionModel, Operation: "contains", Description: "Model name contains the value", ValueType: ValueTypeString},
	{Position: PositionModel, Operation: "glob", Description: "Model name matches glob pattern", ValueType: ValueTypeString},
	{Position: PositionModel, Operation: "equals", Description: "Model name equals the value", ValueType: ValueTypeString},

	// Thinking operations
	{Position: PositionThinking, Operation: "enabled", Description: "Thinking mode is enabled", ValueType: ValueTypeBool},
	{Position: PositionThinking, Operation: "disabled", Description: "Thinking mode is disabled", ValueType: ValueTypeBool},

	// System message operations
	{Position: PositionSystem, Operation: "any_contains", Description: "Any system messages contain the value", ValueType: ValueTypeString},
	{Position: PositionSystem, Operation: "regex", Description: "Any system messages match regex pattern", ValueType: ValueTypeString},

	// User message operations
	{Position: PositionUser, Operation: "any_contains", Description: "Any user messages contain the value", ValueType: ValueTypeString},
	{Position: PositionUser, Operation: "contains", Description: "Lastest message is `user` role and it contains the value", ValueType: ValueTypeString},
	{Position: PositionUser, Operation: "regex", Description: "Combined user messages match regex pattern", ValueType: ValueTypeString},
	{Position: PositionUser, Operation: "request_type", Description: "Lastest message is `user` role and check its content type (e.g., 'image')", ValueType: ValueTypeString},

	// Tool use operations
	{Position: PositionToolUse, Operation: "is", Description: "Latest message is `tool use` and it is name is the value", ValueType: ValueTypeString},
	{Position: PositionToolUse, Operation: "contains", Description: "Latest message is `tool use` and its name or arguments contains the value", ValueType: ValueTypeString},

	// Token operations
	{Position: PositionToken, Operation: "ge", Description: "Token count greater than or equal to value", ValueType: ValueTypeInt},
	{Position: PositionToken, Operation: "gt", Description: "Token count greater than value", ValueType: ValueTypeInt},
	{Position: PositionToken, Operation: "le", Description: "Token count less than or equal to value", ValueType: ValueTypeInt},
	{Position: PositionToken, Operation: "lt", Description: "Token count less than value", ValueType: ValueTypeInt},
}

// SmartOpDefinition defines a single operation with its metadata
type SmartOpDefinition struct {
	Position    SmartOpPosition  `json:"position"`    // Position this operation applies to
	Operation   string           `json:"operation"`   // Operation name
	Description string           `json:"description"` // Human-readable description
	ValueType   SmartOpValueType `json:"value_type"`  // Expected value type
}

const (
	PositionModel    SmartOpPosition = "model"    // Request model name
	PositionThinking SmartOpPosition = "thinking" // Thinking mode enabled
	PositionSystem   SmartOpPosition = "system"   // System message content
	PositionUser     SmartOpPosition = "user"     // User message content
	PositionToolUse  SmartOpPosition = "tool_use" // Tool use/name
	PositionToken    SmartOpPosition = "token"    // Token count
)

// SmartOpPosition represents the field to check in a request for smart routing
type SmartOpPosition string

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
	Position  SmartOpPosition `json:"position" yaml:"position"`
	Operation string          `json:"operation" yaml:"operation"`
	Value     string          `json:"value,omitempty" yaml:"value,omitempty"`
	Meta      SmartOpMeta     `json:"meta,omitempty" yaml:"meta,omitempty"`
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
