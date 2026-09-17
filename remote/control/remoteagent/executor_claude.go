package remoteagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	bot2 "github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/feature"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// ClaudeCodeExecutor executes messages through the Claude Code agent.
//
// It consumes the [agentboot.ExecutionHandle] returned by Agent.Execute
// directly, dispatching MessageEvents to the streaming chat writer and
// routing ApprovalRequestEvent / AskRequestEvent to IMPrompter.
type ClaudeCodeExecutor struct {
	deps *ExecutorDependencies
}

// NewClaudeCodeExecutor creates a new Claude Code executor.
func NewClaudeCodeExecutor(deps *ExecutorDependencies) *ClaudeCodeExecutor {
	return &ClaudeCodeExecutor{deps: deps}
}

// GetAgentType returns the agent type identifier.
func (e *ClaudeCodeExecutor) GetAgentType() agentboot.AgentType {
	return agentClaudeCode
}

// noApprovalModes contains only modes whose contract is unconditional
// approval. Other modes must preserve Claude Code's own deny/plan/classifier
// semantics if a permission callback reaches the host.
var noApprovalModes = map[string]bool{
	string(claude.PermissionModeBypassPermissions): true,
}

// autoApprovePrompter wraps a Prompter to auto-approve every tool permission
// request in bypass mode while still deferring AskUserQuestion prompts to the
// underlying prompter.
type autoApprovePrompter struct{ inner agentboot.Prompter }

func (p autoApprovePrompter) OnApproval(context.Context, agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	return agentboot.ApprovalResponse{Approved: true}, nil
}

func (p autoApprovePrompter) OnAsk(ctx context.Context, req agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	return p.inner.OnAsk(ctx, req)
}

