package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// WorkBuddy's chat endpoint refuses several request shapes that other
// OpenAI-compatible upstreams take. workbuddyFitChat rewrites a
// /chat/completions body into the shapes it takes, meaning the same; the
// rules, and the refusals that taught them, are in
// .design/workbuddy-oauth.md#request-shapes. They follow
// @magpie-community/opencode-workbuddy-auth, which found each against real
// accounts.

// workbuddyToolNameOK is the tool-name alphabet WorkBuddy takes; any other
// character (".", ":", "/", a space, CJK) is refused.
var workbuddyToolNameOK = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var workbuddyToolNameBad = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// workbuddyNulEscape finds a regex's \0 (the NUL character), stepping over
// each \\ so a literal backslash followed by 0 is left; \01 (octal) is left.
var workbuddyNulEscape = regexp.MustCompile(`\\\\|\\0`)

// workbuddyOctalAfterNul undoes workbuddyNulEscape where the \0 began an
// octal escape (\01); RE2 has no lookahead to skip it in one pass.
var workbuddyOctalAfterNul = regexp.MustCompile(`\\u0000([0-9])`)

const workbuddySchemaDepth = 64

// workbuddyFitChat returns body rewritten for WorkBuddy and the tool names it
// changed, each mapped back to the client's own so the model's calls can be
// named back (workbuddyNameBack). A body that is not a chat, or that needs no
// change, is returned as it was.
func workbuddyFitChat(body []byte) ([]byte, map[string]string) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var b map[string]any
	if err := dec.Decode(&b); err != nil || b == nil {
		return body, nil
	}
	if _, ok := b["messages"].([]any); !ok {
		return body, nil
	}

	changed := false
	// max_tokens, not max_completion_tokens.
	if v, ok := b["max_completion_tokens"]; ok {
		if _, has := b["max_tokens"]; !has {
			b["max_tokens"] = v
		}
		delete(b, "max_completion_tokens")
		changed = true
	}
	// system, not developer.
	for _, m := range b["messages"].([]any) {
		if msg, ok := m.(map[string]any); ok && msg["role"] == "developer" {
			msg["role"] = "system"
			changed = true
		}
	}
	names, toolsChanged := workbuddyFitTools(b)
	if toolsChanged {
		changed = true
	}
	// After the tools, so a named choice follows a renamed tool.
	if workbuddyFitToolChoice(b) {
		changed = true
	}
	if !changed {
		return body, names
	}
	out, err := json.Marshal(b)
	if err != nil {
		return body, nil
	}
	return out, names
}

// workbuddyFitToolChoice turns the object tool_choice OpenAI clients send
// into WorkBuddy's string form; "none" drops the tools altogether.
func workbuddyFitToolChoice(b map[string]any) bool {
	choice, ok := b["tool_choice"]
	if !ok {
		return false
	}
	dropTools := func() {
		delete(b, "tool_choice")
		delete(b, "tools")
		delete(b, "functions")
	}
	switch c := choice.(type) {
	case string:
		if strings.EqualFold(strings.TrimSpace(c), "none") {
			dropTools()
			return true
		}
		return false
	case map[string]any:
		kind, _ := c["type"].(string)
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "none":
			dropTools()
		case "auto", "required":
			b["tool_choice"] = strings.ToLower(strings.TrimSpace(kind))
		case "function":
			name := ""
			if fn, ok := c["function"].(map[string]any); ok {
				name, _ = fn["name"].(string)
			}
			if name = strings.TrimSpace(name); name == "" {
				name = "auto"
			}
			b["tool_choice"] = name
		default:
			delete(b, "tool_choice")
		}
		return true
	default:
		delete(b, "tool_choice")
		return true
	}
}

