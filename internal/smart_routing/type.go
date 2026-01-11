package smartrouting

import (
	"tingly-box/internal/loadbalance"
)

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
