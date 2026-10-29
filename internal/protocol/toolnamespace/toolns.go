// Package toolns bridges Responses API `namespace` tools to protocols that only
// know flat function tools (OpenAI Chat, Anthropic).
//
// A Responses client such as Codex groups MCP tools under a namespace:
//
//	{"type":"namespace","name":"mcp__linear__","tools":[{"type":"function","name":"get_issue",...}]}
//
// and sends a call back as {"type":"function_call","namespace":"mcp__linear__","name":"get_issue"}.
// Flat upstreams have no such grouping, so the request side expands each
// namespace member into a function named FlatName(namespace, name), and the
// response side uses an Index built from the same request to split that name
// back into namespace and name.
package toolns

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
)

// Entry is the namespace and member name a flattened function stands for.
type Entry struct {
	Namespace string
	Name      string
}

// Index maps a flattened function name to the namespace member it came from.
// A nil Index restores nothing.
type Index map[string]Entry

// Function is a plain function tool, either declared at the top level or
// expanded out of a namespace.
type Function struct {
	Name        string
	Description param.Opt[string]
	Parameters  map[string]any
}

// FlatName is the function name a namespace member gets on a flat upstream.
// Codex namespaces MCP servers as `mcp__<server>__`, so the usual result is
// the familiar `mcp__<server>__<tool>`; a namespace without a trailing
// underscore gets a `__` separator.
func FlatName(namespace, name string) string {
	switch {
	case namespace == "":
		return name
	case strings.HasSuffix(namespace, "_"):
		return namespace + name
	default:
		return namespace + "__" + name
	}
}

// Functions returns the function tools of a Responses tool list in request
// order, with every namespace member expanded under its flat name, together
// with the Index needed to undo the expansion. Tool types that have no flat
// equivalent are skipped with a warning rather than dropped silently.
func Functions(tools []responses.ToolUnionParam) ([]Function, Index) {
	var out []Function
	var index Index
	for _, tool := range tools {
		switch {
		case !param.IsOmitted(tool.OfFunction):
			fn := tool.OfFunction
			out = append(out, Function{Name: fn.Name, Description: fn.Description, Parameters: fn.Parameters})
		case !param.IsOmitted(tool.OfNamespace):
			ns := tool.OfNamespace
			for _, member := range ns.Tools {
				if member.OfFunction == nil {
					logrus.Warnf("responses namespace %q: dropping a non-function tool, it has no equivalent on this upstream", ns.Name)
					continue
				}
				fn := member.OfFunction
				flat := FlatName(ns.Name, fn.Name)
				if index == nil {
					index = Index{}
				}
				index[flat] = Entry{Namespace: ns.Name, Name: fn.Name}
				out = append(out, Function{Name: flat, Description: fn.Description, Parameters: schemaMap(fn.Parameters)})
			}
		default:
			if kind := tool.GetType(); kind != nil {
				logrus.Warnf("responses tool of type %q has no equivalent on this upstream, dropping it", *kind)
			}
		}
	}
	return out, index
}

// IndexOf returns the Index for a Responses request's tools.
func IndexOf(tools []responses.ToolUnionParam) Index {
	_, index := Functions(tools)
	return index
}

// Restore splits a flattened function name back into namespace and name. A
// name that was not produced from a namespace member comes back unchanged
// with an empty namespace.
func (i Index) Restore(flat string) (namespace, name string) {
	if entry, ok := i[flat]; ok {
		return entry.Namespace, entry.Name
	}
	return "", flat
}

// schemaMap turns a namespace member's untyped `parameters` into the map the
// flat protocols expect.
func schemaMap(v any) map[string]any {
	switch s := v.(type) {
	case nil:
		return nil
	case map[string]any:
		return s
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}
