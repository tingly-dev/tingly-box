package smartrouting

import (
	"fmt"
	"strconv"

	"tingly-box/internal/loadbalance"
)

// SmartOpPosition represents the field to check in a request for smart routing
type SmartOpPosition string

// SmartOpOperation represents the operation to perform on a position
type SmartOpOperation string

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
	UUID      string           `json:"uuid"`
	Position  SmartOpPosition  `json:"position" yaml:"position"`
	Operation SmartOpOperation `json:"operation" yaml:"operation"`
	Value     string           `json:"value,omitempty" yaml:"value,omitempty"`
	Meta      SmartOpMeta      `json:"meta,omitempty" yaml:"meta,omitempty"`
}

// String returns the value as a string with type checking
func (o *SmartOp) String() (string, error) {
	return o.Value, nil
}

// Int returns the value as an integer with type checking and conversion
func (o *SmartOp) Int() (int, error) {
	result, err := parseInt(o.Value)
	if err != nil {
		return 0, &TypeError{Expected: ValueTypeInt, Got: o.Meta.Type, Err: err}
	}
	return result, nil
}

// Bool returns the value as a boolean with type checking and conversion
func (o *SmartOp) Bool() (bool, error) {
	result, err := parseBool(o.Value)
	if err != nil {
		return false, &TypeError{Expected: ValueTypeBool, Got: o.Meta.Type, Err: err}
	}
	return result, nil
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

// TypeError represents a type mismatch error when accessing SmartOp values
type TypeError struct {
	Expected SmartOpValueType
	Got      SmartOpValueType
	Err      error // Underlying conversion error
}

func (e *TypeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("type error: expected %s, got %s: %v", e.Expected, e.Got, e.Err)
	}
	return fmt.Sprintf("type error: expected %s, got %s", e.Expected, e.Got)
}

func (e *TypeError) Unwrap() error {
	return e.Err
}

// parseInt parses a string to int with better error messages
func parseInt(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	return strconv.Atoi(s)
}

// parseBool parses a string to bool with flexible options
func parseBool(s string) (bool, error) {
	if s == "" {
		return false, nil // Empty string defaults to false for boolean ops
	}
	// Try standard parsing first
	if b, err := strconv.ParseBool(s); err == nil {
		return b, nil
	}
	return false, fmt.Errorf("invalid boolean value: %q", s)
}
