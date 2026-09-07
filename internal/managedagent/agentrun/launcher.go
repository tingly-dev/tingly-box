// Package agentrun is the in-process managedagent.Launcher over agentboot:
// it provisions the workspace on the host, runs Claude Code turns, writes
// every event to the session log, and routes approval / ask requests back
// through the Service. It has no runtime of its own — the local runtime is
// agentboot's OS process factory; the docker runtime is the same Launcher
// with a container process factory (.design/managed-agent.md §5.1).
package agentrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/managedagent/gitrepo"
)

// Routing resolves how Claude Code reaches the gateway for one session:
// either a materialised profile settings file or the main scenario's env,
// mirroring .design/remote-cc-profile.md §2. Either return may be empty.
type Routing interface {
	// Resolve returns (env, settingsPath). A non-empty settingsPath wins.
	Resolve(ctx context.Context, ccProfile string) (env []string, settingsPath string, err error)
}

// RoutingFunc adapts a function to Routing.
type RoutingFunc func(ctx context.Context, ccProfile string) ([]string, string, error)

func (f RoutingFunc) Resolve(ctx context.Context, p string) ([]string, string, error) {
	return f(ctx, p)
}

// Provisioner materialises a workspace checkout. gitrepo.Git satisfies it.
type Provisioner interface {
	Provision(ctx context.Context, req gitrepo.ProvisionRequest) error
	Diff(ctx context.Context, dir, baseRef string) (*gitrepo.Diff, error)
}

// Config wires a Launcher.
type Config struct {
	Stores  managedagent.Stores
	Agent   agentboot.Agent
	Git     Provisioner
	Routing Routing // optional
	// TurnTimeout bounds one model turn. Zero uses agentboot's default;
	// negative disables it.
	TurnTimeout time.Duration
	Logger      *logrus.Logger
}

// Launcher implements managedagent.Launcher.
type Launcher struct {
	cfg Config
	log *logrus.Entry

	mu   sync.Mutex
	runs map[string]*run // by session id
}

var _ managedagent.Launcher = (*Launcher)(nil)

