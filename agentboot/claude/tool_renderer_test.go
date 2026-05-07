package claude

import (
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestRenderToolUse_NativeTools(t *testing.T) {
	tests := []struct {
		name            string
		tool            string
		input           map[string]interface{}
		detail          bool
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "Read basic",
			tool:         "Read",
			input:        map[string]interface{}{"file_path": "/repo/main.go"},
			wantContains: []string{"Read", "main.go"},
		},
		{
			name:         "Read with offset/limit",
			tool:         "Read",
			input:        map[string]interface{}{"file_path": "/repo/main.go", "offset": float64(10), "limit": float64(20)},
			wantContains: []string{"Read", "main.go", "L10-30"},
		},
		{
			name:         "Write with content",
			tool:         "Write",
			input:        map[string]interface{}{"file_path": "/x/y.go", "content": "line1\nline2\nline3"},
			wantContains: []string{"Write", "y.go", "3 lines"},
		},
		{
			name:         "Edit basic",
			tool:         "Edit",
			input:        map[string]interface{}{"file_path": "/x/y.go", "old_string": "a", "new_string": "b"},
			wantContains: []string{"Edit", "y.go"},
		},
		{
			name:         "Edit replace_all",
			tool:         "Edit",
			input:        map[string]interface{}{"file_path": "/x/y.go", "old_string": "a", "new_string": "b", "replace_all": true},
			wantContains: []string{"Edit", "y.go", "replace_all"},
		},
		{
			name:         "Edit detail mode shows diff preview",
			tool:         "Edit",
			input:        map[string]interface{}{"file_path": "/x/y.go", "old_string": "fooLine", "new_string": "barLine"},
			detail:       true,
			wantContains: []string{"Edit", "y.go", "- fooLine", "+ barLine"},
		},
		{
			name:         "MultiEdit",
			tool:         "MultiEdit",
			input:        map[string]interface{}{"file_path": "/x/y.go", "edits": []interface{}{1, 2, 3}},
			wantContains: []string{"MultiEdit", "y.go", "3 edits"},
		},
		{
			name:            "Bash short",
			tool:            "Bash",
			input:           map[string]interface{}{"command": "ls -la", "description": "List files"},
			wantContains:    []string{"$ ls -la"},
			wantNotContains: []string{"List files"}, // description hidden by default
		},
		{
			name:         "Bash detail shows description",
			tool:         "Bash",
			input:        map[string]interface{}{"command": "ls -la", "description": "List files"},
			detail:       true,
			wantContains: []string{"$ ls -la", "List files"},
		},
		{
			name:         "Bash long command truncated",
			tool:         "Bash",
			input:        map[string]interface{}{"command": strings.Repeat("a", 300)},
			wantContains: []string{"$"},
		},
		{
			name:         "Bash multi-line collapsed",
			tool:         "Bash",
			input:        map[string]interface{}{"command": "echo hi\nrm -rf /tmp/foo"},
			wantContains: []string{"$ echo hi rm -rf /tmp/foo"},
		},
		{
			name:         "BashOutput with filter",
			tool:         "BashOutput",
			input:        map[string]interface{}{"bash_id": "shell_1", "filter": "error"},
			wantContains: []string{"BashOutput", "shell_1", "/error/"},
		},
		{
			name:         "KillBash",
			tool:         "KillBash",
			input:        map[string]interface{}{"shell_id": "shell_2"},
			wantContains: []string{"KillBash", "shell_2"},
		},
		{
			name:         "Grep with all options",
			tool:         "Grep",
			input:        map[string]interface{}{"pattern": "func", "path": "src/", "glob": "*.go", "output_mode": "files_with_matches"},
			wantContains: []string{"Grep", "/func/", "src/", "glob=*.go", "mode=files_with_matches"},
		},
		{
			name:         "Grep default path",
			tool:         "Grep",
			input:        map[string]interface{}{"pattern": "todo"},
			wantContains: []string{"Grep", "/todo/", "."},
		},
		{
			name:         "Glob with path",
			tool:         "Glob",
			input:        map[string]interface{}{"pattern": "**/*.go", "path": "internal/"},
			wantContains: []string{"Glob", "**/*.go", "internal/"},
		},
		{
			name:         "WebFetch",
			tool:         "WebFetch",
			input:        map[string]interface{}{"url": "https://example.com"},
			wantContains: []string{"Fetch", "https://example.com"},
		},
		{
			name:         "WebSearch",
			tool:         "WebSearch",
			input:        map[string]interface{}{"query": "golang fmt"},
			wantContains: []string{`Search "golang fmt"`},
		},
		{
			name:         "Task",
			tool:         "Task",
			input:        map[string]interface{}{"subagent_type": "Explore", "description": "Find usages"},
			wantContains: []string{"Task", "Explore", "Find usages"},
		},
		{
			name:         "TodoWrite count only",
			tool:         "TodoWrite",
			input:        map[string]interface{}{"todos": []interface{}{map[string]interface{}{"content": "a", "status": "pending"}, map[string]interface{}{"content": "b", "status": "in_progress"}}},
			wantContains: []string{"Todos", "2 items"},
		},
		{
			name:   "TodoWrite detail lists items",
			tool:   "TodoWrite",
			detail: true,
			input: map[string]interface{}{
				"todos": []interface{}{
					map[string]interface{}{"content": "Task one", "status": "pending"},
					map[string]interface{}{"content": "Task two", "status": "in_progress"},
				},
			},
			wantContains: []string{"Todos", "2 items", "[pending] Task one", "[in_progress] Task two"},
		},
		{
			name:         "NotebookEdit",
			tool:         "NotebookEdit",
			input:        map[string]interface{}{"notebook_path": "/n/foo.ipynb", "cell_id": "c1", "edit_mode": "replace"},
			wantContains: []string{"NotebookEdit", "foo.ipynb", "cell=c1", "mode=replace"},
		},
		{
			name:         "ExitPlanMode",
			tool:         "ExitPlanMode",
			input:        map[string]interface{}{"plan": "step 1\nstep 2"},
			wantContains: []string{"Plan ready", "13 chars"},
		},
		{
			name:         "SlashCommand without prefix",
			tool:         "SlashCommand",
			input:        map[string]interface{}{"command": "review"},
			wantContains: []string{"/review"},
		},
		{
			name:         "SlashCommand with prefix",
			tool:         "SlashCommand",
			input:        map[string]interface{}{"command": "/review"},
			wantContains: []string{"/review"},
		},
		{
			name:         "AskUserQuestion legacy",
			tool:         "AskUserQuestion",
			input:        map[string]interface{}{"question": "Which option?"},
			wantContains: []string{"Ask", "Which option?"},
		},
		{
			name: "AskUserQuestion array form",
			tool: "AskUserQuestion",
			input: map[string]interface{}{
				"questions": []interface{}{
					map[string]interface{}{"question": "Pick one"},
				},
			},
			wantContains: []string{"Ask", "Pick one"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderToolUse(tt.tool, tt.input, tt.detail)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("expected output to contain %q\ngot: %q", want, got)
				}
			}
			for _, no := range tt.wantNotContains {
				if strings.Contains(got, no) {
					t.Errorf("expected output to NOT contain %q\ngot: %q", no, got)
				}
			}
		})
	}
}

