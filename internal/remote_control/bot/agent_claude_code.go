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

	agent, err := e.deps.AgentBoot.GetDefaultAgent()
	if err != nil {
		e.deps.SessionMgr.SetFailed(sessionID, "agent not available: "+err.Error())
		e.deps.SendTextWithReply(req.HCtx, "Agent not available", req.ReplyTo)
		return &ExecutionResult{
			SessionID: sessionID,
			Success:   false,
			Error:     err,
			Meta:      meta,
		}, err
	}

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

	startTime := time.Now()
	handle, err := agent.Execute(ctx, req.Text, agentboot.ExecutionOptions{
		ProjectPath:          projectPath,
		SessionID:            sessionID,
		Resume:               shouldResume,
		ChatID:               req.HCtx.ChatID,
		Platform:             string(req.HCtx.Platform),
		BotUUID:              req.HCtx.BotUUID,
		PermissionPromptTool: "stdio",
		PermissionMode:       permissionMode,
		Env:                  execEnv,
		Store:                e.deps.SessionMgr,
	})
	if err != nil {
		duration := time.Since(startTime)
		// Runner already called Store.SetFailed; send the user-facing message.
		e.deps.SendTextWithReply(req.HCtx, fmt.Sprintf("Execution failed: %v", err), req.ReplyTo)
		return &ExecutionResult{
			SessionID:    sessionID,
			Success:      false,
			Error:        err,
			Response:     err.Error(),
			Meta:         meta,
			IsNewSession: req.IsNewSession,
			Duration:     duration,
		}, err
	}

	for ev := range handle.Events() {
		switch e2 := ev.(type) {
		case agentboot.MessageEvent:
			if mErr := streamWriter.OnMessage(e2.Raw); mErr != nil {
				streamWriter.OnError(mErr)
			}

		case agentboot.ApprovalRequestEvent:
			if autoApprove {
				_ = handle.Respond(e2.ID, agentboot.ApprovalResponse{Approved: true})
				continue
			}
			permReq := agentboot.PermissionRequest{
				RequestID: e2.ID,
				AgentType: e2.AgentType,
				ToolName:  e2.ToolName,
				Input:     e2.Input,
				Reason:    e2.Reason,
				SessionID: e2.SessionID,
				BotUUID:   e2.BotUUID,
				ChatID:    e2.ChatID,
				Platform:  e2.Platform,
			}
			res, perr := e.deps.IMPrompter.OnApproval(ctx, permReq)
			if perr != nil {
				logrus.WithError(perr).Warn("ClaudeCodeExecutor: IMPrompter.OnApproval error; denying")
				res = agentboot.PermissionResult{Approved: false, Reason: perr.Error()}
			}
			_ = handle.Respond(e2.ID, agentboot.ApprovalResponse{
				Approved:     res.Approved,
				UpdatedInput: res.UpdatedInput,
				Reason:       res.Reason,
			})

		case agentboot.AskRequestEvent:
			askReq := agentboot.AskRequest{
				ID:        e2.ID,
				Type:      e2.Type,
				AgentType: e2.AgentType,
				Platform:  e2.Platform,
				ChatID:    e2.ChatID,
				BotUUID:   e2.BotUUID,
				SessionID: e2.SessionID,
				ToolName:  e2.ToolName,
				Input:     e2.Input,
				CallID:    e2.CallID,
				Message:   e2.Message,
				Reason:    e2.Reason,
			}
			res, aerr := e.deps.IMPrompter.OnAsk(ctx, askReq)
			if aerr != nil {
				logrus.WithError(aerr).Warn("ClaudeCodeExecutor: IMPrompter.OnAsk error; denying")
				res = agentboot.AskResult{ID: e2.ID, Approved: false, Reason: aerr.Error()}
			}
			_ = handle.Respond(e2.ID, agentboot.AskResponse{
				Approved:     res.Approved,
				UpdatedInput: res.UpdatedInput,
				Reason:       res.Reason,
				Response:     res.Response,
				Selection:    res.Selection,
			})

		case agentboot.ErrorEvent:
			streamWriter.OnError(e2.Err)
		}
	}

	result, werr := handle.Wait()
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
