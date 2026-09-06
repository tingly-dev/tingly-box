package managedagent

import (
	"context"
	"testing"
)

func TestEventLog_SeqSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	log, err := NewEventLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, txt := range []string{"a", "b"} {
		if err := log.AppendEvent(ctx, &Event{SessionID: "s/1", Kind: EventUserMessage, Text: txt}); err != nil {
			t.Fatal(err)
		}
	}
	// A fresh EventLog over the same dir must continue the sequence.
	log2, _ := NewEventLog(dir)
	e := &Event{SessionID: "s/1", Kind: EventSystem, Text: "c"}
	if err := log2.AppendEvent(ctx, e); err != nil {
		t.Fatal(err)
	}
	if e.Seq != 3 {
		t.Fatalf("seq = %d, want 3", e.Seq)
	}
	got, err := log2.ListEvents(ctx, "s/1", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Seq != 2 || got[0].Text != "b" {
		t.Fatalf("ListEvents(after=1, limit=1) = %+v", got)
	}
	if got, _ := log2.ListEvents(ctx, "missing", 0, 0); len(got) != 0 {
		t.Fatalf("missing session should be empty, got %+v", got)
	}
}