func TestRenderToolUse_GenericFallback(t *testing.T) {
	tests := []struct {
		name         string
		tool         string
		input        map[string]interface{}
		wantContains []string
	}{
		{
			name:         "no input",
			tool:         "MysteryTool",
			input:        nil,
			wantContains: []string{"MysteryTool"},
		},
		{
			name:         "with input sorted",
			tool:         "Custom",
			input:        map[string]interface{}{"zeta": "z", "alpha": "a"},
			wantContains: []string{"Custom", "alpha=a", "zeta=z"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderToolUse(tt.tool, tt.input, false)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in %q", want, got)
				}
			}
		})
	}

	// Sorted ordering check
	got := renderToolUse("Custom", map[string]interface{}{"zeta": "z", "alpha": "a", "mid": "m"}, false)
	if i, j, k := strings.Index(got, "alpha"), strings.Index(got, "mid"), strings.Index(got, "zeta"); !(i < j && j < k) {
		t.Errorf("expected keys in sorted order: %q", got)
	}
}

func TestRenderToolUse_MissingFields(t *testing.T) {
	// Each native tool called with empty input should produce non-empty output
	// without panicking. We don't assert specific text; just no crash and a name.
	tools := []string{
		"Read", "Write", "Edit", "MultiEdit", "Bash", "BashOutput", "KillBash",
		"Grep", "Glob", "LS", "WebFetch", "WebSearch", "Task", "TodoWrite",
		"NotebookEdit", "ExitPlanMode", "SlashCommand", "AskUserQuestion",
	}
	for _, name := range tools {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic in %s: %v", name, r)
				}
			}()
			out := renderToolUse(name, nil, false)
			if out == "" {
				t.Errorf("expected non-empty fallback output for %s", name)
			}
		})
	}
}