// Execute processes a user message through Claude Code.
func (e *ClaudeCodeExecutor) Execute(ctx context.Context, req PreparedRequest) error {
	if strings.TrimSpace(req.Text) == "" {
		e.deps.SendText(req.HCtx, "Please provide a message for Claude Code.")
		return fmt.Errorf("empty message text")
	}

	sessionID := req.SessionID
	projectPath := req.ProjectPath
	meta := req.Meta

	e.deps.SessionMgr.Update(sessionID, func(s *session.Session) {
		s.LastActivity = time.Now()
	})
	e.deps.SessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   req.Text,
		Timestamp: time.Now(),
	})

	// The bot's default_agent setting decides which Claude Code configuration
	// serves @cc: the main claude_code scenario or a profile
	// ("claude_code:<id>"). Read dynamically so a profile switch in the web UI
	// applies from the next message without a bot restart.
	profileID := ccProfileID(e.deps.GetBotSettingOrCache())

	statusMsg := "⏳ CC: Processing new session..."
	if !req.IsNewSession {
		statusMsg = "⏳ CC: Resuming session..."
	}
	if profileID != "" {
		statusMsg += fmt.Sprintf(" (profile: %s)", profileID)
	}
	e.deps.SendTextWithReply(req.HCtx, statusMsg+BuildFooter(meta.AgentType, meta.ProjectPath), req.ReplyTo)

	shouldResume := !req.IsNewSession
	permissionMode, autoApprove := claudePermissionPolicy(req.PermissionMode)

	logrus.WithFields(logrus.Fields{
		"chatID":         req.HCtx.ChatID,
		"sessionID":      sessionID,
		"projectPath":    projectPath,
		"shouldResume":   shouldResume,
		"permissionMode": permissionMode,
		"ccProfile":      profileID,
	}).Info("Starting Claude Code execution")

	streamWriter := e.deps.NewStreamingMessageHandler(req.HCtx)

	// Route the Claude Code CLI through the tingly-box gateway. Two distinct
	// mechanisms, matching what a local launch does for each case:
	//
	//   - Main scenario (no profile): process env vars (ANTHROPIC_BASE_URL,
	//     etc.). Without this, @cc fails whenever no direct Anthropic
	//     credentials are present on the host.
	//   - A selected profile: the CLI's --settings flag REPLACES
	//     ~/.claude/settings.json rather than merging with it, so a profile's
	//     routing/models/overrides only take effect when its materialized
	//     settings.json is referenced via --settings — exactly what
	//     `tingly-box cc --profile <id>` does locally. Injecting the
	//     profile's values as process env instead is not enough: with no
	//     --settings flag the CLI still reads ~/.claude/settings.json, whose
	//     main-scenario values would silently win.
	//
	// If the profile can no longer be resolved (e.g. deleted in the UI), fall
	// back to the main scenario and tell the user rather than silently
	// running with different routing than they selected.
	var execEnv []string
	var settingsPath string
	if e.deps.TBClient != nil {
		if profileID != "" {
			path, perr := e.deps.TBClient.GetClaudeCodeSettingsPathForProfile(ctx, profileID)
			if perr != nil {
				logrus.WithError(perr).WithField("ccProfile", profileID).Warn("ClaudeCodeExecutor: failed to materialize profile settings; falling back to main claude_code scenario")
				e.deps.SendText(req.HCtx, fmt.Sprintf("⚠️ Claude Code profile '%s' could not be resolved (%v).\nRunning with the default claude_code scenario instead. Pick another profile in the tingly-box web UI (Remote → this bot).", profileID, perr))
			} else {
				settingsPath = path
			}
		}
		if settingsPath == "" {
			ccEnv, eerr := e.deps.TBClient.GetClaudeCodeEnv(ctx)
			if eerr != nil {
				logrus.WithError(eerr).Warn("ClaudeCodeExecutor: failed to resolve gateway env; @cc may not reach the configured provider")
			} else {
				execEnv = ccEnv
			}
		}
	}

	// Drive the run through AgentService.Run: it streams MessageEvent.Raw to the
	// sink and routes Approval/Ask to the prompter. autoApprove modes bypass
	// IMPrompter for permission requests but still defer Ask prompts to it.
	var prompter agentboot.Prompter = e.deps.IMPrompter
	if autoApprove {
		prompter = autoApprovePrompter{inner: e.deps.IMPrompter}
	}
	sink := func(raw any) {
		// Terminal/non-fatal agent errors arrive as ErrorEvent; surface them to
		// the chat instead of letting them die in the server log.
		if ev, ok := raw.(agentboot.ErrorEvent); ok {
			if ev.Err != nil {
				streamWriter.OnError(ev.Err)
			}
			return
		}
		if mErr := streamWriter.OnMessage(raw); mErr != nil {
			streamWriter.OnError(mErr)
		}
	}

	execOpts := agentboot.ExecutionOptions{
		SessionID: sessionID,
		Resume:    shouldResume,
		ControlMetadata: map[string]string{
			claude.ContextKeyChatID:   req.HCtx.ChatID,
			claude.ContextKeyPlatform: string(req.HCtx.Platform),
			claude.ContextKeyBotUUID:  req.HCtx.BotUUID,
		},
		PermissionPromptTool: "stdio",
		PermissionMode:       permissionMode,
		Env:                  execEnv,
		SettingsPath:         settingsPath,
	}

	startTime := time.Now()
	var result *agentboot.Result
	var werr error
	persistentHandled := false
	if e.deps.SessionPool != nil && e.deps.GetBotSettingOrCache().IsPersistentSession() {
		result, werr, persistentHandled = e.runPersistentTurn(ctx, req, projectPath, sessionID, execOpts, prompter, sink)
	}
	if !persistentHandled {
		execOpts.Store = e.deps.SessionMgr
		result, werr = e.deps.AgentService.Run(ctx, agentboot.RunRequest{
			ProjectPath: projectPath,
			Prompt:      req.Text,
			Opts:        execOpts,
		}, prompter, sink)
	}
	duration := time.Since(startTime)
	logrus.WithFields(logrus.Fields{
		"chatID":    req.HCtx.ChatID,
		"sessionID": sessionID,
		"hasError":  werr != nil,
		"duration":  duration,
	}).Info("Claude Code execution completed")

	if werr != nil {
		errMsg := werr.Error()
		response := ""
		if result != nil {
			response = result.TextOutput()
		}
		if response == "" {
			response = fmt.Sprintf("Execution failed: %v", werr)
		}
		if isSessionInUseText(errMsg) {
			response = fmt.Sprintf("⚠️ Session ID conflict: This session is already active in another Claude Code process.\n\nSession ID: %s\n\nPossible solutions:\n• Wait for the other session to complete\n• Use /stop to end the current session and try again\n• If the other process is stuck, terminate it manually", sessionID)
		}
		// The runner marks the session failed once it actually starts; if Run
		// failed earlier (e.g. agent resolution, before the runner ran) nothing
		// did, so mark it here to avoid leaving the session in a non-terminal
		// state. SetFailed is idempotent.
		e.deps.SessionMgr.SetFailed(sessionID, errMsg)
		e.deps.SendTextWithReply(req.HCtx, response, req.ReplyTo)
		return werr
	}

	// Success: the one-shot path's runner calls Store.SetCompleted inside
	// Wait(); the persistent path's runPersistentTurn calls it directly
	// (Runner.Open/PersistentSession never see opts.Store). Either way,
	// SetCompleted has already run — send the "Task done" card.
	sendTaskDoneCard(req.HCtx, meta)

	return nil
}

