package project

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestParseCommand(t *testing.T) {
	for text, want := range map[string]command{
		"/compact":                    {cmdCompact, "compact", ""},
		"/compact keep the API notes": {cmdCompact, "compact", "keep the API notes"},
		"  /compact  \n":              {cmdCompact, "compact", ""},
		"/compact\nline two":          {cmdCompact, "compact", "line two"},
		"/clear":                      {cmdUnsupported, "clear", ""},
		"/model sonnet":               {cmdUnsupported, "model", ""},
		"/usage":                      {cmdUnsupported, "usage", ""},
		"/Compact":                    {},
		"/etc/hosts is broken":        {},
		"/usage-report failed":        {},
		"/ look":                      {},
		"//compact":                   {},
		"please /compact":             {},
		"compact":                     {},
		"":                            {},
	} {
		assert.Equal(t, want, parseCommand(text), "%q", text)
	}
}

func TestLiteralPromptKeepsAgentFromRunningACommand(t *testing.T) {
	assert.Equal(t, " /etc/hosts is broken", literalPrompt("/etc/hosts is broken"))
	assert.Equal(t, "fix /etc/hosts", literalPrompt("fix /etc/hosts"))
	assert.Equal(t, " /usage-report failed", literalPrompt("/usage-report failed"))
}

// commandAgent answers a compaction like the CLI does and records every
// prompt the agent was sent.
type commandAgent struct {
	mu      sync.Mutex
	prompts []string
	resumed []bool
	refuse  string
	gate    chan struct{} // when set, a "work" turn waits for it
}

func (c *commandAgent) script(turn *enginetest.Turn) error {
	c.mu.Lock()
	c.prompts = append(c.prompts, turn.Prompt)
	c.resumed = append(c.resumed, turn.Opts.Resume)
	refuse, gate := c.refuse, c.gate
	c.mu.Unlock()
	switch {
	case strings.HasPrefix(turn.Prompt, "/compact"):
		if refuse != "" {
			turn.CompactRefused(refuse)
			return nil
		}
		turn.Compacted(agentboot.CompactResult{Trigger: agentboot.CompactManual, TokensBefore: 120000, TokensAfter: 18000, DurationMS: 4200, Summary: "the summary",
			Reported: agentboot.CompactReported{Tokens: true, Summary: true}})
	case turn.Prompt == "work" && gate != nil:
		<-gate
		turn.Say("worked", 1, 1)
	default:
		turn.Say("ok", 1, 1)
	}
	return nil
}

func (c *commandAgent) sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.prompts...)
}

func commandFixture(t *testing.T, ca *commandAgent) (*testFixture, *Service, *fakeSink, *db.Thread) {
	t.Helper()
	f, svc, sink := channelFixtureWith(t, ca.script)
	th := folderChat(t, f, svc, FolderThreadSpec{})
	return f, svc, sink, th
}