// New builds a Launcher.
func New(cfg Config) (*Launcher, error) {
	if cfg.Agent == nil {
		return nil, errors.New("agentrun: agent is required")
	}
	if cfg.Git == nil {
		return nil, errors.New("agentrun: git provisioner is required")
	}
	if cfg.Stores.Sessions == nil || cfg.Stores.Workspaces == nil || cfg.Stores.Events == nil {
		return nil, errors.New("agentrun: stores are required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = logrus.StandardLogger()
	}
	return &Launcher{cfg: cfg, log: logger.WithField("component", "agentrun"), runs: map[string]*run{}}, nil
}

// pendingKind remembers which control shape a request id expects.
type pendingKind int

const (
	pendingApproval pendingKind = iota + 1
	pendingAsk
)

// run is one live session: at most one turn at a time, steering messages
// queued for the next turn, and the control requests awaiting an answer.
type run struct {
	sessionID string
	cancel    context.CancelFunc

	mu      sync.Mutex
	handle  agentboot.ExecutionHandle // current turn; nil while idle
	pending map[string]pendingKind
	queue   []string
	busy    bool
}

// Start provisions the workspace if needed and runs the first turn. It
// returns once the run is registered; provisioning and execution happen on
// a goroutine and report through the stores.
func (l *Launcher) Start(ctx context.Context, r managedagent.Run) error {
	if r.Session == nil || r.Workspace == nil || r.Source == nil || r.Environment == nil {
		return errors.New("agentrun: incomplete run")
	}
	if r.Environment.Runtime != managedagent.RuntimeLocal {
		return fmt.Errorf("agentrun: runtime %q is not available yet", r.Environment.Runtime)
	}
	rn := l.register(r.Session.ID)
	go l.drive(rn, r, r.Session.Prompt)
	return nil
}

// Send queues a steering message. A running turn picks it up as the next
// prompt; an idle session starts a turn immediately.
func (l *Launcher) Send(ctx context.Context, sessionID, text string) error {
	rn := l.get(sessionID)
	if rn == nil {
		// Not live in this process (restart, or a queued session): rebuild
		// the run from the stores and start a turn with this text.
		r, err := l.load(ctx, sessionID)
		if err != nil {
			return err
		}
		rn = l.register(sessionID)
		go l.drive(rn, r, text)
		return nil
	}
	rn.mu.Lock()
	defer rn.mu.Unlock()
	if rn.busy {
		rn.queue = append(rn.queue, text)
		return nil
	}
	rn.busy = true
	r, err := l.load(ctx, sessionID)
	if err != nil {
		rn.busy = false
		return err
	}
	go l.drive(rn, r, text)
	return nil
}

// Respond answers a pending approval or ask request.
func (l *Launcher) Respond(ctx context.Context, sessionID string, resp managedagent.Response) error {
	rn := l.get(sessionID)
	if rn == nil {
		return fmt.Errorf("session %s: %w", sessionID, managedagent.ErrConflict)
	}
	rn.mu.Lock()
	handle := rn.handle
	kind, ok := rn.pending[resp.RequestID]
	if ok {
		delete(rn.pending, resp.RequestID)
	}
	stillWaiting := len(rn.pending) > 0
	rn.mu.Unlock()
	if !ok || handle == nil {
		return fmt.Errorf("request %s is not pending: %w", resp.RequestID, managedagent.ErrNotFound)
	}

	var cr agentboot.ControlResponse
	var ev managedagent.Event
	switch kind {
	case pendingAsk:
		cr = agentboot.AskResponse{Approved: true, Response: resp.Answer}
		ev = managedagent.Event{Kind: managedagent.EventAskResponse, Text: resp.Answer}
	default:
		cr = agentboot.ApprovalResponse{Approved: resp.Approved}
		txt := "denied"
		if resp.Approved {
			txt = "approved"
		}
		ev = managedagent.Event{Kind: managedagent.EventApprovalResponse, Text: txt}
	}
	if err := handle.Respond(resp.RequestID, cr); err != nil {
		return err
	}
	ev.SessionID, ev.RequestID = sessionID, resp.RequestID
	l.append(ctx, ev)
	if !stillWaiting {
		l.setStatus(ctx, sessionID, managedagent.SessionRunning, "")
	}
	return nil
}

// Interrupt cancels the current turn. The session stays resumable.
func (l *Launcher) Interrupt(ctx context.Context, sessionID string) error {
	rn := l.get(sessionID)
	if rn == nil {
		return nil
	}
	rn.mu.Lock()
	handle := rn.handle
	rn.queue = nil
	rn.mu.Unlock()
	if handle != nil {
		handle.Cancel()
	}
	return nil
}

// Stop tears the run down for good.
func (l *Launcher) Stop(ctx context.Context, sessionID string) error {
	l.mu.Lock()
	rn := l.runs[sessionID]
	delete(l.runs, sessionID)
	l.mu.Unlock()
	if rn != nil {
		rn.cancel()
	}
	return nil
}

// ---------- internals ----------

func (l *Launcher) register(sessionID string) *run {
	l.mu.Lock()
	defer l.mu.Unlock()
	if rn, ok := l.runs[sessionID]; ok {
		return rn
	}
	_, cancel := context.WithCancel(context.Background())
	rn := &run{sessionID: sessionID, cancel: cancel, pending: map[string]pendingKind{}, busy: true}
	l.runs[sessionID] = rn
	return rn
}

func (l *Launcher) get(sessionID string) *run {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.runs[sessionID]
}

// load rebuilds a Run from the stores for a session that is not live.
func (l *Launcher) load(ctx context.Context, sessionID string) (managedagent.Run, error) {
	sess, err := l.cfg.Stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return managedagent.Run{}, err
	}
	ws, err := l.cfg.Stores.Workspaces.GetWorkspace(ctx, sess.WorkspaceID)
	if err != nil {
		return managedagent.Run{}, err
	}
	env, err := l.cfg.Stores.Environments.GetEnvironment(ctx, ws.EnvironmentID)
	if err != nil {
		return managedagent.Run{}, err
	}
	src, err := l.cfg.Stores.Sources.GetSource(ctx, ws.SourceID)
	if err != nil {
		return managedagent.Run{}, err
	}
	return managedagent.Run{Session: sess, Workspace: ws, Environment: env, Source: src}, nil
}

// drive runs provisioning (once) and then turns until the steer queue is
// empty. It owns rn.busy.
func (l *Launcher) drive(rn *run, r managedagent.Run, prompt string) {
	ctx := context.Background()
	defer func() {
		rn.mu.Lock()
		rn.busy = false
		rn.mu.Unlock()
	}()

	if r.Workspace.State == managedagent.WorkspaceProvisioning {
		if err := l.provision(ctx, r); err != nil {
			l.setStatus(ctx, r.Session.ID, managedagent.SessionFailed, err.Error())
			return
		}
	}
	if r.Workspace.State != managedagent.WorkspaceReady {
		l.setStatus(ctx, r.Session.ID, managedagent.SessionFailed, "workspace is "+string(r.Workspace.State))
		return
	}

	for {
		l.turn(ctx, rn, r, prompt)
		rn.mu.Lock()
		if len(rn.queue) == 0 {
			rn.mu.Unlock()
			return
		}
		prompt = strings.Join(rn.queue, "\n\n")
		rn.queue = nil
		rn.mu.Unlock()
	}
}