// workbuddyFitTools mends the function tools: names WorkBuddy refuses are
// made safe (and the chat's history and tool_choice follow), and each
// tool's parameters get a shape every model behind WorkBuddy takes.
func workbuddyFitTools(b map[string]any) (map[string]string, bool) {
	tools, _ := b["tools"].([]any)
	if len(tools) == 0 {
		return nil, false
	}
	changed := false
	taken := map[string]bool{}
	for _, t := range tools {
		if fn := workbuddyToolFunction(t); fn != nil {
			if n, ok := fn["name"].(string); ok && workbuddyToolNameOK.MatchString(n) {
				taken[n] = true
			}
		}
	}
	back := map[string]string{} // safe -> client's
	safeOf := map[string]string{}
	for _, t := range tools {
		fn := workbuddyToolFunction(t)
		if fn == nil {
			continue
		}
		if n, ok := fn["name"].(string); ok && !workbuddyToolNameOK.MatchString(n) {
			safe := workbuddySafeName(n, taken)
			back[safe] = n
			safeOf[n] = safe
			fn["name"] = safe
			changed = true
		}
		params, present := fn["parameters"]
		if !present {
			continue
		}
		ps, ok := params.(map[string]any)
		if !ok {
			fn["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
			changed = true
			continue
		}
		if workbuddyFitSchema(ps, 0) {
			changed = true
		}
		if workbuddyFitShape(ps, 0) {
			changed = true
		}
		if workbuddyObjectRoot(ps, ps, 0) {
			changed = true
		}
	}
	if len(safeOf) > 0 {
		for _, m := range b["messages"].([]any) {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}
			calls, _ := msg["tool_calls"].([]any)
			for _, c := range calls {
				if call, ok := c.(map[string]any); ok {
					if fn, ok := call["function"].(map[string]any); ok {
						if n, ok := fn["name"].(string); ok && safeOf[n] != "" {
							fn["name"] = safeOf[n]
						}
					}
				}
			}
			if msg["role"] == "tool" {
				if n, ok := msg["name"].(string); ok && safeOf[n] != "" {
					msg["name"] = safeOf[n]
				}
			}
		}
		if c, ok := b["tool_choice"].(map[string]any); ok {
			if fn, ok := c["function"].(map[string]any); ok {
				if n, ok := fn["name"].(string); ok && safeOf[n] != "" {
					fn["name"] = safeOf[n]
				}
			}
		}
	}
	if len(back) == 0 {
		back = nil
	}
	return back, changed
}

func workbuddyToolFunction(t any) map[string]any {
	tool, ok := t.(map[string]any)
	if !ok {
		return nil
	}
	fn, _ := tool["function"].(map[string]any)
	return fn
}

func workbuddySafeName(name string, taken map[string]bool) string {
	base := workbuddyToolNameBad.ReplaceAllString(name, "_")
	if base == "" {
		base = "tool"
	}
	n := base
	for i := 2; taken[n]; i++ {
		n = fmt.Sprintf("%s_%d", base, i)
	}
	taken[n] = true
	return n
}

// workbuddyFitSchema mends what DeepSeek's validator refuses, at every depth:
// a pattern's \0 becomes \u0000 (matching the same), and a tuple's items (a
// list) becomes one schema any of them fits.
func workbuddyFitSchema(v any, depth int) bool {
	if depth > workbuddySchemaDepth {
		return false
	}
	changed := false
	switch s := v.(type) {
	case []any:
		for _, x := range s {
			if workbuddyFitSchema(x, depth+1) {
				changed = true
			}
		}
	case map[string]any:
		if p, ok := s["pattern"].(string); ok && strings.Contains(p, `\0`) {
			fixed := workbuddyNulEscape.ReplaceAllStringFunc(p, func(m string) string {
				if m == `\\` {
					return m
				}
				return `\u0000`
			})
			// \01 is an octal escape, not NUL: restore any that were hit.
			fixed = workbuddyOctalAfterNul.ReplaceAllString(fixed, `\0$1`)
			if fixed != p {
				s["pattern"] = fixed
				changed = true
			}
		}
		if items, ok := s["items"].([]any); ok {
			var list []any
			for _, x := range items {
				if _, ok := x.(map[string]any); ok {
					list = append(list, x)
				}
			}
			switch len(list) {
			case 0:
				s["items"] = map[string]any{}
			case 1:
				s["items"] = list[0]
			default:
				s["items"] = map[string]any{"anyOf": list}
			}
			delete(s, "additionalItems")
			changed = true
		}
		for _, x := range s {
			if workbuddyFitSchema(x, depth+1) {
				changed = true
			}
		}
	}
	return changed
}

