package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// newProject makes a project with its main thread.
func newProject(t *testing.T, s *Store, name string) (*Project, *Thread) {
	t.Helper()
	p, main := &Project{Name: name}, &Thread{}
	require.NoError(t, s.CreateProjectWithMain(p, main, ""))
	return p, main
}

func TestChannelBindingUpsertMovesAChat(t *testing.T) {
	s := newStore(t)
	a, aMain := newProject(t, s, "A")
	b, bMain := newProject(t, s, "B")

	first := &ChannelBinding{Platform: "telegram", ChatID: "42", ChatTitle: "Me", ThreadID: aMain.ID}
	require.NoError(t, s.UpsertChannelBinding(first))
	assert.NotEmpty(t, first.ID)
	assert.Equal(t, a.ID, first.ProjectID, "the binding reports its thread's project")

	// Same chat again: it moves, keeping its id, and keeps its title when the
	// new request has none.
	moved := &ChannelBinding{Platform: "telegram", ChatID: "42", ThreadID: bMain.ID}
	require.NoError(t, s.UpsertChannelBinding(moved))
	assert.Equal(t, first.ID, moved.ID)
	assert.Equal(t, bMain.ID, moved.ThreadID)
	assert.Equal(t, b.ID, moved.ProjectID)
	assert.Equal(t, "Me", moved.ChatTitle)

	// Another platform's chat with the same id is a different chat.
	require.NoError(t, s.UpsertChannelBinding(&ChannelBinding{Platform: "feishu", ChatID: "42", ThreadID: aMain.ID}))

	all, err := s.ListChannelBindings(BindingFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 2)
	onB, err := s.ListChannelBindings(BindingFilter{ProjectID: b.ID})
	require.NoError(t, err)
	require.Len(t, onB, 1)
	assert.Equal(t, "telegram", onB[0].Platform)
	assert.Equal(t, b.ID, onB[0].ProjectID)
	onA, err := s.ListChannelBindings(BindingFilter{ThreadID: aMain.ID})
	require.NoError(t, err)
	require.Len(t, onA, 1)
	assert.Equal(t, "feishu", onA[0].Platform)

	got, err := s.GetChannelBinding("telegram", "42")
	require.NoError(t, err)
	assert.Equal(t, b.ID, got.ProjectID)
	_, err = s.GetChannelBinding("telegram", "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteChannelBinding(t *testing.T) {
	s := newStore(t)
	_, main := newProject(t, s, "P")
	b := &ChannelBinding{Platform: "telegram", ChatID: "1", ThreadID: main.ID}
	require.NoError(t, s.UpsertChannelBinding(b))
	require.NoError(t, s.DeleteChannelBinding(b.ID))
	assert.ErrorIs(t, s.DeleteChannelBinding(b.ID), ErrNotFound)
	left, err := s.ListChannelBindings(BindingFilter{})
	require.NoError(t, err)
	assert.Empty(t, left)
}

func TestThreadEventKeepsItsSource(t *testing.T) {
	s := newStore(t)
	_, main := newProject(t, s, "P")
	require.NoError(t, s.AddThreadEvent(&ThreadEvent{ThreadID: main.ID, Kind: EvUser, Content: "web"}))
	require.NoError(t, s.AddThreadEvent(&ThreadEvent{ThreadID: main.ID, Kind: EvUser, Content: "im",
		Source: SourceIM, SourcePlatform: "telegram", SourceChatID: "42"}))
	require.NoError(t, s.AddThreadEvent(&ThreadEvent{ThreadID: main.ID, Kind: EvUser, Content: "from a thread",
		Source: SourceThread, SourceRef: "t1"}))
	evs, err := s.ListThreadEvents(main.ID)
	require.NoError(t, err)
	require.Len(t, evs, 3)
	assert.Empty(t, evs[0].Source)
	assert.Equal(t, SourceIM, evs[1].Source)
	assert.Equal(t, "telegram", evs[1].SourcePlatform)
	assert.Equal(t, "42", evs[1].SourceChatID)
	assert.Equal(t, SourceThread, evs[2].Source)
	assert.Equal(t, "t1", evs[2].SourceRef)

	recent, err := s.ListRecentThreadEvents(main.ID, 2)
	require.NoError(t, err)
	require.Len(t, recent, 2)
	assert.Equal(t, "im", recent[0].Content, "the latest N, oldest first")
}

func TestMainThreadIsNotListedOrCounted(t *testing.T) {
	s := newStore(t)
	p, main := newProject(t, s, "P")
	assert.Equal(t, main.ID, p.MainThreadID)
	assert.Equal(t, AgentLLM, main.Agent)

	for i := 0; i < 2; i++ {
		require.NoError(t, s.CreateThread(&Thread{ProjectID: p.ID, ParentID: main.ID, Status: ThreadWorking}))
	}
	ts, err := s.ListThreads(p.ID)
	require.NoError(t, err)
	require.Len(t, ts, 2)
	seqs := []int{ts[0].Seq, ts[1].Seq}
	assert.ElementsMatch(t, []int{1, 2}, seqs, "numbered per project from 1; the main thread is 0")
	assert.Equal(t, AgentClaudeCode, ts[0].Agent)

	counts, err := s.ThreadCounts()
	require.NoError(t, err)
	assert.Equal(t, map[string]int{ThreadWorking: 2}, counts[p.ID])
	working, err := s.ListThreadsByStatus(ThreadWorking, ThreadIdle)
	require.NoError(t, err)
	assert.Len(t, working, 2)

	got, err := s.GetMainThread(p.ID)
	require.NoError(t, err)
	assert.Equal(t, main.ID, got.ID)

	line, err := s.Lineage(ts[0].ID)
	require.NoError(t, err)
	require.Len(t, line, 2)
	assert.Equal(t, main.ID, line[1].ID)
}

func TestOpenFolderIsOneProjectPerFolder(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	a, err := s.OpenFolder(dir, "")
	require.NoError(t, err)
	assert.Equal(t, ProjectFolder, a.Type)
	assert.Empty(t, a.MainThreadID)

	// A trailing slash and a symlink are the same folder.
	b, err := s.OpenFolder(dir+"/", "")
	require.NoError(t, err)
	assert.Equal(t, a.ID, b.ID)

	// Archived, then opened again: it is back.
	now := a.CreatedAt
	a.ArchivedAt = &now
	require.NoError(t, s.SaveProject(a))
	c, err := s.OpenFolder(dir, "")
	require.NoError(t, err)
	assert.Equal(t, a.ID, c.ID)
	assert.Nil(t, c.ArchivedAt)
}

func TestPageThreadEvents(t *testing.T) {
	s := newStore(t)
	_, main := newProject(t, s, "P")
	var ids []string
	for i := range 5 {
		e := &ThreadEvent{ThreadID: main.ID, Kind: EvUser, Content: string(rune('a' + i))}
		require.NoError(t, s.AddThreadEvent(e))
		ids = append(ids, e.ID)
	}
	contents := func(evs []ThreadEvent) string {
		out := ""
		for _, e := range evs {
			out += e.Content
		}
		return out
	}

	tail, more, err := s.PageThreadEvents(main.ID, EventPage{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, "de", contents(tail), "the latest N, oldest first")
	assert.True(t, more)

	earlier, more, err := s.PageThreadEvents(main.ID, EventPage{Limit: 2, Before: tail[0].ID})
	require.NoError(t, err)
	assert.Equal(t, "bc", contents(earlier))
	assert.True(t, more)
	first, more, err := s.PageThreadEvents(main.ID, EventPage{Limit: 2, Before: earlier[0].ID})
	require.NoError(t, err)
	assert.Equal(t, "a", contents(first))
	assert.False(t, more, "nothing older")

	_, _, err = s.PageThreadEvents(main.ID, EventPage{Before: "missing"})
	assert.ErrorIs(t, err, ErrNotFound)

	// Since: what was added or changed at or after it, an old event changed in place included.
	mark := time.Now()
	old := mustEvent(t, s, ids[0])
	old.Content = "A"
	require.NoError(t, s.UpdateThreadEvent(old))
	require.NoError(t, s.AddThreadEvent(&ThreadEvent{ThreadID: main.ID, Kind: EvUser, Content: "f"}))
	since, more, err := s.PageThreadEvents(main.ID, EventPage{Since: mark, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, "Af", contents(since), "since ignores limit")
	assert.False(t, more)
	assert.False(t, mustEvent(t, s, ids[0]).UpdatedAt.Before(mark))
}

// A subagent's steps are a line of their own: the conversation pages
// without them, and they page by their Agent call.
func TestPageThreadEventsBySubagent(t *testing.T) {
	s := newStore(t)
	_, main := newProject(t, s, "P")
	add := func(content, parent string) {
		require.NoError(t, s.AddThreadEvent(&ThreadEvent{ThreadID: main.ID, Kind: EvAssistant, Content: content, Parent: parent}))
	}
	add("ask", "")
	add("call", "")
	for i := range 5 {
		add(string(rune('1'+i)), "a1")
	}
	add("report", "")
	contents := func(evs []ThreadEvent) (out string) {
		for _, e := range evs {
			out += e.Content + ","
		}
		return
	}

	conv, more, err := s.PageThreadEvents(main.ID, EventPage{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, "call,report,", contents(conv), "the subagent's steps do not use up the page")
	assert.True(t, more)

	steps, more, err := s.PageThreadEvents(main.ID, EventPage{Parent: "a1", Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, "3,4,5,", contents(steps))
	assert.True(t, more)
	steps, more, err = s.PageThreadEvents(main.ID, EventPage{Parent: "a1", Limit: 3, Before: steps[0].ID})
	require.NoError(t, err)
	assert.Equal(t, "1,2,", contents(steps))
	assert.False(t, more)
}

// The status line totals every turn, so a page of the conversation brings
// the usage events before it.
func TestConversationPageBringsEveryUsageEvent(t *testing.T) {
	s := newStore(t)
	_, main := newProject(t, s, "P")
	add := func(kind, content string) {
		require.NoError(t, s.AddThreadEvent(&ThreadEvent{ThreadID: main.ID, Kind: kind, Content: content}))
	}
	add(EvUser, "a")
	add(EvUsage, "u1")
	add(EvUser, "b")
	add(EvUsage, "u2")
	add(EvUser, "c")
	add(EvUser, "d")
	page, more, err := s.PageThreadEvents(main.ID, EventPage{Limit: 2})
	require.NoError(t, err)
	assert.True(t, more)
	var got []string
	for _, e := range page {
		got = append(got, e.Content)
	}
	assert.Equal(t, []string{"u1", "u2", "c", "d"}, got)
}

// A subagent found in the list is reached without paging: its own events by
// call, and the conversation from its call to the loaded page in one read.
func TestLocateSubagent(t *testing.T) {
	s := newStore(t)
	_, main := newProject(t, s, "P")
	add := func(e ThreadEvent) *ThreadEvent {
		e.ThreadID = main.ID
		require.NoError(t, s.AddThreadEvent(&e))
		return &e
	}
	add(ThreadEvent{Kind: EvUser, Content: "first"})
	call := add(ThreadEvent{Kind: EvToolUse, Content: "Agent", RequestID: "a1", Payload: `{"description":"Scan","subagent_type":"Explore"}`})
	for range 3 {
		add(ThreadEvent{Kind: EvAssistant, Content: "step", Parent: "a1"})
	}
	add(ThreadEvent{Kind: EvToolResult, Content: "report", RequestID: "a1"})
	add(ThreadEvent{Kind: EvToolUse, Content: "Agent", RequestID: "a2", Payload: `{"description":"Watch"}`})
	add(ThreadEvent{Kind: EvTask, RequestID: "a2", Payload: `{"event":"task_started","background":true}`})
	add(ThreadEvent{Kind: EvToolUse, Content: "Agent", RequestID: "a3", Payload: `{"description":"Fail"}`})
	add(ThreadEvent{Kind: EvTask, RequestID: "a3", Payload: `{"event":"task_notification","status":"failed"}`})
	add(ThreadEvent{Kind: EvToolUse, Content: "Bash", RequestID: "b1"})
	add(ThreadEvent{Kind: EvAssistant, Content: "mid"})
	last := add(ThreadEvent{Kind: EvAssistant, Content: "last"})

	list, err := s.ThreadSubagents(main.ID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, Subagent{Call: "a1", EventID: call.ID, Description: "Scan", SubagentType: "Explore", Status: "completed", Steps: 3, CreatedAt: list[0].CreatedAt}, list[0])
	assert.Equal(t, "running", list[1].Status)
	assert.True(t, list[1].Background)
	assert.Equal(t, "failed", list[2].Status)

	own, _, err := s.PageThreadEvents(main.ID, EventPage{Call: "a1"})
	require.NoError(t, err)
	require.Len(t, own, 2)
	assert.Equal(t, []string{EvToolUse, EvToolResult}, []string{own[0].Kind, own[1].Kind})

	span, more, err := s.PageThreadEvents(main.ID, EventPage{From: call.ID, Before: last.ID})
	require.NoError(t, err)
	assert.True(t, more, "the first message is still before it")
	assert.Equal(t, call.ID, span[0].ID)
	assert.Equal(t, "mid", span[len(span)-1].Content)
	for _, e := range span {
		assert.Empty(t, e.Parent, "steps stay with their subagent")
	}
}
