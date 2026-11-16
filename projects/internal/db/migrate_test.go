package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The models as they were before the unified Thread model, to build an old
// database the migration has to carry over.

type oldProject struct {
	ID                   string `gorm:"primaryKey;size:36"`
	Name                 string `gorm:"size:200;not null"`
	Goal                 string `gorm:"type:text"`
	Instructions         string `gorm:"type:text"`
	Repos                []Repo `gorm:"serializer:json;type:text"`
	CoordinatorModel     string `gorm:"size:200"`
	ThreadModel          string `gorm:"size:200"`
	ThreadPermissionMode string `gorm:"size:40"`
	Paused               bool
	Starred              bool
	ArchivedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (oldProject) TableName() string { return "projects" }

type oldMessage struct {
	ID             string `gorm:"primaryKey;size:36"`
	ProjectID      string `gorm:"size:36;index;not null"`
	Role           string `gorm:"size:20;not null"`
	Kind           string `gorm:"size:30;not null"`
	Content        string `gorm:"type:text"`
	Payload        string `gorm:"type:text"`
	Source         string `gorm:"size:10"`
	SourcePlatform string `gorm:"size:40"`
	SourceChatID   string `gorm:"size:200"`
	CreatedAt      time.Time
}

func (oldMessage) TableName() string { return "project_messages" }

type oldThread struct {
	ID             string `gorm:"primaryKey;size:36"`
	ProjectID      string `gorm:"size:36;index;not null"`
	Title          string `gorm:"size:300"`
	Task           string `gorm:"type:text"`
	Status         string `gorm:"size:20;index"`
	WaitReason     string `gorm:"type:text"`
	Origin         string `gorm:"size:20"`
	RepoPath       string `gorm:"type:text"`
	BaseBranch     string `gorm:"size:200"`
	Branch         string `gorm:"size:200"`
	Worktree       string `gorm:"type:text"`
	SessionID      string `gorm:"size:64"`
	Model          string `gorm:"size:200"`
	PermissionMode string `gorm:"size:40"`
	Turns          int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (oldThread) TableName() string { return "threads" }

type oldEvent struct {
	ID        string `gorm:"primaryKey;size:36"`
	ThreadID  string `gorm:"size:36;index;not null"`
	Kind      string `gorm:"size:20;not null"`
	Content   string `gorm:"type:text"`
	Payload   string `gorm:"type:text"`
	From      string `gorm:"size:20"`
	RequestID string `gorm:"size:100"`
	Parent    string `gorm:"size:100"`
	CreatedAt time.Time
}

func (oldEvent) TableName() string { return "thread_events" }

type oldWorkdir struct {
	Path       string `gorm:"primaryKey;size:1000"`
	CreatedAt  time.Time
	LastUsedAt time.Time
}

func (oldWorkdir) TableName() string { return "workdirs" }

type oldBinding struct {
	ID            string `gorm:"primaryKey;size:36"`
	Platform      string `gorm:"size:40;not null;uniqueIndex:idx_channel_chat"`
	ChatID        string `gorm:"size:200;not null;uniqueIndex:idx_channel_chat"`
	ChatTitle     string `gorm:"size:300"`
	TargetKind    string `gorm:"size:20;not null"`
	ProjectID     string `gorm:"size:36;index"`
	WorkdirID     *string
	FocusThreadID *string `gorm:"size:36"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (oldBinding) TableName() string { return "channel_bindings" }

func rawOpen(t *testing.T, path string) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	return g
}

func closeRaw(t *testing.T, g *gorm.DB) {
	t.Helper()
	sqlDB, err := g.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func rawCount(t *testing.T, g *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, g.Raw(query, args...).Scan(&n).Error)
	return n
}

// oldFixture is what the old database held, for assertions.
type oldFixture struct {
	path             string
	folderA, folderB string // real folders
	folderC          string // kept in the sidebar, no sessions
	threads          int64
	events           int64
	messages         int64
	sessionIDs       map[string]string // thread id -> session id
}

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

// buildOldDB writes a database as the previous version left it: two projects
// (one with a coordinator conversation, three threads, a running and a waiting
// one), folder sessions (one folder spelled three ways), saved workdirs, and
// IM bindings (one focused on a thread, one dangling).
func buildOldDB(t *testing.T) *oldFixture {
	t.Helper()
	root := t.TempDir()
	f := &oldFixture{path: filepath.Join(root, "projects.db"), sessionIDs: map[string]string{}}
	f.folderA, f.folderB, f.folderC = filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "c")
	for _, d := range []string{f.folderA, f.folderB, f.folderC} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	linkA := filepath.Join(root, "link-to-a")
	if err := os.Symlink(f.folderA, linkA); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}

	g := rawOpen(t, f.path)
	require.NoError(t, g.AutoMigrate(&oldProject{}, &oldMessage{}, &oldThread{}, &oldEvent{}, &Conversation{}, &Message{}, &Setting{}, &oldWorkdir{}, &oldBinding{}))

	create := func(v any) { require.NoError(t, g.Create(v).Error) }
	create(&oldProject{ID: "p1", Name: "Alpha", Goal: "g", ThreadPermissionMode: "acceptEdits", CreatedAt: at(0), UpdatedAt: at(50)})
	create(&oldProject{ID: "p2", Name: "Beta", CreatedAt: at(1), UpdatedAt: at(2)})

	msgs := []oldMessage{
		{ID: "m1", ProjectID: "p1", Role: "user", Kind: "text", Content: "hello", Payload: `[{"path":"uploads/x"}]`, CreatedAt: at(10)},
		{ID: "m2", ProjectID: "p1", Role: "assistant", Kind: "text", Content: "hi there", CreatedAt: at(11)},
		{ID: "m3", ProjectID: "p1", Role: "assistant", Kind: "thread_started", Payload: `{"thread_ids":["t1"]}`, CreatedAt: at(12)},
		{ID: "m4", ProjectID: "p1", Role: "system", Kind: "thread_update", Content: "done", Payload: `{"thread_id":"t1","kind":"finished"}`, CreatedAt: at(13)},
		{ID: "m5", ProjectID: "p1", Role: "assistant", Kind: "suggestions", Payload: `{"suggestions":[{"title":"x"}]}`, CreatedAt: at(14)},
		{ID: "m6", ProjectID: "p1", Role: "user", Kind: "text", Content: "from chat", Source: "im", SourcePlatform: "telegram", SourceChatID: "1", CreatedAt: at(15)},
		{ID: "m7", ProjectID: "p1", Role: "assistant", Kind: "error", Content: "boom", CreatedAt: at(16)},
	}
	for i := range msgs {
		create(&msgs[i])
	}
	f.messages = int64(len(msgs))

	threads := []oldThread{
		{ID: "t1", ProjectID: "p1", Title: "one", Status: "working", Origin: "coordinator", Worktree: "/data/p1/worktrees/t1", SessionID: "s1", CreatedAt: at(20), UpdatedAt: at(30)},
		{ID: "t2", ProjectID: "p1", Title: "two", Status: "waiting", WaitReason: "Approve: x", Origin: "suggestion", Worktree: "/data/p1/worktrees/t2", SessionID: "s2", CreatedAt: at(21), UpdatedAt: at(31)},
		{ID: "t3", ProjectID: "p1", Title: "three", Status: "idle", Origin: "user", Worktree: "/data/p1/worktrees/t3", SessionID: "s3", CreatedAt: at(22), UpdatedAt: at(32)},
		{ID: "t4", ProjectID: "p2", Title: "four", Status: "resolved", Worktree: "/data/p2/worktrees/t4", SessionID: "s4", CreatedAt: at(23), UpdatedAt: at(33)},
		// Folder sessions, with no project: folder A spelled three ways.
		{ID: "f1", Title: "folder one", Status: "idle", Worktree: f.folderA, SessionID: "sf1", CreatedAt: at(40), UpdatedAt: at(41)},
		{ID: "f2", Title: "folder two", Status: "waiting", Worktree: f.folderA + "/", SessionID: "sf2", CreatedAt: at(42), UpdatedAt: at(43)},
		{ID: "f3", Title: "folder three", Status: "working", Worktree: linkA, SessionID: "sf3", CreatedAt: at(44), UpdatedAt: at(45)},
		{ID: "f4", Title: "folder b", Status: "idle", Worktree: f.folderB, SessionID: "sf4", CreatedAt: at(46), UpdatedAt: at(47)},
	}
	for i := range threads {
		create(&threads[i])
		f.sessionIDs[threads[i].ID] = threads[i].SessionID
	}
	f.threads = int64(len(threads))

	events := []oldEvent{
		{ID: "e1", ThreadID: "t1", Kind: "user", Content: "do it", From: "coordinator", CreatedAt: at(20)},
		{ID: "e2", ThreadID: "t1", Kind: "assistant", Content: "ok", CreatedAt: at(21)},
		{ID: "e3", ThreadID: "t2", Kind: "user", Content: "mine", From: "user", CreatedAt: at(22)},
		{ID: "e4", ThreadID: "t2", Kind: "approval", Content: "Run ls", Payload: `{"state":"pending"}`, CreatedAt: at(23)},
		{ID: "e5", ThreadID: "f1", Kind: "user", Content: "hello folder", CreatedAt: at(40)},
	}
	for i := range events {
		create(&events[i])
	}
	f.events = int64(len(events))

	create(&oldWorkdir{Path: f.folderA, CreatedAt: at(-5), LastUsedAt: at(44)})
	create(&oldWorkdir{Path: f.folderC, CreatedAt: at(-4), LastUsedAt: at(-3)})

	focus := "t2"
	create(&oldBinding{ID: "b1", Platform: "telegram", ChatID: "1", ChatTitle: "Me", TargetKind: "project", ProjectID: "p1", FocusThreadID: &focus, CreatedAt: at(60), UpdatedAt: at(60)})
	create(&oldBinding{ID: "b2", Platform: "telegram", ChatID: "2", TargetKind: "project", ProjectID: "p2", CreatedAt: at(61), UpdatedAt: at(61)})
	create(&oldBinding{ID: "b3", Platform: "feishu", ChatID: "9", TargetKind: "project", ProjectID: "gone", CreatedAt: at(62), UpdatedAt: at(62)})
	closeRaw(t, g)
	return f
}

func TestMigrationCarriesAnOldDatabaseToThreads(t *testing.T) {
	f := buildOldDB(t)
	s, err := Open(f.path, WithProjectsDir("/data"))
	require.NoError(t, err)
	defer s.Close()

	projects, err := s.ListProjects("", false)
	require.NoError(t, err)
	byName := map[string]Project{}
	for _, p := range projects {
		byName[p.Name] = p
	}
	require.Len(t, projects, 5, "2 projects and 3 folders (a, b, c)")

	// Every project has its main thread; threads were added for exactly them.
	var total int64
	require.NoError(t, s.db.Model(&Thread{}).Count(&total).Error)
	assert.Equal(t, f.threads+2, total, "old threads + one main thread per project")
	alpha, beta := byName["Alpha"], byName["Beta"]
	assert.Equal(t, ProjectProject, alpha.Type)
	for _, p := range []Project{alpha, beta} {
		main, err := s.GetThread(p.MainThreadID)
		require.NoError(t, err)
		assert.Equal(t, AgentLLM, main.Agent)
		assert.Equal(t, p.ID, main.ProjectID)
		assert.Equal(t, filepath.Join("/data", p.ID), main.Cwd)
		assert.Empty(t, main.ParentID)
	}

	// Threads: parent, agent, seq in creation order, cwd renamed and untouched,
	// session_id and ownership preserved.
	for id, parent := range map[string]string{"t1": alpha.MainThreadID, "t2": alpha.MainThreadID, "t3": alpha.MainThreadID, "t4": beta.MainThreadID} {
		th, err := s.GetThread(id)
		require.NoError(t, err)
		assert.Equal(t, parent, th.ParentID, id)
		assert.Equal(t, AgentClaudeCode, th.Agent, id)
		assert.Equal(t, f.sessionIDs[id], th.SessionID, id)
	}
	for i, id := range []string{"t1", "t2", "t3"} {
		th, _ := s.GetThread(id)
		assert.Equal(t, "p1", th.ProjectID)
		assert.Equal(t, i+1, th.Seq)
	}
	t4, _ := s.GetThread("t4")
	assert.Equal(t, 1, t4.Seq, "numbering is per project")
	assert.Equal(t, "/data/p1/worktrees/t1", mustThread(t, s, "t1").Cwd, "worktree directories on disk are not moved")
	assert.Equal(t, ThreadWorking, mustThread(t, s, "t1").Status)
	assert.Equal(t, "Approve: x", mustThread(t, s, "t2").WaitReason)

	// Folder sessions: one folder project per distinct folder, however spelled.
	folderA, err := s.FindFolder(f.folderA)
	require.NoError(t, err)
	assert.Equal(t, ProjectFolder, folderA.Type)
	assert.Equal(t, "a", folderA.Name)
	assert.Empty(t, folderA.MainThreadID)
	folderB, err := s.FindFolder(f.folderB)
	require.NoError(t, err)
	folderC, err := s.FindFolder(f.folderC)
	require.NoError(t, err)
	assert.NotEqual(t, folderA.ID, folderB.ID)
	for i, id := range []string{"f1", "f2", "f3"} {
		th := mustThread(t, s, id)
		assert.Equal(t, folderA.ID, th.ProjectID, id)
		assert.Empty(t, th.ParentID)
		assert.Equal(t, AgentClaudeCode, th.Agent)
		assert.Equal(t, i+1, th.Seq)
	}
	assert.Equal(t, folderB.ID, mustThread(t, s, "f4").ProjectID)
	assert.Equal(t, f.folderA+"/", mustThread(t, s, "f2").Cwd, "a folder thread keeps its cwd: Claude Code sessions are keyed by it")
	counts, err := s.ThreadCounts()
	require.NoError(t, err)
	assert.Equal(t, 3, counts[folderA.ID][ThreadIdle]+counts[folderA.ID][ThreadWaiting]+counts[folderA.ID][ThreadWorking])
	assert.Empty(t, counts[folderC.ID], "a kept folder with no sessions is still a project")
	assert.Equal(t, 3, counts["p1"][ThreadWorking]+counts["p1"][ThreadWaiting]+counts["p1"][ThreadIdle], "the main thread is not counted")

	// The coordinator conversation became the main thread's events, with ids
	// and times kept, and each kind in its place.
	evs, err := s.ListThreadEvents(alpha.MainThreadID)
	require.NoError(t, err)
	require.Len(t, evs, int(f.messages), "all of p1's messages")
	wantKinds := []string{EvUser, EvAssistant, EvThreadStarted, EvThreadUpdate, EvSuggestions, EvUser, EvError}
	for i, e := range evs {
		assert.Equal(t, "m"+string(rune('1'+i)), e.ID)
		assert.Equal(t, wantKinds[i], e.Kind, e.ID)
		assert.True(t, e.CreatedAt.Equal(at(10+i)), e.ID)
		assert.True(t, e.UpdatedAt.Equal(e.CreatedAt), "an old event was last written when created: %s", e.ID)
	}
	assert.Equal(t, RoleUser, EventRole(evs[0].Kind))
	assert.Equal(t, RoleSystem, EventRole(evs[3].Kind))
	assert.Equal(t, "hello", evs[0].Content)
	assert.Equal(t, `[{"path":"uploads/x"}]`, evs[0].Payload)
	assert.Equal(t, `{"suggestions":[{"title":"x"}]}`, evs[4].Payload)
	assert.Equal(t, SourceIM, evs[5].Source)
	assert.Equal(t, "telegram", evs[5].SourcePlatform)
	assert.Equal(t, "1", evs[5].SourceChatID)
	assert.Empty(t, evs[0].Source)
	var allEvents int64
	require.NoError(t, s.db.Model(&ThreadEvent{}).Count(&allEvents).Error)
	assert.Equal(t, f.events+f.messages, allEvents, "events are the sum of both old tables")

	// Who sent a thread's user events: the coordinator became its main thread.
	e1 := mustEvent(t, s, "e1")
	assert.Equal(t, SourceThread, e1.Source)
	assert.Equal(t, alpha.MainThreadID, e1.SourceRef)
	e3 := mustEvent(t, s, "e3")
	assert.Empty(t, e3.Source, "a message from the user came from the web")
	assert.Contains(t, mustEvent(t, s, "e4").Payload, `"state":"pending"`)

	// Bindings point at a thread: the focused one, else the main thread; one
	// whose project is gone is dropped.
	bs, err := s.ListChannelBindings(BindingFilter{})
	require.NoError(t, err)
	require.Len(t, bs, 2)
	b1, err := s.GetChannelBinding("telegram", "1")
	require.NoError(t, err)
	assert.Equal(t, "t2", b1.ThreadID)
	assert.Equal(t, "p1", b1.ProjectID)
	assert.Equal(t, "Me", b1.ChatTitle)
	assert.Equal(t, "b1", b1.ID)
	b2, err := s.GetChannelBinding("telegram", "2")
	require.NoError(t, err)
	assert.Equal(t, beta.MainThreadID, b2.ThreadID)
	_, err = s.GetChannelBinding("feishu", "9")
	assert.ErrorIs(t, err, ErrNotFound)

	// The old tables are kept, untouched, for a version.
	assert.EqualValues(t, f.messages, rawCount(t, s.db, "SELECT COUNT(*) FROM project_messages"))
	assert.EqualValues(t, 2, rawCount(t, s.db, "SELECT COUNT(*) FROM workdirs"))

	// The unique path index is what stops a duplicate folder.
	assert.Error(t, s.db.Create(&Project{ID: "dup", Name: "dup", Type: ProjectFolder, Path: folderA.Path}).Error)
}

func mustThread(t *testing.T, s *Store, id string) *Thread {
	t.Helper()
	th, err := s.GetThread(id)
	require.NoError(t, err)
	return th
}

func mustEvent(t *testing.T, s *Store, id string) *ThreadEvent {
	t.Helper()
	e, err := s.GetThreadEvent(id)
	require.NoError(t, err)
	return e
}

type shape struct {
	Projects, Threads, Events, Bindings int64
	Mains                               []string
}

func shapeOf(t *testing.T, s *Store) shape {
	t.Helper()
	var sh shape
	sh.Projects = rawCount(t, s.db, "SELECT COUNT(*) FROM projects")
	sh.Threads = rawCount(t, s.db, "SELECT COUNT(*) FROM threads")
	sh.Events = rawCount(t, s.db, "SELECT COUNT(*) FROM thread_events")
	sh.Bindings = rawCount(t, s.db, "SELECT COUNT(*) FROM channel_bindings")
	require.NoError(t, s.db.Raw("SELECT main_thread_id FROM projects WHERE type = 'project' ORDER BY id").Scan(&sh.Mains).Error)
	return sh
}

func TestMigrationIsIdempotent(t *testing.T) {
	f := buildOldDB(t)
	s, err := Open(f.path)
	require.NoError(t, err)
	first := shapeOf(t, s)
	require.NoError(t, s.Close())

	// A second start finds the marker and leaves everything as it is.
	s, err = Open(f.path)
	require.NoError(t, err)
	assert.Equal(t, first, shapeOf(t, s))

	// Even with the marker gone, running the steps again changes nothing: no
	// second folder project, main thread, copy of a message or binding.
	require.NoError(t, s.db.Exec("DELETE FROM settings WHERE key = ?", migrationMarker).Error)
	require.NoError(t, s.Close())
	s, err = Open(f.path)
	require.NoError(t, err)
	defer s.Close()
	assert.Equal(t, first, shapeOf(t, s))
	assert.EqualValues(t, 1, rawCount(t, s.db, "SELECT COUNT(*) FROM settings WHERE key = ?", migrationMarker))
	seqs := rawCount(t, s.db, "SELECT COUNT(DISTINCT seq) FROM threads WHERE project_id = 'p1' AND agent <> 'llm'")
	assert.EqualValues(t, 3, seqs)
}

func TestMigrationThatCannotBeVerifiedLeavesTheOldDatabase(t *testing.T) {
	f := buildOldDB(t)
	// A trigger stands in for any bug that makes the migration lose data: it
	// blanks a session id when the migration creates a project's main thread.
	g := rawOpen(t, f.path)
	require.NoError(t, g.Exec(`CREATE TRIGGER lose_session AFTER INSERT ON threads WHEN NEW.title = 'Alpha'
		BEGIN UPDATE threads SET session_id = '' WHERE id = 't1'; END`).Error)
	closeRaw(t, g)

	s, err := Open(f.path)
	require.Error(t, err)
	assert.Nil(t, s)
	assert.Contains(t, err.Error(), "verification failed")

	// Nothing was committed: the old schema and rows are exactly as they were.
	g = rawOpen(t, f.path)
	defer closeRaw(t, g)
	m := g.Migrator()
	assert.True(t, m.HasColumn("threads", "worktree"))
	assert.False(t, m.HasColumn("threads", "cwd"))
	assert.False(t, m.HasColumn("projects", "main_thread_id"))
	assert.True(t, m.HasColumn("channel_bindings", "target_kind"))
	assert.EqualValues(t, f.threads, rawCount(t, g, "SELECT COUNT(*) FROM threads"))
	assert.EqualValues(t, f.messages, rawCount(t, g, "SELECT COUNT(*) FROM project_messages"))
	assert.EqualValues(t, 3, rawCount(t, g, "SELECT COUNT(*) FROM channel_bindings"))
	assert.EqualValues(t, f.events, rawCount(t, g, "SELECT COUNT(*) FROM thread_events"))
}

func TestMigrationRecoversThreadsOfAMissingProject(t *testing.T) {
	f := buildOldDB(t)
	shared := filepath.Join(filepath.Dir(f.path), "shared")
	require.NoError(t, os.MkdirAll(shared, 0o755))
	g := rawOpen(t, f.path)
	create := func(v any) { require.NoError(t, g.Create(v).Error) }
	// Two threads of a deleted project that ran in one folder, one in a third
	// place, and one of another deleted project that ran in folder A (which
	// already has a folder project).
	create(&oldThread{ID: "g1", ProjectID: "ghost", Status: "idle", Worktree: shared, SessionID: "sg1", CreatedAt: at(70), UpdatedAt: at(71)})
	create(&oldThread{ID: "g2", ProjectID: "ghost", Status: "idle", Worktree: shared, SessionID: "sg2", CreatedAt: at(72), UpdatedAt: at(73)})
	create(&oldThread{ID: "h1", ProjectID: "ghost2", Status: "idle", Worktree: "/x/one", SessionID: "sh1", CreatedAt: at(74), UpdatedAt: at(75)})
	create(&oldThread{ID: "h2", ProjectID: "ghost2", Status: "idle", Worktree: "/x/two", SessionID: "sh2", CreatedAt: at(76), UpdatedAt: at(77)})
	create(&oldThread{ID: "k1", ProjectID: "ghost3", Status: "idle", Worktree: f.folderA, SessionID: "sk1", CreatedAt: at(78), UpdatedAt: at(79)})
	closeRaw(t, g)

	s, err := Open(f.path, WithProjectsDir("/data"))
	require.NoError(t, err, "a missing project must not stop startup")
	defer s.Close()

	owner := func(id string) Project {
		th := mustThread(t, s, id)
		p, err := s.GetProject(th.ProjectID)
		require.NoError(t, err)
		return *p
	}
	g1, g2, h1, h2, k1 := owner("g1"), owner("g2"), owner("h1"), owner("h2"), owner("k1")
	assert.Equal(t, ProjectFolder, g1.Type)
	assert.Equal(t, g1.ID, g2.ID, "threads of one missing project share a project")
	assert.Equal(t, CanonicalPath(shared), g1.Path, "the folder they all ran in")
	assert.Contains(t, g1.Name, "(recovered)")
	assert.Equal(t, h1.ID, h2.ID)
	assert.Equal(t, filepath.Join("/data", "recovered", "ghost2"), h1.Path, "no common folder: a stable synthetic path")
	assert.Contains(t, h1.Name, "(recovered)")
	assert.NotContains(t, k1.Name, "(recovered)", "an existing folder project is reused")
	assert.Equal(t, CanonicalPath(f.folderA), k1.Path)
	assert.Equal(t, "sg1", mustThread(t, s, "g1").SessionID)
}

func TestMigrationBacksUpTheOldDatabaseOnce(t *testing.T) {
	f := buildOldDB(t)
	backup := f.path + BackupSuffix
	_, err := os.Stat(backup)
	require.True(t, os.IsNotExist(err))

	s, err := Open(f.path)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// The copy is the old database, readable as it was.
	g := rawOpen(t, backup)
	assert.True(t, g.Migrator().HasColumn("threads", "worktree"))
	assert.EqualValues(t, f.threads, rawCount(t, g, "SELECT COUNT(*) FROM threads"))
	closeRaw(t, g)

	// Later starts leave it alone, even if it is changed or the marker is gone.
	require.NoError(t, os.WriteFile(backup, []byte("mine"), 0o644))
	s, err = Open(f.path)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	b, err := os.ReadFile(backup)
	require.NoError(t, err)
	assert.Equal(t, "mine", string(b), "an existing backup is never overwritten")
}

func TestFreshDatabaseIsNotBackedUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	s, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	_, err = os.Stat(path + BackupSuffix)
	assert.True(t, os.IsNotExist(err))
	s, err = Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	_, err = os.Stat(path + BackupSuffix)
	assert.True(t, os.IsNotExist(err))
}

func TestMigrationVerifiesBindingCounts(t *testing.T) {
	// A bug that loses a migrated binding must fail the migration rather than
	// drop it silently.
	tx := rawOpen(t, ":memory:")
	defer closeRaw(t, tx)
	require.NoError(t, tx.AutoMigrate(&Project{}, &Thread{}, &ThreadEvent{}, &ChannelBinding{}))
	require.NoError(t, tx.Create(&ChannelBinding{ID: "x", Platform: "p", ChatID: "c", ThreadID: "t"}).Error)

	// migrated(1) + dropped(0) != old(2): a binding vanished.
	err := verifyMigration(tx, nil, 0, 0, 0, 0, 0, 2, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bindings")
	// One dropped on purpose balances it.
	assert.NoError(t, verifyMigration(tx, nil, 0, 0, 0, 0, 0, 2, 1))
}

func TestFreshDatabaseNeedsNoMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	s, err := Open(path)
	require.NoError(t, err)
	p, main := newProject(t, s, "P")
	require.NoError(t, s.Close())

	s, err = Open(path)
	require.NoError(t, err)
	defer s.Close()
	got, err := s.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, main.ID, got.MainThreadID)
	assert.EqualValues(t, 1, rawCount(t, s.db, "SELECT COUNT(*) FROM settings WHERE key = ?", migrationMarker))
}

func TestCanonicalPathAgreesOnSpellings(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, PathKey(dir), PathKey(dir+"/"))
	assert.Equal(t, PathKey(dir), PathKey(filepath.Join(dir, "x", "..")))
	assert.Equal(t, "", PathKey(" "))
	assert.Equal(t, "(unknown folder)", FolderName(""))
}

// A database made by a release from before IM bindings has a project_messages
// table without the source columns; the migration must still start.
func TestMigrationCarriesAReleaseDatabaseWithoutSourceColumns(t *testing.T) {
	f := buildOldDB(t)
	g := rawOpen(t, f.path)
	for _, c := range []string{"source", "source_platform", "source_chat_id"} {
		require.NoError(t, g.Exec("ALTER TABLE project_messages DROP COLUMN "+c).Error)
	}
	closeRaw(t, g)

	s, err := Open(f.path)
	require.NoError(t, err)
	defer s.Close()
	events := rawCount(t, s.db, "SELECT COUNT(*) FROM thread_events WHERE thread_id IN (SELECT main_thread_id FROM projects WHERE type = 'project')")
	assert.EqualValues(t, f.messages, events)
	assert.EqualValues(t, 0, rawCount(t, s.db, "SELECT COUNT(*) FROM thread_events WHERE source = 'im'"))
}
