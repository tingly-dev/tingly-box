package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// writeLines plays a captured turn's stream-json lines into the fake process.
func writeLines(t *testing.T, h *process.FakeHandle, lines []string) {
	t.Helper()
	for _, line := range lines {
		if _, err := h.WriteOutput([]byte(line + "\n")); err != nil {
			t.Error(err)
		}
	}
}

func compactEntries(store *memStore, id string) []agentboot.CompactEvent {
	var out []agentboot.CompactEvent
	for _, en := range entriesOf(store, id, KindCompact) {
		var ev agentboot.CompactEvent
		if err := json.Unmarshal(en.Payload, &ev); err == nil {
			out = append(out, ev)
		}
	}
	return out
}

// What a real Claude Code 2.1.259 process's local commands leave in the
// transcript (fixture: two ordinary turns, then /cost, /compact, /clear, each
// a plain user message). Hosts no longer send these texts themselves; the
// point is that the converter makes sense of the stream.
func TestEngine_LocalCommandStreamInTheTranscript(t *testing.T) {
	turns := fixtureTurns(t, "claude-2.1.259-slash-commands.jsonl")
	if len(turns) != 5 {
		t.Fatalf("fixture has %d turns, want 5", len(turns))
	}
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		for _, turn := range turns {
			if read(dec, in) == nil {
				return
			}
			writeLines(t, h, turn)
		}
		read(dec, in)
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, rec := p.engine(t)
	prompts := []string{"remember the word banana", "and apples?", "/cost", "/compact", "/clear"}
	for i, prompt := range prompts {
		if !e.Start("id", t.TempDir(), prompt, i > 0, nil) {
			t.Fatal("Start refused")
		}
		if end := rec.wait(t); end.Err != nil {
			t.Fatalf("turn %d (%q) ended with %v", i+1, prompt, end.Err)
		}
	}

	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	byPrompt := map[string][]Entry{}
	var cur string
	for _, en := range p.store.entries["id"] {
		if en.Role == "user" {
			cur = en.Content
			continue
		}
		byPrompt[cur] = append(byPrompt[cur], en)
	}
	text := func(prompt string) (out []string) {
		for _, en := range byPrompt[prompt] {
			if en.Role == "assistant" {
				out = append(out, en.Content)
			}
		}
		return out
	}
	kinds := func(prompt string) (out []string) {
		for _, en := range byPrompt[prompt] {
			if en.Kind != "" && en.Kind != KindSystem {
				out = append(out, en.Kind)
			}
		}
		return out
	}

	if got := text("/cost"); len(got) != 1 || !strings.HasPrefix(got[0], "Total cost:") {
		t.Errorf("/cost left %q, want the CLI's cost report", got)
	}
	// A turn that made no model call spends nothing: no usage entry.
	for _, prompt := range []string{"/cost", "/compact", "/clear"} {
		for _, k := range kinds(prompt) {
			if k == KindUsage {
				t.Errorf("%s left a usage entry", prompt)
			}
		}
	}
	// /compact: a started and a done entry, the figures, the summary; and not
	// the earlier /cost report, which the CLI emits again after a compaction.
	if got := text("/compact"); len(got) != 0 {
		t.Errorf("/compact left text %q, want none (the replayed /cost report is a duplicate)", got)
	}
	var ev []agentboot.CompactEvent
	for _, en := range byPrompt["/compact"] {
		if en.Kind == KindCompact {
			var ce agentboot.CompactEvent
			if err := json.Unmarshal(en.Payload, &ce); err != nil {
				t.Fatal(err)
			}
			ev = append(ev, ce)
		}
	}
	if len(ev) != 2 || ev[0].Phase != agentboot.CompactStarted || ev[1].Phase != agentboot.CompactDone {
		t.Fatalf("/compact entries = %+v, want started then done", ev)
	}
	if r := ev[1].Result; r.Trigger != agentboot.CompactManual || r.TokensBefore != 254 || r.TokensAfter != 1024 || !r.Reported.Summary {
		t.Errorf("compaction result = %+v", r)
	}
	// /clear: the CLI moves to a new session id and the transcript says nothing else.
	var ids []string
	for _, en := range p.store.entries["id"] {
		if en.Kind == KindSystem && strings.HasPrefix(en.Content, "claude code session ") {
			ids = append(ids, en.Content)
		}
	}
	if len(ids) != 5 || ids[3] != ids[0] || ids[4] == ids[0] {
		t.Errorf("session ids per turn = %q, want /compact to keep the id and /clear to change it", ids)
	}
	if got := text("/clear"); len(got) != 0 {
		t.Errorf("/clear left %q, want nothing", got)
	}
}