func (l *Launcher) provision(ctx context.Context, r managedagent.Run) error {
	ws := r.Workspace
	l.append(ctx, managedagent.Event{SessionID: r.Session.ID, Kind: managedagent.EventSystem,
		Text: fmt.Sprintf("provisioning workspace from %s (%s)", r.Source.URL, ws.BaseRef)})
	logLine := func(line string) {
		l.append(ctx, managedagent.Event{SessionID: r.Session.ID, Kind: managedagent.EventSystem, Text: line})
	}
	// A previous attempt cut short (crash mid-clone) leaves a directory the
	// clone would refuse; the workspace is still "provisioning", so nothing
	// in it is worth keeping.
	if _, statErr := os.Stat(ws.Path); statErr == nil {
		logLine("removing incomplete checkout from a previous attempt")
		if rmErr := os.RemoveAll(ws.Path); rmErr != nil {
			return fmt.Errorf("clean incomplete checkout: %w", rmErr)
		}
	}
	err := l.cfg.Git.Provision(ctx, gitrepo.ProvisionRequest{
		URL: r.Source.URL, BaseRef: ws.BaseRef, Branch: ws.Branch, Dir: ws.Path, Log: logLine,
	})
	now := time.Now()
	if err != nil {
		ws.State, ws.Error, ws.LastActiveAt = managedagent.WorkspaceFailed, err.Error(), now
		_ = l.cfg.Stores.Workspaces.UpdateWorkspace(ctx, ws)
		return fmt.Errorf("provision workspace: %w", err)
	}
	ws.State, ws.Error, ws.LastActiveAt = managedagent.WorkspaceReady, "", now
	if err := l.cfg.Stores.Workspaces.UpdateWorkspace(ctx, ws); err != nil {
		return err
	}
	l.append(ctx, managedagent.Event{SessionID: r.Session.ID, Kind: managedagent.EventSystem,
		Text: "workspace ready on branch " + ws.Branch})
	return nil
}

// turn executes one prompt and consumes its event stream.
func (l *Launcher) turn(ctx context.Context, rn *run, r managedagent.Run, prompt string) {
	sess, err := l.cfg.Stores.Sessions.GetSession(ctx, r.Session.ID)
	if err != nil {
		l.log.WithError(err).Warn("session vanished before turn")
		return
	}
	if !sess.Status.IsActive() {
		return
	}

	// A new Claude Code session gets its id up front (--session-id) so the
	// index knows what to --resume even if the first turn dies early.
	resume := sess.CCSessionID != ""
	if !resume {
		sess.CCSessionID = uuid.NewString()
	}
	sess.Status, sess.Error, sess.LastActiveAt = managedagent.SessionRunning, "", time.Now()
	if err := l.cfg.Stores.Sessions.UpdateSession(ctx, sess); err != nil {
		l.log.WithError(err).Warn("failed to mark session running")
	}
	l.append(ctx, managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventStatus, Text: string(managedagent.SessionRunning)})

	var env []string
	var settings string
	if l.cfg.Routing != nil {
		env, settings, err = l.cfg.Routing.Resolve(ctx, r.Environment.CCProfile)
		if err != nil {
			l.append(ctx, managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventError,
				Text: "gateway routing unavailable, running with host defaults: " + err.Error()})
		}
	}
	for k, v := range r.Environment.Env {
		env = append(env, k+"="+v)
	}

	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	handle, err := l.cfg.Agent.Execute(turnCtx, prompt, agentboot.ExecutionOptions{
		ProjectPath:          r.Workspace.AgentCwd,
		OutputFormat:         agentboot.OutputFormatStreamJSON,
		SessionID:            sess.CCSessionID,
		Resume:               resume,
		PermissionPromptTool: "stdio",
		PermissionMode:       sess.PermissionMode,
		Env:                  env,
		SettingsPath:         settings,
		Timeout:              l.cfg.TurnTimeout,
		ControlMetadata:      map[string]string{"managed_session_id": sess.ID},
	})
	if err != nil {
		l.setStatus(ctx, sess.ID, managedagent.SessionFailed, "start agent: "+err.Error())
		return
	}
	rn.mu.Lock()
	rn.handle = handle
	rn.mu.Unlock()
	defer func() {
		rn.mu.Lock()
		rn.handle = nil
		rn.pending = map[string]pendingKind{}
		rn.mu.Unlock()
	}()

	conv := newConverter(sess.ID)
	for ev := range handle.Events() {
		switch e := ev.(type) {
		case agentboot.MessageEvent:
			for _, out := range conv.message(e.Raw) {
				l.append(ctx, out)
			}
		case agentboot.ApprovalRequestEvent:
			payload, _ := json.Marshal(map[string]any{"tool": e.ToolName, "input": e.Input, "reason": e.Reason})
			rn.mu.Lock()
			rn.pending[e.ID] = pendingApproval
			rn.mu.Unlock()
			l.append(ctx, managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventApprovalRequest,
				RequestID: e.ID, Text: e.ToolName, Payload: payload})
			l.setStatus(ctx, sess.ID, managedagent.SessionWaitingInput, "")
		case agentboot.AskRequestEvent:
			payload, _ := json.Marshal(map[string]any{"tool": e.ToolName, "input": e.Input, "message": e.Message})
			rn.mu.Lock()
			rn.pending[e.ID] = pendingAsk
			rn.mu.Unlock()
			l.append(ctx, managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventAskRequest,
				RequestID: e.ID, Text: e.Message, Payload: payload})
			l.setStatus(ctx, sess.ID, managedagent.SessionWaitingInput, "")
		case agentboot.ErrorEvent:
			if e.Err != nil {
				l.append(ctx, managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventError, Text: e.Err.Error()})
			}
		}
	}
	res, werr := handle.Wait()

	// Finish: fold usage and the change count into the index. The result
	// message is terminal for the transport and never reaches the stream, so
	// usage comes from the aggregated Result.
	usage, resultErr := foldResult(res)
	if resultErr != "" {
		l.append(ctx, managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventError, Text: resultErr})
	}
	sess, err = l.cfg.Stores.Sessions.GetSession(ctx, r.Session.ID)
	if err != nil {
		return
	}
	sess.Usage.InputTokens += usage.InputTokens
	sess.Usage.OutputTokens += usage.OutputTokens
	sess.Usage.CacheReadTokens += usage.CacheReadTokens
	sess.Usage.Cost += usage.Cost
	if d, derr := l.cfg.Git.Diff(ctx, r.Workspace.Path, r.Workspace.BaseRef); derr == nil {
		sess.Artifact.Changed = d.ChangedFiles
	}
	sess.LastActiveAt = time.Now()
	switch {
	case sess.Status == managedagent.SessionArchived:
		// Archived mid-turn (Stop cancelled us); leave the status alone.
	case werr != nil && !errors.Is(turnCtx.Err(), context.Canceled):
		sess.Status, sess.Error = managedagent.SessionFailed, werr.Error()
	default:
		sess.Status, sess.Error = managedagent.SessionIdle, ""
	}
	if err := l.cfg.Stores.Sessions.UpdateSession(ctx, sess); err != nil {
		l.log.WithError(err).Warn("failed to finish turn")
	}
	msg := string(sess.Status)
	if sess.Error != "" {
		msg += ": " + sess.Error
	}
	l.append(ctx, managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventStatus, Text: msg})
}

