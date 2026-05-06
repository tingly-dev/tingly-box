package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ToolRenderer renders a tool_use call to a single (possibly multi-line) string.
type ToolRenderer func(name string, input map[string]interface{}) string

// ToolResultRenderer renders a tool_result message to a single (possibly
// multi-line) string. The tool name is looked up via ToolUseID by the formatter.
type ToolResultRenderer func(name string, m *ToolResultMessage) string

var (
	toolRenderers       = map[string]ToolRenderer{}
	toolResultRenderers = map[string]ToolResultRenderer{}
)

// RegisterToolRenderer registers a renderer for a tool name.
func RegisterToolRenderer(name string, fn ToolRenderer) { toolRenderers[name] = fn }

// RegisterToolResultRenderer registers a result renderer for a tool name.
func RegisterToolResultRenderer(name string, fn ToolResultRenderer) {
	toolResultRenderers[name] = fn
}

// renderToolUse dispatches to a per-tool renderer or falls back to a generic one.
// detail toggles slightly more verbose per-tool output (e.g. Edit diff preview).
func renderToolUse(name string, input map[string]interface{}, detail bool) string {
	if fn, ok := toolRenderers[name]; ok {
		out := fn(name, input)
		if detail {
			out = appendDetail(name, input, out)
		}
		return out
	}
	return renderGenericToolUse(name, input)
}

// renderToolResult dispatches to a per-tool result renderer or falls back.
func renderToolResult(name string, m *ToolResultMessage) string {
	if name != "" {
		if fn, ok := toolResultRenderers[name]; ok {
			return fn(name, m)
		}
	}
	return renderGenericToolResult(name, m)
}

// ToolUseRef is a lightweight view of a tool_use block used for grouping.
type ToolUseRef struct {
	ID    string
	Name  string
	Input map[string]interface{}
}

// renderToolUseGroup renders a sequence of tool_use blocks, coalescing
// consecutive blocks of the same groupable tool onto one line.
func renderToolUseGroup(refs []ToolUseRef, detail bool) string {
	if len(refs) == 0 {
		return ""
	}
	var lines []string
	i := 0
	for i < len(refs) {
		j := i + 1
		if isGroupable(refs[i].Name) && !detail {
			for j < len(refs) && refs[j].Name == refs[i].Name {
				j++
			}
		}
		if j-i > 1 {
			lines = append(lines, renderGroup(refs[i:j]))
		} else {
			lines = append(lines, renderToolUse(refs[i].Name, refs[i].Input, detail))
		}
		i = j
	}
	return strings.Join(lines, "\n")
}

func isGroupable(name string) bool {
	switch name {
	case "Read", "Write", "Edit", "WebFetch":
		return true
	}
	return false
}

// renderGroup renders a coalesced run of same-tool blocks on one line.
func renderGroup(refs []ToolUseRef) string {
	if len(refs) == 0 {
		return ""
	}
	name := refs[0].Name
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, "`"+basename(getStr(r.Input, "file_path"))+"`")
	}
	switch name {
	case "Read":
		return "Read " + strings.Join(parts, ", ")
	case "Write":
		return "Write " + strings.Join(parts, ", ")
	case "Edit":
		return "Edit " + strings.Join(parts, ", ")
	case "WebFetch":
		urls := make([]string, 0, len(refs))
		for _, r := range refs {
			urls = append(urls, getStr(r.Input, "url"))
		}
		return "Fetch " + strings.Join(urls, ", ")
	}
	// Unreachable given isGroupable, but be defensive.
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, renderToolUse(r.Name, r.Input, false))
	}
	return strings.Join(out, "\n")
}

func appendDetail(name string, input map[string]interface{}, base string) string {
	switch name {
	case "Edit":
		oldS := firstLine(getStr(input, "old_string"))
		newS := firstLine(getStr(input, "new_string"))
		if oldS == "" && newS == "" {
			return base
		}
		return base + "\n- " + truncate(oldS, 80) + "\n+ " + truncate(newS, 80)
	case "Bash":
		desc := getStr(input, "description")
		if desc != "" {
			return base + " — " + desc
		}
	case "TodoWrite":
		todos := getSlice(input, "todos")
		if len(todos) == 0 {
			return base
		}
		var b strings.Builder
		b.WriteString(base)
		for i, t := range todos {
			if i >= 5 {
				break
			}
			tm, _ := t.(map[string]interface{})
			b.WriteString("\n  - [")
			b.WriteString(getStr(tm, "status"))
			b.WriteString("] ")
			b.WriteString(truncate(getStr(tm, "content"), 80))
		}
		return b.String()
	}
	return base
}

