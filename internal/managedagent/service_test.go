package managedagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestService(t *testing.T) (*Service, *MemStores) {
	t.Helper()
	mem, stores := NewMemStores()
	svc := NewService(Config{Stores: stores, WorkspacesDir: t.TempDir()})
	if err := svc.EnsureDefaults(context.Background()); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	return svc, mem
}

func TestEnsureDefaults_IsIdempotent(t *testing.T) {
	svc, _ := newTestService(t)
	if err := svc.EnsureDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	envs, _ := svc.ListEnvironments(context.Background())
	if len(envs) != 1 || envs[0].ID != DefaultLocalEnvironmentID || !envs[0].IsDefault || envs[0].Runtime != RuntimeLocal {
		t.Fatalf("unexpected environments: %+v", envs)
	}
}

func TestSource_ValidationAndDefaults(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateSource(ctx, SourceInput{URL: "not a url"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
	src, err := svc.CreateSource(ctx, SourceInput{URL: "https://github.com/org/repo.git"})
	if err != nil {
		t.Fatal(err)
	}
	if src.Name != "repo" || src.DefaultBranch != "main" || src.Kind != SourceKindGit {
		t.Fatalf("defaults not applied: %+v", src)
	}
	if _, err := svc.CreateSource(ctx, SourceInput{URL: "git@github.com:org/repo.git"}); err != nil {
		t.Fatalf("scp-like url rejected: %v", err)
	}
	if _, err := svc.CreateSource(ctx, SourceInput{URL: "/definitely/not/here"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing local directory must be rejected, got %v", err)
	}
	local, err := svc.CreateSource(ctx, SourceInput{URL: t.TempDir()})
	if err != nil || local.Kind != SourceKindLocal || local.DefaultBranch != "" {
		t.Fatalf("local directory source: %+v, %v", local, err)
	}
}

func TestEnvironment_DockerNotAvailableYet(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.CreateEnvironment(context.Background(), EnvironmentInput{Name: "box", Runtime: RuntimeDocker, Image: "x"})
	if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "not available yet") {
		t.Fatalf("want 'not available yet' validation error, got %v", err)
	}
	if err := svc.DeleteEnvironment(context.Background(), DefaultLocalEnvironmentID); !errors.Is(err, ErrConflict) {
		t.Fatalf("default env must not be deletable, got %v", err)
	}
}

func TestCreateSession_MaterialisesWorkspaceAndLogsPrompt(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	src, _ := svc.CreateSource(ctx, SourceInput{URL: "https://github.com/org/repo.git"})

	sess, err := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "Add a README\nwith details"})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Status != SessionQueued || sess.Title != "Add a README" || sess.CreatedBy != "web" {
		t.Fatalf("unexpected session: %+v", sess)
	}
	ws, err := svc.GetWorkspace(ctx, sess.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.EnvironmentID != DefaultLocalEnvironmentID || ws.BaseRef != "main" || ws.State != WorkspaceProvisioning {
		t.Fatalf("unexpected workspace: %+v", ws)
	}
	if !strings.HasPrefix(ws.Branch, "tb/add-a-readme-") || sess.Artifact.Branch != ws.Branch {
		t.Fatalf("branch naming: ws=%q sess=%q", ws.Branch, sess.Artifact.Branch)
	}
	events, _ := svc.ListEvents(ctx, sess.ID, 0, 0)
	if len(events) != 1 || events[0].Kind != EventUserMessage || events[0].Seq != 1 {
		t.Fatalf("unexpected events: %+v", events)
	}

	// Source deletion is blocked while the workspace is live.
	if err := svc.DeleteSource(ctx, src.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}

	// A follow-up session in the same workspace inherits the CC session id.
	sess.CCSessionID = "cc-1"
	_ = svc.stores.Sessions.UpdateSession(ctx, sess)
	next, err := svc.CreateSession(ctx, CreateSessionInput{WorkspaceID: ws.ID, Prompt: "now add tests"})
	if err != nil {
		t.Fatal(err)
	}
	if next.CCSessionID != "cc-1" || next.WorkspaceID != ws.ID {
		t.Fatalf("resume linkage lost: %+v", next)
	}
}

func TestSendMessage_And_Archive(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	src, _ := svc.CreateSource(ctx, SourceInput{URL: "https://github.com/org/repo.git"})
	sess, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "do it"})

	if err := svc.SendMessage(ctx, sess.ID, "  "); !errors.Is(err, ErrValidation) {
		t.Fatalf("empty steer must be rejected, got %v", err)
	}
	if err := svc.SendMessage(ctx, sess.ID, "also this"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Respond(ctx, sess.ID, Response{RequestID: "r1"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("respond on non-waiting session must conflict, got %v", err)
	}
	archived, err := svc.Archive(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != SessionArchived || archived.FinishedAt == nil {
		t.Fatalf("unexpected archived session: %+v", archived)
	}
	if err := svc.SendMessage(ctx, sess.ID, "too late"); !errors.Is(err, ErrConflict) {
		t.Fatalf("steer after archive must conflict, got %v", err)
	}
	events, _ := svc.ListEvents(ctx, sess.ID, 1, 0)
	if len(events) != 2 || events[0].Text != "also this" || events[1].Kind != EventStatus {
		t.Fatalf("unexpected events after seq 1: %+v", events)
	}
	if _, err := svc.GetSession(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestBranchName(t *testing.T) {
	got := branchName("", "Fix: the LOGIN bug!! in auth module, please, with a very long description that goes on", "abcdef12-3456")
	if !strings.HasPrefix(got, "tb/fix-the-login-bug-in-auth-module-pleas") || !strings.HasSuffix(got, "-abcdef12") {
		t.Fatalf("branchName = %q", got)
	}
	if got := branchName("", "!!!", "id"); got != "tb/task-id" {
		t.Fatalf("empty slug fallback = %q", got)
	}
}

func TestPermissionModes(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	src, _ := svc.CreateSource(ctx, SourceInput{URL: "https://github.com/org/repo.git"})

	if _, err := svc.CreateEnvironment(ctx, EnvironmentInput{Name: "x", PermissionMode: "yolo"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("unknown mode must be rejected, got %v", err)
	}
	env, err := svc.CreateEnvironment(ctx, EnvironmentInput{Name: "auto", PermissionMode: PermissionAuto})
	if err != nil {
		t.Fatal(err)
	}
	// Inherit from the environment, or override per session.
	inherited, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, EnvironmentID: env.ID, Prompt: "a"})
	if inherited.PermissionMode != PermissionAuto {
		t.Fatalf("session mode = %q, want auto from environment", inherited.PermissionMode)
	}
	over, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, EnvironmentID: env.ID, Prompt: "b", PermissionMode: PermissionPlan})
	if over.PermissionMode != PermissionPlan {
		t.Fatalf("session mode = %q, want plan override", over.PermissionMode)
	}
	if _, err := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "c", PermissionMode: "nope"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("unknown session mode must be rejected, got %v", err)
	}

	// Changing mid-session is recorded and applies from the next turn.
	over.Status = SessionRunning
	_ = svc.stores.Sessions.UpdateSession(ctx, over)
	changed, err := svc.SetPermissionMode(ctx, over.ID, PermissionBypassPermissions)
	if err != nil || changed.PermissionMode != PermissionBypassPermissions {
		t.Fatalf("set mode: %+v, %v", changed, err)
	}
	events, _ := svc.ListEvents(ctx, over.ID, 0, 0)
	last := events[len(events)-1]
	if last.Kind != EventSystem || !strings.Contains(last.Text, "bypassPermissions") || !strings.Contains(last.Text, "next turn") {
		t.Fatalf("mode change event = %+v", last)
	}
	if _, err := svc.Archive(ctx, over.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPermissionMode(ctx, over.ID, PermissionAuto); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived session must refuse a mode change, got %v", err)
	}
	if !PermissionBypassPermissions.AutoApproves() || PermissionAuto.AutoApproves() || PermissionDefault.AutoApproves() {
		t.Fatal("only bypassPermissions auto-approves")
	}
}