// Compact sends the backend's compaction request through the session's
// resident process and returns what it reported.
func TestEngine_CompactThroughTheResidentProcess(t *testing.T) {
	turns := fixtureTurns(t, "claude-2.1.259-slash-commands.jsonl")
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		for _, i := range []int{0, 3} {
			if read(dec, in) == nil {
				return
			}
			writeLines(t, h, turns[i])
		}
		read(dec, in)
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, rec := p.engine(t)
	if !e.Start("id", t.TempDir(), "remember the word banana", false, nil) {
		t.Fatal("Start refused")
	}
	rec.wait(t)
	next(t, p.in)

	res, err := e.Compact(context.Background(), "id", t.TempDir(), "keep banana", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trigger != agentboot.CompactManual || res.TokensBefore != 254 || res.TokensAfter != 1024 || res.DurationMS != 27 || !res.Reported.Tokens {
		t.Errorf("result = %+v", res)
	}
	if res.Reported.Usage || res.Usage != nil {
		t.Errorf("the CLI reports no tokens for the summary call, got %+v", res.Usage)
	}
	sent := next(t, p.in)
	if msg, _ := sent["message"].(map[string]any); msg["content"] != "/compact keep banana" {
		t.Errorf("the process was sent %v, want the compaction request with the instructions", sent)
	}
	if end := rec.wait(t); end.Err != nil {
		t.Errorf("the compaction turn ended with %v", end.Err)
	}
	for _, en := range p.store.entries["id"] {
		if en.Role == "user" && strings.HasPrefix(en.Content, "/compact") {
			t.Errorf("the compaction request was recorded as the user's message: %q", en.Content)
		}
	}
	if got := compactEntries(p.store, "id"); len(got) != 2 {
		t.Errorf("recorded %d compaction entries, want started and done", len(got))
	}
	if entriesOf(p.store, "id", KindError) != nil {
		t.Error("a clean compaction recorded an error")
	}
}

// A refusal is an error, and its text is the compact entry's, not a reply.
func TestEngine_CompactRefused(t *testing.T) {
	const sid = `"session_id":"s1"`
	refused := []string{
		`{"type":"system","subtype":"init",` + sid + `,"model":"m"}`,
		`{"type":"system","subtype":"status","status":"compacting",` + sid + `}`,
		`{"type":"system","subtype":"status","status":null,"compact_result":"failed","compact_error":"Not enough messages to compact.",` + sid + `}`,
		`{"type":"assistant","uuid":"u1","message":{"id":"x","type":"message","role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"Not enough messages to compact."}],"usage":{"input_tokens":0,"output_tokens":0}},` + sid + `}`,
		`{"type":"result","subtype":"success","is_error":false,"num_turns":0,"result":"Not enough messages to compact.",` + sid + `,"usage":{"input_tokens":0,"output_tokens":0},"modelUsage":{}}`,
	}
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		if read(dec, in) == nil {
			return
		}
		writeLines(t, h, refused)
		read(dec, in)
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, rec := p.engine(t)
	_, err := e.Compact(context.Background(), "id", t.TempDir(), "", nil)
	var failed *agentboot.CompactFailedError
	if !errors.As(err, &failed) || failed.Reason != "Not enough messages to compact." {
		t.Fatalf("err = %v, want a CompactFailedError with the CLI's reason", err)
	}
	rec.wait(t)
	got := compactEntries(p.store, "id")
	if len(got) != 2 || got[1].Phase != agentboot.CompactFailed {
		t.Errorf("entries = %+v, want started then failed", got)
	}
	for _, en := range p.store.entries["id"] {
		if en.Role == "assistant" {
			t.Errorf("the refusal text was recorded as a reply: %q", en.Content)
		}
	}
}

// A backend with no compaction says so, before anything is started.
func TestEngine_CompactUnsupportedBackend(t *testing.T) {
	e, store, _ := newScripted(t, func(*enginetest.Turn) error { return nil }, nil)
	_, err := e.Compact(context.Background(), "id", t.TempDir(), "", nil)
	if !errors.Is(err, agentboot.ErrCompactUnsupported) {
		t.Fatalf("err = %v, want ErrCompactUnsupported", err)
	}
	if n := store.count("id"); n != 0 {
		t.Errorf("recorded %d entries for a refused compaction", n)
	}
}

func TestEngine_CompactRefusesWhileATurnRuns(t *testing.T) {
	release := make(chan struct{})
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		read(dec, in)
		<-release
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, _ := p.engine(t)
	defer close(release)
	if !e.Start("id", t.TempDir(), "work", false, nil) {
		t.Fatal("Start refused")
	}
	if _, err := e.Compact(context.Background(), "id", t.TempDir(), "", nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
}

// The context size is asked of the live process.
func TestEngine_ContextUsage(t *testing.T) {
	turns := fixtureTurns(t, "claude-2.1.259-slash-commands.jsonl")
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		read(dec, in)
		writeLines(t, h, turns[0])
		req := read(dec, in)
		send(t, h, map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": req["request_id"],
			"response": map[string]any{"totalTokens": 32006, "maxTokens": 1000000, "autoCompactThreshold": 967000, "isAutoCompactEnabled": true}}})
		read(dec, in)
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, rec := p.engine(t)
	if _, err := e.ContextUsage(context.Background(), "id"); !errors.Is(err, agentboot.ErrNoProcess) {
		t.Fatalf("before any turn: err = %v, want ErrNoProcess", err)
	}
	if !e.Start("id", t.TempDir(), "hello", false, nil) {
		t.Fatal("Start refused")
	}
	rec.wait(t)
	next(t, p.in)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := e.ContextUsage(ctx, "id")
	if err != nil {
		t.Fatal(err)
	}
	if want := (agentboot.ContextUsage{Used: 32006, Max: 1000000, AutoCompactAt: 967000}); got != want {
		t.Errorf("usage = %+v, want %+v", got, want)
	}
}

