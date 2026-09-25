package desk

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
	"github.com/tingly-dev/tingly-box/remote/session"
)

func assistantCall(id string, parent *string, in, cacheRead, out int64) *claude.AssistantMessage {
	return &claude.AssistantMessage{
		Message:         anthropic.Message{ID: id, Usage: anthropic.Usage{InputTokens: in, CacheReadInputTokens: cacheRead, OutputTokens: out}},
		ParentToolUseID: parent,
	}
}

func usageOf(t *testing.T, msgs []session.Message) turnUsage {
	t.Helper()
	for _, m := range msgs {
		if m.Kind == "usage" {
			var u turnUsage
			if err := json.Unmarshal(m.Payload, &u); err != nil {
				t.Fatalf("usage payload: %v", err)
			}
			return u
		}
	}
	t.Fatalf("no usage entry in %+v", msgs)
	return turnUsage{}
}

func TestConverter_TurnUsage(t *testing.T) {
	c := newConverter()
	c.messages(&claude.SystemMessage{SubType: claude.SystemSubtypeInit, SessionID: "s", Raw: map[string]interface{}{"model": "tingly/cc"}})
	// One API call arrives as several assistant events carrying the same usage.
	c.messages(assistantCall("m1", nil, 100, 1000, 20))
	c.messages(assistantCall("m1", nil, 100, 1000, 20))
	// A subagent's call costs tokens but isn't the main context.
	parent := "tool-1"
	c.messages(assistantCall("m2", &parent, 50, 0, 5))
	c.messages(assistantCall("m3", nil, 30, 1500, 40))

	u := usageOf(t, c.messages(&claude.ResultMessage{
		DurationMS: 4200,
		ModelUsage: map[string]claude.ModelUsage{"tingly/cc": {OutputTokens: 60, ContextWindow: 200000}},
	}))
	want := turnUsage{
		Model: "tingly/cc", InputTokens: 180, OutputTokens: 65, CacheReadTokens: 2500,
		ContextTokens: 1530, ContextWindow: 200000, DurationMS: 4200,
	}
	if u != want {
		t.Fatalf("usage = %+v\nwant    %+v", u, want)
	}
}

func TestConverter_TurnUsageWithoutInitFallsBackToModelUsage(t *testing.T) {
	// A resident process's later turns have no init message.
	c := newConverter()
	c.messages(assistantCall("m1", nil, 10, 0, 5))
	u := usageOf(t, c.messages(&claude.ResultMessage{ModelUsage: map[string]claude.ModelUsage{
		"tingly/cc-haiku": {OutputTokens: 2, ContextWindow: 200000},
		"tingly/cc":       {OutputTokens: 900, ContextWindow: 1000000},
	}}))
	if u.Model != "tingly/cc" || u.ContextWindow != 1000000 {
		t.Fatalf("model/window = %q/%d, want the model that did the most work", u.Model, u.ContextWindow)
	}
}

func TestConverter_ModelUsageTieIsStable(t *testing.T) {
	for range 20 {
		c := newConverter()
		c.messages(assistantCall("m1", nil, 10, 0, 5))
		u := usageOf(t, c.messages(&claude.ResultMessage{ModelUsage: map[string]claude.ModelUsage{
			"tingly/cc-haiku": {}, "tingly/cc": {}, "tingly/cc-opus": {},
		}}))
		if u.Model != "tingly/cc" {
			t.Fatalf("tied pick = %q, want the smallest id every time", u.Model)
		}
	}
}

func TestConverter_NoUsageForATurnWithoutCalls(t *testing.T) {
	c := newConverter()
	for _, m := range c.messages(&claude.ResultMessage{IsError: true, Result: "boom"}) {
		if m.Kind == "usage" {
			t.Fatalf("usage entry for a turn that never reached the model: %+v", m)
		}
	}
}

func TestRequestedModel_IsTheLatestTurnsModel(t *testing.T) {
	svc, mgr := newTestService(t, completingScript)
	sess := mgr.CreateWith(webChatID, agentType, t.TempDir())
	if got := svc.RequestedModel(sess.ID); got != "" {
		t.Fatalf("before any turn: %q, want empty", got)
	}
	for _, model := range []string{"tingly/cc", "tingly/cc-opus"} {
		payload, _ := json.Marshal(turnUsage{Model: model})
		mgr.AppendMessage(sess.ID, session.Message{Kind: "usage", Payload: payload})
	}
	mgr.AppendMessage(sess.ID, session.Message{Role: "assistant", Content: "after"})
	if got := svc.RequestedModel(sess.ID); got != "tingly/cc-opus" {
		t.Fatalf("RequestedModel = %q, want the latest turn's model", got)
	}
}

