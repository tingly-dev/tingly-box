package managedagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	now      func() time.Time
}

// Config configures a Service.
type Config struct {
	Stores   Stores
	Launcher Launcher // optional; nil leaves sessions queued
	Git      Git      // optional; nil disables the change summary
}

// NewService builds a Service. It does not touch the stores.
func NewService(cfg Config) *Service {
	return &Service{stores: cfg.Stores, launcher: cfg.Launcher, git: cfg.Git, now: time.Now}
}

// ---------- folders ----------

// AddFolder hands a directory to the agent. This is the grant: from here on
// the agent may work in it and the picker may browse inside it. Adding the
// same path twice returns the existing folder rather than failing — the
// grant is the point, not the row.
func (s *Service) AddFolder(ctx context.Context, path string) (*Folder, error) {
	clean, err := cleanFolderPath(path)
	if err != nil {
		return nil, err
	}
	if existing, err := s.stores.Folders.GetFolderByPath(ctx, clean); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	now := s.now()
	f := &Folder{ID: uuid.NewString(), Path: clean, Name: filepath.Base(clean), CreatedAt: now, LastUsedAt: now}
	if err := s.stores.Folders.CreateFolder(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

// cleanFolderPath validates a folder path the way every entry point needs it:
// absolute, existing, a directory.
func cleanFolderPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", invalid("a folder path is required")
	}
	if !filepath.IsAbs(path) {
		return "", invalid("folder path must be absolute, got %q", path)
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return "", invalid("%s: %v", clean, err)
	}
	if !info.IsDir() {
		return "", invalid("%s is not a directory", clean)
	}
	return clean, nil
}

func (s *Service) GetFolder(ctx context.Context, id string) (*Folder, error) {
	return s.stores.Folders.GetFolder(ctx, id)
}

func (s *Service) ListFolders(ctx context.Context) ([]Folder, error) {
	return s.stores.Folders.ListFolders(ctx)
}

// RemoveFolder withdraws the grant. The directory itself is never touched:
// tingly-box only forgets it (.design/managed-agent.md §13). Sessions that
// ran there keep their logs; an active one has to end first, because it is
// working in that directory right now.
func (s *Service) RemoveFolder(ctx context.Context, id string) error {
	f, err := s.stores.Folders.GetFolder(ctx, id)
	if err != nil {
		return err
	}
	live, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{FolderID: f.ID, Active: true})
	if err != nil {
		return err
	}
	if len(live) > 0 {
		return conflict("%s still has an active task; archive it first", f.Name)
	}
	return s.stores.Folders.DeleteFolder(ctx, f.ID)
}

// touchFolder records that a session started here, which is also the order
// the picker offers folders in.
func (s *Service) touchFolder(ctx context.Context, f *Folder) {
	f.LastUsedAt = s.now()
	_ = s.stores.Folders.UpdateFolder(ctx, f)
}

// ---------- sessions ----------

// CreateSessionInput is the composer's request: a folder (by id, or by path
// which adds it) and what the agent should do.
type CreateSessionInput struct {
	FolderID       string
	Path           string
	Prompt         string
	Title          string
	PermissionMode PermissionMode
	CreatedBy      string
}

// CreateSession opens a conversation in a folder and asks the Launcher to
// run its first turn.
//
// One active session per folder: the agent edits the directory in place, so
// two live sessions would be two processes writing the same files (and two
// Claude Code sessions keyed on the same cwd). A second task has to wait for
// the first to be archived — the conflict says so.
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
		folder *Folder
		err    error
	)
	switch {
	case strings.TrimSpace(in.Path) != "":
		folder, err = s.AddFolder(ctx, in.Path)
	case in.FolderID != "":
		folder, err = s.stores.Folders.GetFolder(ctx, in.FolderID)
	default:
		return nil, invalid("folder_id or path is required")
	}
	if err != nil {
		return nil, err
	}
	if _, err := cleanFolderPath(folder.Path); err != nil {
		return nil, err
	}

	live, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{FolderID: folder.ID, Active: true})
	if err != nil {
		return nil, err
	}
	if len(live) > 0 {
		return nil, conflict("%s already has an active task (%q); archive it or keep steering it", folder.Name, live[0].Title)
	}

	now := s.now()
	sess := &Session{
		ID:             uuid.NewString(),
		Title:          sessionTitle(in.Title, in.Prompt),
		FolderID:       folder.ID,
		Status:         SessionQueued,
		Prompt:         in.Prompt,
		PermissionMode: in.PermissionMode,
		CreatedBy:      in.CreatedBy,
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
	s.touchFolder(ctx, folder)
	if s.launcher == nil {
		return sess, nil
	}
	if err := s.launcher.Start(ctx, Run{Session: sess, Folder: folder}); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Service) GetSession(ctx context.Context, id string) (*Session, error) {
	return s.stores.Sessions.GetSession(ctx, id)
}

func (s *Service) ListSessions(ctx context.Context, f SessionFilter) ([]Session, error) {
	return s.stores.Sessions.ListSessions(ctx, f)
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
		return conflict("session is %s; start a new task in the folder instead", sess.Status)
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
// failed with its folder still there (a failed turn — a rejected permission
// mode, an upstream error — is retried by changing what caused it and
// sending again; done ≠ locked).
func (s *Service) canRetry(ctx context.Context, sess *Session) bool {
	if sess.Status.IsActive() {
		return true
	}
	if sess.Status != SessionFailed {
		return false
	}
	f, err := s.stores.Folders.GetFolder(ctx, sess.FolderID)
	if err != nil {
		return false
	}
	_, err = cleanFolderPath(f.Path)
	return err == nil
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

// Archive ends a session for good. Nothing on disk is touched: the folder,
// its files and the conversation log all stay (.design/managed-agent.md §13).
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

// ---------- changes ----------

// Diff summarises what the agent changed in the folder. A folder that is not
// a git work tree has no baseline, so the answer is an empty diff rather
// than an error — working in a plain directory is a legitimate use.
func (s *Service) Diff(ctx context.Context, sessionID string) (*Diff, error) {
	sess, folder, err := s.sessionFolder(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if s.git == nil || !s.git.IsRepo(ctx, folder.Path) {
		return &Diff{}, nil
	}
	d, err := s.git.Diff(ctx, folder.Path, sess.BaseCommit)
	if err != nil {
		return nil, err
	}
	// Refresh the cached count only while nothing else is writing the row.
	if d.ChangedFiles != sess.ChangedFiles && sess.Status != SessionRunning && sess.Status != SessionQueued {
		sess.ChangedFiles = d.ChangedFiles
		_ = s.stores.Sessions.UpdateSession(ctx, sess)
	}
	return d, nil
}

// sessionFolder resolves a session and the folder it works in.
func (s *Service) sessionFolder(ctx context.Context, sessionID string) (*Session, *Folder, error) {
	sess, err := s.stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	folder, err := s.stores.Folders.GetFolder(ctx, sess.FolderID)
	if err != nil {
		return nil, nil, err
	}
	return sess, folder, nil
}

// sessionTitle is the list's label: the given title, else the prompt's first
// line, trimmed to something a row can show.
func sessionTitle(title, prompt string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		t = strings.TrimSpace(strings.SplitN(prompt, "\n", 2)[0])
	}
	const max = 80
	if len(t) > max {
		t = strings.TrimSpace(t[:max]) + "…"
	}
	if t == "" {
		t = "Task"
	}
	return t
}
