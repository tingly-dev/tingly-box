// Package desk is a web front door onto the SAME machinery IM's
// `@cc` already uses: `remote/session.Manager` for session identity and
// transcript, and `agentboot/engine` (over `agentboot.AgentService`) for
// actually driving the Claude Code CLI. A web page is just another caller of that shared core — it
// gets its own chatID ("web") so its sessions are its own, but the store,
// the process-execution path, the --resume bookkeeping and the profile /
// gateway routing are the ones @cc already exercises in production. There
// is no separate domain model, no new database table, no separate
// checkout/workspace directory: the agent works directly in the folder the
// user points it at, the same way a local `claude` does.
//
// This is deliberately a base to grow from (a folder picker, a session
// list), not the fuller design in .design/desk.md §6 —
// that shape (its own Source/Environment/Workspace model, cloned
// checkouts, push/PR) is preserved on the branch
// claude/managed-agent-heavy-v2-backup for when this path has proven
// itself.
package desk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/typ"
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
	// GetClaudeCodeSettingsPathForProfile materializes a Claude Code profile's
	// settings file and returns its path, for --settings.
	GetClaudeCodeSettingsPathForProfile(ctx context.Context, profileID string) (string, error)
}

// Service is the single entry point every HTTP handler goes through.
type Service struct {
	sessions *session.Manager
	routing  Routing
	// engine runs the turns: processes, approvals, background tasks.
	engine *engine.Engine
	pool   *pool.Pool // the engine's, kept for tests; nil keeps every turn one-shot

	mu sync.Mutex
	// model is each session's latest requested model (from its usage
	// entries), so Status needn't reload the transcript to find it.
	model map[string]string
	// launcher is the tingly-box binary Handoff's command runs.
	launcher string
}

// archiveWaitTimeout bounds how long Archive waits for a cancelled turn to
// exit. Cancellation closes the process, so this only covers its graceful
// shutdown; past it, archiving proceeds anyway.
const archiveWaitTimeout = 15 * time.Second

// Config wires a Service to its dependencies.
type Config struct {
	Sessions *session.Manager
	Agent    *agentboot.AgentService
	Routing  Routing // optional
	// Pool, if set, drives turns through a long-lived Claude Code process
	// per session instead of spawning one per message — the same
	// agentboot/pool mechanism @cc's ClaudeCodeExecutor uses for its
	// persistent_session setting (.design/claude-code-session.md §5.3). A setup
	// failure always falls back to a one-shot turn, so this is safe to
	// enable unconditionally; nil keeps every turn one-shot, unchanged
	// from before this existed.
	Pool *pool.Pool
	// Launcher is the tingly-box binary Handoff's command runs. Empty means
	// this process's own executable, by absolute path: it works whether
	// tingly-box is on PATH, run through npx, or started from a build dir.
	Launcher string
}

// NewService builds a Service. Construct it once per process: it treats
// every web session still marked running or pending as left over from a
// previous process (see recoverInterrupted).
func NewService(cfg Config) *Service {
	s := &Service{sessions: cfg.Sessions, routing: cfg.Routing, pool: cfg.Pool, model: map[string]string{}}
	s.engine = engine.New(engine.Config{
		Agent: cfg.Agent,
		Store: deskStore{s},
		Pool:  cfg.Pool,
		PermissionMode: func(id string) string {
			snap, _ := s.sessions.Snapshot(id)
			return snap.PermissionMode
		},
	})
	s.launcher = cfg.Launcher
	if s.launcher == "" {
		s.launcher = selfExecutable()
	}
	s.recoverInterrupted()
	return s
}

// selfExecutable is this process's binary by absolute, symlink-resolved
// path, or the bare command name if that can't be determined.
func selfExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return "tingly-box"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

