package db

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// This file is the one-shot move to the unified Thread model described in
// .design/projects-sessions.md ("迁移"). It runs inside a single transaction
// and verifies its own result before committing; any mismatch rolls back and
// Open fails, leaving the old database exactly as it was.

// migrationMarker is the settings key set once the move has been committed.
const migrationMarker = "migrations.unified_threads"

// CanonicalPath is a folder path in the one spelling Projects stores: cleaned
// (no trailing slash, no "." or ".."), with symlinks resolved when the path
// exists.
func CanonicalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

// PathKey is what two spellings of one folder have in common: the canonical
// path, folded to lower case on file systems that ignore case.
func PathKey(p string) string {
	p = CanonicalPath(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}

// FolderName is the name a folder project gets: the folder's own name.
func FolderName(path string) string {
	if path == "" {
		return "(unknown folder)"
	}
	if b := filepath.Base(path); b != "." && b != string(filepath.Separator) {
		return b
	}
	return path
}

// legacy rows of tables the new model no longer has. They stay in the file for
// one version, read-only.
type legacyMessage struct {
	ID             string
	ProjectID      string
	Role           string
	Kind           string
	Content        string
	Payload        string
	Source         string
	SourcePlatform string
	SourceChatID   string
	CreatedAt      time.Time
}

// threadSnapshot is a thread as it was before the move. Owned is false when
// its project_id pointed at a project that did not exist.
type threadSnapshot struct {
	ID, ProjectID, SessionID string
	Owned                    bool
}

type legacyWorkdir struct {
	Path       string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

type legacyBinding struct {
	ID            string
	Platform      string
	ChatID        string
	ChatTitle     string
	TargetKind    string
	ProjectID     string
	FocusThreadID string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// migrate brings the schema and data to the current model. It is idempotent.
func migrate(g *gorm.DB, o openOptions) (err error) {
	tx := g.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	m := tx.Migrator()

	done := false
	if m.HasTable("settings") {
		var n int64
		tx.Raw("SELECT COUNT(*) FROM settings WHERE key = ?", migrationMarker).Scan(&n)
		done = n > 0
	}

	// 1. Schema. Old bindings are read first and their table dropped: it is
	// rebuilt with thread_id below, and its columns cannot be reshaped in place.
	var oldBindings []legacyBinding
	if !done && m.HasTable("channel_bindings") && m.HasColumn("channel_bindings", "target_kind") {
		if err = tx.Raw(`SELECT id, platform, chat_id, COALESCE(chat_title,'') chat_title, COALESCE(target_kind,'') target_kind,
			COALESCE(project_id,'') project_id, COALESCE(focus_thread_id,'') focus_thread_id, created_at, updated_at FROM channel_bindings`).Scan(&oldBindings).Error; err != nil {
			return fmt.Errorf("read channel bindings: %w", err)
		}
		if err = tx.Exec("DROP TABLE channel_bindings").Error; err != nil {
			return err
		}
	}
	if m.HasTable("threads") && m.HasColumn("threads", "worktree") && !m.HasColumn("threads", "cwd") {
		if err = tx.Exec("ALTER TABLE threads RENAME COLUMN worktree TO cwd").Error; err != nil {
			return fmt.Errorf("rename threads.worktree: %w", err)
		}
	}
	if err = tx.AutoMigrate(&Project{}, &Thread{}, &ThreadEvent{}, &Conversation{}, &Message{}, &Setting{}, &ChannelBinding{}); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Columns added to old rows are NULL, not "".
	for _, q := range []string{
		"UPDATE projects SET type = 'project' WHERE type IS NULL OR type = ''",
		"UPDATE projects SET path = '' WHERE path IS NULL",
		"UPDATE projects SET main_thread_id = '' WHERE main_thread_id IS NULL",
		"UPDATE threads SET parent_id = '' WHERE parent_id IS NULL",
		"UPDATE threads SET agent = '' WHERE agent IS NULL",
		"UPDATE threads SET seq = 0 WHERE seq IS NULL",
		"UPDATE threads SET cwd = '' WHERE cwd IS NULL",
		"UPDATE thread_events SET source = '' WHERE source IS NULL",
		"UPDATE thread_events SET source_ref = '' WHERE source_ref IS NULL",
		"UPDATE thread_events SET source_platform = '' WHERE source_platform IS NULL",
		"UPDATE thread_events SET source_chat_id = '' WHERE source_chat_id IS NULL",
		"UPDATE channel_bindings SET new_thread = false WHERE new_thread IS NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_path ON projects(path) WHERE path <> ''",
	} {
		if err = tx.Exec(q).Error; err != nil {
			return err
		}
	}

	if !done {
		if err = migrateData(tx, o, oldBindings); err != nil {
			return err
		}
		if err = tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_threads_project_seq ON threads(project_id, seq) WHERE seq > 0").Error; err != nil {
			return err
		}
		if err = tx.Exec("INSERT INTO settings (key, value, updated_at) VALUES (?, '1', ?)", migrationMarker, time.Now()).Error; err != nil {
			return err
		}
	}
	return tx.Commit().Error
}

func count(tx *gorm.DB, query string, args ...any) int64 {
	var n int64
	tx.Raw(query, args...).Scan(&n)
	return n
}

// migrateData is steps 2-7 of the design.
func migrateData(tx *gorm.DB, o openOptions, oldBindings []legacyBinding) error {
	m := tx.Migrator()
	var before []threadSnapshot
	if err := tx.Raw(`SELECT id, project_id, COALESCE(session_id,'') session_id,
		project_id IN (SELECT id FROM projects) AS owned FROM threads`).Scan(&before).Error; err != nil {
		return err
	}
	threadsBefore, eventsBefore := int64(len(before)), count(tx, "SELECT COUNT(*) FROM thread_events")
	bindingsBefore := count(tx, "SELECT COUNT(*) FROM channel_bindings")

	// 2. Folder projects, from workdirs and from threads without a project.
	if err := migrateFolders(tx); err != nil {
		return fmt.Errorf("folders: %w", err)
	}

	// A thread whose project is gone would fail verification forever, so it is
	// adopted by a recovered folder project instead.
	if err := recoverOrphanThreads(tx, o); err != nil {
		return fmt.Errorf("recover threads of missing projects: %w", err)
	}

	// 3. A main thread for every project; its other threads become children.
	if err := tx.Exec("UPDATE threads SET agent = ? WHERE agent = ''", AgentClaudeCode).Error; err != nil {
		return err
	}
	var needMain []Project
	if err := tx.Where("type = ? AND main_thread_id = ''", ProjectProject).Order("created_at, id").Find(&needMain).Error; err != nil {
		return err
	}
	for _, p := range needMain {
		main := Thread{
			ID: uuid.NewString(), ProjectID: p.ID, Title: p.Name, Status: ThreadIdle, Origin: "user", Agent: AgentLLM,
			CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		}
		if o.projectsDir != "" {
			main.Cwd = filepath.Join(o.projectsDir, p.ID)
		}
		if err := tx.Create(&main).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE projects SET main_thread_id = ? WHERE id = ?", main.ID, p.ID).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE threads SET parent_id = ? WHERE project_id = ? AND agent = ? AND parent_id = '' AND id <> ?",
			main.ID, p.ID, AgentClaudeCode, main.ID).Error; err != nil {
			return err
		}
	}

	// 4. The old conversation and event senders.
	moved, err := migrateMessages(tx)
	if err != nil {
		return fmt.Errorf("messages: %w", err)
	}
	if m.HasColumn("thread_events", "from") {
		for _, q := range []string{
			`UPDATE thread_events SET source = 'thread', source_ref = COALESCE((SELECT p.main_thread_id FROM threads t JOIN projects p ON p.id = t.project_id WHERE t.id = thread_events.thread_id), '')
				WHERE "from" = 'coordinator' AND source = ''`,
			`UPDATE thread_events SET "from" = '' WHERE "from" IS NOT NULL AND "from" <> ''`,
		} {
			if err := tx.Exec(q).Error; err != nil {
				return err
			}
		}
	}

	// 5. Per-project numbering, in creation order.
	var projectIDs []string
	if err := tx.Raw("SELECT id FROM projects").Scan(&projectIDs).Error; err != nil {
		return err
	}
	for _, id := range projectIDs {
		var ts []struct {
			ID  string
			Seq int
		}
		if err := tx.Raw("SELECT id, seq FROM threads WHERE project_id = ? AND agent <> ? ORDER BY created_at, rowid", id, AgentLLM).Scan(&ts).Error; err != nil {
			return err
		}
		next := 1
		for _, t := range ts {
			if t.Seq >= next {
				next = t.Seq + 1
			}
		}
		for _, t := range ts {
			if t.Seq == 0 {
				if err := tx.Exec("UPDATE threads SET seq = ? WHERE id = ?", next, t.ID).Error; err != nil {
					return err
				}
				next++
			}
		}
	}

	// 6. Bindings point at a thread: the focused one, else the main thread.
	dropped := 0
	for _, b := range oldBindings {
		threadID := ""
		if b.FocusThreadID != "" {
			tx.Raw("SELECT id FROM threads WHERE id = ? AND project_id = ?", b.FocusThreadID, b.ProjectID).Scan(&threadID)
		}
		if threadID == "" {
			tx.Raw("SELECT main_thread_id FROM projects WHERE id = ?", b.ProjectID).Scan(&threadID)
		}
		if threadID == "" {
			dropped++
			continue
		}
		nb := ChannelBinding{ID: b.ID, Platform: b.Platform, ChatID: b.ChatID, ChatTitle: b.ChatTitle, ThreadID: threadID, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
		if err := tx.Create(&nb).Error; err != nil {
			return err
		}
	}
	if dropped > 0 {
		logrus.Warnf("projects: dropped %d channel binding(s) whose project no longer exists", dropped)
	}

	// 7. Verify before committing.
	return verifyMigration(tx, before, threadsBefore, eventsBefore, int64(len(needMain)), moved,
		bindingsBefore, int64(len(oldBindings)), int64(dropped))
}

func verifyMigration(tx *gorm.DB, before []threadSnapshot, threadsBefore, eventsBefore, mains, moved, bindingsBefore, bindingsOld, bindingsDropped int64) error {
	fail := func(format string, a ...any) error {
		return fmt.Errorf("projects migration verification failed, database left unchanged: "+format, a...)
	}
	if n := count(tx, "SELECT COUNT(*) FROM threads"); n != threadsBefore+mains {
		return fail("threads: %d, want %d existing + %d main", n, threadsBefore, mains)
	}
	if n := count(tx, "SELECT COUNT(*) FROM thread_events"); n != eventsBefore+moved {
		return fail("events: %d, want %d existing + %d moved", n, eventsBefore, moved)
	}
	if n := count(tx, "SELECT COUNT(*) FROM channel_bindings"); n+bindingsDropped != bindingsBefore+bindingsOld {
		return fail("bindings: %d migrated + %d dropped, want %d old", n-bindingsBefore, bindingsDropped, bindingsOld)
	}
	if n := count(tx, "SELECT COUNT(*) FROM threads WHERE project_id = '' OR project_id NOT IN (SELECT id FROM projects)"); n > 0 {
		return fail("%d thread(s) without a project", n)
	}
	if n := count(tx, `SELECT COUNT(*) FROM projects p WHERE p.type = 'project' AND NOT EXISTS
		(SELECT 1 FROM threads t WHERE t.id = p.main_thread_id AND t.project_id = p.id AND t.agent = 'llm')`); n > 0 {
		return fail("%d project(s) without a main thread", n)
	}
	if n := count(tx, `SELECT COUNT(*) FROM threads t JOIN projects p ON p.id = t.project_id
		WHERE p.type = 'project' AND t.agent <> 'llm' AND t.parent_id <> p.main_thread_id`); n > 0 {
		return fail("%d project thread(s) not under their main thread", n)
	}
	if tx.Migrator().HasTable("project_messages") {
		if n := count(tx, `SELECT COUNT(*) FROM project_messages m WHERE m.project_id IN (SELECT id FROM projects)
			AND m.id NOT IN (SELECT id FROM thread_events)`); n > 0 {
			return fail("%d conversation message(s) not moved", n)
		}
	}
	for _, b := range before {
		var after struct{ ProjectID, SessionID string }
		if err := tx.Raw("SELECT project_id, COALESCE(session_id,'') session_id FROM threads WHERE id = ?", b.ID).Scan(&after).Error; err != nil {
			return err
		}
		if after.SessionID != b.SessionID {
			return fail("session_id of thread %s changed", b.ID)
		}
		if b.Owned && after.ProjectID != b.ProjectID {
			return fail("thread %s changed project", b.ID)
		}
	}
	return nil
}

// migrateFolders makes a type=folder project for every distinct folder in
// workdirs and in the working directory of threads that had no project, and
// gives those threads their project.
func migrateFolders(tx *gorm.DB) error {
	m := tx.Migrator()
	type group struct {
		path    string
		created time.Time
		used    time.Time
	}
	groups := map[string]*group{}
	add := func(path string, created, used time.Time) {
		key := PathKey(path)
		g, ok := groups[key]
		if !ok {
			groups[key] = &group{path: CanonicalPath(path), created: created, used: used}
			return
		}
		if created.Before(g.created) {
			g.created = created
		}
		if used.After(g.used) {
			g.used = used
		}
	}
	if m.HasTable("workdirs") {
		var ws []legacyWorkdir
		if err := tx.Raw("SELECT path, created_at, last_used_at FROM workdirs").Scan(&ws).Error; err != nil {
			return err
		}
		for _, w := range ws {
			add(w.Path, w.CreatedAt, w.LastUsedAt)
		}
	}
	var orphans []struct {
		ID        string
		Cwd       string
		CreatedAt time.Time
		UpdatedAt time.Time
	}
	if err := tx.Raw("SELECT id, cwd, created_at, updated_at FROM threads WHERE project_id = ''").Scan(&orphans).Error; err != nil {
		return err
	}
	for _, t := range orphans {
		add(t.Cwd, t.CreatedAt, t.UpdatedAt)
	}

	// Folder projects that already exist (a re-run) are reused.
	var existing []Project
	if err := tx.Where("type = ?", ProjectFolder).Find(&existing).Error; err != nil {
		return err
	}
	ids := map[string]string{}
	for _, p := range existing {
		ids[PathKey(p.Path)] = p.ID
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := ids[k]; ok {
			continue
		}
		g := groups[k]
		p := Project{
			ID: uuid.NewString(), Type: ProjectFolder, Path: g.path, Name: FolderName(g.path),
			CreatedAt: g.created, UpdatedAt: g.used,
		}
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		ids[k] = p.ID
	}
	for _, t := range orphans {
		if err := tx.Exec("UPDATE threads SET project_id = ? WHERE id = ?", ids[PathKey(t.Cwd)], t.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// migrateMessages moves the old project conversation into its main thread's
// events, keeping ids, order and timestamps, and returns how many it moved.
// Messages of a project that no longer exists have nowhere to go and are
// skipped (and not counted).
func migrateMessages(tx *gorm.DB) (int64, error) {
	if !tx.Migrator().HasTable("project_messages") {
		return 0, nil
	}
	// A database from before IM bindings has no source columns on the table.
	col := func(name string) string {
		if tx.Migrator().HasColumn("project_messages", name) {
			return "COALESCE(m." + name + ",'')"
		}
		return "''"
	}
	var msgs []legacyMessage
	err := tx.Raw(`SELECT m.id, m.project_id, m.role, m.kind, COALESCE(m.content,'') content, COALESCE(m.payload,'') payload,
		` + col("source") + ` source, ` + col("source_platform") + ` source_platform, ` + col("source_chat_id") + ` source_chat_id, m.created_at
		FROM project_messages m WHERE m.id NOT IN (SELECT id FROM thread_events) ORDER BY m.created_at, m.rowid`).Scan(&msgs).Error
	if err != nil {
		return 0, err
	}
	mains := map[string]string{}
	var moved int64
	for _, m := range msgs {
		main, ok := mains[m.ProjectID]
		if !ok {
			if err := tx.Raw("SELECT main_thread_id FROM projects WHERE id = ?", m.ProjectID).Scan(&main).Error; err != nil {
				return 0, err
			}
			mains[m.ProjectID] = main
		}
		if main == "" {
			continue
		}
		kind := m.Kind
		if kind == "text" {
			kind = EvAssistant
			if m.Role == RoleUser {
				kind = EvUser
			}
		}
		ev := ThreadEvent{
			ID: m.ID, ThreadID: main, Kind: kind, Content: m.Content, Payload: m.Payload,
			Source: m.Source, SourcePlatform: m.SourcePlatform, SourceChatID: m.SourceChatID, CreatedAt: m.CreatedAt,
		}
		if err := tx.Create(&ev).Error; err != nil {
			return 0, err
		}
		moved++
	}
	return moved, nil
}

// recoverOrphanThreads gives the threads of a project that no longer exists a
// folder project to live in, one per missing project id: the folder they all
// ran in when they share one (reusing its folder project if there is one),
// else a synthetic, stable path. The project is named "(recovered) ...", so the
// user can see what happened and archive it.
func recoverOrphanThreads(tx *gorm.DB, o openOptions) error {
	var ts []struct {
		ID        string
		ProjectID string
		Cwd       string
		CreatedAt time.Time
		UpdatedAt time.Time
	}
	if err := tx.Raw(`SELECT id, project_id, COALESCE(cwd,'') cwd, created_at, updated_at FROM threads
		WHERE project_id <> '' AND project_id NOT IN (SELECT id FROM projects) ORDER BY created_at, rowid`).Scan(&ts).Error; err != nil {
		return err
	}
	if len(ts) == 0 {
		return nil
	}
	byGhost := map[string][]int{}
	var ghosts []string
	for i, t := range ts {
		if _, ok := byGhost[t.ProjectID]; !ok {
			ghosts = append(ghosts, t.ProjectID)
		}
		byGhost[t.ProjectID] = append(byGhost[t.ProjectID], i)
	}
	sort.Strings(ghosts)
	for _, ghost := range ghosts {
		idx := byGhost[ghost]
		shared := PathKey(ts[idx[0]].Cwd)
		for _, i := range idx {
			if PathKey(ts[i].Cwd) != shared {
				shared = ""
				break
			}
		}
		created, used := ts[idx[0]].CreatedAt, ts[idx[0]].UpdatedAt
		for _, i := range idx {
			if ts[i].CreatedAt.Before(created) {
				created = ts[i].CreatedAt
			}
			if ts[i].UpdatedAt.After(used) {
				used = ts[i].UpdatedAt
			}
		}
		var pid string
		path := CanonicalPath(ts[idx[0]].Cwd)
		if shared == "" {
			path = filepath.Join(o.projectsDir, "recovered", ghost)
		}
		var existing []Project
		if err := tx.Where("type = ?", ProjectFolder).Find(&existing).Error; err != nil {
			return err
		}
		for _, p := range existing {
			if PathKey(p.Path) == PathKey(path) {
				pid = p.ID
			}
		}
		if pid == "" {
			name := "(recovered) " + FolderName(path)
			if shared == "" {
				name = "(recovered) " + ghost
			}
			p := Project{ID: uuid.NewString(), Type: ProjectFolder, Path: path, Name: name, CreatedAt: created, UpdatedAt: used}
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
			pid = p.ID
		}
		if err := tx.Exec("UPDATE threads SET project_id = ? WHERE project_id = ?", pid, ghost).Error; err != nil {
			return err
		}
		logrus.Warnf("projects: %d thread(s) belonged to project %s, which no longer exists; moved to folder project %q", len(idx), ghost, path)
	}
	return nil
}

// BackupSuffix is appended to the database path for the copy made before the
// first migration to the unified Thread model.
const BackupSuffix = ".pre-unified-threads"

// backupBeforeMigration copies an existing, not yet migrated database to
// <path>.pre-unified-threads, once. The copy is a consistent snapshot (VACUUM
// INTO, so the WAL is included). An existing backup is never overwritten.
func backupBeforeMigration(g *gorm.DB, path string) error {
	m := g.Migrator()
	if !m.HasTable("projects") && !m.HasTable("threads") {
		return nil // new or empty file: nothing to protect
	}
	if m.HasTable("settings") {
		var n int64
		if err := g.Raw("SELECT COUNT(*) FROM settings WHERE key = ?", migrationMarker).Scan(&n).Error; err == nil && n > 0 {
			return nil // already migrated
		}
	}
	dst := path + BackupSuffix
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := g.Exec("VACUUM INTO ?", dst).Error; err != nil {
		os.Remove(dst)
		return fmt.Errorf("back up database before migrating: %w", err)
	}
	logrus.Infof("projects: copied the database to %s before migrating it", dst)
	return nil
}