func TestFailedSessionCanRetryWhenWorkspaceReady(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	src, _ := svc.CreateSource(ctx, SourceInput{URL: "https://github.com/org/repo.git"})
	sess, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "a"})
	sess.Status, sess.Error = SessionFailed, "argument 'bogus' is invalid"
	_ = svc.stores.Sessions.UpdateSession(ctx, sess)

	// Provisioning still failed → nothing to retry into.
	if err := svc.SendMessage(ctx, sess.ID, "again"); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict while workspace is not ready, got %v", err)
	}
	ws, _ := svc.GetWorkspace(ctx, sess.WorkspaceID)
	ws.State = WorkspaceReady
	_ = svc.stores.Workspaces.UpdateWorkspace(ctx, ws)

	if _, err := svc.SetPermissionMode(ctx, sess.ID, PermissionAcceptEdits); err != nil {
		t.Fatalf("mode change on a failed session must be allowed: %v", err)
	}
	if err := svc.SendMessage(ctx, sess.ID, "again"); err != nil {
		t.Fatalf("retry on a failed session must be allowed: %v", err)
	}
	if _, err := svc.Archive(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.SendMessage(ctx, sess.ID, "again"); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived stays closed, got %v", err)
	}
}

// fakeGit is the Git seam for service tests: a plain directory is not a
// repo; push records the call.
type fakeGit struct{ pushed int }

