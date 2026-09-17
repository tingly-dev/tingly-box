// Package managedagent is a web front door onto the SAME machinery IM's
// `@cc` already uses: `remote/session.Manager` for session identity and
// transcript, `agentboot.AgentService.Run` for actually driving the Claude
// Code CLI. A web page is just another caller of that shared core — it
// gets its own chatID ("web") so its sessions are its own, but the store,
// the process-execution path, the --resume bookkeeping and the profile /
// gateway routing are the ones @cc already exercises in production. There
// is no separate domain model, no new database table, no separate
// checkout/workspace directory: the agent works directly in the folder the
// user points it at, the same way a local `claude` does.
//
// This is deliberately a base to grow from (a folder picker, a session
// list), not the full Managed Agent proposal in .design/managed-agent.md —
// that fuller shape (its own Source/Environment/Workspace model, cloned
// checkouts, push/PR) is preserved on the branch
// claude/managed-agent-heavy-v2-backup for when this path has proven
// itself.
package managedagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// webChatID is the fixed ChatID that groups every session this package
// creates, the same way an IM chat id groups @cc's sessions — it is what
// lets ListSessions show only the web's own sessions from the store the
// two front doors share.
const webChatID = "web"

// agentType is the `session.Session.Agent` value, matching what @cc's
// ClaudeCodeExecutor uses so both surfaces list under the same identity.
const agentType = "claude"

// Routing resolves the env / settings a turn should launch the CLI with,
// the same two mechanisms @cc's ClaudeCodeExecutor uses (see
// .design/remote-cc-profile.md §2): process env for the main scenario, or
// a materialized profile's --settings path. Implemented by tbclient.TBClient
// in production; a nil Routing runs with the host's own environment.
type Routing interface {
	GetClaudeCodeEnv(ctx context.Context) ([]string, error)
}

// Service is the single entry point every HTTP handler goes through.
type Service struct {
	sessions *session.Manager
	agent    *agentboot.AgentService
	routing  Routing

	mu   sync.Mutex
	runs map[string]*run // sessionID -> live turn, while one is in flight
}

// run is what Interrupt and Respond need for a session with a turn in
// flight. It is removed once the turn ends, so Interrupt/Respond outside a
// turn correctly find nothing to act on.
type run struct {
	cancel   context.CancelFunc
	prompter *webPrompter
}

// Config wires a Service to its dependencies.
type Config struct {
	Sessions *session.Manager
	Agent    *agentboot.AgentService
	Routing  Routing // optional
}

// NewService builds a Service. It does not touch the store.
func NewService(cfg Config) *Service {
	return &Service{sessions: cfg.Sessions, agent: cfg.Agent, routing: cfg.Routing, runs: map[string]*run{}}
}

// ---------- folders (a thin, un-persisted convenience) ----------

