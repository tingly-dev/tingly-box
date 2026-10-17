package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestSessionRunsInFolderWithoutProject(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Looked around.", 7, 3)
		return nil
	})
	folder := t.TempDir()
	s, err := f.threads.StartSession(SessionSpec{Folder: folder, Task: "what is here?", PermissionMode: "plan"})
	require.NoError(t, err)
	assert.Equal(t, "", s.ProjectID)
	assert.Equal(t, folder, s.Worktree)

	got := waitStatus(t, f.store, s.ID, db.ThreadIdle)
	assert.EqualValues(t, 7, got.InputTokens)
	assert.Equal(t, "", got.Model, "a session keeps its owner's pick, not the model that answered")

	spec := f.agent.Launches()[0]
	assert.Equal(t, "default-model", spec.Model)
	assert.Equal(t, folder, spec.ProjectPath)
	assert.Equal(t, "plan", spec.PermissionMode)
	assert.Empty(t, spec.AppendSystemPrompt, "no project instructions")
	assert.Empty(t, spec.SettingsPath)
	assert.Contains(t, spec.Env, "ANTHROPIC_BASE_URL=http://gw")

	select {
	case r := <-f.reports:
		t.Fatalf("a session has no coordinator to report to: %+v", r)
	case <-time.After(100 * time.Millisecond):
	}
	events, _ := f.store.ListThreadEvents(s.ID)
	for _, e := range events {
		assert.NotEqual(t, db.EvReport, e.Kind)
	}

	// The folder is kept, and sessions stay apart from project threads.
	saved, _ := f.store.ListWorkdirs()
	require.Len(t, saved, 1)
	assert.Equal(t, folder, saved[0].Path)
	sessions, _ := f.store.ListSessions("")
	assert.Len(t, sessions, 1)
	threads, _ := f.store.ListThreads(f.project.ID)
	assert.Empty(t, threads)
	_, err = f.store.GetSession("nope")
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestSessionNeedsAFolderAndAMode(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	file := filepath.Join(t.TempDir(), "f.txt")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	for _, folder := range []string{"", "relative/dir", file, filepath.Join(t.TempDir(), "missing")} {
		_, err := f.threads.StartSession(SessionSpec{Folder: folder, Task: "x"})
		assert.Error(t, err, folder)
	}
	_, err := f.threads.StartSession(SessionSpec{Folder: t.TempDir(), Task: "x", PermissionMode: "yolo"})
	assert.ErrorIs(t, err, ErrBadPermission)
	_, err = f.threads.StartSession(SessionSpec{Folder: t.TempDir(), Task: "  "})
	assert.Error(t, err)
}

func TestSessionApprovalsMessagesAndModeChange(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		resp, err := turn.Approve("req-1", "Bash", map[string]any{"command": "ls"})
		if err != nil {
			return err
		}
		turn.Say(map[bool]string{true: "listed", false: "denied"}[resp.Approved], 1, 1)
		return nil
	})
	s, err := f.threads.StartSession(SessionSpec{Folder: t.TempDir(), Task: "list"})
	require.NoError(t, err)
	got := waitStatus(t, f.store, s.ID, db.ThreadWaiting)
	assert.Equal(t, "Approve: Run `ls`", got.WaitReason)

	require.NoError(t, f.threads.Respond(s.ID, "req-1", Answer{Allow: true}))
	waitStatus(t, f.store, s.ID, db.ThreadIdle)

	// A message and a new mode for the next turn; the engine resumes.
	_, err = f.threads.SetPermissionMode(s.ID, "acceptEdits")
	require.NoError(t, err)
	_, err = f.threads.SetPermissionMode(s.ID, "yolo")
	assert.ErrorIs(t, err, ErrBadPermission)
	_, err = f.threads.Send(context.Background(), s.ID, "again", "user")
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadWaiting)
	require.NoError(t, f.threads.Respond(s.ID, "req-1", Answer{Allow: false}))
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	launches := f.agent.Launches()
	require.Len(t, launches, 2)
	assert.True(t, launches[1].Resume)
	assert.Equal(t, "acceptEdits", launches[1].PermissionMode)

	// Archive and reopen.
	archived, err := f.threads.Resolve(s.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadResolved, archived.Status)
	_, err = f.threads.Send(context.Background(), s.ID, "late", "user")
	assert.ErrorIs(t, err, ErrResolved)
	reopened, err := f.threads.Reopen(s.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, reopened.Status)
}