// ----- per-tool renderers (tool_use) -----

func init() {
	RegisterToolRenderer("Read", renderRead)
	RegisterToolRenderer("Write", renderWrite)
	RegisterToolRenderer("Edit", renderEdit)
	RegisterToolRenderer("MultiEdit", renderMultiEdit)
	RegisterToolRenderer("Bash", renderBash)
	RegisterToolRenderer("BashOutput", renderBashOutput)
	RegisterToolRenderer("KillBash", renderKillBash)
	RegisterToolRenderer("KillShell", renderKillBash)
	RegisterToolRenderer("Grep", renderGrep)
	RegisterToolRenderer("Glob", renderGlob)
	RegisterToolRenderer("LS", renderLS)
	RegisterToolRenderer("WebFetch", renderWebFetch)
	RegisterToolRenderer("WebSearch", renderWebSearch)
	RegisterToolRenderer("Task", renderTask)
	RegisterToolRenderer("Agent", renderTask)
	RegisterToolRenderer("TodoWrite", renderTodoWrite)
	RegisterToolRenderer("NotebookEdit", renderNotebookEdit)
	RegisterToolRenderer("ExitPlanMode", renderExitPlanMode)
	RegisterToolRenderer("SlashCommand", renderSlashCommand)
	RegisterToolRenderer("AskUserQuestion", renderAskUserQuestion)

	RegisterToolResultRenderer("Read", renderReadResult)
	RegisterToolResultRenderer("Bash", renderBashResult)
	RegisterToolResultRenderer("TodoWrite", renderTodoWriteResult)
	RegisterToolResultRenderer("Grep", renderGrepResult)
	RegisterToolResultRenderer("Glob", renderGlobResult)
	RegisterToolResultRenderer("WebFetch", renderWebFetchResult)
	RegisterToolResultRenderer("WebSearch", renderWebSearchResult)
	RegisterToolResultRenderer("Edit", renderQuietResult)
	RegisterToolResultRenderer("Write", renderQuietResult)
	RegisterToolResultRenderer("MultiEdit", renderQuietResult)
}

func renderRead(_ string, in map[string]interface{}) string {
	path := getStr(in, "file_path")
	out := "Read `" + basename(path) + "`"
	offset := getInt(in, "offset")
	limit := getInt(in, "limit")
	if offset > 0 || limit > 0 {
		end := offset + limit
		if limit == 0 {
			end = offset
		}
		out += fmt.Sprintf(" L%d-%d", offset, end)
	}
	return out
}

func renderWrite(_ string, in map[string]interface{}) string {
	path := getStr(in, "file_path")
	content := getStr(in, "content")
	lines := 0
	if content != "" {
		lines = strings.Count(content, "\n") + 1
	}
	if lines > 0 {
		return fmt.Sprintf("Write `%s` (%d lines)", basename(path), lines)
	}
	return "Write `" + basename(path) + "`"
}

func renderEdit(_ string, in map[string]interface{}) string {
	path := getStr(in, "file_path")
	out := "Edit `" + basename(path) + "`"
	if getBool(in, "replace_all") {
		out += " (replace_all)"
	}
	return out
}

func renderMultiEdit(_ string, in map[string]interface{}) string {
	path := getStr(in, "file_path")
	edits := getSlice(in, "edits")
	return fmt.Sprintf("MultiEdit `%s` (%d edits)", basename(path), len(edits))
}

func renderBash(_ string, in map[string]interface{}) string {
	cmd := singleLine(getStr(in, "command"))
	return "`$ " + truncate(cmd, 160) + "`"
}

func renderBashOutput(_ string, in map[string]interface{}) string {
	id := getStr(in, "bash_id")
	out := "BashOutput " + id
	if f := getStr(in, "filter"); f != "" {
		out += " /" + f + "/"
	}
	return out
}

func renderKillBash(name string, in map[string]interface{}) string {
	id := getStr(in, "shell_id")
	if id == "" {
		id = getStr(in, "bash_id")
	}
	return name + " " + id
}

