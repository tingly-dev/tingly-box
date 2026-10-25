package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestFolderThreadRunsInItsFolder(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Looked around.", 7, 3)
		return nil
	})
	folder := t.TempDir()
	s, err := f.startFolder(folder, FolderThreadSpec{Task: "what is here?", PermissionMode: "plan"})
	require.NoError(t, err)
	folderProject, err := f.store.FindFolder(folder)
	require.NoError(t, err)
	assert.Equal(t, folderProject.ID, s.ProjectID)
	assert.Empty(t, s.ParentID, "a folder thread has no main thread above it")
	assert.Equal(t, 1, s.Seq)
	assert.Equal(t, db.AgentClaudeCode, s.Agent)
	assert.Equal(t, folder, s.Cwd)

	got := waitStatus(t, f.store, s.ID, db.ThreadIdle)
	assert.EqualValues(t, 7, got.InputTokens)
	assert.Equal(t, "", got.Model, "a session keeps its owner's pick, not the model that answered")

	spec := f.agent.Launches()[0]
	assert.Equal(t, "default-model", spec.Model)
	assert.Equal(t, folder, spec.ProjectPath)
	assert.Equal(t, "plan", spec.PermissionMode)
	assert.Empty(t, spec.AppendSystemPrompt, "no project instructions")
	assert.Equal(t, f.threads.layout.SessionSettingsFile(s.ID), spec.SettingsPath, "a session reads its own uploads")
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

	// The folder is a project of its own, apart from the project's threads.
	assert.Equal(t, db.ProjectFolder, folderProject.Type)
	assert.Equal(t, folder, folderProject.Path)
	inFolder, _ := f.store.ListThreads(folderProject.ID)
	assert.Len(t, inFolder, 1)
	threads, _ := f.store.ListThreads(f.project.ID)
	assert.Empty(t, threads)

	// A second thread in the same folder is the same project, numbered next.
	again, err := f.startFolder(folder+"/", FolderThreadSpec{Task: "more"})
	require.NoError(t, err)
	assert.Equal(t, folderProject.ID, again.ProjectID)
	assert.Equal(t, 2, again.Seq)
	// Let it finish: a turn still writing into the folder races its cleanup.
	waitStatus(t, f.store, again.ID, db.ThreadIdle)
}

func TestSessionNeedsAFolderAndAMode(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	file := filepath.Join(t.TempDir(), "f.txt")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	for _, folder := range []string{"", "relative/dir", file, filepath.Join(t.TempDir(), "missing")} {
		_, err := f.startFolder(folder, FolderThreadSpec{Task: "x"})
		assert.Error(t, err, folder)
	}
	_, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "x", PermissionMode: "yolo"})
	assert.ErrorIs(t, err, ErrBadPermission)
	_, err = f.startFolder(t.TempDir(), FolderThreadSpec{Task: "  "})
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
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "list"})
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
	_, err = f.threads.Send(context.Background(), s.ID, "again", Source{}, nil)
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
	_, err = f.threads.Send(context.Background(), s.ID, "late", Source{}, nil)
	assert.ErrorIs(t, err, ErrResolved)
	reopened, err := f.threads.Reopen(s.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, reopened.Status)
}

func TestSessionIsNeverAutoArchived(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "hi"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	f.threads.AutoResolve(-time.Hour)
	got, _ := f.store.GetThread(s.ID)
	assert.Equal(t, db.ThreadIdle, got.Status)
}

func TestFolderIsArchivedNotRemoved(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		_, err := turn.Approve("r", "Bash", map[string]any{"command": "ls"})
		return err
	})
	svc := &Service{Store: f.store, Threads: f.threads, Hub: f.threads.hub}
	folder := t.TempDir()
	s, err := f.startFolder(folder, FolderThreadSpec{Task: "go"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadWaiting)
	p, err := f.store.GetProject(s.ProjectID)
	require.NoError(t, err)

	require.NoError(t, svc.SetArchived(p, true))
	waitStatus(t, f.store, s.ID, db.ThreadIdle) // archiving stops the folder's threads; they stay
	_, err = f.threads.Send(context.Background(), s.ID, "late", Source{}, nil)
	assert.ErrorIs(t, err, ErrArchived)
	again, err := f.startFolder(folder, FolderThreadSpec{Task: "again"})
	require.NoError(t, err, "opening the folder again brings it back")
	waitStatus(t, f.store, again.ID, db.ThreadWaiting)
	f.threads.Stop(again.ID)
	waitStatus(t, f.store, again.ID, db.ThreadIdle)
	back, _ := f.store.GetProject(p.ID)
	assert.Nil(t, back.ArchivedAt)
}

