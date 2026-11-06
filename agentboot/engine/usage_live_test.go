package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// fixtureTurns splits a captured stream-json run into its turns, each ending
// at its result line.
func fixtureTurns(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var turns [][]string
	var cur []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		cur = append(cur, sc.Text())
		var head struct{ Type string }
		if json.Unmarshal(sc.Bytes(), &head) == nil && head.Type == "result" {
			turns = append(turns, cur)
			cur = nil
		}
	}
	return turns
}

func entriesOf(store *memStore, id, kind string) []Entry {
	store.mu.Lock()
	defer store.mu.Unlock()
	var out []Entry
	for _, e := range store.entries[id] {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// Captured from Claude Code 2.1.259 running as one resident process for two
// turns (turn one: a Bash call and a closing answer; turn two: one answer).
// The fixture is replayed through the real transport and a resident process,
// because the usage numbers only exist if the result line reaches the
// converter: the process layer used to swallow it as a turn boundary, so no
// turn ever recorded usage in real use.
func TestEngine_ResidentProcessRecordsEachTurnsUsage(t *testing.T) {
	turns := fixtureTurns(t, "claude-2.1.259-two-turns-usage.jsonl")
	if len(turns) != 2 {
		t.Fatalf("fixture has %d turns, want 2", len(turns))
	}
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		for _, turn := range turns {
			if read(dec, in) == nil {
				return
			}
			for _, line := range turn {
				if _, err := h.WriteOutput([]byte(line + "\n")); err != nil {
					t.Error(err)
				}
			}
		}
		read(dec, in)
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, rec := p.engine(t)
	for i, prompt := range []string{"first", "second"} {
		if !e.Start("id", t.TempDir(), prompt, i > 0, nil) {
			t.Fatal("Start refused")
		}
		if end := rec.wait(t); end.Err != nil {
			t.Fatalf("turn %d ended with %v", i+1, end.Err)
		}
	}

	usage := entriesOf(p.store, "id", KindUsage)
	if len(usage) != 2 {
		t.Fatalf("recorded %d usage entries, want one per turn", len(usage))
	}
	want := []TurnUsage{
		// Two API calls: the Bash call and the answer. Output is the result's
		// figure: the assistant events carry only the message_start count.
		{Model: "cc[1m]", InputTokens: 12, OutputTokens: 80, CacheReadTokens: 60000, CacheWriteTokens: 2400, ContextTokens: 32006, ContextWindow: 1000000},
		// One call. The process-wide totals of the first turn must not be added in.
		{Model: "cc[1m]", InputTokens: 6, OutputTokens: 40, CacheReadTokens: 30000, CacheWriteTokens: 1200, ContextTokens: 32006, ContextWindow: 1000000},
	}
	for i, entry := range usage {
		var got TurnUsage
		if err := json.Unmarshal(entry.Payload, &got); err != nil {
			t.Fatal(err)
		}
		got.DurationMS = 0
		if got != want[i] {
			t.Errorf("turn %d usage = %+v\nwant         %+v", i+1, got, want[i])
		}
	}
}

// The one-shot path (no pool) hands the result to the converter too.
func TestEngine_OneShotTurnRecordsUsage(t *testing.T) {
	turns := fixtureTurns(t, "claude-2.1.259-two-turns-usage.jsonl")
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		read(dec, in)
		for _, line := range turns[1] {
			if _, err := h.WriteOutput([]byte(line + "\n")); err != nil {
				t.Error(err)
			}
		}
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, rec := p.engineWith(t, nil)
	if !e.Start("id", t.TempDir(), "only", false, nil) {
		t.Fatal("Start refused")
	}
	if end := rec.wait(t); end.Err != nil {
		t.Fatalf("turn ended with %v", end.Err)
	}
	usage := entriesOf(p.store, "id", KindUsage)
	if len(usage) != 1 {
		t.Fatalf("recorded %d usage entries, want 1", len(usage))
	}
	var got TurnUsage
	if err := json.Unmarshal(usage[0].Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.OutputTokens != 40 || got.CacheReadTokens != 30000 || got.ContextWindow != 1000000 {
		t.Errorf("usage = %+v", got)
	}
}

// A failed result is recorded once: as the converter's error entry, whether
// the host's message started the turn or Claude did by itself (a background
// task finished). The unsolicited case also had the engine report the turn's
// error, which would repeat it; the error text differs when the result carries
// an errors list, so the check cannot be by text. The failing result lines are
// hand-built in the shape Claude Code reports an API error.
func TestEngine_FailedResultIsRecordedOnce(t *testing.T) {
	const tail = `"session_id":"f80f40db-376d-4ab6-b5d4-a783b39f8b21","usage":{"input_tokens":6,"output_tokens":1,"cache_read_input_tokens":30000,"cache_creation_input_tokens":0},"modelUsage":{}}`
	for name, failed := range map[string]string{
		"result text only":    `{"type":"result","subtype":"success","is_error":true,"result":"API Error: 529 overloaded",` + tail,
		"with an errors list": `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"API Error: 529 overloaded","errors":["overloaded_error"],` + tail,
	} {
		t.Run(name, func(t *testing.T) {
			turns := fixtureTurns(t, "claude-2.1.259-two-turns-usage.jsonl")
			p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
				if read(dec, in) == nil {
					return
				}
				for _, line := range turns[1] {
					if _, err := h.WriteOutput([]byte(line + "\n")); err != nil {
						t.Error(err)
					}
				}
				// Now idle: the turn that opens with an init is Claude's own.
				time.Sleep(200 * time.Millisecond)
				for _, line := range append(append([]string(nil), turns[1][:len(turns[1])-1]...), failed) {
					if _, err := h.WriteOutput([]byte(line + "\n")); err != nil {
						t.Error(err)
					}
				}
				read(dec, in)
				h.FinishOutput()
				h.SignalExit(nil)
			})
			e, rec := p.engine(t)
			if !e.Start("id", t.TempDir(), "first", false, nil) {
				t.Fatal("Start refused")
			}
			rec.wait(t)
			end := rec.wait(t)
			if !end.Unsolicited || end.Err == nil {
				t.Fatalf("second turn end = %+v, want Claude's own turn, failed", end)
			}
			if errs := entriesOf(p.store, "id", KindError); len(errs) != 1 {
				t.Fatalf("recorded %d error entries for one failed turn, want 1: %+v", len(errs), errs)
			}
		})
	}
}

// A failure an earlier turn left must not hide the next turn's error, even
// when Claude's own turn finds the session already claimed (the host's message
// won the race and the engine adopts it).
func TestEngine_BeginUnsolicitedTurnClearsAStaleFailedResult(t *testing.T) {
	e := New(Config{Store: newMemStore()})
	t.Cleanup(func() { e.Shutdown(context.Background()) })
	conv := newConverter()
	conv.failedResult = true
	e.residents["s"] = &resident{conv: conv}
	e.runs["s"] = &run{cancel: func() {}, done: make(chan struct{})}
	e.beginUnsolicitedTurn("s")
	if conv.failedResult {
		t.Fatal("the flag a previous failed result set survived into the next turn")
	}
}
