package managedagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeGit is the Git seam for service tests: a directory is a repo when it
// has a .git entry; the diff is fixed.
type fakeGit struct{ diffed int }

func (g *fakeGit) IsRepo(_ context.Context, dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}
func (g *fakeGit) Head(context.Context, string) (string, error)              { return "base0", nil }
func (g *fakeGit) ChangedFiles(context.Context, string, string) (int, error) { return 2, nil }
func (g *fakeGit) Diff(context.Context, string, string) (*Diff, error) {
	g.diffed++
	return &Diff{ChangedFiles: 2, Stat: "x | 2"}, nil
}

// fakeLauncher records what the Service asked of it without running anything.
type fakeLauncher struct {
	started   []string
	sent      []string
	responded []string
	stopped   []string
}

func (l *fakeLauncher) Start(_ context.Context, r Run) error {
	l.started = append(l.started, r.Session.ID)
	return nil
}
func (l *fakeLauncher) Send(_ context.Context, id, text string) error {
	l.sent = append(l.sent, id+":"+text)
	return nil
}
func (l *fakeLauncher) Respond(_ context.Context, id string, _ Response) error {
	l.responded = append(l.responded, id)
	return nil
}
func (l *fakeLauncher) Interrupt(context.Context, string) error { return nil }
func (l *fakeLauncher) Stop(_ context.Context, id string) error {
	l.stopped = append(l.stopped, id)
	return nil
}

func newTestService(t *testing.T) (*Service, *MemStores, *fakeLauncher) {
	t.Helper()
	mem, stores := NewMemStores()
	l := &fakeLauncher{}
	return NewService(Config{Stores: stores, Launcher: l, Git: &fakeGit{}}), mem, l
}

// A folder is the grant: the agent works in it in place, and adding the same
// path twice is the same folder.
func TestAddFolder(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	dir := t.TempDir()

	f, err := svc.AddFolder(ctx, dir)
	if err != nil || f.Path != dir || f.Name != filepath.Base(dir) {
		t.Fatalf("add: %+v %v", f, err)
	}
	again, err := svc.AddFolder(ctx, dir+string(filepath.Separator))
	if err != nil || again.ID != f.ID {
		t.Fatalf("adding the same path twice must return the same folder: %+v %v", again, err)
	}
	list, _ := svc.ListFolders(ctx)
	if len(list) != 1 {
		t.Fatalf("folders = %+v", list)
	}

	for _, bad := range []string{"", "relative/dir", filepath.Join(dir, "missing")} {
		if _, err := svc.AddFolder(ctx, bad); !errors.Is(err, ErrValidation) {
			t.Fatalf("%q: want ErrValidation, got %v", bad, err)
		}
	}
	file := filepath.Join(dir, "f.txt")
	os.WriteFile(file, []byte("x"), 0o644)
	if _, err := svc.AddFolder(ctx, file); !errors.Is(err, ErrValidation) {
		t.Fatalf("a file is not a folder: %v", err)
	}
}