// recoverInterrupted marks web sessions that were mid-turn when the previous
// process exited as interrupted-but-resumable. No turn can be in flight yet
// in this process, so a stored running/pending status is necessarily stale —
// and session.Manager's cleanup skips running sessions, so without this they
// would read as running forever.
func (s *Service) recoverInterrupted() {
	for _, sess := range s.sessions.SnapshotsByChat(webChatID) {
		if sess.Agent != agentType || (sess.Status != session.StatusRunning && sess.Status != session.StatusPending) {
			continue
		}
		s.sessions.Update(sess.ID, func(sess *session.Session) {
			sess.Status, sess.Error = session.StatusCompleted, ""
		})
		s.appendSystem(sess.ID, "interrupted by a tingly-box restart; send a message to resume")
	}
}

// Shutdown stops every in-flight turn and closes every resident persistent
// process, so none outlives the server holding its Claude session file open.
func (s *Service) Shutdown(ctx context.Context) { s.engine.Shutdown(ctx) }

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
	// Profile is a Claude Code profile id; "" uses the main claude_code
	// scenario's routing.
	Profile string
	// Model is a model tier alias (see Models); "" is the profile's default.
	Model string
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
	in.Profile = strings.TrimSpace(in.Profile)
	if err := s.checkProfile(ctx, in.Profile); err != nil {
		return nil, err
	}
	in.Model = strings.TrimSpace(in.Model)
	if err := s.checkModel(ctx, in.Profile, in.Model); err != nil {
		return nil, err
	}

	sess := s.sessions.CreateWith(webChatID, agentType, path)
	id := sess.ID
	s.sessions.SetRequest(id, in.Prompt)
	s.sessions.Update(id, func(sess *session.Session) {
		sess.PermissionMode = in.PermissionMode
		sess.Profile = in.Profile
		sess.Model = in.Model
		// CreateWith stamps now+Timeout, and the manager's expiry sweep
		// deletes the session and its transcript from the store once that
		// passes. A Desk session lives until it is archived, the same way
		// @cc's sessions clear ExpiresAt.
		sess.ExpiresAt = time.Time{}
	})
	s.startTurn(id, path, in.Prompt, turnSettings{PermissionMode: in.PermissionMode, Profile: in.Profile, Model: in.Model}, false)
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

// Scenario is the gateway scenario a session's turns are routed through:
// the main claude_code scenario, or its profile's.
func Scenario(profile string) string {
	if profile == "" {
		return string(typ.ScenarioClaudeCode)
	}
	return string(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profile))
}

// RequestedModel is the model id the session's latest completed turn asked
// the gateway for (the id routing rules match on), or "" before any turn
// has reached the model.
func (s *Service) RequestedModel(id string) string {
	s.mu.Lock()
	model, ok := s.model[id]
	s.mu.Unlock()
	if ok {
		return model
	}
	// Not seen since this server started: find it once in the transcript.
	msgs, _ := s.sessions.GetMessages(id)
	for i := len(msgs) - 1; i >= 0; i-- {
		if m := usageModel(msgs[i]); m != "" {
			model = m
			break
		}
	}
	// Only a found model is kept: with none yet, the next turn's usage
	// arrives through noteUsage, and the scan stays cheap until then.
	if model != "" {
		s.mu.Lock()
		if _, raced := s.model[id]; !raced {
			s.model[id] = model
		}
		model = s.model[id]
		s.mu.Unlock()
	}
	return model
}

// noteUsage keeps RequestedModel current as a turn records its usage.
func (s *Service) noteUsage(id string, m session.Message) {
	if model := usageModel(m); model != "" {
		s.mu.Lock()
		s.model[id] = model
		s.mu.Unlock()
	}
}

func usageModel(m session.Message) string {
	if m.Kind != engine.KindUsage {
		return ""
	}
	var u engine.TurnUsage
	if json.Unmarshal(m.Payload, &u) != nil {
		return ""
	}
	return u.Model
}

func (s *Service) Messages(id string) ([]session.Message, error) {
	// SnapshotOrLoad, not Get: an archived session is no longer in the
	// manager's live index (and is not reloaded at startup), but its
	// transcript is meant to stay readable.
	if _, ok := s.sessions.SnapshotOrLoad(id); !ok {
		return nil, notFound("session", id)
	}
	msgs, _ := s.sessions.GetMessages(id)
	return msgs, nil
}

