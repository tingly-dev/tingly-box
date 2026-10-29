// Package toolnamespace bridges Responses API `namespace` tools to protocols that only
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
package toolnamespace

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
// order, with every namespace member expanded under its flat name. Whatever
// cannot be offered upstream is logged rather than dropped silently: a
// namespace member lost to a name clash or because it is not a function is a
// warning, a top-level tool type with no flat equivalent (web_search, custom
// and the like, which a flat upstream never had) is debug output, since
// clients such as Codex send those on every request.
func Functions(tools []responses.ToolUnionParam) []responses.FunctionToolParam {
	functions, _ := expand(tools, true)
	return functions
}

// IndexOf returns the Index for a Responses request's tools: one entry per
// function inside a namespace. It is nil when the request has none. It makes
// the same decisions as Functions, so the names it knows are exactly the ones
// the upstream was offered.
func IndexOf(tools []responses.ToolUnionParam) Index {
	_, index := expand(tools, false)
	return index
}

// expand is the one place that decides what a tool list becomes upstream. A
// flat name is not guaranteed unique (namespace "a_" with member "_b" and
// namespace "a" with member "b" both give "a__b", and a plain function can
// share it too), and an upstream that is offered one name twice cannot be
// restored unambiguously, so a plain function always keeps its name, the
// first namespace member declared keeps a contested one, and any later
// member that collides is dropped. report logs what was dropped; the response
// side passes false so each request logs it once.
func expand(tools []responses.ToolUnionParam, report bool) ([]responses.FunctionToolParam, Index) {
	var functions []responses.FunctionToolParam
	var index Index
	// Plain functions keep their names whatever the declaration order.
	taken := map[string]bool{}
	for _, tool := range tools {
		if !param.IsOmitted(tool.OfFunction) {
			taken[tool.OfFunction.Name] = true
		}
	}
	for _, tool := range tools {
		switch {
		case !param.IsOmitted(tool.OfFunction):
			functions = append(functions, *tool.OfFunction)
		case !param.IsOmitted(tool.OfNamespace):
			ns := tool.OfNamespace
			for _, member := range ns.Tools {
				if member.OfFunction == nil {
					if report {
						logrus.Warnf("responses namespace %q: dropping a non-function tool, it has no equivalent on this upstream", ns.Name)
					}
					continue
				}
				fn := member.OfFunction
				flat := FlatName(ns.Name, fn.Name)
				if taken[flat] {
					if report {
						logrus.Warnf("responses namespace %q: dropping tool %q, its flat name %q is already taken by another tool, so the model will not be offered it", ns.Name, fn.Name, flat)
					}
					continue
				}
				taken[flat] = true
				if index == nil {
					index = Index{}
				}
				index[flat] = Entry{Namespace: ns.Name, Name: fn.Name}
				functions = append(functions, responses.FunctionToolParam{
					Name:        flat,
					Description: fn.Description,
					Parameters:  schemaMap(fn.Parameters),
					Strict:      fn.Strict,
				})
			}
		default:
			if kind := tool.GetType(); report && kind != nil {
				logrus.Debugf("responses tool of type %q has no equivalent on this upstream, dropping it", *kind)
			}
		}
	}
	return functions, index
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