func renderGrep(_ string, in map[string]interface{}) string {
	pattern := getStr(in, "pattern")
	path := getStr(in, "path")
	if path == "" {
		path = "."
	}
	out := fmt.Sprintf("Grep /%s/ in `%s`", pattern, path)
	if g := getStr(in, "glob"); g != "" {
		out += " glob=" + g
	}
	if mode := getStr(in, "output_mode"); mode != "" {
		out += " mode=" + mode
	}
	return out
}

func renderGlob(_ string, in map[string]interface{}) string {
	pattern := getStr(in, "pattern")
	out := "Glob " + pattern
	if path := getStr(in, "path"); path != "" {
		out += " in `" + path + "`"
	}
	return out
}

func renderLS(_ string, in map[string]interface{}) string {
	return "LS `" + getStr(in, "path") + "`"
}

func renderWebFetch(_ string, in map[string]interface{}) string {
	return "Fetch " + getStr(in, "url")
}

func renderWebSearch(_ string, in map[string]interface{}) string {
	return `Search "` + getStr(in, "query") + `"`
}

func renderTask(_ string, in map[string]interface{}) string {
	subagent := getStr(in, "subagent_type")
	desc := truncate(getStr(in, "description"), 80)
	if subagent == "" {
		return "Task " + desc
	}
	return "Task " + subagent + ": " + desc
}

func renderTodoWrite(_ string, in map[string]interface{}) string {
	todos := getSlice(in, "todos")
	return fmt.Sprintf("Todos: %d items", len(todos))
}

func renderNotebookEdit(_ string, in map[string]interface{}) string {
	path := getStr(in, "notebook_path")
	cell := getStr(in, "cell_id")
	mode := getStr(in, "edit_mode")
	out := "NotebookEdit `" + basename(path) + "`"
	if cell != "" {
		out += " cell=" + cell
	}
	if mode != "" {
		out += " mode=" + mode
	}
	return out
}

func renderExitPlanMode(_ string, in map[string]interface{}) string {
	plan := getStr(in, "plan")
	return fmt.Sprintf("Plan ready (%d chars)", len(plan))
}

func renderSlashCommand(_ string, in map[string]interface{}) string {
	cmd := getStr(in, "command")
	if cmd == "" {
		return "SlashCommand"
	}
	return "/" + strings.TrimPrefix(cmd, "/")
}

func renderAskUserQuestion(_ string, in map[string]interface{}) string {
	q := truncate(firstLine(getStr(in, "question")), 100)
	if q == "" {
		// Newer schema uses "questions" array.
		if qs := getSlice(in, "questions"); len(qs) > 0 {
			if first, ok := qs[0].(map[string]interface{}); ok {
				q = truncate(firstLine(getStr(first, "question")), 100)
			}
		}
	}
	if q == "" {
		return "Ask"
	}
	return "Ask: " + q
}

// renderGenericToolUse formats unknown tools without a JSON dump.
func renderGenericToolUse(name string, in map[string]interface{}) string {
	if len(in) == 0 {
		return name
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+truncate(stringifyValue(in[k]), 60))
	}
	out := name + ": " + strings.Join(parts, " ")
	return truncate(out, 200)
}

// ----- per-tool renderers (tool_result) -----

func renderReadResult(_ string, m *ToolResultMessage) string {
	body := resultBody(m)
	if m.IsError {
		return "Read ✗: " + truncate(firstLine(body), 120)
	}
	lines := strings.Count(body, "\n") + 1
	if body == "" {
		lines = 0
	}
	return fmt.Sprintf("Read ✓ (%d lines)", lines)
}

func renderBashResult(_ string, m *ToolResultMessage) string {
	body := resultBody(m)
	mark := "✓"
	if m.IsError {
		mark = "✗"
	}
	if body == "" {
		return "Bash " + mark
	}
	tail, more := lastNLines(body, 3)
	out := "Bash " + mark + "\n" + tail
	if more > 0 {
		out += fmt.Sprintf("\n… (+%d more lines)", more)
	}
	return out
}

func renderTodoWriteResult(_ string, m *ToolResultMessage) string {
	if m.IsError {
		return "Todos ✗: " + truncate(firstLine(resultBody(m)), 120)
	}
	return "Todos updated"
}

