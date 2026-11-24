package project

import (
	"encoding/json"
	"testing"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The all-projects stream is the sidebar's: it carries thread, project and
// coordinator frames, and — since notifications watch it — a decision card
// the user must pick, but no other transcript entry.
func TestHubAllProjectsSeesDecisionsOnly(t *testing.T) {
	hub := NewHub()
	read := func(key string) <-chan []byte {
		ch, cancel := hub.Subscribe(key)
		t.Cleanup(cancel)
		return ch
	}
	all, project := read(AllProjects), read("p1")

	hub.Publish(Event{Type: EventThreadEvent, ProjectID: "p1", Data: &db.ThreadEvent{ID: "d1", Kind: db.EvDecision}})
	hub.Publish(Event{Type: EventThreadEvent, ProjectID: "p1", Data: &db.ThreadEvent{ID: "t1", Kind: db.EvToolResult}})
	hub.Publish(Event{Type: EventThread, ProjectID: "p1", Data: &db.Thread{}})

	kind := func(ch <-chan []byte) string {
		var ev Event
		if err := json.Unmarshal(<-ch, &ev); err != nil {
			t.Fatal(err)
		}
		// Unmarshalled, Data is a map; the kind sits next to the other fields.
		if m, ok := ev.Data.(map[string]any); ok {
			if k, ok := m["kind"].(string); ok {
				return ev.Type + ":" + k
			}
		}
		return ev.Type
	}
	if got := kind(all); got != "thread_event:"+db.EvDecision {
		t.Fatalf("all-projects stream first frame %q, want the decision card", got)
	}
	if got := kind(all); got != "thread" {
		t.Fatalf("all-projects stream second frame %q, want the thread frame (no tool_result)", got)
	}
	if extra := len(all); extra != 0 {
		t.Fatalf("all-projects stream got %d more frames, want none", extra)
	}
	for i, want := range []string{"thread_event:" + db.EvDecision, "thread_event:" + db.EvToolResult, "thread"} {
		if got := kind(project); got != want {
			t.Fatalf("project stream frame %d = %q, want %q", i, got, want)
		}
	}
}