func (g *fakeGit) Diff(context.Context, *Workspace) (*Diff, error) {
	return &Diff{ChangedFiles: 2, Stat: "x | 2"}, nil
}
func (g *fakeGit) Push(context.Context, *Workspace, func(string)) error { g.pushed++; return nil }
func (g *fakeGit) IsRepo(_ context.Context, ws *Workspace) bool {
	_, err := os.Stat(filepath.Join(ws.Path, ".git"))
	return err == nil
}

func TestLocalDirectorySource_WorksInPlace(t *testing.T) {
	ctx := context.Background()
	_, stores := NewMemStores()
	root := t.TempDir()
	git := &fakeGit{}
	svc := NewService(Config{Stores: stores, Git: git, WorkspacesDir: filepath.Join(root, "workspaces")})
	_ = svc.EnsureDefaults(ctx)
	dir := filepath.Join(root, "myproject")
	os.MkdirAll(dir, 0o755)

	src, err := svc.CreateSource(ctx, SourceInput{URL: dir})
	if err != nil || src.Kind != SourceKindLocal || src.Name != "myproject" {
		t.Fatalf("source = %+v, %v", src, err)
	}
	first, err := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "a"})
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := svc.GetWorkspace(ctx, first.WorkspaceID)
	if ws.Path != dir || ws.AgentCwd != dir || ws.State != WorkspaceReady || ws.Branch != "" || ws.BaseRef != "HEAD" {
		t.Fatalf("in-place workspace = %+v", ws)
	}
	// The same directory is the same workspace, and the next session resumes it.
	first.CCSessionID = "cc-1"
	_ = stores.Sessions.UpdateSession(ctx, first)
	second, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "b"})
	if second.WorkspaceID != ws.ID || second.CCSessionID != "cc-1" {
		t.Fatalf("second session must reuse the workspace and resume: %+v", second)
	}
	// Not a git repo → empty diff, no error; push is refused in place.
	d, err := svc.Diff(ctx, second.ID)
	if err != nil || d.ChangedFiles != 0 {
		t.Fatalf("diff on plain dir = %+v, %v", d, err)
	}
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	if d, _ := svc.Diff(ctx, second.ID); d.ChangedFiles != 2 {
		t.Fatalf("diff on repo dir = %+v", d)
	}
	if _, err := svc.Push(ctx, second.ID); !errors.Is(err, ErrConflict) || git.pushed != 0 {
		t.Fatalf("push in place must be refused, got %v", err)
	}
	// Reclaim never deletes a user's directory, and the sweep skips it.
	for _, id := range []string{first.ID, second.ID} {
		_, _ = svc.Archive(ctx, id)
	}
	svc.now = func() time.Time { return time.Now().Add(30 * 24 * time.Hour) }
	if n, _ := svc.ReclaimIdleWorkspaces(ctx, DefaultWorkspaceTTL); n != 0 {
		t.Fatalf("sweep must skip in-place workspaces, reclaimed %d", n)
	}
	if _, err := svc.ReclaimWorkspace(ctx, ws.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("user directory must survive reclaim")
	}
	// A local directory cannot run in a non-local environment.
	SupportedRuntimes[RuntimeDocker] = true
	defer delete(SupportedRuntimes, RuntimeDocker)
	docker, _ := svc.CreateEnvironment(ctx, EnvironmentInput{Name: "box", Runtime: RuntimeDocker, Image: "x"})
	if _, err := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, EnvironmentID: docker.ID, Prompt: "c"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("local dir in docker env must be rejected, got %v", err)
	}
}