func TestRenderToolUseGroup_GroupsConsecutive(t *testing.T) {
	refs := []ToolUseRef{
		{ID: "1", Name: "Read", Input: map[string]interface{}{"file_path": "a.go"}},
		{ID: "2", Name: "Read", Input: map[string]interface{}{"file_path": "b.go"}},
		{ID: "3", Name: "Bash", Input: map[string]interface{}{"command": "ls"}},
		{ID: "4", Name: "Read", Input: map[string]interface{}{"file_path": "c.go"}},
	}
	got := renderToolUseGroup(refs, false)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (Read x2, Bash, Read), got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "a.go") || !strings.Contains(lines[0], "b.go") {
		t.Errorf("first line should group reads of a.go and b.go: %q", lines[0])
	}
	if !strings.Contains(lines[1], "$ ls") {
		t.Errorf("second line should be Bash: %q", lines[1])
	}
	if !strings.Contains(lines[2], "c.go") || strings.Contains(lines[2], "a.go") {
		t.Errorf("third line should be standalone Read of c.go: %q", lines[2])
	}
}

func TestRenderToolUseGroup_DetailDisablesGrouping(t *testing.T) {
	// In detail mode each Read should be on its own line so the diff/preview
	// per-call is preserved.
	refs := []ToolUseRef{
		{ID: "1", Name: "Read", Input: map[string]interface{}{"file_path": "a.go"}},
		{ID: "2", Name: "Read", Input: map[string]interface{}{"file_path": "b.go"}},
	}
	got := renderToolUseGroup(refs, true)
	if lines := strings.Split(got, "\n"); len(lines) != 2 {
		t.Fatalf("expected 2 lines in detail mode, got %d:\n%s", len(lines), got)
	}
}

func TestRenderToolResult_PerTool(t *testing.T) {
	tests := []struct {
		name         string
		tool         string
		msg          *ToolResultMessage
		wantContains []string
	}{
		{
			name:         "Read success",
			tool:         "Read",
			msg:          &ToolResultMessage{Output: "line1\nline2\nline3"},
			wantContains: []string{"Read ✓", "3 lines"},
		},
		{
			name:         "Read error",
			tool:         "Read",
			msg:          &ToolResultMessage{Output: "no such file", IsError: true},
			wantContains: []string{"Read ✗", "no such file"},
		},
		{
			name:         "Bash success short",
			tool:         "Bash",
			msg:          &ToolResultMessage{Output: "hello\nworld"},
			wantContains: []string{"Bash ✓", "hello", "world"},
		},
		{
			name:         "Bash success long truncated",
			tool:         "Bash",
			msg:          &ToolResultMessage{Output: "a\nb\nc\nd\ne\nf\ng"},
			wantContains: []string{"Bash ✓", "e", "f", "g", "+4 more"},
		},
		{
			name:         "TodoWrite success",
			tool:         "TodoWrite",
			msg:          &ToolResultMessage{Output: "ignored noisy json"},
			wantContains: []string{"Todos updated"},
		},
		{
			name:         "Grep with matches",
			tool:         "Grep",
			msg:          &ToolResultMessage{Output: "a.go\nb.go\nc.go\nd.go"},
			wantContains: []string{"Grep ✓", "4 matches", "a.go", "+1 more"},
		},
		{
			name:         "Edit success silent",
			tool:         "Edit",
			msg:          &ToolResultMessage{Output: "File edited"},
			wantContains: nil, // expects empty output — see direct check below
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderToolResult(tt.tool, tt.msg)
			if tt.name == "Edit success silent" {
				if got != "" {
					t.Errorf("Edit success should be silent, got %q", got)
				}
				return
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in %q", want, got)
				}
			}
		})
	}
}

func TestRenderToolResult_GenericFallback(t *testing.T) {
	m := &ToolResultMessage{Output: strings.Repeat("line\n", 20)}
	got := renderToolResult("UnknownTool", m)
	if !strings.Contains(got, "UnknownTool ✓") {
		t.Errorf("expected tool name and ✓: %q", got)
	}
	if !strings.Contains(got, "+15 more lines") {
		t.Errorf("expected truncation marker: %q", got)
	}
	// Error branch
	got = renderToolResult("UnknownTool", &ToolResultMessage{Output: "boom", IsError: true})
	if !strings.Contains(got, "✗") {
		t.Errorf("expected ✗ marker: %q", got)
	}
}

func TestRenderToolResult_NoTrackedNameUsesGeneric(t *testing.T) {
	got := renderToolResult("", &ToolResultMessage{Output: "ok"})
	if !strings.Contains(got, "Result") {
		t.Errorf("expected generic 'Result' prefix when name is empty: %q", got)
	}
}