func TestSessionIsNeverAutoArchived(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	s, err := f.threads.StartSession(SessionSpec{Folder: t.TempDir(), Task: "hi"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	f.threads.AutoResolve(-time.Hour)
	got, _ := f.store.GetThread(s.ID)
	assert.Equal(t, db.ThreadIdle, got.Status)
}

func TestWorkdirsListSavedAndUsed(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		_, err := turn.Approve("r", "Bash", map[string]any{"command": "ls"})
		return err
	})
	svc := &Service{Store: f.store, Threads: f.threads}
	saved, used := t.TempDir(), t.TempDir()
	require.NoError(t, f.store.SaveWorkdir(saved))
	s, err := f.threads.StartSession(SessionSpec{Folder: used, Task: "go"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadWaiting)
	require.NoError(t, f.store.RemoveWorkdir(used))

	list, err := svc.Workdirs()
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, Workdir{Path: saved, Saved: true}, list[0])
	assert.Equal(t, Workdir{Path: used, Waiting: 1, Sessions: 1}, list[1], "a folder with a session is listed even when not kept")
	f.threads.Stop(s.ID)
}

func TestSessionKeepsSubagentsAndTasksProjectThreadsDoNot(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	s, err := f.threads.StartSession(SessionSpec{Folder: t.TempDir(), Task: "go"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "go", Repo: "none"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	for _, id := range []string{s.ID, th.ID} {
		st := threadStore{f.threads}
		sid := f.threads.sessionOf(id)
		st.Append(sid, engine.Entry{Kind: engine.KindToolUse, Content: "Bash", RequestID: "call-1", Parent: "agent-1", Payload: []byte(`{}`)})
		st.Append(sid, engine.Entry{Kind: engine.KindTask, RequestID: "agent-1", Payload: []byte(`{"event":"task_started"}`)})
	}
	count := func(id, kind string) (n int, parent string) {
		events, _ := f.store.ListThreadEvents(id)
		for _, e := range events {
			if e.Kind == kind {
				n++
				parent = e.Parent
			}
		}
		return
	}
	n, parent := count(s.ID, db.EvToolUse)
	assert.Equal(t, 1, n)
	assert.Equal(t, "agent-1", parent)
	n, _ = count(s.ID, db.EvTask)
	assert.Equal(t, 1, n)
	n, _ = count(th.ID, db.EvToolUse)
	assert.Zero(t, n, "a project thread still shows only the main conversation")
	n, _ = count(th.ID, db.EvTask)
	assert.Zero(t, n)
}

// profiles is a host with launch profiles: "work" routes through its own
// settings file and runs only the tiers it lists.
type profiles struct{ staticEnv }

func (profiles) LaunchEnv(_ context.Context, profile string) ([]string, string, error) {
	switch profile {
	case "":
		return []string{"ANTHROPIC_BASE_URL=http://gw/main"}, "", nil
	case "work":
		return nil, "/settings/work.json", nil
	}
	return nil, "", errors.New("no such profile")
}

func (profiles) CheckLaunch(_ context.Context, profile, model string) error {
	if profile != "" && profile != "work" {
		return errors.New("no such profile")
	}
	if profile == "work" && model == "opus" {
		return errors.New("work routes every tier to one model")
	}
	return nil
}

func (profiles) HandoffLaunch(profile string) []string {
	if profile == "" {
		return []string{"tb", "cc"}
	}
	return []string{"tb", "profile", profile}
}

func TestSessionWithoutLauncherHasNoProfiles(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	_, err := f.threads.StartSession(SessionSpec{Folder: t.TempDir(), Task: "x", Profile: "work"})
	assert.ErrorIs(t, err, ErrNoProfiles)

	folder := t.TempDir()
	s, err := f.threads.StartSession(SessionSpec{Folder: folder, Task: "x"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	_, err = f.threads.SetProfile(context.Background(), s.ID, "work")
	assert.ErrorIs(t, err, ErrNoProfiles)
	got, err := f.threads.SetModel(context.Background(), s.ID, "haiku")
	require.NoError(t, err)
	assert.Equal(t, "haiku", got.Model)

	cmd, err := f.threads.Handoff(s.ID)
	require.NoError(t, err)
	assert.Equal(t, "cd '"+folder+"' && claude --resume "+s.SessionID+" --model haiku", cmd)
}

func TestSessionLaunchesUnderItsProfile(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	f.threads.env = profiles{}
	ctx := context.Background()

	_, err := f.threads.StartSession(SessionSpec{Folder: t.TempDir(), Task: "x", Profile: "nope"})
	assert.Error(t, err)
	s, err := f.threads.StartSession(SessionSpec{Folder: t.TempDir(), Task: "x", Profile: "work"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	spec := f.agent.Launches()[0]
	assert.Equal(t, "/settings/work.json", spec.SettingsPath)
	assert.Empty(t, spec.Env)
	assert.Empty(t, spec.Model, "the profile decides when no model is picked")

	// Back to the default routing with a tier; a profile that cannot run
	// the tier clears it.
	_, err = f.threads.SetProfile(ctx, s.ID, "")
	require.NoError(t, err)
	got, err := f.threads.SetModel(ctx, s.ID, "opus")
	require.NoError(t, err)
	assert.Equal(t, "opus", got.Model)
	got, err = f.threads.SetProfile(ctx, s.ID, "work")
	require.NoError(t, err)
	assert.Equal(t, "work", got.Profile)
	assert.Empty(t, got.Model)
	_, err = f.threads.SetModel(ctx, s.ID, "opus")
	assert.Error(t, err)

	_, err = f.threads.SetProfile(ctx, s.ID, "")
	require.NoError(t, err)
	_, err = f.threads.Send(ctx, s.ID, "again", "user")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(f.agent.Launches()) == 2 }, 5*time.Second, 10*time.Millisecond)
	spec = f.agent.Launches()[1]
	assert.Contains(t, spec.Env, "ANTHROPIC_BASE_URL=http://gw/main")
	assert.Empty(t, spec.SettingsPath)

	events, _ := f.store.ListThreadEvents(s.ID)
	var notes []string
	for _, e := range events {
		if e.Kind == db.EvSystem {
			notes = append(notes, e.Content)
		}
	}
	assert.Contains(t, notes, "profile: work")
	assert.Contains(t, notes, "model: opus")

	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	cmd, err := f.threads.Handoff(s.ID)
	require.NoError(t, err)
	assert.Equal(t, "cd '"+s.Worktree+"' && tb cc --resume "+s.SessionID, cmd)
}

func TestTerminalCommandQuotesForTheShell(t *testing.T) {
	argv := []string{"/opt/tingly box/tb", "profile", "it's", "--resume", "sid-1"}
	assert.Equal(t, `cd '/w/it'\''s' && '/opt/tingly box/tb' profile 'it'\''s' --resume sid-1`, terminalCommand("linux", "/w/it's", argv))
	assert.Equal(t, `Set-Location -LiteralPath 'C:\w\it''s'; & '/opt/tingly box/tb' 'profile' 'it''s' '--resume' 'sid-1'`, terminalCommand("windows", `C:\w\it's`, argv))
}