// SendMessage records a user turn and starts it.
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
	// canSteer above is only a pre-check; startTurn's own claim in the engine is
	// the atomic one, and it records the message only once it has won, so a
	// losing concurrent send leaves nothing in the transcript.
	if !s.startTurn(id, sess.Project, text, settingsOf(sess), true) {
		return conflict("session is %s", sess.Status)
	}
	return nil
}

func (s *Service) canSteer(sess session.Session) bool {
	if sess.Status == session.StatusClosed || sess.Status == session.StatusExpired {
		return false
	}
	return !s.engine.Busy(sess.ID)
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

// SetProfile changes which Claude Code profile the session's next turn runs
// with ("" for the main claude_code routing). A resident process started with
// another profile is restarted for that turn (the engine compares launch settings).
func (s *Service) SetProfile(ctx context.Context, id, profile string) (*session.Session, error) {
	profile = strings.TrimSpace(profile)
	if _, ok := s.sessions.Get(id); !ok {
		return nil, notFound("session", id)
	}
	if err := s.checkProfile(ctx, profile); err != nil {
		return nil, err
	}
	// A unified profile has one model for every tier, so a tier picked under
	// the previous profile no longer means anything.
	resetModel := false
	if choice, err := s.Models(ctx, profile); err == nil && choice.Unified {
		resetModel = true
	}
	s.sessions.Update(id, func(sess *session.Session) {
		sess.Profile = profile
		if resetModel {
			sess.Model = ""
		}
	})
	s.appendSystem(id, "profile: "+profileLabel(profile))
	snap, _ := s.sessions.Snapshot(id)
	return &snap, nil
}

// checkProfile rejects a profile that can't be launched, so the mistake
// surfaces on the request instead of as a silent fallback on the next turn.
func (s *Service) checkProfile(ctx context.Context, profile string) error {
	if profile == "" || s.routing == nil {
		return nil
	}
	if _, err := s.routing.GetClaudeCodeSettingsPathForProfile(ctx, profile); err != nil {
		return invalid("profile %q: %v", profile, err)
	}
	return nil
}

// SetModel picks the model tier the session's next turns ask for. A resident
// process started with another tier restarts with --resume (the engine compares launch settings).
func (s *Service) SetModel(ctx context.Context, id, model string) (*session.Session, error) {
	model = strings.TrimSpace(model)
	// A copy: the live *Session is written by a turn's goroutine.
	sess, ok := s.sessions.Snapshot(id)
	if !ok {
		return nil, notFound("session", id)
	}
	if err := s.checkModel(ctx, sess.Profile, model); err != nil {
		return nil, err
	}
	s.sessions.Update(id, func(sess *session.Session) { sess.Model = model })
	s.appendSystem(id, "model: "+modelLabel(model))
	snap, _ := s.sessions.Snapshot(id)
	return &snap, nil
}

// checkModel accepts the default tier always, and a named tier only where
// the profile routes tiers separately: under a unified profile every tier
// is the same model, so choosing one would change nothing.
func (s *Service) checkModel(ctx context.Context, profile, model string) error {
	if model == "" {
		return nil
	}
	if !slices.Contains(agent.ClaudeCodeTierAliases, model) {
		return invalid("unknown model tier %q", model)
	}
	if choice, err := s.Models(ctx, profile); err == nil && choice.Unified {
		return invalid("profile %s routes every tier to one model; edit its rules to change it", profileLabel(profile))
	}
	return nil
}

// Models reads a profile's tiers from the env Claude Code itself is given —
// the main scenario's env, or the profile's settings file — so they are
// exactly what the process will request. It backs validation here; the
// public listing is the scenario module's GET /scenario/claude_code/models.
func (s *Service) Models(ctx context.Context, profile string) (agent.ClaudeCodeTiers, error) {
	env, err := s.claudeEnv(ctx, profile)
	if err != nil {
		return agent.ClaudeCodeTiers{}, err
	}
	return agent.ClaudeCodeTiersFromEnv(env), nil
}

// TierModel is the gateway model a session's chosen tier requests, or "".
func (s *Service) TierModel(ctx context.Context, sess *session.Session) string {
	choice, err := s.Models(ctx, sess.Profile)
	if err != nil {
		return ""
	}
	for _, t := range choice.Tiers {
		if t.Alias == sess.Model {
			return t.Model
		}
	}
	return choice.Tiers[0].Model
}

func (s *Service) claudeEnv(ctx context.Context, profile string) (map[string]string, error) {
	if s.routing == nil {
		return nil, errors.New("no gateway routing configured")
	}
	env := map[string]string{}
	if profile == "" {
		list, err := s.routing.GetClaudeCodeEnv(ctx)
		if err != nil {
			return nil, err
		}
		for _, kv := range list {
			if k, v, ok := strings.Cut(kv, "="); ok {
				env[k] = v
			}
		}
		return env, nil
	}
	path, err := s.routing.GetClaudeCodeSettingsPathForProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	return agent.ReadClaudeCodeSettingsEnv(path)
}

func modelLabel(model string) string {
	if model == "" {
		return "default"
	}
	return model
}

func profileLabel(profile string) string {
	if profile == "" {
		return "default"
	}
	return profile
}

// AwaitingInput reports whether the session's live turn is blocked on the
// user: an approval or a question nobody has answered yet.
func (s *Service) AwaitingInput(id string) bool { return s.engine.AwaitingInput(id) }

// Handoff releases a session so it can be continued in a terminal, and
// returns the command that does it. The resident process is closed first:
// two processes writing one Claude session file corrupts it (see
// engine.Release). A turn in flight is refused rather than killed.
//
// The command goes through tingly-box (`cc`, or `profile <id>` for a
// profile; the binary by absolute path, see Config.Launcher) rather than
// bare `claude`, so the terminal routes through the
// same gateway and settings the web turns used, with no token in the
// command itself.
func (s *Service) Handoff(id string) (string, error) {
	sess, ok := s.sessions.SnapshotOrLoad(id)
	if !ok {
		return "", notFound("session", id)
	}
	if err := s.engine.Release(id); err != nil {
		return "", conflict("stop the current turn before continuing in a terminal")
	}

	launch := shellQuote(s.launcher) + " cc"
	if sess.Profile != "" {
		launch = shellQuote(s.launcher) + " profile " + shellQuote(sess.Profile)
	}
	return "cd " + shellQuote(sess.Project) + " && " + launch + " --resume " + shellQuote(id), nil
}

// shellQuote single-quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Respond answers a pending approval or ask request.
func (s *Service) Respond(id, requestID string, approved bool, answer string) error {
	if strings.TrimSpace(requestID) == "" {
		return invalid("request_id is required")
	}
	switch err := s.engine.Respond(id, requestID, approved, answer); {
	case errors.Is(err, engine.ErrNoTurn):
		return conflict("session has no pending request")
	case errors.Is(err, engine.ErrUnknownRequest):
		return notFound("request", requestID)
	default:
		return err
	}
}

// Interrupt stops the current turn; the session stays resumable.
func (s *Service) Interrupt(id string) error {
	if _, ok := s.engine.Stop(id); !ok {
		return conflict("session has no turn in progress")
	}
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
		// Wait for a cancelled turn to exit first: otherwise its late status
		// write can revive the session after Close, and a persistent Open
		// still in flight can register a new process after the eviction.
		if done, ok := s.engine.Stop(id); ok {
			select {
			case <-done:
			case <-time.After(archiveWaitTimeout):
			}
		}
		// A resident persistent process must not survive archiving: it
		// would keep the on-disk Claude session file open, corrupting any
		// later resume attempt (the exact bug .design/claude-code-session.md §5.4
		// documents fixing for @cc's own bot-stop/setting-off paths).
		s.engine.Evict(id)
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

func (s *Service) appendSystem(id, text string) {
	s.sessions.AppendMessage(id, session.Message{Kind: engine.KindSystem, Content: text, Timestamp: time.Now()})
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