func eventsOf(t *testing.T, f *testFixture, threadID, kind string) []db.ThreadEvent {
	t.Helper()
	all, err := f.store.ListThreadEvents(threadID)
	require.NoError(t, err)
	var out []db.ThreadEvent
	for _, e := range all {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func commandCard(t *testing.T, f *testFixture, threadID string) commandPayload {
	t.Helper()
	var pl commandPayload
	require.Eventually(t, func() bool {
		cards := eventsOf(t, f, threadID, db.EvCommand)
		if len(cards) != 1 {
			return false
		}
		pl = commandPayload{}
		_ = json.Unmarshal([]byte(cards[0].Payload), &pl)
		return pl.State == "ok" || pl.State == "failed"
	}, 5*time.Second, 10*time.Millisecond)
	return pl
}

func TestCompactRunsAsTheThreadsTurnAndLeavesOneCard(t *testing.T) {
	ca := &commandAgent{}
	f, svc, sink, th := commandFixture(t, ca)

	_, err := svc.SendWith(context.Background(), th.ID, "/compact keep the API notes", nil, Source{}, SendNow)
	require.NoError(t, err)
	waitQuiet(t, f, svc, th.ID)

	pl := commandCard(t, f, th.ID)
	assert.Equal(t, commandPayload{Name: "compact", State: "ok", Trigger: "manual", TokensBefore: 120000, TokensAfter: 18000, TokensReported: true, DurationMS: 4200, Summary: "the summary"}, pl)
	assert.Equal(t, []string{"go", "/compact keep the API notes"}, ca.sent(), "the agent was asked to compact, with the instructions")
	assert.True(t, ca.resumed[1], "compaction continues the thread's session")

	users := eventsOf(t, f, th.ID, db.EvUser)
	require.Len(t, users, 2)
	assert.Equal(t, "/compact keep the API notes", users[1].Content, "the user's line stays in the transcript")
	assert.Len(t, eventsOf(t, f, th.ID, db.EvAssistant), 1, "the compaction left no reply of its own (only the first turn's)")
	assert.Empty(t, eventsOf(t, f, th.ID, db.EvError))

	got, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, got.Status)
	assert.Equal(t, int64(18000), got.ContextTokens, "no live process to ask: the compaction's own figure")

	assert.Contains(t, sink.next(t).text, "Compacted: 120k → 18k tokens.", "the chat bound to the thread hears the outcome")
}

func TestCompactIsNotReportedToTheCoordinator(t *testing.T) {
	ca := &commandAgent{}
	f, svc, _, th := commandFixture(t, ca)
	reports := make(chan Report, 10)
	f.threads.OnReport = func(r Report) { reports <- r }
	// A project worker thread, whose turns report to the coordinator.
	w, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Work", Task: "do it", Origin: "coordinator"})
	require.NoError(t, err)
	select {
	case r := <-reports:
		require.Equal(t, ReportFinished, r.Kind)
	case <-time.After(5 * time.Second):
		t.Fatal("the worker's own turn did not report")
	}
	waitQuiet(t, f, svc, w.ID)

	_, err = svc.SendWith(context.Background(), w.ID, "/compact", nil, Source{}, SendNow)
	require.NoError(t, err)
	waitQuiet(t, f, svc, w.ID)
	commandCard(t, f, w.ID)
	select {
	case r := <-reports:
		t.Fatalf("the compaction was reported to the coordinator: %q %q", r.Kind, r.Text)
	case <-time.After(200 * time.Millisecond):
	}
	assert.Len(t, eventsOf(t, f, w.ID, db.EvReport), 1, "no report event for the compaction, only for the worker's own turn")
	_ = th
}

func TestCompactRefusedIsACardNotAFailure(t *testing.T) {
	ca := &commandAgent{refuse: "Not enough messages to compact."}
	f, svc, sink, th := commandFixture(t, ca)

	_, err := svc.Send(context.Background(), th.ID, "/compact", nil, Source{})
	require.NoError(t, err)
	waitQuiet(t, f, svc, th.ID)

	pl := commandCard(t, f, th.ID)
	assert.Equal(t, "failed", pl.State)
	assert.Equal(t, "Not enough messages to compact.", pl.Reason)
	got, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, got.Status, "a refused compaction is not a failed thread")
	assert.Contains(t, sink.next(t).text, "Could not compact: Not enough messages to compact.")
}

func TestUnsupportedCommandsAreRefusedNotSent(t *testing.T) {
	ca := &commandAgent{}
	f, svc, _, th := commandFixture(t, ca)

	for _, text := range []string{"/clear", "/model opus", "/usage"} {
		_, err := svc.SendWith(context.Background(), th.ID, text, nil, Source{}, SendNow)
		var unsupported *ErrCommandUnsupported
		require.ErrorAs(t, err, &unsupported, text)
		_, err = svc.Send(context.Background(), th.ID, text, nil, Source{Kind: db.SourceIM, Platform: "telegram", ChatID: "fold"})
		require.ErrorAs(t, err, &unsupported, text)
	}
	assert.Equal(t, []string{"go"}, ca.sent(), "nothing reached the agent")
	assert.Len(t, eventsOf(t, f, th.ID, db.EvUser), 1, "and nothing was recorded")
}