// foldResult extracts usage and an error string from the terminal result
// event(s) of a turn.
func foldResult(res *agentboot.Result) (managedagent.Usage, string) {
	var u managedagent.Usage
	var errText string
	if res == nil {
		return u, ""
	}
	num := func(m map[string]any, k string) int64 {
		if f, ok := m[k].(float64); ok {
			return int64(f)
		}
		return 0
	}
	for _, ev := range res.Events {
		if ev.Type != claude.SDKResultMessage {
			continue
		}
		if usage, ok := ev.Data["usage"].(map[string]any); ok {
			u.InputTokens += num(usage, "input_tokens")
			u.OutputTokens += num(usage, "output_tokens")
			u.CacheReadTokens += num(usage, "cache_read_input_tokens")
		}
		if cost, ok := ev.Data["total_cost_usd"].(float64); ok {
			u.Cost += cost
		}
		if isErr, _ := ev.Data["is_error"].(bool); isErr {
			if txt, _ := ev.Data["result"].(string); txt != "" {
				errText = txt
			}
		}
	}
	return u, errText
}

func (l *Launcher) setStatus(ctx context.Context, sessionID string, st managedagent.SessionStatus, errMsg string) {
	sess, err := l.cfg.Stores.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	if sess.Status == managedagent.SessionArchived {
		return
	}
	sess.Status, sess.Error, sess.LastActiveAt = st, errMsg, time.Now()
	if st == managedagent.SessionFailed && sess.FinishedAt == nil {
		now := sess.LastActiveAt
		sess.FinishedAt = &now
	}
	if err := l.cfg.Stores.Sessions.UpdateSession(ctx, sess); err != nil {
		l.log.WithError(err).Warn("failed to update session status")
	}
	text := string(st)
	if errMsg != "" {
		text += ": " + errMsg
	}
	l.append(ctx, managedagent.Event{SessionID: sessionID, Kind: managedagent.EventStatus, Text: text})
}

func (l *Launcher) append(ctx context.Context, e managedagent.Event) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if err := l.cfg.Stores.Events.AppendEvent(ctx, &e); err != nil {
		l.log.WithError(err).WithField("session", e.SessionID).Warn("failed to append event")
	}
}