var workbuddySchemaTypes = map[string]bool{
	"string": true, "number": true, "integer": true, "boolean": true,
	"object": true, "array": true, "null": true,
}

// Keywords whose value is one schema, a list of schemas, or a map of names to
// schemas. A name under properties is never read as a keyword.
var (
	workbuddySubOne  = []string{"items", "additionalItems", "unevaluatedItems", "contains", "not", "if", "then", "else", "propertyNames", "additionalProperties", "unevaluatedProperties"}
	workbuddySubList = []string{"anyOf", "oneOf", "allOf", "prefixItems"}
	workbuddySubMap  = []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"}
)

// workbuddySubSchema is v, found under key, as an object: true is {}, false is
// {"not": {}}. additionalProperties / unevaluatedProperties keep a boolean.
func workbuddySubSchema(v any, key string) any {
	bv, ok := v.(bool)
	if !ok || key == "additionalProperties" || key == "unevaluatedProperties" {
		return v
	}
	if bv {
		return map[string]any{}
	}
	return map[string]any{"not": map[string]any{}}
}

// workbuddyFitShape mends what Kimi refuses, at every depth: unknown or
// miscased types, a non-list required, untyped / multi-typed / empty enums,
// and boolean schemas. None of it narrows or widens what a call may hold,
// except an empty enum (which nothing fits) is dropped.
func workbuddyFitShape(v any, depth int) bool {
	s, ok := v.(map[string]any)
	if !ok || depth > workbuddySchemaDepth {
		return false
	}
	changed := false
	if t, has := s["type"]; has {
		var list []any
		if arr, ok := t.([]any); ok {
			list = arr
		} else {
			list = []any{t}
		}
		var uniq []string
		seen := map[string]bool{}
		for _, x := range list {
			name, ok := x.(string)
			if !ok {
				continue
			}
			name = strings.ToLower(name)
			if workbuddySchemaTypes[name] && !seen[name] {
				seen[name] = true
				uniq = append(uniq, name)
			}
		}
		var want any
		switch len(uniq) {
		case 0:
		case 1:
			want = uniq[0]
		default:
			arr := make([]any, len(uniq))
			for i, u := range uniq {
				arr[i] = u
			}
			want = arr
		}
		if !reflect.DeepEqual(want, t) {
			if want == nil {
				delete(s, "type")
			} else {
				s["type"] = want
			}
			changed = true
		}
	}
	if r, has := s["required"]; has {
		arr, ok := r.([]any)
		if !ok {
			delete(s, "required")
			changed = true
		} else {
			var keep []any
			for _, k := range arr {
				if _, ok := k.(string); ok {
					keep = append(keep, k)
				}
			}
			if len(keep) != len(arr) {
				if keep == nil {
					keep = []any{}
				}
				s["required"] = keep
				changed = true
			}
		}
	}
	if workbuddyFitEnum(s) {
		changed = true
	}
	for _, k := range workbuddySubOne {
		x, has := s[k]
		if !has {
			continue
		}
		y := workbuddySubSchema(x, k)
		if _, wasBool := x.(bool); wasBool && !reflect.DeepEqual(x, y) {
			s[k] = y
			changed = true
		}
		if workbuddyFitShape(y, depth+1) {
			changed = true
		}
	}
	for _, k := range workbuddySubList {
		arr, ok := s[k].([]any)
		if !ok {
			continue
		}
		for i, x := range arr {
			y := workbuddySubSchema(x, k)
			if _, wasBool := x.(bool); wasBool {
				arr[i] = y
				changed = true
			}
			if workbuddyFitShape(y, depth+1) {
				changed = true
			}
		}
	}
	for _, k := range workbuddySubMap {
		m, ok := s[k].(map[string]any)
		if !ok {
			continue
		}
		for name, x := range m {
			y := workbuddySubSchema(x, k)
			if _, wasBool := x.(bool); wasBool {
				m[name] = y
				changed = true
			}
			if workbuddyFitShape(y, depth+1) {
				changed = true
			}
		}
	}
	return changed
}

