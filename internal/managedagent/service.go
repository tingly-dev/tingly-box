package managedagent

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service is the one entry point every caller surface (HTTP, IM, CLI) goes
// through. It validates, keeps the index consistent, and delegates execution
// to the Launcher.
type Service struct {
	stores   Stores
	launcher Launcher
	git      Git
	// workspacesDir is the host directory checkouts are materialised under
	// (~/.tingly-box/agent/workspaces).
	workspacesDir string
	now           func() time.Time
}

// Config configures a Service.
type Config struct {
	Stores        Stores
	Launcher      Launcher // optional; nil leaves sessions queued
	Git           Git      // optional; nil disables diff / push
	WorkspacesDir string
}

// NewService builds a Service. It does not touch the stores; call
// EnsureDefaults once at startup.
func NewService(cfg Config) *Service {
	return &Service{
		stores:        cfg.Stores,
		launcher:      cfg.Launcher,
		git:           cfg.Git,
		workspacesDir: cfg.WorkspacesDir,
		now:           time.Now,
	}
}

// EnsureDefaults creates the auto-provided local environment if it does not
// exist yet. The row is idempotent on its fixed id, so an operator's rename
// survives restarts.
func (s *Service) EnsureDefaults(ctx context.Context) error {
	_, err := s.stores.Environments.GetEnvironment(ctx, DefaultLocalEnvironmentID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	now := s.now()
	return s.stores.Environments.CreateEnvironment(ctx, &Environment{
		ID:        DefaultLocalEnvironmentID,
		Name:      "Local",
		Runtime:   RuntimeLocal,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// ---------- Sources ----------

// SourceInput is the caller-editable part of a Source.
type SourceInput struct {
	Name          string
	URL           string
	DefaultBranch string
	CredentialID  string
}

func (s *Service) CreateSource(ctx context.Context, in SourceInput) (*Source, error) {
	src := &Source{ID: uuid.NewString(), Kind: SourceKindGit, CreatedAt: s.now()}
	if err := applySourceInput(src, in); err != nil {
		return nil, err
	}
	src.UpdatedAt = src.CreatedAt
	if err := s.stores.Sources.CreateSource(ctx, src); err != nil {
		return nil, err
	}
	return src, nil
}

func (s *Service) GetSource(ctx context.Context, id string) (*Source, error) {
	return s.stores.Sources.GetSource(ctx, id)
}

func (s *Service) ListSources(ctx context.Context) ([]Source, error) {
	return s.stores.Sources.ListSources(ctx)
}

func (s *Service) UpdateSource(ctx context.Context, id string, in SourceInput) (*Source, error) {
	src, err := s.stores.Sources.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := applySourceInput(src, in); err != nil {
		return nil, err
	}
	src.UpdatedAt = s.now()
	if err := s.stores.Sources.UpdateSource(ctx, src); err != nil {
		return nil, err
	}
	return src, nil
}

// DeleteSource refuses while any non-reclaimed workspace still references
// the source: the checkout on disk would otherwise be orphaned with no way
// back to its remote.
func (s *Service) DeleteSource(ctx context.Context, id string) error {
	if _, err := s.stores.Sources.GetSource(ctx, id); err != nil {
		return err
	}
	live, err := s.liveWorkspaces(ctx, WorkspaceFilter{SourceID: id})
	if err != nil {
		return err
	}
	if live > 0 {
		return conflict("source has %d live workspace(s); archive their sessions first", live)
	}
	return s.stores.Sources.DeleteSource(ctx, id)
}

func applySourceInput(src *Source, in SourceInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.URL = strings.TrimSpace(in.URL)
	in.DefaultBranch = strings.TrimSpace(in.DefaultBranch)
	if in.URL == "" {
		return invalid("url is required")
	}
	if !looksLikeGitURL(in.URL) {
		return invalid("url %q is not a git URL", in.URL)
	}
	if in.Name == "" {
		in.Name = repoNameFromURL(in.URL)
	}
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	src.Name = in.Name
	src.URL = in.URL
	src.DefaultBranch = in.DefaultBranch
	src.CredentialID = strings.TrimSpace(in.CredentialID)
	return nil
}

var scpLikeGitURL = regexp.MustCompile(`^[\w.-]+@[\w.-]+:[\w./-]+$`)

func looksLikeGitURL(raw string) bool {
	if scpLikeGitURL.MatchString(raw) {
		return true
	}
	// A local repository (absolute path or file://) is a valid git remote:
	// git clones it like any other, and it is how tests and a future
	// local-directory source reach the same code path.
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "file://") {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https", "http", "ssh", "git":
		return true
	}
	return false
}

// repoNameFromURL turns ".../org/repo.git" into "repo".
func repoNameFromURL(raw string) string {
	trimmed := strings.TrimSuffix(strings.TrimRight(raw, "/"), ".git")
	if i := strings.LastIndexAny(trimmed, "/:"); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	if trimmed == "" {
		return "repo"
	}
	return trimmed
}

// ---------- Environments ----------

// EnvironmentInput is the caller-editable part of an Environment.
type EnvironmentInput struct {
	Name           string
	Runtime        Runtime
	Image          string
	SetupScript    string
	Env            map[string]string
	SecretRefs     []string
	Network        NetworkPolicy
	Resources      Resources
	CCProfile      string
	PermissionMode PermissionMode
}

func (s *Service) CreateEnvironment(ctx context.Context, in EnvironmentInput) (*Environment, error) {
	env := &Environment{ID: uuid.NewString(), CreatedAt: s.now()}
	if err := applyEnvironmentInput(env, in); err != nil {
		return nil, err
	}
	env.UpdatedAt = env.CreatedAt
	if err := s.stores.Environments.CreateEnvironment(ctx, env); err != nil {
		return nil, err
	}
	return env, nil
}

func (s *Service) GetEnvironment(ctx context.Context, id string) (*Environment, error) {
	return s.stores.Environments.GetEnvironment(ctx, id)
}

func (s *Service) ListEnvironments(ctx context.Context) ([]Environment, error) {
	return s.stores.Environments.ListEnvironments(ctx)
}

func (s *Service) UpdateEnvironment(ctx context.Context, id string, in EnvironmentInput) (*Environment, error) {
	env, err := s.stores.Environments.GetEnvironment(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := applyEnvironmentInput(env, in); err != nil {
		return nil, err
	}
	env.UpdatedAt = s.now()
	if err := s.stores.Environments.UpdateEnvironment(ctx, env); err != nil {
		return nil, err
	}
	return env, nil
}

// DeleteEnvironment refuses for the default environment (there must always
// be one place to run) and while live workspaces reference it.
func (s *Service) DeleteEnvironment(ctx context.Context, id string) error {
	env, err := s.stores.Environments.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	if env.IsDefault {
		return conflict("the default environment cannot be deleted")
	}
	live, err := s.liveWorkspaces(ctx, WorkspaceFilter{EnvironmentID: id})
	if err != nil {
		return err
	}
	if live > 0 {
		return conflict("environment has %d live workspace(s); archive their sessions first", live)
	}
	return s.stores.Environments.DeleteEnvironment(ctx, id)
}

// SupportedRuntimes lists what the Service accepts today. Docker is modelled
// but rejected until its process factory exists, so a user gets a clear
// "not yet" instead of a session that never starts.
var SupportedRuntimes = map[Runtime]bool{RuntimeLocal: true}

func applyEnvironmentInput(env *Environment, in EnvironmentInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name is required")
	}
	if in.Runtime == "" {
		in.Runtime = RuntimeLocal
	}
	switch in.Runtime {
	case RuntimeLocal, RuntimeDocker:
	default:
		return invalid("unknown runtime %q", in.Runtime)
	}
	if !SupportedRuntimes[in.Runtime] {
		return invalid("runtime %q is not available yet", in.Runtime)
	}
	if in.Runtime == RuntimeDocker {
		if strings.TrimSpace(in.Image) == "" {
			return invalid("image is required for the docker runtime")
		}
		if in.Network == "" {
			in.Network = NetworkProxy
		}
		switch in.Network {
		case NetworkNone, NetworkProxy, NetworkFull:
		default:
			return invalid("unknown network policy %q", in.Network)
		}
	} else {
		// Container-only settings are dropped rather than stored, so a local
		// environment never shows a network policy it does not enforce.
		in.Image, in.Network, in.Resources = "", "", Resources{}
	}
	for k := range in.Env {
		if strings.TrimSpace(k) == "" || strings.ContainsAny(k, "= \t\n") {
			return invalid("invalid env var name %q", k)
		}
	}
	env.Name = in.Name
	env.Runtime = in.Runtime
	env.Image = strings.TrimSpace(in.Image)
	env.SetupScript = in.SetupScript
	env.Env = in.Env
	env.SecretRefs = in.SecretRefs
	env.Network = in.Network
	env.Resources = in.Resources
	env.CCProfile = strings.TrimSpace(in.CCProfile)
	if !ValidPermissionMode(in.PermissionMode) {
		return invalid("unknown permission mode %q", in.PermissionMode)
	}
	env.PermissionMode = in.PermissionMode
	return nil
}

func (s *Service) liveWorkspaces(ctx context.Context, f WorkspaceFilter) (int, error) {
	list, err := s.stores.Workspaces.ListWorkspaces(ctx, f)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range list {
		if list[i].State != WorkspaceReclaimed {
			n++
		}
	}
	return n, nil
}

// ---------- Sessions ----------

// CreateSessionInput opens a new session. WorkspaceID continues in an
// existing checkout; otherwise SourceID (+ optional EnvironmentID, defaulting
// to the default environment) materialises a fresh one.
type CreateSessionInput struct {
	SourceID      string
	EnvironmentID string
	WorkspaceID   string
	BaseRef       string
	Prompt        string
	Title         string
	CreatedBy     string
	// PermissionMode overrides the environment's default; empty inherits.
	PermissionMode PermissionMode
}

func (s *Service) CreateSession(ctx context.Context, in CreateSessionInput) (*Session, error) {
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Prompt == "" {
		return nil, invalid("prompt is required")
	}
	if in.CreatedBy == "" {
		in.CreatedBy = "web"
	}
	if !ValidPermissionMode(in.PermissionMode) {
		return nil, invalid("unknown permission mode %q", in.PermissionMode)
	}

	var (
		ws  *Workspace
		err error
	)
	if in.WorkspaceID != "" {
		ws, err = s.stores.Workspaces.GetWorkspace(ctx, in.WorkspaceID)
		if err != nil {
			return nil, err
		}
		switch ws.State {
		case WorkspaceReclaimed:
			return nil, conflict("workspace %s has been reclaimed; start from its source instead", ws.ID)
		case WorkspaceFailed:
			return nil, conflict("workspace %s failed to provision (%s); start from its source instead", ws.ID, ws.Error)
		}
	}

	var src *Source
	if ws != nil {
		src, err = s.stores.Sources.GetSource(ctx, ws.SourceID)
	} else {
		if in.SourceID == "" {
			return nil, invalid("source_id or workspace_id is required")
		}
		src, err = s.stores.Sources.GetSource(ctx, in.SourceID)
	}
	if err != nil {
		return nil, err
	}

	envID := in.EnvironmentID
	if ws != nil {
		envID = ws.EnvironmentID
	} else if envID == "" {
		envID = DefaultLocalEnvironmentID
	}
	env, err := s.stores.Environments.GetEnvironment(ctx, envID)
	if err != nil {
		return nil, err
	}
	if !SupportedRuntimes[env.Runtime] {
		return nil, invalid("environment %q uses runtime %q, which is not available yet", env.Name, env.Runtime)
	}

	now := s.now()
	if ws == nil {
		baseRef := strings.TrimSpace(in.BaseRef)
		if baseRef == "" {
			baseRef = src.DefaultBranch
		}
		ws = &Workspace{
			ID:            uuid.NewString(),
			SourceID:      src.ID,
			EnvironmentID: env.ID,
			BaseRef:       baseRef,
			State:         WorkspaceProvisioning,
			CreatedAt:     now,
			LastActiveAt:  now,
		}
		ws.Path = filepath.Join(s.workspacesDir, ws.ID, "repo")
		ws.AgentCwd = ws.Path // RuntimeLocal; the docker runtime substitutes its mount point
		ws.Branch = branchName(in.Title, in.Prompt, ws.ID)
		if err := s.stores.Workspaces.CreateWorkspace(ctx, ws); err != nil {
			return nil, err
		}
	}

	// A workspace resumes its most recent session's Claude Code session so
	// the agent keeps its context across tb sessions.
	ccSessionID := ""
	if in.WorkspaceID != "" {
		prev, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{WorkspaceID: ws.ID, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(prev) > 0 {
			ccSessionID = prev[0].CCSessionID
		}
	}

	mode := in.PermissionMode
	if mode == PermissionInherit {
		mode = env.PermissionMode
	}
	sess := &Session{
		ID:             uuid.NewString(),
		Title:          sessionTitle(in.Title, in.Prompt),
		WorkspaceID:    ws.ID,
		Status:         SessionQueued,
		Prompt:         in.Prompt,
		PermissionMode: mode,
		CCSessionID:    ccSessionID,
		CreatedBy:      in.CreatedBy,
		Artifact:       Artifact{Branch: ws.Branch},
		CreatedAt:      now,
		LastActiveAt:   now,
	}
	if err := s.stores.Sessions.CreateSession(ctx, sess); err != nil {
		return nil, err
	}
	if err := s.stores.Events.AppendEvent(ctx, &Event{
		SessionID: sess.ID, Kind: EventUserMessage, Text: in.Prompt, At: now,
	}); err != nil {
		return nil, err
	}

	if s.launcher != nil {
		if err := s.launcher.Start(ctx, Run{Session: sess, Workspace: ws, Environment: env, Source: src}); err != nil {
			sess.Status = SessionFailed
			sess.Error = err.Error()
			_ = s.stores.Sessions.UpdateSession(ctx, sess)
			return sess, fmt.Errorf("start session: %w", err)
		}
	}
	return sess, nil
}

func (s *Service) GetSession(ctx context.Context, id string) (*Session, error) {
	return s.stores.Sessions.GetSession(ctx, id)
}

func (s *Service) ListSessions(ctx context.Context, f SessionFilter) ([]Session, error) {
	return s.stores.Sessions.ListSessions(ctx, f)
}

func (s *Service) GetWorkspace(ctx context.Context, id string) (*Workspace, error) {
	return s.stores.Workspaces.GetWorkspace(ctx, id)
}

func (s *Service) ListEvents(ctx context.Context, sessionID string, after int64, limit int) ([]Event, error) {
	if _, err := s.stores.Sessions.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	return s.stores.Events.ListEvents(ctx, sessionID, after, limit)
}

// SendMessage appends a user turn. It is recorded first so the transcript is
// complete even if the launcher is absent or refuses; a queued/idle session
// picks the message up when its next turn starts.
func (s *Service) SendMessage(ctx context.Context, sessionID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return invalid("text is required")
	}
	sess, err := s.stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if !s.canRetry(ctx, sess) {
		return conflict("session is %s; create a new session in its workspace instead", sess.Status)
	}
	// Only the event is written here. The index row is the Launcher's to
	// update (status, activity) — a second writer racing it on the whole
	// row could put a stale status back.
	if err := s.stores.Events.AppendEvent(ctx, &Event{
		SessionID: sess.ID, Kind: EventUserMessage, Text: text, At: s.now(),
	}); err != nil {
		return err
	}
	if s.launcher == nil {
		return nil
	}
	return s.launcher.Send(ctx, sess.ID, text)
}

// canRetry reports whether a session can take another message: active, or
// failed with its checkout still in place (a failed turn — a rejected
// permission mode, an upstream error — is retried by changing what caused
// it and sending again; done ≠ locked). A failed provisioning is not
// retryable here: the workspace itself is failed.
func (s *Service) canRetry(ctx context.Context, sess *Session) bool {
	if sess.Status.IsActive() {
		return true
	}
	if sess.Status != SessionFailed {
		return false
	}
	ws, err := s.stores.Workspaces.GetWorkspace(ctx, sess.WorkspaceID)
	return err == nil && ws.State == WorkspaceReady
}

// Respond answers a pending approval or ask request.
func (s *Service) Respond(ctx context.Context, sessionID string, r Response) error {
	if strings.TrimSpace(r.RequestID) == "" {
		return invalid("request_id is required")
	}
	sess, err := s.stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.Status != SessionWaitingInput {
		return conflict("session is %s, not waiting for input", sess.Status)
	}
	if s.launcher == nil {
		return conflict("no launcher is configured")
	}
	return s.launcher.Respond(ctx, sess.ID, r)
}

// SetPermissionMode changes an active session's mode. It applies from the
// next turn: the running Claude Code process keeps the mode it was launched
// with, which the status event says explicitly.
func (s *Service) SetPermissionMode(ctx context.Context, sessionID string, mode PermissionMode) (*Session, error) {
	if !ValidPermissionMode(mode) {
		return nil, invalid("unknown permission mode %q", mode)
	}
	sess, err := s.stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !s.canRetry(ctx, sess) {
		return nil, conflict("session is %s", sess.Status)
	}
	if sess.PermissionMode == mode {
		return sess, nil
	}
	sess.PermissionMode = mode
	if err := s.stores.Sessions.UpdateSession(ctx, sess); err != nil {
		return nil, err
	}
	label := string(mode)
	if mode == PermissionInherit {
		label = "inherit"
	}
	note := "permission mode: " + label
	if sess.Status == SessionRunning || sess.Status == SessionWaitingInput {
		note += " (from the next turn)"
	}
	_ = s.stores.Events.AppendEvent(ctx, &Event{SessionID: sess.ID, Kind: EventSystem, Text: note, At: s.now()})
	return sess, nil
}

// Interrupt stops the current turn. The session remains active.
func (s *Service) Interrupt(ctx context.Context, sessionID string) error {
	sess, err := s.stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.Status != SessionRunning && sess.Status != SessionWaitingInput {
		return conflict("session is %s; nothing to interrupt", sess.Status)
	}
	if s.launcher == nil {
		return conflict("no launcher is configured")
	}
	return s.launcher.Interrupt(ctx, sess.ID)
}

// Archive ends a session for good. The workspace and its branch are left in
// place (the branch may already be pushed); reclaiming the directory is a
// separate, later sweep.
func (s *Service) Archive(ctx context.Context, sessionID string) (*Session, error) {
	sess, err := s.stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if sess.Status == SessionArchived {
		return sess, nil
	}
	wasActive := sess.Status.IsActive()
	// The row is marked archived BEFORE the run is stopped: the cancelled
	// turn's finish path re-reads the session and leaves an archived one
	// alone, so ordering it this way is what keeps it from writing idle
	// back over the archive.
	now := s.now()
	sess.Status = SessionArchived
	sess.LastActiveAt = now
	if sess.FinishedAt == nil {
		sess.FinishedAt = &now
	}
	if err := s.stores.Sessions.UpdateSession(ctx, sess); err != nil {
		return nil, err
	}
	if err := s.stores.Events.AppendEvent(ctx, &Event{
		SessionID: sess.ID, Kind: EventStatus, Text: string(SessionArchived), At: now,
	}); err != nil {
		return nil, err
	}
	if s.launcher != nil && wasActive {
		if err := s.launcher.Stop(ctx, sess.ID); err != nil {
			return nil, err
		}
	}
	return sess, nil
}

// ---------- artifacts ----------

// Diff returns the session workspace's change summary against its base ref.
func (s *Service) Diff(ctx context.Context, sessionID string) (*Diff, error) {
	sess, ws, err := s.sessionWorkspace(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if s.git == nil {
		return nil, conflict("git is not configured")
	}
	if ws.State != WorkspaceReady {
		return nil, conflict("workspace is %s", ws.State)
	}
	d, err := s.git.Diff(ctx, ws)
	if err != nil {
		return nil, err
	}
	// Refresh the cached count only while nothing else is writing the row.
	if d.ChangedFiles != sess.Artifact.Changed && sess.Status != SessionRunning && sess.Status != SessionQueued {
		sess.Artifact.Changed = d.ChangedFiles
		_ = s.stores.Sessions.UpdateSession(ctx, sess)
	}
	return d, nil
}

// Push pushes the workspace branch to its origin and records the fact on the
// session's artifact. Push is an explicit control-plane action, never
// something the agent does from inside the checkout (§5.3).
func (s *Service) Push(ctx context.Context, sessionID string) (*Session, error) {
	sess, ws, err := s.sessionWorkspace(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if s.git == nil {
		return nil, conflict("git is not configured")
	}
	if ws.State != WorkspaceReady {
		return nil, conflict("workspace is %s", ws.State)
	}
	if sess.Status == SessionRunning {
		return nil, conflict("session is running; interrupt it or wait for the turn to finish before pushing")
	}
	now := s.now()
	logLine := func(line string) {
		_ = s.stores.Events.AppendEvent(ctx, &Event{SessionID: sess.ID, Kind: EventSystem, Text: line, At: s.now()})
	}
	if err := s.git.Push(ctx, ws, logLine); err != nil {
		logLine("push failed: " + err.Error())
		return nil, err
	}
	sess.Artifact.Pushed = true
	sess.Artifact.Branch = ws.Branch
	sess.LastActiveAt = now
	if err := s.stores.Sessions.UpdateSession(ctx, sess); err != nil {
		return nil, err
	}
	logLine("pushed " + ws.Branch)
	return sess, nil
}

func (s *Service) sessionWorkspace(ctx context.Context, sessionID string) (*Session, *Workspace, error) {
	sess, err := s.stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	ws, err := s.stores.Workspaces.GetWorkspace(ctx, sess.WorkspaceID)
	if err != nil {
		return nil, nil, err
	}
	return sess, ws, nil
}

// ---------- naming ----------

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// branchName derives "tb/<slug>-<short id>" from the title or prompt. The
// short id keeps two sessions on the same task from colliding on the remote.
func branchName(title, prompt, wsID string) string {
	base := title
	if strings.TrimSpace(base) == "" {
		base = prompt
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(base), "-"), "-")
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	if slug == "" {
		slug = "task"
	}
	short := strings.ReplaceAll(wsID, "-", "")
	if len(short) > 8 {
		short = short[:8]
	}
	return "tb/" + slug + "-" + short
}

// sessionTitle is the explicit title, or the prompt's first line clipped.
func sessionTitle(title, prompt string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	first := prompt
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSpace(first)
	if r := []rune(first); len(r) > 80 {
		return string(r[:77]) + "..."
	}
	return first
}