func TestCreateSession_LocalPathIsDirect(t *testing.T) {
	ctx := context.Background()
	_, stores := NewMemStores()
	svc := NewService(Config{Stores: stores, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	dir := t.TempDir()

	a, err := svc.CreateSession(ctx, CreateSessionInput{LocalPath: dir, Prompt: "one"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateSession(ctx, CreateSessionInput{LocalPath: dir + string(filepath.Separator), Prompt: "two"})
	if err != nil {
		t.Fatal(err)
	}
	if a.WorkspaceID != b.WorkspaceID {
		t.Fatal("the same folder must map to one workspace")
	}
	sources, _ := svc.ListSources(ctx)
	if len(sources) != 1 || sources[0].Kind != SourceKindLocal || sources[0].URL != dir {
		t.Fatalf("exactly one local source expected: %+v", sources)
	}
	if _, err := svc.CreateSession(ctx, CreateSessionInput{LocalPath: "relative/dir", Prompt: "x"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("relative local_path must be rejected, got %v", err)
	}
	if _, err := svc.CreateSession(ctx, CreateSessionInput{Prompt: "x"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("no target must be rejected, got %v", err)
	}

	// Recent folders: the local source first, then Claude Code's projects
	// that still exist, deduplicated.
	other := t.TempDir()
	recent := func(context.Context) ([]string, error) { return []string{dir, other, "/definitely/gone"}, nil }
	folders, err := svc.RecentFolders(ctx, recent, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 || folders[0].Path != dir || folders[0].Source != "tasks" || folders[1].Path != other || folders[1].Source != "claude_code" {
		t.Fatalf("recent folders = %+v", folders)
	}
}

func TestBrowse(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "b-repo", ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "A-plain"), 0o755)
	os.MkdirAll(filepath.Join(root, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644)

	l, err := Browse(root)
	if err != nil {
		t.Fatal(err)
	}
	if l.Path != root || l.Parent != filepath.Dir(root) || len(l.Entries) != 2 {
		t.Fatalf("listing = %+v", l)
	}
	if l.Entries[0].Name != "A-plain" || l.Entries[1].Name != "b-repo" || !l.Entries[1].IsRepo || l.Entries[0].IsRepo {
		t.Fatalf("entries = %+v", l.Entries)
	}
	if _, err := Browse("relative"); !errors.Is(err, ErrValidation) {
		t.Fatalf("relative path: want ErrValidation, got %v", err)
	}
	if _, err := Browse(filepath.Join(root, "file.txt")); !errors.Is(err, ErrValidation) {
		t.Fatalf("file path: want ErrValidation, got %v", err)
	}
	if home, err := Browse(""); err != nil || !filepath.IsAbs(home.Path) {
		t.Fatalf("empty path must list home: %+v %v", home, err)
	}
}
