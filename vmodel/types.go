package vmodel

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// VirtualModelType represents the type/category of a virtual model.
type VirtualModelType string

const (
	// VirtualModelTypeStatic represents static mock models that return fixed responses.
	VirtualModelTypeStatic VirtualModelType = "static"

	// VirtualModelTypeProxy represents proxy/transform models that modify requests before forwarding.
	VirtualModelTypeProxy VirtualModelType = "proxy"

	// VirtualModelTypeTool represents tool models that return tool_use blocks.
	VirtualModelTypeTool VirtualModelType = "tool"

	// VirtualModelTypeSequence represents sequence models that walk a configured
	// program of per-request outcomes (e.g. 200, 200, 429) to simulate a flaky
	// upstream provider.
	VirtualModelTypeSequence VirtualModelType = "sequence"

	// VirtualModelTypeDecision represents models that answer OpenAI-style
	// decisions requests by choosing among caller-supplied options.
	VirtualModelTypeDecision VirtualModelType = "decision"
)

// Model represents a virtual model in the models list (OpenAI-compatible format).
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ToolCallConfig defines a tool call to be returned by the virtual model.
type ToolCallConfig struct {
	// ID is the tool_use / tool_call id. Optional: the models fall back to a
	// fixed placeholder, and scripted sequences assign one per step.
	ID        string                 `json:"id,omitempty" yaml:"id,omitempty"`
	Name      string                 `json:"name" yaml:"name"`
	Arguments map[string]interface{} `json:"arguments" yaml:"arguments"`
}

// decodeStrict decodes the mapping node into dst (a pointer to a type that has
// no UnmarshalYAML, so no recursion) after rejecting any key not in allowed.
// It is what makes a script fail loudly on a typo (`args:` for `arguments:`,
// `input_tokens:` for `input:`) at every nesting level — yaml.v3's
// KnownFields only reaches the outermost struct.
func decodeStrict(node *yaml.Node, what string, allowed []string, dst any) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: %s must be a mapping", node.Line, what)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if k := node.Content[i]; !slices.Contains(allowed, k.Value) {
			return fmt.Errorf("line %d: unknown %s field %q (want %s)", k.Line, what, k.Value, strings.Join(allowed, ", "))
		}
	}
	return node.Decode(dst)
}

// UnmarshalYAML is strict about keys; see decodeStrict.
func (t *ToolCallConfig) UnmarshalYAML(node *yaml.Node) error {
	type plain ToolCallConfig
	return decodeStrict(node, "tool", []string{"id", "name", "arguments"}, (*plain)(t))
}

// ToolCallDisplayContent extracts display text from tool call arguments.
// It checks for "message" and "question" keys, returning the first non-empty value found.
func ToolCallDisplayContent(args map[string]interface{}) string {
	if msg, ok := args["message"].(string); ok {
		return msg
	}
	if question, ok := args["question"].(string); ok {
		return question
	}
	return ""
}