func TestProjectsAreCreatedWithAMainThreadAndFoldersWithout(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	svc := &Service{Store: f.store, Layout: f.threads.layout, Threads: f.threads, Hub: f.threads.hub}
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Fresh"})
	require.NoError(t, err)
	assert.Equal(t, db.ProjectProject, p.Type)
	main, err := f.store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	assert.Equal(t, db.AgentLLM, main.Agent)
	assert.Equal(t, filepath.Join(svc.Layout.Root, p.ID), main.Cwd)

	folder := t.TempDir()
	fp, err := svc.Create(context.Background(), NewProjectSpec{Type: db.ProjectFolder, Path: folder})
	require.NoError(t, err)
	assert.Equal(t, db.ProjectFolder, fp.Type)
	assert.Empty(t, fp.MainThreadID)
	again, err := svc.Create(context.Background(), NewProjectSpec{Type: db.ProjectFolder, Path: folder + "/"})
	require.NoError(t, err)
	assert.Equal(t, fp.ID, again.ID)
	_, err = svc.Create(context.Background(), NewProjectSpec{Type: db.ProjectFolder, Path: filepath.Join(folder, "missing")})
	assert.Error(t, err)

	// Only a project's main thread starts threads in it, and a main thread is
	// not an agent session to send to directly.
	_, err = f.threads.Start(context.Background(), fp, StartSpec{Task: "x"})
	assert.ErrorIs(t, err, ErrNotProject)
	_, err = f.threads.Send(context.Background(), main.ID, "x", Source{}, nil)
	assert.ErrorIs(t, err, ErrMainThread)
}

func TestEveryThreadKeepsSubagentStepsAndTasks(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "go"})
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
	n, parent = count(th.ID, db.EvToolUse)
	assert.Equal(t, 1, n, "a project thread keeps the steps too: they are read when the subagent is opened")
	assert.Equal(t, "agent-1", parent)
	n, _ = count(th.ID, db.EvTask)
	assert.Equal(t, 1, n, "a subagent's card reads its state from them")
}

// profiles is a host with launch profiles: "work" routes through its own
// settings file and runs only the tiers it lists.
type profiles struct {
	staticEnv
	// settings is the profile's settings file, when the test wrote one.
	settings string
}

func (p profiles) LaunchEnv(_ context.Context, profile string) ([]string, string, error) {
	switch profile {
	case "":
		return []string{"ANTHROPIC_BASE_URL=http://gw/main"}, "", nil
	case "work":
		return nil, p.settings, nil
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
	_, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "x", Profile: "work"})
	assert.ErrorIs(t, err, ErrNoProfiles)

	folder := t.TempDir()
	s, err := f.startFolder(folder, FolderThreadSpec{Task: "x"})
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
	work := filepath.Join(t.TempDir(), "work.json")
	require.NoError(t, os.WriteFile(work, []byte(`{"env":{"X":"1"},"permissions":{"additionalDirectories":["/data"]}}`), 0o644))
	f.threads.env = profiles{settings: work}
	ctx := context.Background()

	_, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "x", Profile: "nope"})
	assert.Error(t, err)
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "x", Profile: "work"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	spec := f.agent.Launches()[0]
	assert.Equal(t, f.threads.layout.SessionSettingsFile(s.ID), spec.SettingsPath, "the profile's settings plus the session's uploads")
	merged, err := os.ReadFile(spec.SettingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(merged), `"X":"1"`, "the profile's own settings are kept")
	assert.Contains(t, string(merged), `"/data"`)
	assert.Contains(t, string(merged), f.threads.layout.LibraryDir(s.ProjectID))
	assert.Contains(t, string(merged), `"autoMemoryDirectory":"`+f.threads.layout.MemoryDir(s.ProjectID)+`"`, "memory is the project's, not the cwd's")
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
	_, err = f.threads.Send(ctx, s.ID, "again", Source{}, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(f.agent.Launches()) == 2 }, 5*time.Second, 10*time.Millisecond)
	spec = f.agent.Launches()[1]
	assert.Contains(t, spec.Env, "ANTHROPIC_BASE_URL=http://gw/main")
	assert.Equal(t, f.threads.layout.SessionSettingsFile(s.ID), spec.SettingsPath)

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
	assert.Equal(t, "cd '"+s.Cwd+"' && tb cc --resume "+s.SessionID, cmd)
}