func TestFormatter_AssistantBundlesTextAndTools(t *testing.T) {
	f := NewTextFormatter()
	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Text: "Reading both files."},
				{Type: "tool_use", ID: "T1", Name: "Read", Input: []byte(`{"file_path":"/x/a.go"}`)},
				{Type: "tool_use", ID: "T2", Name: "Read", Input: []byte(`{"file_path":"/x/b.go"}`)},
			},
		},
	}
	got := f.Format(msg)
	if !strings.Contains(got, "Reading both files.") {
		t.Errorf("expected text content: %q", got)
	}
	if !strings.Contains(got, "a.go") || !strings.Contains(got, "b.go") {
		t.Errorf("expected grouped Read line for a.go,b.go: %q", got)
	}
	// Text should appear before the tool line.
	if i, j := strings.Index(got, "Reading both"), strings.Index(got, "a.go"); !(i >= 0 && i < j) {
		t.Errorf("text should precede tools: %q", got)
	}
	// IDs should not be present in user-facing output.
	if strings.Contains(got, "T1") || strings.Contains(got, "T2") {
		t.Errorf("tool_use IDs leaked into output: %q", got)
	}
}

func TestFormatter_StandaloneToolUseSuppressedAfterAssistant(t *testing.T) {
	f := NewTextFormatter()
	asm := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "tool_use", ID: "TX", Name: "Read", Input: []byte(`{"file_path":"/x/a.go"}`)},
			},
		},
	}
	if out := f.Format(asm); out == "" {
		t.Fatalf("expected non-empty assistant output")
	}
	dup := &ToolUseMessage{ToolUseID: "TX", Name: "Read", Input: map[string]interface{}{"file_path": "/x/a.go"}}
	if out := f.Format(dup); out != "" {
		t.Errorf("expected duplicate standalone ToolUseMessage to be suppressed, got %q", out)
	}
}

func TestFormatter_StandaloneToolUseShownIfNotInAssistant(t *testing.T) {
	f := NewTextFormatter()
	msg := &ToolUseMessage{ToolUseID: "T_NEW", Name: "Bash", Input: map[string]interface{}{"command": "echo hi"}}
	out := f.Format(msg)
	if !strings.Contains(out, "$ echo hi") {
		t.Errorf("expected standalone ToolUseMessage to render, got %q", out)
	}
}

func TestFormatter_ToolNameCorrelation(t *testing.T) {
	f := NewTextFormatter()
	asm := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "tool_use", ID: "T1", Name: "Read", Input: []byte(`{"file_path":"/x/a.go"}`)},
			},
		},
	}
	_ = f.Format(asm)

	res := &ToolResultMessage{ToolUseID: "T1", Output: "line1\nline2"}
	got := f.Format(res)
	if !strings.Contains(got, "Read ✓") {
		t.Errorf("expected Read-specific result rendering, got %q", got)
	}
	if !strings.Contains(got, "2 lines") {
		t.Errorf("expected line count from Read renderer, got %q", got)
	}

	// Unknown ID falls back to generic.
	got = f.Format(&ToolResultMessage{ToolUseID: "UNKNOWN", Output: "ok"})
	if !strings.Contains(got, "Result") {
		t.Errorf("expected generic fallback for unknown ID, got %q", got)
	}
}

func TestFormatter_ConcurrentToolNameMap(t *testing.T) {
	f := NewTextFormatter()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "id" + string(rune('A'+i%26))
			f.rememberToolName(id, "Read")
			_ = f.lookupToolName(id)
			_ = f.consumeToolName(id)
			f.markAssistantToolID(id)
			_ = f.wasAssistantToolID(id)
		}(i)
	}
	wg.Wait()
}

func TestInputFromRaw(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"raw bytes", []byte(`{"file_path":"/a.go"}`), "/a.go"},
		{"json.RawMessage", anthropicRawJSON(`{"file_path":"/a.go"}`), "/a.go"},
		{"string", `{"file_path":"/a.go"}`, "/a.go"},
		{"already a map", map[string]interface{}{"file_path": "/a.go"}, "/a.go"},
		{"non-object json", []byte(`"hello"`), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := inputFromRaw(tc.in)
			if tc.want == "" {
				if got := getStr(m, "file_path"); got != "" {
					t.Errorf("expected empty file_path, got %q (m=%v)", got, m)
				}
				return
			}
			if got := getStr(m, "file_path"); got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// anthropicRawJSON is a tiny helper to avoid importing encoding/json here.
func anthropicRawJSON(s string) []byte { return []byte(s) }