// runPersistentTurn attempts to run req through the bot's persistent-session
// pool (e.deps.SessionPool) instead of a one-shot process. Callers must
// already have confirmed the bot opted in and the pool is non-nil.
//
// handled=false means the persistent path could not be used for this
// message — no capacity, a stale/crashed entry that was just evicted, or
// the agent doesn't support Open — and the caller should fall back to a
// fresh one-shot AgentService.Run, exactly as if persistent mode were off.
// This is always safe: nothing has been sent to any process yet in the
// handled=false case.
//
// handled=true means the persistent path actually drove this turn to
// completion, successfully or not; result/err are the same shape
// AgentService.Run would have produced. A turn that fails because the
// session terminated mid-turn (a crash) is NOT retried as one-shot here —
// unlike an eviction/idle-timeout, side effects (tool calls) may have
// already run, so silently re-running the prompt could double them up. The
// session is removed from the pool either way so the next message opens a
// fresh one.
func (e *ClaudeCodeExecutor) runPersistentTurn(
	ctx context.Context,
	req PreparedRequest,
	projectPath string,
	sessionID string,
	opts agentboot.ExecutionOptions,
	prompter agentboot.Prompter,
	sink agentboot.MessageSink,
) (result *agentboot.Result, err error, handled bool) {
	poolKey := persistentPoolKey(req.HCtx, projectPath)

	persistentSession, found := e.deps.SessionPool.Acquire(poolKey)
	if !found {
		opened, operr := e.deps.AgentService.Open(ctx, agentClaudeCode, projectPath, req.Text, opts)
		if operr != nil {
			logrus.WithError(operr).WithField("poolKey", poolKey).Info("ClaudeCodeExecutor: could not open persistent session, falling back to one-shot")
			return nil, nil, false
		}
		if perr := e.deps.SessionPool.Open(ctx, poolKey, opened); perr != nil {
			logrus.WithError(perr).WithField("poolKey", poolKey).Info("ClaudeCodeExecutor: could not register persistent session, falling back to one-shot")
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = opened.Close(closeCtx)
			cancel()
			return nil, nil, false
		}
		persistentSession = opened
	} else if serr := persistentSession.Send(ctx, req.Text); serr != nil {
		logrus.WithError(serr).WithField("poolKey", poolKey).Info("ClaudeCodeExecutor: persistent session Send failed, evicting and falling back to one-shot")
		e.deps.SessionPool.Remove(poolKey)
		return nil, nil, false
	}

	// A one-shot run is always bounded by opts.Timeout or the runner's
	// configured default (30 min in production) — Runner.Execute applies it
	// to the whole process. Runner.Open deliberately does not (§5.1: it
	// would kill a session that's legitimately idle between turns), so
	// nothing else bounds a single persistent turn. Apply the same
	// zero/negative/positive semantics as ExecutionOptions.Timeout
	// documents, scoped to just this turn via RunTurnWithPrompter's own
	// ctx.Done() handling (which ends the whole session on timeout, same as
	// a one-shot's process getting killed).
	turnCtx := ctx
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = e.deps.AgentService.Config().DefaultExecutionTimeout
	}
	if timeout > 0 {
		var turnCancel context.CancelFunc
		turnCtx, turnCancel = context.WithTimeout(ctx, timeout)
		defer turnCancel()
	}

	e.deps.SessionMgr.SetRunning(sessionID)
	result, err = agentboot.RunTurnWithPrompter(turnCtx, persistentSession, prompter, sink)
	if err != nil && persistentSession.Status() == agentboot.SessionStateTerminated {
		e.deps.SessionPool.Remove(poolKey)
	} else {
		e.deps.SessionPool.Touch(poolKey)
	}
	if err == nil {
		e.deps.SessionMgr.SetCompleted(sessionID, "")
	}
	return result, err, true
}

