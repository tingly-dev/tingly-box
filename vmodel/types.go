package vmodel

import (
	"fmt"

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

var toolCallKeys = map[string]bool{"id": true, "name": true, "arguments": true}

// UnmarshalYAML rejects unknown keys (e.g. `args:` for `arguments:`) so a
// scripted tool call never silently loses its arguments to a typo.
func (t *ToolCallConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: tool must be a mapping with name and arguments", node.Line)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if k := node.Content[i]; !toolCallKeys[k.Value] {
			return fmt.Errorf("line %d: unknown tool field %q (want id, name, arguments)", k.Line, k.Value)
		}
	}
	type plain ToolCallConfig
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	*t = ToolCallConfig(p)
	return nil
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