func workbuddyValueType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		if strings.ContainsAny(string(x), ".eE") {
			return "number"
		}
		return "integer"
	case float64:
		if x == float64(int64(x)) {
			return "integer"
		}
		return "number"
	default:
		return "string"
	}
}

// workbuddyFitEnum gives an enum the type its values have (or that and
// "null"); values of several types become an anyOf of one typed enum each.
func workbuddyFitEnum(s map[string]any) bool {
	raw, has := s["enum"]
	if !has {
		return false
	}
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		delete(s, "enum")
		return true
	}
	var listed []string
	switch t := s["type"].(type) {
	case string:
		listed = []string{t}
	case []any:
		for _, x := range t {
			if n, ok := x.(string); ok {
				listed = append(listed, n)
			}
		}
	}
	nonNull := 0
	for _, t := range listed {
		if t != "null" {
			nonNull++
		}
	}
	if nonNull == 1 {
		return false
	}
	var order []string
	byType := map[string][]any{}
	for _, v := range values {
		t := workbuddyValueType(v)
		if _, seen := byType[t]; !seen {
			order = append(order, t)
		}
		byType[t] = append(byType[t], v)
	}
	if ints, ok := byType["integer"]; ok {
		if _, hasNum := byType["number"]; hasNum {
			byType["number"] = append(byType["number"], ints...)
			delete(byType, "integer")
		}
	}
	_, hasNull := byType["null"]
	delete(byType, "null")
	var kinds []string
	for _, t := range order {
		if _, ok := byType[t]; ok {
			kinds = append(kinds, t)
		}
	}
	if len(kinds) <= 1 {
		var want any = "null"
		if len(kinds) == 1 {
			want = kinds[0]
			if hasNull {
				want = []any{kinds[0], "null"}
			}
		}
		if reflect.DeepEqual(want, s["type"]) {
			return false
		}
		s["type"] = want
		return true
	}
	split := make([]any, 0, len(kinds)+1)
	for _, t := range kinds {
		split = append(split, map[string]any{"type": t, "enum": byType[t]})
	}
	if hasNull {
		split = append(split, map[string]any{"type": "null"})
	}
	delete(s, "enum")
	delete(s, "type")
	if _, ok := s["anyOf"].([]any); ok {
		allOf, _ := s["allOf"].([]any)
		s["allOf"] = append(allOf, map[string]any{"anyOf": split})
	} else {
		s["anyOf"] = split
	}
	return true
}

// workbuddyLocalRef resolves a local $ref ("#/$defs/x") against root.
func workbuddyLocalRef(root map[string]any, v any) any {
	s, ok := v.(map[string]any)
	if !ok {
		return v
	}
	ref, ok := s["$ref"].(string)
	if !ok || !strings.HasPrefix(ref, "#/") {
		return v
	}
	var at any = root
	for _, k := range strings.Split(ref[2:], "/") {
		m, ok := at.(map[string]any)
		if !ok {
			return v
		}
		at = m[strings.ReplaceAll(strings.ReplaceAll(k, "~1", "/"), "~0", "~")]
	}
	if m, ok := at.(map[string]any); ok {
		return m
	}
	return v
}

func workbuddyHasUnion(s map[string]any) bool {
	for _, k := range []string{"allOf", "anyOf", "oneOf"} {
		if _, ok := s[k].([]any); ok {
			return true
		}
	}
	return false
}

// workbuddyDeepCopy copies a decoded JSON value, so a $ref target merged into
// a root is not changed where it is defined.
func workbuddyDeepCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, y := range x {
			out[k] = workbuddyDeepCopy(y)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, y := range x {
			out[i] = workbuddyDeepCopy(y)
		}
		return out
	default:
		return v
	}
}

