package vmodel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const refactorFlow = `
id: refactor-flow
on_exhaust: clamp
steps:
  - say: "Let me look at it."
    tool: {name: Read, arguments: {file_path: /tmp/a.go}}
  - tool: {name: Edit, arguments: {file_path: /tmp/a.go, old_string: foo, new_string: bar}}
  - 529
  - midstream: {mode: event, after_events: 2}
    say: "cut off"
  - say: "Done."
    stop_reason: end_turn
    usage: {input: 1200, output: 40, cache_read: 1000}
`

func TestParseScript_FullFlow(t *testing.T) {
	cfg, err := ParseScript([]byte(refactorFlow), "ignored")
	require.NoError(t, err)
	assert.Equal(t, "refactor-flow", cfg.ID)
	assert.Equal(t, "refactor-flow", cfg.Name, "name defaults to id")
	assert.Equal(t, ExhaustClamp, cfg.OnExhaust)
	require.Len(t, cfg.Steps, 5)

	assert.Equal(t, "Read", cfg.Steps[0].Tool.Name)
	assert.Equal(t, "/tmp/a.go", cfg.Steps[0].Tool.Arguments["file_path"])
	assert.Equal(t, 529, cfg.Steps[2].Status, "bare number is a status")
	assert.Equal(t, "event", cfg.Steps[3].MidStream.Mode)
	assert.Equal(t, int64(1200), cfg.Steps[4].Usage.PromptTokens)
	assert.Equal(t, int64(1000), cfg.Steps[4].Usage.CachedInputTokens)
}

func TestParseScript_Defaults(t *testing.T) {
	cfg, err := ParseScript([]byte("steps: [200, 429]"), "from-file")
	require.NoError(t, err)
	assert.Equal(t, "from-file", cfg.ID, "id falls back to the file name")
	assert.Equal(t, ExhaustLoop, cfg.OnExhaust)

	cfg, err = ParseScript([]byte("on_exhaust: loop\nsteps: [200]"), "x")
	require.NoError(t, err)
	assert.Equal(t, ExhaustLoop, cfg.OnExhaust, "explicit loop normalises to the zero value")
}

func TestParseScript_Errors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"empty", "", "empty"},
		{"no steps", "id: a\nsteps: []", "no steps"},
		{"unknown top-level field", "steps: [200]\nstep: [1]", "step"},
		{"unknown step field", "steps:\n  - sya: hi", `unknown step field "sya"`},
		{"tool args typo", "steps:\n  - tool: {name: Read, args: {a: 1}}", `unknown tool field "args"`},
		{"tool without name", "steps:\n  - tool: {arguments: {a: 1}}", "step 1: tool needs a name"},
		{"status out of range", "steps: [200, 302]", "step 2: status 302"},
		{"error step with say", "steps:\n  - status: 429\n    say: hi", "step 1: status 429 is an error step"},
		{"error step with midstream", "steps:\n  - status: 500\n    midstream: {mode: close}", "error step"},
		{"bad midstream mode", "steps:\n  - midstream: {mode: explode}", `unknown midstream mode "explode"`},
		{"bad exhaust", "on_exhaust: nope\nsteps: [200]", "unknown on_exhaust"},
		{"bad id", "id: has space\nsteps: [200]", "invalid id"},
		{"non-numeric bare step", "steps: [hello]", "bare step must be an HTTP status"},
		{"negative repeat", "steps:\n  - repeat: -1", "repeat"},
		{"error fields on a success step", "steps:\n  - say: ok\n    error_message: boom", "only apply to an error step"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseScript([]byte(c.src), "f")
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), c.want), "error %q should contain %q", err, c.want)
		})
	}
}

func TestSequence_ScriptedSteps(t *testing.T) {
	cfg, err := ParseScript([]byte(refactorFlow), "")
	require.NoError(t, err)
	seq := NewSequence(cfg)

	s1 := seq.Next()
	assert.Equal(t, "Let me look at it.", s1.Content)
	require.NotNil(t, s1.Tool)
	assert.Equal(t, "toolu_refactor-flow_1", s1.Tool.ID)
	assert.Equal(t, 200, s1.HTTPStatus())

	s2 := seq.Next()
	assert.Empty(t, s2.Content, "a tool step with no say carries no default text")
	assert.Equal(t, "toolu_refactor-flow_2", s2.Tool.ID, "each tool call gets a distinct id")

	s3 := seq.Next()
	assert.Equal(t, 529, s3.HTTPStatus())
	assert.Equal(t, "overloaded_error", s3.Error.Type)

	s4 := seq.Next()
	assert.Equal(t, 200, s4.HTTPStatus(), "a mid-stream cut has already sent its 200")
	require.NotNil(t, s4.Error)
	assert.Equal(t, ErrorStageMidStream, s4.Error.Stage)
	assert.Equal(t, MidStreamModeErrorEvent, s4.Error.MidStreamMode)
	assert.Equal(t, 2, s4.Error.AfterEvents)

	s5 := seq.Next()
	assert.Equal(t, "end_turn", s5.StopReason)
	assert.Equal(t, int64(40), s5.Usage.CompletionTokens)

	assert.Equal(t, "Done.", seq.Next().Content, "clamp repeats the last step")
}

func TestSequence_BareSuccessUsesDefaultContent(t *testing.T) {
	seq := NewSequence(SequenceConfig{ID: "x", DefaultContent: "dflt", Steps: Steps(200)})
	assert.Equal(t, "dflt", seq.Next().Content)
	seq = NewSequence(SequenceConfig{ID: "x", Steps: Steps(200)})
	assert.Equal(t, FallbackSequenceContent, seq.Next().Content)
}

func TestSequence_ToolIDsAreUniquePerServedRequest(t *testing.T) {
	// repeat, loop and clamp must never hand the same tool_use id twice.
	cfg, err := ParseScript([]byte("steps:\n  - tool: {name: Read}\n    repeat: 2\n  - tool: {name: Edit, id: fixed}\n"), "s")
	require.NoError(t, err)
	seq := NewSequence(cfg)
	seen := map[string]bool{}
	for i := 0; i < 9; i++ { // three full loops
		step := seq.Next()
		id := step.Tool.ID
		if step.Tool.Name == "Edit" {
			assert.Equal(t, "fixed", id, "an explicit id is kept")
			continue
		}
		assert.False(t, seen[id], "tool id %q served twice", id)
		seen[id] = true
	}
	assert.Len(t, seen, 6)
}