func renderGrepResult(_ string, m *ToolResultMessage) string {
	body := resultBody(m)
	if m.IsError {
		return "Grep ✗: " + truncate(firstLine(body), 120)
	}
	if body == "" {
		return "Grep ✓: 0 matches"
	}
	lines := strings.Split(body, "\n")
	n := len(lines)
	if lines[n-1] == "" {
		n--
	}
	preview, more := firstNLines(body, 3)
	out := fmt.Sprintf("Grep ✓: %d matches", n)
	if preview != "" {
		out += "\n" + preview
	}
	if more > 0 {
		out += fmt.Sprintf("\n… (+%d more)", more)
	}
	return out
}

func renderGlobResult(_ string, m *ToolResultMessage) string {
	body := resultBody(m)
	if m.IsError {
		return "Glob ✗: " + truncate(firstLine(body), 120)
	}
	if body == "" {
		return "Glob ✓: 0 matches"
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	preview, more := firstNLines(body, 3)
	out := fmt.Sprintf("Glob ✓: %d matches\n%s", len(lines), preview)
	if more > 0 {
		out += fmt.Sprintf("\n… (+%d more)", more)
	}
	return out
}

func renderWebFetchResult(_ string, m *ToolResultMessage) string {
	if m.IsError {
		return "Fetch ✗: " + truncate(firstLine(resultBody(m)), 120)
	}
	return "Fetch ✓: " + truncate(singleLine(resultBody(m)), 200)
}

func renderWebSearchResult(_ string, m *ToolResultMessage) string {
	if m.IsError {
		return "Search ✗: " + truncate(firstLine(resultBody(m)), 120)
	}
	return "Search ✓: " + truncate(firstLine(resultBody(m)), 200)
}

// renderQuietResult is used by Edit/Write/MultiEdit: only print on error.
func renderQuietResult(name string, m *ToolResultMessage) string {
	if m.IsError {
		return name + " ✗: " + truncate(firstLine(resultBody(m)), 120)
	}
	return ""
}

// renderGenericToolResult is the fallback for tools without a result renderer.
func renderGenericToolResult(name string, m *ToolResultMessage) string {
	mark := "✓"
	if m.IsError {
		mark = "✗"
	}
	prefix := "Result"
	if name != "" {
		prefix = name
	}
	body := resultBody(m)
	if body == "" {
		return prefix + " " + mark
	}
	preview, more := firstNLines(body, 5)
	out := prefix + " " + mark + "\n" + preview
	if more > 0 {
		out += fmt.Sprintf("\n… (+%d more lines)", more)
	}
	return out
}

// resultBody returns the textual body of a ToolResultMessage, preferring the
// explicit Output field but falling back to ToolResultContentBlock content.
func resultBody(m *ToolResultMessage) string {
	if m == nil {
		return ""
	}
	if m.Output != "" {
		return m.Output
	}
	for _, c := range m.Content {
		if tr, ok := c.(*ToolResultContentBlock); ok && tr.Content != "" {
			return tr.Content
		}
	}
	return ""
}

// ----- helpers -----

// inputFromRaw converts a json.RawMessage / interface{} input shape (as used
// by anthropic.ContentBlockUnion) into a map[string]interface{}.
// Returns nil on any error or non-object payload.
func inputFromRaw(v any) map[string]interface{} {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	var data []byte
	switch x := v.(type) {
	case json.RawMessage:
		data = x
	case []byte:
		data = x
	case string:
		data = []byte(x)
	default:
		var err error
		data, err = json.Marshal(x)
		if err != nil {
			return nil
		}
	}
	if len(data) == 0 {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

// getStr is a thin wrapper around the package-level getString that tolerates a
// nil map (callers in this file frequently receive nil from inputFromRaw).
func getStr(m map[string]interface{}, k string) string {
	if m == nil {
		return ""
	}
	return getString(m, k)
}

func getSlice(m map[string]interface{}, k string) []interface{} {
	if m == nil {
		return nil
	}
	if s, ok := m[k].([]interface{}); ok {
		return s
	}
	return nil
}

func basename(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

// firstNLines returns the first n lines and the count of remaining lines.
func firstNLines(s string, n int) (string, int) {
	if s == "" || n <= 0 {
		return "", 0
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n"), 0
	}
	return strings.Join(lines[:n], "\n"), len(lines) - n
}

// lastNLines returns the last n lines and the count of preceding (omitted) lines.
func lastNLines(s string, n int) (string, int) {
	if s == "" || n <= 0 {
		return "", 0
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n"), 0
	}
	return strings.Join(lines[len(lines)-n:], "\n"), len(lines) - n
}

func stringifyValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}