// persistentPoolKey identifies a persistent session's slot in the shared,
// process-wide pool. BotUUID alone already uniquely identifies one bot on
// one platform (it's the imbot_settings primary key), so it — not
// platform — is what has to lead the key: EvictPersistentSessionsForBot
// matches on a "<botUUID>|" prefix to drop every session belonging to one
// bot, e.g. when that bot's persistent-session setting is turned off or the
// bot stops (see internal/server/module/imbot's BotManager).
func persistentPoolKey(hCtx HandlerContext, projectPath string) string {
	return strings.Join([]string{hCtx.BotUUID, hCtx.ChatID, projectPath}, "|")
}

// EvictPersistentSessionsForBot closes and removes every persistent session
// belonging to botUUID from sessionPool. Callers: the bot's
// persistent-session setting was turned off (the abandoned process would
// otherwise keep the Claude on-disk session file open while the next
// message resumes it in a separate one-shot process — a
// session-file-conflict race), or the bot is stopping/restarting/being
// deleted. sessionPool may be nil (persistent sessions disabled
// process-wide); a nil pool has nothing to evict.
func EvictPersistentSessionsForBot(sessionPool *pool.Pool, botUUID string) int {
	if sessionPool == nil {
		return 0
	}
	prefix := botUUID + "|"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return sessionPool.CloseAllWhere(ctx, func(key string) bool {
		return strings.HasPrefix(key, prefix)
	})
}

// ccProfileID extracts the Claude Code profile ID from a bot's DefaultAgent
// setting. Returns "" for the main claude_code scenario (unset, "claude_code",
// or a value whose base scenario isn't claude_code).
func ccProfileID(s bot2.BotSetting) string {
	raw := strings.TrimSpace(s.DefaultAgent)
	if raw == "" {
		return ""
	}
	base, profileID := typ.ParseScenarioProfile(typ.RuleScenario(raw))
	if base != typ.ScenarioClaudeCode {
		return ""
	}
	return profileID
}

// claudePermissionPolicy keeps an empty session mode empty so Claude Code can
// inherit defaultMode from the selected settings/profile. A non-empty session
// value is an explicit per-session override (for example /yolo).
func claudePermissionPolicy(sessionMode string) (string, bool) {
	mode := strings.TrimSpace(sessionMode)
	return mode, noApprovalModes[mode]
}

// isSessionInUseText reports whether an error's text is the Claude CLI's
// "session file already in use by another process" complaint. The CLI
// surfaces this only as text (no typed error crosses the agentboot
// boundary), so the one substring match lives here — every session-conflict
// rendering goes through this predicate.
func isSessionInUseText(s string) bool {
	return strings.Contains(s, "Session ID") && strings.Contains(s, "already in use")
}

// sendTaskDoneCard emits the "Task done" action keyboard that closes every
// successful run — Claude Code and SmartGuide alike.
func sendTaskDoneCard(hCtx HandlerContext, meta *ResponseMeta) {
	kb := feature.BuildActionKeyboard()

	opts := &imbot.SendMessageOptions{
		Text:    IconDone + " " + MsgTaskDone + ". " + MsgContinueOrHelp + BuildFooter(meta.AgentType, meta.ProjectPath),
		Actions: kb.BuildActions(),
	}
	bot2.ForwardReplyContext(opts, hCtx.Message)
	if _, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts); err != nil {
		logrus.WithError(err).Warn("Failed to send Task done card")
	}
}