// Removing a folder withdraws the grant and never touches the directory.
func TestRemoveFolderKeepsTheDirectory(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("mine"), 0o644)
	f, _ := svc.AddFolder(ctx, dir)

	sess, err := svc.CreateSession(ctx, CreateSessionInput{FolderID: f.ID, Prompt: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveFolder(ctx, f.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("a folder with an active task must be refused, got %v", err)
	}
	if _, err := svc.Archive(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveFolder(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("removing a folder must not touch its contents: %v", err)
	}
	if list, _ := svc.ListFolders(ctx); len(list) != 0 {
		t.Fatalf("folder still listed: %+v", list)
	}
}

// A task can be started straight from a path: that submission adds the
// folder, so there is no separate registration step.
func TestCreateSessionFromPath(t *testing.T) {
	ctx := context.Background()
	svc, _, launcher := newTestService(t)
	dir := t.TempDir()

	sess, err := svc.CreateSession(ctx, CreateSessionInput{Path: dir, Prompt: "add tests"})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Status != SessionQueued || sess.Title != "add tests" {
		t.Fatalf("session = %+v", sess)
	}
	folders, _ := svc.ListFolders(ctx)
	if len(folders) != 1 || folders[0].Path != dir || sess.FolderID != folders[0].ID {
		t.Fatalf("the path should have been added: %+v", folders)
	}
	if len(launcher.started) != 1 {
		t.Fatalf("launcher not started: %+v", launcher.started)
	}
	events, _ := svc.ListEvents(ctx, sess.ID, 0, 0)
	if len(events) != 1 || events[0].Kind != EventUserMessage || events[0].Text != "add tests" {
		t.Fatalf("the prompt must open the log: %+v", events)
	}

	for _, in := range []CreateSessionInput{
		{Path: dir},
		{Prompt: "x"},
		{Path: "relative", Prompt: "x"},
		{Path: dir, Prompt: "x", PermissionMode: "yolo"},
	} {
		if _, err := svc.CreateSession(ctx, in); !errors.Is(err, ErrValidation) {
			t.Fatalf("%+v: want ErrValidation, got %v", in, err)
		}
	}
}

// One task at a time per folder: the agent edits the directory in place, so
// a second live session would be a second process writing the same files.
func TestOneActiveSessionPerFolder(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	dir := t.TempDir()
	other := t.TempDir()

	first, err := svc.CreateSession(ctx, CreateSessionInput{Path: dir, Prompt: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(ctx, CreateSessionInput{Path: dir, Prompt: "second"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a second task in the same folder must be refused, got %v", err)
	}
	// A different folder is unaffected.
	if _, err := svc.CreateSession(ctx, CreateSessionInput{Path: other, Prompt: "elsewhere"}); err != nil {
		t.Fatal(err)
	}
	// Archiving the first frees the folder.
	if _, err := svc.Archive(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(ctx, CreateSessionInput{Path: dir, Prompt: "second"}); err != nil {
		t.Fatalf("after archiving, the folder is free again: %v", err)
	}
}

// Steering: a message is logged first, then handed to the launcher; an
// archived session takes none.
func TestSendMessageAndArchive(t *testing.T) {
	ctx := context.Background()
	svc, mem, launcher := newTestService(t)
	dir := t.TempDir()
	sess, _ := svc.CreateSession(ctx, CreateSessionInput{Path: dir, Prompt: "go"})

	if err := svc.SendMessage(ctx, sess.ID, "  and also this  "); err != nil {
		t.Fatal(err)
	}
	if len(launcher.sent) != 1 || launcher.sent[0] != sess.ID+":and also this" {
		t.Fatalf("launcher.sent = %+v", launcher.sent)
	}
	if err := svc.SendMessage(ctx, sess.ID, "   "); !errors.Is(err, ErrValidation) {
		t.Fatalf("empty text: %v", err)
	}

	archived, err := svc.Archive(ctx, sess.ID)
	if err != nil || archived.Status != SessionArchived || archived.FinishedAt == nil {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	if len(launcher.stopped) != 1 {
		t.Fatalf("the run must be stopped: %+v", launcher.stopped)
	}
	if err := svc.SendMessage(ctx, sess.ID, "too late"); !errors.Is(err, ErrConflict) {
		t.Fatalf("steering an archived session: %v", err)
	}
	// Archiving twice is idempotent, not an error.
	if _, err := svc.Archive(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ListSessions(ctx, SessionFilter{Active: true}); len(got) != 0 {
		t.Fatalf("archived sessions are not active: %+v", got)
	}
}

// A failed turn is retryable while the folder is still there: the user fixes
// what caused it (often the permission mode) and sends again.
func TestFailedSessionCanRetry(t *testing.T) {
	ctx := context.Background()
	svc, mem, _ := newTestService(t)
	dir := t.TempDir()
	sess, _ := svc.CreateSession(ctx, CreateSessionInput{Path: dir, Prompt: "go"})
	sess.Status = SessionFailed
	sess.Error = "boom"
	_ = mem.UpdateSession(ctx, sess)

	if _, err := svc.SetPermissionMode(ctx, sess.ID, PermissionBypassPermissions); err != nil {
		t.Fatalf("mode change on a failed session: %v", err)
	}
	if err := svc.SendMessage(ctx, sess.ID, "try again"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := svc.SetPermissionMode(ctx, sess.ID, "yolo"); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad mode: %v", err)
	}
	// Every advertised mode is accepted.
	for _, m := range PermissionModes {
		if _, err := svc.SetPermissionMode(ctx, sess.ID, m); err != nil {
			t.Fatalf("mode %s: %v", m, err)
		}
	}
}

// Diff answers for a git folder and stays empty for a plain one, rather
// than failing: working in a plain directory is a legitimate use.
func TestDiff(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)

	plain := t.TempDir()
	plainSess, _ := svc.CreateSession(ctx, CreateSessionInput{Path: plain, Prompt: "go"})
	d, err := svc.Diff(ctx, plainSess.ID)
	if err != nil || d.ChangedFiles != 0 || d.Patch != "" {
		t.Fatalf("plain folder: %+v %v", d, err)
	}

	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
	repoSess, _ := svc.CreateSession(ctx, CreateSessionInput{Path: repo, Prompt: "go"})
	if d, err = svc.Diff(ctx, repoSess.ID); err != nil || d.ChangedFiles != 2 {
		t.Fatalf("repo folder: %+v %v", d, err)
	}
}

// Respond and Interrupt only make sense in the states that have something
// to answer or stop.
func TestRespondAndInterruptGuards(t *testing.T) {
	ctx := context.Background()
	svc, mem, launcher := newTestService(t)
	dir := t.TempDir()
	sess, _ := svc.CreateSession(ctx, CreateSessionInput{Path: dir, Prompt: "go"})

	if err := svc.Respond(ctx, sess.ID, Response{RequestID: "r1", Approved: true}); !errors.Is(err, ErrConflict) {
		t.Fatalf("responding while queued: %v", err)
	}
	if err := svc.Respond(ctx, sess.ID, Response{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing request_id: %v", err)
	}
	if err := svc.Interrupt(ctx, sess.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("interrupting a queued session: %v", err)
	}

	sess.Status = SessionWaitingInput
	_ = mem.UpdateSession(ctx, sess)
	if err := svc.Respond(ctx, sess.ID, Response{RequestID: "r1", Approved: true}); err != nil {
		t.Fatal(err)
	}
	if len(launcher.responded) != 1 {
		t.Fatalf("launcher.responded = %+v", launcher.responded)
	}
	if err := svc.Interrupt(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSessionTitle(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	cases := []struct{ title, prompt, want string }{
		{"", "first line\nsecond", "first line"},
		{"given", "ignored", "given"},
		{"", "   ", "Task"},
	}
	for _, c := range cases {
		if got := sessionTitle(c.title, c.prompt); got != c.want {
			t.Errorf("sessionTitle(%q, %q) = %q, want %q", c.title, c.prompt, got, c.want)
		}
	}
	if got := sessionTitle("", long); len([]rune(got)) > 81 {
		t.Errorf("a long prompt must be trimmed, got %d chars", len([]rune(got)))
	}
}

func TestBrowse(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "b-repo", ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "A-plain"), 0o755)
	os.MkdirAll(filepath.Join(root, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644)
	roots := []string{root}

	// The top level is the allowlist itself.
	top, err := Browse("", roots)
	if err != nil || top.Path != "" || len(top.Entries) != 1 || top.Entries[0].Path != root {
		t.Fatalf("top = %+v %v", top, err)
	}
	l, err := Browse(root, roots)
	if err != nil {
		t.Fatal(err)
	}
	if l.Path != root || l.Parent != "" || len(l.Entries) != 2 {
		t.Fatalf("listing = %+v", l)
	}
	if l.Entries[0].Name != "A-plain" || l.Entries[1].Name != "b-repo" || !l.Entries[1].IsRepo || l.Entries[0].IsRepo {
		t.Fatalf("entries = %+v", l.Entries)
	}
	sub, err := Browse(filepath.Join(root, "A-plain"), roots)
	if err != nil || sub.Parent != root {
		t.Fatalf("sub = %+v %v", sub, err)
	}
	// Outside the allowlist: refused, whether it exists or not.
	for _, p := range []string{filepath.Dir(root), t.TempDir(), root + "-sibling", "/"} {
		if _, err := Browse(p, roots); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: want ErrForbidden, got %v", p, err)
		}
	}
	if _, err := Browse("relative", roots); !errors.Is(err, ErrValidation) {
		t.Fatalf("relative path: want ErrValidation, got %v", err)
	}
	if _, err := Browse(filepath.Join(root, "file.txt"), roots); !errors.Is(err, ErrValidation) {
		t.Fatalf("file path: want ErrValidation, got %v", err)
	}
	if _, err := Browse("", nil); err != nil {
		t.Fatalf("empty allowlist must list nothing, not fail: %v", err)
	}
}

// The browse allowlist is exactly the folders that were handed over.
func TestServiceBrowseFollowsTheFolders(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	dir := t.TempDir()

	if _, err := svc.Browse(ctx, dir); !errors.Is(err, ErrForbidden) {
		t.Fatalf("before adding: want ErrForbidden, got %v", err)
	}
	f, _ := svc.AddFolder(ctx, dir)
	if l, err := svc.Browse(ctx, dir); err != nil || l.Path != dir {
		t.Fatalf("after adding: %+v %v", l, err)
	}
	if _, err := svc.Browse(ctx, filepath.Dir(dir)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("the parent of an added folder stays closed, got %v", err)
	}
	if err := svc.RemoveFolder(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Browse(ctx, dir); !errors.Is(err, ErrForbidden) {
		t.Fatalf("removing the folder withdraws browsing too, got %v", err)
	}
}