// RecentFolder is a directory a web session has worked in before — derived
// live from the session list, not a stored entity. There is nothing to add
// or remove: starting a task in a folder is what makes it "recent", the
// same way @cc's directory browser has no separate allowlist to manage.
//
// This never scans the filesystem: every path here is one the caller
// already typed and successfully started a session in, so there is no new
// read surface to reason about.
type RecentFolder struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// RecentFolders lists the distinct folders web sessions have run in, most
// recently used first.
func (s *Service) RecentFolders(limit int) []RecentFolder {
	all := s.sessions.SnapshotsByChat(webChatID)
	byPath := map[string]*RecentFolder{}
	for _, sess := range all {
		if sess.Agent != agentType || sess.Project == "" {
			continue
		}
		f, ok := byPath[sess.Project]
		if !ok {
			f = &RecentFolder{Path: sess.Project, Name: filepath.Base(sess.Project)}
			byPath[sess.Project] = f
		}
		if sess.LastActivity.After(f.LastUsedAt) {
			f.LastUsedAt = sess.LastActivity
		}
	}
	out := make([]RecentFolder, 0, len(byPath))
	for _, f := range byPath {
		out = append(out, *f)
	}
	sortRecentFolders(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// cleanFolderPath validates a folder path the way every entry point needs
// it: absolute, existing, a directory. There is no allowlist beyond that —
// this surface already requires the same host authentication @cc's own
// directory browser relies on.
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

// ---------- sessions ----------

// CreateSessionInput is the composer's request.
type CreateSessionInput struct {
	Path           string
	Prompt         string
	PermissionMode string
}

// CreateSession opens a conversation in a folder and starts its first turn.
// A folder takes as many concurrent sessions as the user wants, the same
// way several `claude` processes can run in one directory locally — each
// is its own Claude Code session, keyed by its own id.
func (s *Service) CreateSession(ctx context.Context, in CreateSessionInput) (*session.Session, error) {
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Prompt == "" {
		return nil, invalid("prompt is required")
	}
	path, err := cleanFolderPath(in.Path)
	if err != nil {
		return nil, err
	}
	if !ValidPermissionMode(in.PermissionMode) {
		return nil, invalid("unknown permission mode %q", in.PermissionMode)
	}

	sess := s.sessions.CreateWith(webChatID, agentType, path)
	id := sess.ID
	s.sessions.SetRequest(id, in.Prompt)
	s.sessions.Update(id, func(sess *session.Session) { sess.PermissionMode = in.PermissionMode })
	s.appendUserMessage(id, in.Prompt)
	s.startTurn(id, path, in.Prompt, in.PermissionMode, false)
	snap, _ := s.sessions.Snapshot(id)
	return &snap, nil
}

func (s *Service) GetSession(id string) (*session.Session, error) {
	snap, ok := s.sessions.SnapshotOrLoad(id)
	if !ok {
		return nil, notFound("session", id)
	}
	return &snap, nil
}

// ListSessions lists the web's own sessions, most recently active first.
func (s *Service) ListSessions(active bool) []session.Session {
	var out []session.Session
	for _, sess := range s.sessions.SnapshotsByChat(webChatID) {
		if sess.Agent != agentType {
			continue
		}
		if active && !isActive(sess.Status) {
			continue
		}
		out = append(out, sess)
	}
	sortSessionsByActivity(out)
	return out
}

func (s *Service) Messages(id string) ([]session.Message, error) {
	if _, ok := s.sessions.Get(id); !ok {
		return nil, notFound("session", id)
	}
	msgs, _ := s.sessions.GetMessages(id)
	return msgs, nil
}

// SendMessage appends a user turn and starts it. It is recorded first so
// the transcript is complete even if starting the turn fails.
func (s *Service) SendMessage(ctx context.Context, id, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return invalid("text is required")
	}
	sess, ok := s.sessions.SnapshotOrLoad(id)
	if !ok {
		return notFound("session", id)
	}
	if !s.canSteer(sess) {
		return conflict("session is %s", sess.Status)
	}
	s.appendUserMessage(id, text)
	s.startTurn(id, sess.Project, text, sess.PermissionMode, true)
	return nil
}

func (s *Service) canSteer(sess session.Session) bool {
	if sess.Status == session.StatusClosed || sess.Status == session.StatusExpired {
		return false
	}
	s.mu.Lock()
	_, busy := s.runs[sess.ID]
	s.mu.Unlock()
	return !busy
}

// SetPermissionMode changes a session's mode for its next turn.
func (s *Service) SetPermissionMode(id, mode string) (*session.Session, error) {
	if !ValidPermissionMode(mode) {
		return nil, invalid("unknown permission mode %q", mode)
	}
	if _, ok := s.sessions.Get(id); !ok {
		return nil, notFound("session", id)
	}
	s.sessions.Update(id, func(sess *session.Session) { sess.PermissionMode = mode })
	s.appendSystem(id, "permission mode: "+modeLabel(mode))
	snap, _ := s.sessions.Snapshot(id)
	return &snap, nil
}

// Respond answers a pending approval or ask request.
func (s *Service) Respond(id, requestID string, approved bool, answer string) error {
	if strings.TrimSpace(requestID) == "" {
		return invalid("request_id is required")
	}
	s.mu.Lock()
	r, ok := s.runs[id]
	s.mu.Unlock()
	if !ok {
		return conflict("session has no pending request")
	}
	if !r.prompter.resolve(requestID, approved, answer) {
		return notFound("request", requestID)
	}
	return nil
}

// Interrupt stops the current turn; the session stays resumable.
func (s *Service) Interrupt(id string) error {
	s.mu.Lock()
	r, ok := s.runs[id]
	s.mu.Unlock()
	if !ok {
		return conflict("session has no turn in progress")
	}
	r.cancel()
	return nil
}

// Archive ends a session for good. Nothing on disk is touched: the folder
// and the transcript both stay.
func (s *Service) Archive(id string) (*session.Session, error) {
	snap, ok := s.sessions.SnapshotOrLoad(id)
	if !ok {
		return nil, notFound("session", id)
	}
	if snap.Status != session.StatusClosed {
		s.mu.Lock()
		r, ok2 := s.runs[id]
		s.mu.Unlock()
		if ok2 {
			r.cancel()
		}
		// Close removes the session from the manager's live index (it stays
		// only in the store), so re-reading it via Snapshot afterward would
		// spuriously find nothing; the transition is known here, so just
		// reflect it locally.
		s.sessions.Close(id)
		snap.Status = session.StatusClosed
	}
	return &snap, nil
}

func isActive(st session.Status) bool {
	switch st {
	case session.StatusPending, session.StatusRunning, session.StatusCompleted, session.StatusFailed:
		return true
	}
	return false
}

func modeLabel(mode string) string {
	if mode == "" {
		return "inherit"
	}
	return mode
}

func (s *Service) appendUserMessage(id, text string) {
	s.sessions.AppendMessage(id, session.Message{Role: "user", Content: text, Timestamp: time.Now()})
}

func (s *Service) appendSystem(id, text string) {
	s.sessions.AppendMessage(id, session.Message{Kind: "system", Content: text, Timestamp: time.Now()})
}

var (
	// ErrNotFound is returned for a missing id.
	ErrNotFound = errors.New("not found")
	// ErrValidation is a rejected input.
	ErrValidation = errors.New("validation")
	// ErrConflict means the operation is not allowed in the current state.
	ErrConflict = errors.New("conflict")
)

func notFound(entity, id string) error { return fmt.Errorf("%s %s: %w", entity, id, ErrNotFound) }
func invalid(format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrValidation)
}
func conflict(format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrConflict)
}