func TestTerminalCommandQuotesForTheShell(t *testing.T) {
	argv := []string{"/opt/tingly box/tb", "profile", "it's", "--resume", "sid-1"}
	assert.Equal(t, `cd '/w/it'\''s' && '/opt/tingly box/tb' profile 'it'\''s' --resume sid-1`, terminalCommand("linux", "/w/it's", argv))
	assert.Equal(t, `Set-Location -LiteralPath 'C:\w\it''s'; & '/opt/tingly box/tb' 'profile' 'it''s' '--resume' 'sid-1'`, terminalCommand("windows", `C:\w\it's`, argv))
}

func TestSessionFirstMessageCarriesAttachments(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Saw it.", 1, 1)
		return nil
	})
	folder := t.TempDir()
	p, err := f.store.OpenFolder(folder, "")
	require.NoError(t, err)
	a, err := f.threads.layout.SaveAttachment(p.ID, "log.txt", strings.NewReader("boom"), 0)
	require.NoError(t, err)
	atts, err := f.threads.layout.ResolveAttachments(p.ID, []string{a.Path})
	require.NoError(t, err)

	s, err := f.startFolder(folder, FolderThreadSpec{Attachments: atts})
	require.NoError(t, err, "a file alone is a first message")
	assert.Equal(t, p.ID, s.ProjectID)
	assert.Equal(t, "log.txt", s.Title)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)

	events, err := f.store.ListThreadEvents(s.ID)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Equal(t, atts, ParseAttachments(events[0].Payload))

	// The file stays in the folder's library, where every thread of the folder reads it.
	got, err := f.threads.layout.AttachmentPath(p, s, a.Path)
	require.NoError(t, err)
	assert.FileExists(t, got)
	raw, err := os.ReadFile(f.threads.layout.SessionSettingsFile(s.ID))
	require.NoError(t, err)
	assert.Contains(t, string(raw), f.threads.layout.LibraryDir(p.ID))
}

func TestFolderThreadsShareUploads(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	folder := t.TempDir()
	s1, err := f.startFolder(folder, FolderThreadSpec{Task: "a"})
	require.NoError(t, err)
	s2, err := f.startFolder(folder, FolderThreadSpec{Task: "b"})
	require.NoError(t, err)
	require.Equal(t, s1.ProjectID, s2.ProjectID)
	assert.Equal(t, UploadOwner(nil, s1), UploadOwner(nil, s2), "one upload area per folder")
}

// A thread from before uploads were shared per folder still opens its files.
func TestAttachmentPathFallsBackToLegacyArea(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "a"})
	require.NoError(t, err)
	p, err := f.store.GetProject(s.ProjectID)
	require.NoError(t, err)
	a, err := f.threads.layout.SaveAttachment(SessionOwner(s.ID), "old.txt", strings.NewReader("x"), 0)
	require.NoError(t, err)
	got, err := f.threads.layout.AttachmentPath(p, s, a.Path)
	require.NoError(t, err)
	assert.FileExists(t, got)
}

// A thread event carries its project's counts after the change, so the
// project list updates that row from the stream instead of re-fetching.
func TestThreadEventCarriesProjectCounts(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	ch, cancel := f.threads.hub.Subscribe(AllProjects)
	defer cancel()
	folder := t.TempDir()
	s, err := f.startFolder(folder, FolderThreadSpec{Task: "x"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)

	var last Event
	for {
		select {
		case b := <-ch:
			var ev Event
			require.NoError(t, json.Unmarshal(b, &ev))
			if ev.Type != EventThread {
				continue
			}
			require.NotNil(t, ev.Threads, "every thread event has counts")
			last = ev
			continue
		case <-time.After(200 * time.Millisecond):
		}
		break
	}
	assert.Equal(t, s.ProjectID, last.ProjectID)
	assert.Equal(t, db.ThreadCounts{Idle: 1}, *last.Threads, "the last event's counts are the settled state")
}

func TestSessionWithBackgroundTasksIsNotDone(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("started a server", 1, 1)
		return nil
	})
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "serve"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	require.Eventually(t, func() bool { return !f.threads.Running(s.ID) }, 5*time.Second, 10*time.Millisecond)

	changed := func(tasks ...map[string]string) engine.Entry {
		payload, _ := json.Marshal(map[string]any{"event": "background_tasks_changed", "tasks": tasks})
		return engine.Entry{Kind: engine.KindTask, Payload: payload}
	}
	store := threadStore{f.threads}
	sid := f.threads.sessionOf(s.ID)

	store.Append(sid, changed(map[string]string{"task_id": "b1", "task_type": "local_bash", "description": "dev server"}))
	waitStatus(t, f.store, s.ID, db.ThreadWorking)

	store.Append(sid, changed())
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
}