func TestScenario(t *testing.T) {
	if got := Scenario(""); got != "claude_code" {
		t.Errorf("Scenario(\"\") = %q", got)
	}
	if got := Scenario("p1"); got != "claude_code:p1" {
		t.Errorf("Scenario(\"p1\") = %q", got)
	}
}

// replayFixture feeds a captured Claude Code stream-json run through the
// real agentboot accumulator and the converter, the way a live process's
// output reaches the transcript.
func replayFixture(t *testing.T, name string) []session.Message {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tr := claude.NewTransport()
	conv := newConverter()
	var out []session.Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var data map[string]any
		if err := json.Unmarshal(sc.Bytes(), &data); err != nil {
			t.Fatalf("fixture line: %v", err)
		}
		typ, _ := data["type"].(string)
		for _, m := range tr.AccumulateMessage(protocol.Event{Type: typ, Data: data, Raw: sc.Text()}) {
			out = append(out, conv.messages(m)...)
		}
	}
	return out
}

// Captured from Claude Code 2.1.282: one turn starts a background subagent,
// a background shell command and a foreground subagent that runs a command
// of its own.
func TestConverter_AttributesSubagentsAndRecordsTasks(t *testing.T) {
	msgs := replayFixture(t, "claude-2.1.282-agents-and-background.jsonl")

	find := func(pred func(session.Message) bool) (session.Message, bool) {
		for _, m := range msgs {
			if pred(m) {
				return m, true
			}
		}
		return session.Message{}, false
	}

	// The subagents' own output is attributed to the Agent call that ran them…
	if m, ok := find(func(m session.Message) bool { return m.Content == "sub bg done" && m.Role == "assistant" }); !ok || m.Parent != "toolu_agent_bg" {
		t.Fatalf("background subagent text = %+v, want parent toolu_agent_bg", m)
	}
	if m, ok := find(func(m session.Message) bool { return m.Kind == "tool_use" && m.RequestID == "toolu_subfg_bash" }); !ok || m.Parent != "toolu_agent_fg" {
		t.Fatalf("subagent's tool call = %+v, want parent toolu_agent_fg", m)
	}
	if m, ok := find(func(m session.Message) bool { return m.Kind == "tool_result" && m.RequestID == "toolu_subfg_bash" }); !ok || m.Parent != "toolu_agent_fg" {
		t.Fatalf("subagent's tool result = %+v, want parent toolu_agent_fg (inherited from its call)", m)
	}
	// …and the main conversation's is not.
	if m, ok := find(func(m session.Message) bool { return m.Kind == "tool_use" && m.RequestID == "toolu_agent_fg" }); !ok || m.Parent != "" {
		t.Fatalf("main Agent call = %+v, want no parent", m)
	}

	task := func(event, toolUseID string) (taskEvent, bool) {
		for _, m := range msgs {
			if m.Kind != "task" || m.RequestID != toolUseID {
				continue
			}
			var ev taskEvent
			_ = json.Unmarshal(m.Payload, &ev)
			if ev.Event == event {
				return ev, true
			}
		}
		return taskEvent{}, false
	}
	started, ok := task("task_started", "toolu_agent_bg")
	if !ok || started.SubagentType != "general-purpose" || started.Background == nil || !*started.Background || started.TaskType != "local_agent" {
		t.Fatalf("background agent start = %+v", started)
	}
	if done, ok := task("task_notification", "toolu_agent_fg"); !ok || done.Status != "completed" || done.Usage == nil || done.Usage.ToolUses != 1 {
		t.Fatalf("foreground agent end = %+v", done)
	}
	if progress, ok := task("task_progress", "toolu_agent_fg"); !ok || progress.LastTool != "Bash" {
		t.Fatalf("foreground agent progress = %+v", progress)
	}
	// task_updated names only the task; it is still put on the call that started it.
	if updated, ok := task("task_updated", "toolu_bash_bg"); !ok || updated.Status != "completed" {
		t.Fatalf("background shell update = %+v", updated)
	}
}

func TestConverter_RecordsWhereABackgroundTaskWritesItsOutput(t *testing.T) {
	msgs := replayFixture(t, "claude-2.1.282-agents-and-background.jsonl")
	outputs := map[string]taskEvent{}
	for _, m := range msgs {
		var ev taskEvent
		if m.Kind == "task" && json.Unmarshal(m.Payload, &ev) == nil && ev.Event == "output_file" {
			outputs[m.RequestID] = ev
		}
	}
	for call, id := range map[string]string{"toolu_bash_bg": "bjtpax2er", "toolu_agent_bg": "ac07a7160a001cde7"} {
		ev, ok := outputs[call]
		if !ok || ev.TaskID != id || filepath.Base(ev.OutputFile) != id+".output" {
			t.Fatalf("output of %s = %+v, want task %s's .output file", call, ev, id)
		}
	}
}
