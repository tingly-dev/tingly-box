package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// Characterization of what a user message that is a Claude Code slash command
// produces today (design: .design/slash-commands.md). The fixture is one
// resident Claude Code 2.1.259 process: two ordinary turns, then /cost,
// /compact and /clear, each sent as the plain user message "/name".
func TestEngine_SlashCommandsToday(t *testing.T) {
	turns := fixtureTurns(t, "claude-2.1.259-slash-commands.jsonl")
	if len(turns) != 5 {
		t.Fatalf("fixture has %d turns, want 5", len(turns))
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
	prompts := []string{"remember the word banana", "and apples?", "/cost", "/compact", "/clear"}
	for i, prompt := range prompts {
		if !e.Start("id", t.TempDir(), prompt, i > 0, nil) {
			t.Fatal("Start refused")
		}
		if end := rec.wait(t); end.Err != nil {
			t.Fatalf("turn %d (%q) ended with %v", i+1, prompt, end.Err)
		}
	}

	// What each turn left in the transcript, by the user entry that opened it.
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

	// /cost: the CLI's own report arrives as an assistant message.
	if got := text("/cost"); len(got) != 1 || !strings.HasPrefix(got[0], "Total cost:") {
		t.Errorf("/cost left %q, want the CLI's cost report", got)
	}
	// /compact: nothing says it compacted (compact_boundary and the status
	// events are dropped); the only text is the earlier /cost report, which
	// the process emits again after a compaction.
	if got := text("/compact"); len(got) != 1 || got[0] != text("/cost")[0] {
		t.Errorf("/compact left %q, want only the stale /cost report", got)
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