func TestTextThatOnlyStartsWithASlashIsSentAsText(t *testing.T) {
	ca := &commandAgent{}
	f, svc, _, th := commandFixture(t, ca)

	_, err := svc.SendWith(context.Background(), th.ID, "/etc/hosts is broken", nil, Source{}, SendNow)
	require.NoError(t, err)
	waitQuiet(t, f, svc, th.ID)
	assert.Equal(t, []string{"go", " /etc/hosts is broken"}, ca.sent(), "a leading space keeps the agent from reading a command")
	users := eventsOf(t, f, th.ID, db.EvUser)
	assert.Equal(t, "/etc/hosts is broken", users[1].Content, "the transcript keeps what was typed")
}

func TestOnlyAPersonsMessageIsACommand(t *testing.T) {
	ca := &commandAgent{}
	f, svc, _, th := commandFixture(t, ca)

	for _, src := range []Source{fromThread("other"), {Kind: db.SourceSystem}} {
		_, err := svc.Send(context.Background(), th.ID, "/compact", nil, src)
		require.NoError(t, err)
		waitQuiet(t, f, svc, th.ID)
	}
	sent := ca.sent()
	require.Len(t, sent, 3)
	assert.True(t, strings.HasPrefix(sent[1], coordinatorNote) && strings.HasSuffix(sent[1], "\n\n/compact"), "a relayed message keeps its coordinator note: %q", sent[1])
	assert.Equal(t, " /compact", sent[2], "system text is sent as text")
	assert.Empty(t, eventsOf(t, f, th.ID, db.EvCommand))
}

func TestCompactWaitsForTheRunningTurn(t *testing.T) {
	ca := &commandAgent{gate: make(chan struct{})}
	f, svc, _, th := commandFixture(t, ca)

	_, err := svc.SendWith(context.Background(), th.ID, "work", nil, Source{}, SendNow)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(ca.sent()) == 2 }, 5*time.Second, 5*time.Millisecond)

	_, err = svc.SendWith(context.Background(), th.ID, "/compact", nil, Source{}, SendNow)
	require.NoError(t, err)
	chips := eventsOf(t, f, th.ID, db.EvQueued)
	require.Len(t, chips, 1, "while a turn runs the command waits as a chip, it is never injected into the turn")
	assert.Equal(t, "/compact", chips[0].Content)
	assert.Equal(t, []string{"go", "work"}, ca.sent())

	close(ca.gate)
	waitQuiet(t, f, svc, th.ID)
	commandCard(t, f, th.ID)
	assert.Equal(t, []string{"go", "work", "/compact"}, ca.sent())
}

func TestCompactFromAChat(t *testing.T) {
	ca := &commandAgent{}
	f, svc, sink, th := commandFixture(t, ca)

	handled, reply, err := svc.ChannelCommand("telegram", "fold", ChatCompact, "keep the API notes")
	require.NoError(t, err)
	require.True(t, handled)
	assert.Contains(t, reply, "Compacting [")
	waitQuiet(t, f, svc, th.ID)
	assert.Equal(t, "ok", commandCard(t, f, th.ID).State)
	assert.Equal(t, []string{"go", "/compact keep the API notes"}, ca.sent())
	assert.Contains(t, sink.next(t).text, "Compacted: 120k → 18k tokens.")
	users := eventsOf(t, f, th.ID, db.EvUser)
	assert.Equal(t, db.SourceIM, users[1].Source)

	// A chat waiting to start a new session has nothing to compact.
	_, reply, err = svc.ChannelCommand("telegram", "fold", ChatNew, "")
	require.NoError(t, err)
	_, reply, err = svc.ChannelCommand("telegram", "fold", ChatCompact, "")
	require.NoError(t, err)
	assert.Contains(t, reply, "Nothing to compact")
}

func TestCompactOfTheProjectConversationIsNotAvailableYet(t *testing.T) {
	ca := &commandAgent{}
	f, svc, _, _ := commandFixture(t, ca)
	_, err := svc.Send(context.Background(), f.project.MainThreadID, "/compact", nil, Source{})
	assert.ErrorIs(t, err, ErrCompactUnavailable)
}