// workbuddyObjectRoot makes a tool's parameters an object at the root: allOf
// branches are merged whole; of anyOf / oneOf object branches the properties
// are merged and a field every branch requires stays required. A root with no
// type is an object.
func workbuddyObjectRoot(ps, root map[string]any, depth int) bool {
	if ps["type"] == "object" && !workbuddyHasUnion(ps) {
		return false
	}
	props, _ := ps["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	var required []any
	if r, ok := ps["required"].([]any); ok {
		required = append(required, r...)
	}
	branch := func(v any) map[string]any {
		b, ok := workbuddyLocalRef(root, v).(map[string]any)
		if !ok {
			return nil
		}
		if depth < 8 && workbuddyHasUnion(b) {
			b = workbuddyDeepCopy(b).(map[string]any)
			workbuddyObjectRoot(b, root, depth+1)
		}
		return b
	}
	allOf, _ := ps["allOf"].([]any)
	for _, v := range allOf {
		b := branch(v)
		if b == nil {
			continue
		}
		if bp, ok := b["properties"].(map[string]any); ok {
			for k, x := range bp {
				if _, has := props[k]; !has {
					props[k] = x
				}
			}
		}
		if r, ok := b["required"].([]any); ok {
			required = append(required, r...)
		}
	}
	var branches []map[string]any
	for _, key := range []string{"anyOf", "oneOf"} {
		list, _ := ps[key].([]any)
		for _, v := range list {
			b := branch(v)
			if b == nil {
				continue
			}
			if _, hasProps := b["properties"]; b["type"] != "object" && !hasProps {
				continue
			}
			branches = append(branches, b)
		}
	}
	var fromOrder []string
	from := map[string][]any{}
	for _, b := range branches {
		bp, _ := b["properties"].(map[string]any)
		for k, x := range bp {
			if _, has := props[k]; has {
				continue
			}
			if _, seen := from[k]; !seen {
				fromOrder = append(fromOrder, k)
			}
			dup := false
			for _, y := range from[k] {
				if reflect.DeepEqual(x, y) {
					dup = true
					break
				}
			}
			if !dup {
				from[k] = append(from[k], x)
			}
		}
	}
	for _, k := range fromOrder {
		if list := from[k]; len(list) == 1 {
			props[k] = list[0]
		} else {
			props[k] = map[string]any{"anyOf": list}
		}
	}
	if len(branches) > 0 {
		first, _ := branches[0]["properties"].(map[string]any)
		for k := range first {
			all := true
			for _, b := range branches {
				if !workbuddyRequires(b, k) {
					all = false
					break
				}
			}
			if all {
				required = append(required, k)
			}
		}
	}
	for _, k := range []string{"allOf", "anyOf", "oneOf"} {
		delete(ps, k)
	}
	ps["type"] = "object"
	ps["properties"] = props
	var req []any
	seen := map[string]bool{}
	for _, k := range required {
		name, ok := k.(string)
		if _, has := props[name]; ok && has && !seen[name] {
			seen[name] = true
			req = append(req, name)
		}
	}
	if len(req) > 0 {
		ps["required"] = req
	} else {
		delete(ps, "required")
	}
	return true
}

func workbuddyRequires(b map[string]any, key string) bool {
	r, _ := b["required"].([]any)
	for _, x := range r {
		if x == key {
			return true
		}
	}
	return false
}

// workbuddyNameBack gives the model's tool calls in one chat answer or stream
// chunk the client's own tool names. It reports whether it changed anything.
func workbuddyNameBack(v map[string]any, names map[string]string) bool {
	changed := false
	choices, _ := v["choices"].([]any)
	for _, c := range choices {
		choice, ok := c.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"message", "delta"} {
			at, ok := choice[key].(map[string]any)
			if !ok {
				continue
			}
			calls, _ := at["tool_calls"].([]any)
			for _, tc := range calls {
				call, ok := tc.(map[string]any)
				if !ok {
					continue
				}
				fn, ok := call["function"].(map[string]any)
				if !ok {
					continue
				}
				if n, ok := fn["name"].(string); ok && names[n] != "" {
					fn["name"] = names[n]
					changed = true
				}
			}
		}
	}
	return changed
}