// The agent turn that started in the instant before the compaction request
// reached the session: the host's OnTurnEnd sees NotSent (and re-queues), so
// Compact says so with its own error, not ErrBusy, which means nothing was
// started and the caller must re-queue itself.
func TestEngine_CompactReportsAnAgentTurnThatCameFirst(t *testing.T) {
	turns := fixtureTurns(t, "claude-2.1.259-slash-commands.jsonl")
	release := make(chan struct{})
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		read(dec, in)
		writeLines(t, h, turns[0])
		read(dec, in) // the turn the test starts behind the engine's back
		<-release
		writeLines(t, h, turns[0])
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, rec := p.engine(t)
	if !e.Start("id", t.TempDir(), "hello", false, nil) {
		t.Fatal("Start refused")
	}
	rec.wait(t)
	res, ok := e.residentFor("id")
	if !ok {
		t.Fatal("no resident process")
	}
	if err := res.c.Session().Send(context.Background(), "background"); err != nil {
		t.Fatal(err)
	}
	next(t, p.in)
	next(t, p.in)

	_, err := e.Compact(context.Background(), "id", t.TempDir(), "", nil)
	close(release)
	if !errors.Is(err, ErrNotSent) {
		t.Fatalf("err = %v, want ErrNotSent", err)
	}
	if end := rec.wait(t); !end.NotSent {
		t.Errorf("the host's OnTurnEnd saw %+v, want NotSent", end)
	}
}
