package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
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

// noApprovalModes are permission modes that auto-approve every tool request
// without going through IMPrompter.
var noApprovalModes = map[string]bool{
	string(claude.PermissionModeAuto):              true,
	string(claude.PermissionModeBypassPermissions): true,
	string(claude.PermissionModeDontAsk):           true,
	string(claude.PermissionModePlan):              true,
}

// autoApprovePrompter wraps a Prompter to auto-approve every tool permission
// request (for permission modes like plan/bypass) while still deferring
// AskUserQuestion prompts to the underlying prompter.
type autoApprovePrompter struct{ inner agentboot.Prompter }

func (p autoApprovePrompter) OnApproval(context.Context, agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	return agentboot.PermissionResult{Approved: true}, nil
}

func (p autoApprovePrompter) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	return p.inner.OnAsk(ctx, req)
}

// Execute processes a user message through Claude Code.
func (e *ClaudeCodeExecutor) Execute(ctx context.Context, req PreparedRequest) (*ExecutionResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		e.deps.SendText(req.HCtx, "Please provide a message for Claude Code.")
		return nil, fmt.Errorf("empty message text")
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

	statusMsg := "⏳ CC: Processing new session..."
	if !req.IsNewSession {
		statusMsg = "⏳ CC: Resuming session..."
	}
	e.deps.SendTextWithReply(req.HCtx, e.deps.FormatResponseWithFooter(*meta, statusMsg), req.ReplyTo)

	shouldResume := !req.IsNewSession
	permissionMode := req.PermissionMode
	if permissionMode == "" {
		permissionMode = string(claude.PermissionModeDefault)
	}
	autoApprove := noApprovalModes[permissionMode]

	logrus.WithFields(logrus.Fields{
		"chatID":         req.HCtx.ChatID,
		"sessionID":      sessionID,
		"projectPath":    projectPath,
		"shouldResume":   shouldResume,
		"permissionMode": permissionMode,
	}).Info("Starting Claude Code execution")

	streamWriter := e.deps.NewStreamingMessageHandler(req.HCtx, meta)

	// Route the Claude Code CLI through the tingly-box gateway so it uses the
	// configured provider (including third-party model services) instead of
	// talking to the Anthropic API directly. Without this, @cc fails whenever
	// no direct Anthropic credentials are present on the host.
	var execEnv []string
	if e.deps.TBClient != nil {
		ccEnv, eerr := e.deps.TBClient.GetClaudeCodeEnv(ctx)
		if eerr != nil {
			logrus.WithError(eerr).Warn("ClaudeCodeExecutor: failed to resolve gateway env; @cc may not reach the configured provider")
		} else {
			execEnv = ccEnv
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
		if mErr := streamWriter.OnMessage(raw); mErr != nil {
			streamWriter.OnError(mErr)
		}
	}

	startTime := time.Now()
	result, werr := e.deps.AgentService.Run(ctx, agentboot.RunRequest{
		ProjectPath: projectPath,
		Prompt:      req.Text,
		Opts: agentboot.ExecutionOptions{
			SessionID:            sessionID,
			Resume:               shouldResume,
			ChatID:               req.HCtx.ChatID,
			Platform:             string(req.HCtx.Platform),
			BotUUID:              req.HCtx.BotUUID,
			PermissionPromptTool: "stdio",
			PermissionMode:       permissionMode,
			Env:                  execEnv,
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

	response := ""
	if result != nil {
		response = result.TextOutput()
	}

	if werr != nil {
		errMsg := werr.Error()
		if response == "" {
			response = fmt.Sprintf("Execution failed: %v", werr)
		}
		if strings.Contains(errMsg, "Session ID") && strings.Contains(errMsg, "already in use") {
			response = fmt.Sprintf("⚠️ Session ID conflict: This session is already active in another Claude Code process.\n\nSession ID: %s\n\nPossible solutions:\n• Wait for the other session to complete\n• Use /stop to end the current session and try again\n• If the other process is stuck, terminate it manually", sessionID)
		}
		// Runner already called Store.SetFailed inside Wait(); send the user-facing message.
		e.deps.SendTextWithReply(req.HCtx, response, req.ReplyTo)
		return &ExecutionResult{
			SessionID:    sessionID,
			Success:      false,
			Error:        werr,
			Response:     response,
			Meta:         meta,
			IsNewSession: req.IsNewSession,
			Duration:     duration,
		}, werr
	}

	// Success: runner called Store.SetCompleted inside Wait(); send the "Task done" card.
	e.sendTaskDoneCard(req.HCtx, meta)

	return &ExecutionResult{
		SessionID:    sessionID,
		Success:      true,
		Response:     response,
		Meta:         meta,
		IsNewSession: req.IsNewSession,
		Duration:     duration,
	}, nil
}

// sendTaskDoneCard emits the "Task done" action keyboard at the end of a
// successful execution. Replaces the legacy CompletionCallback.OnComplete.
func (e *ClaudeCodeExecutor) sendTaskDoneCard(hCtx HandlerContext, meta *ResponseMeta) {
	kb := feature.BuildActionKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())
	actionCard := feature.BuildActionCard()

	doneText := IconDone + " " + MsgTaskDone + ". " + MsgContinueOrHelp + BuildFooter(meta.AgentType, meta.ProjectPath)
	if _, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:     doneText,
		Metadata: buildTrackedActionMenuMetadata(hCtx, tgKeyboard, actionCard),
	}); err != nil {
		logrus.WithError(err).Warn("ClaudeCodeExecutor: failed to send Task done card")
	}
}
