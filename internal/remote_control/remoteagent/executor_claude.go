package remoteagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/remote_control/feature"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
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

	startTime := time.Now()
	result, werr := e.deps.AgentService.Run(ctx, agentboot.RunRequest{
		ProjectPath: projectPath,
		Prompt:      req.Text,
		Opts: agentboot.ExecutionOptions{
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
			Store:                e.deps.SessionMgr,
		},
	}, prompter, sink)
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

	// Success: runner called Store.SetCompleted inside Wait(); send the "Task done" card.
	sendTaskDoneCard(req.HCtx, meta)

	return nil
}

// ccProfileID extracts the Claude Code profile ID from a bot's DefaultAgent
// setting. Returns "" for the main claude_code scenario (unset, "claude_code",
// or a value whose base scenario isn't claude_code).
func ccProfileID(s bot.BotSetting) string {
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
	bot.ForwardReplyContext(opts, hCtx.Message)
	if _, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts); err != nil {
		logrus.WithError(err).Warn("Failed to send Task done card")
	}
}
