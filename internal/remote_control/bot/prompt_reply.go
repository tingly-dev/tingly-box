package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
)

// promptReplyRouter builds the bot-host-level inbound handler that routes the
// user's answers back to the bot's shared channel prompter. The host runs it
// BEFORE any consumer: the "perm" callback namespace and pending-answer texts
// belong entirely to the one prompter, so an unknown "perm" request ID is
// claimed and reported as expired rather than falling through.
func promptReplyRouter(mgr *imbot.Manager, prompter *imchannel.IMPrompter) OnMessage {
	return func(msg imbot.Message, platform imbot.Platform, botUUID string) bool {
		chatID := msg.GetReplyTarget()
		if chatID == "" {
			return false
		}

		send := func(text string) {
			bot := mgr.GetBot(botUUID, platform)
			if bot == nil {
				return
			}
			opts := &imbot.SendMessageOptions{
				Text:      text,
				ParseMode: imbot.ParseModeMarkdown,
			}
			ForwardReplyContext(opts, msg)
			_, _ = bot.SendMessage(context.Background(), chatID, opts)
		}

		if msg.IsCallback() {
			if msg.Payload.Name() != "perm" {
				return false
			}
			return HandlePromptCallback(prompter, send, msg.Sender.ID, msg.Payload, true)
		}

		if !msg.IsTextContent() {
			return false
		}
		// Trimmed for parity with HandlerContext.Text(), which the standalone
		// BotHandler path feeds into the same helper.
		text := strings.TrimSpace(msg.GetText())
		if text == "" {
			return false
		}
		return HandlePromptTextReply(prompter, send, chatID, msg.Sender.ID, text)
	}
}

// This file is the reply-routing mechanism for IMPrompter-backed prompts:
// inline-keyboard "perm" callbacks and plain-text answers. The bot host runs
// promptReplyRouter against the bot's shared channel prompter ahead of every
// consumer, so replies resolve no matter which purposes are mounted; the
// remote-agent BotHandler keeps its own delegating paths for the standalone
// (host-less) mode used by the CLI and the test harness.

// HandlePromptCallback routes one "perm" callback payload (segment 0 ==
// "perm") to the prompter's pending request. send delivers user-facing
// feedback to the originating chat.
//
// claimUnknown controls who answers for an expired/foreign request ID: the
// terminal consumer (remote agent) passes true and tells the user the request
// expired; a first-in-line consumer (channel) passes false so the callback
// falls through to the next consumer, which may own the request.
func HandlePromptCallback(prompter *imchannel.IMPrompter, send func(string), senderID string, payload imbot.Payload, claimUnknown bool) bool {
	subAction := payload.Arg(1)
	requestID := payload.Arg(2)
	if subAction == "" || requestID == "" {
		logrus.WithField("payload", payload).Warn("Invalid permission callback data")
		return claimUnknown
	}

	// Check if the request exists
	pendingReq, exists := prompter.GetPendingRequest(requestID)
	if !exists {
		if !claimUnknown {
			return false
		}
		logrus.WithField("request_id", requestID).Warn("Permission request not found or expired")
		send("⚠️ This permission request has expired or already been answered.")
		return true
	}

	var resultText string

	switch subAction {
	case "noop":
		// Label-only button (e.g., question header), do nothing
		return true

	case "option":
		// Handle multi-option selection (e.g., AskUserQuestion)
		// Payload segments: perm, option, reqID, qIdx, optIdx
		qIdxStr := payload.Arg(3)
		optIdxStr := payload.Arg(4)
		if qIdxStr == "" || optIdxStr == "" {
			logrus.WithField("payload", payload).Warn("Invalid option callback data")
			return true
		}

		// Resolve question text and option label from the pending request
		var questionText, optionLabel string
		if questions, ok := pendingReq.Input["questions"].([]interface{}); ok {
			var qIdx int
			if _, err := fmt.Sscanf(qIdxStr, "%d", &qIdx); err == nil && qIdx >= 0 && qIdx < len(questions) {
				if question, ok := questions[qIdx].(map[string]interface{}); ok {
					questionText, _ = question["question"].(string)
					if options, ok := question["options"].([]interface{}); ok {
						var optIdx int
						if _, err := fmt.Sscanf(optIdxStr, "%d", &optIdx); err == nil && optIdx >= 0 && optIdx < len(options) {
							if option, ok := options[optIdx].(map[string]interface{}); ok {
								optionLabel, _ = option["label"].(string)
							}
						}
					}
				}
			}
		}
		if questionText == "" || optionLabel == "" {
			logrus.WithField("payload", payload).Warn("Could not resolve question or option from callback data")
			send("⚠️ Failed to process selection.")
			return true
		}

		done, err := prompter.SubmitPartialAnswer(requestID, questionText, optionLabel)
		if err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit partial answer")
			send(fmt.Sprintf("Failed to process option selection: %v", err))
			return true
		}

		if done {
			resultText = "✅ All answered"
		} else {
			send(fmt.Sprintf("✅ Q: %s → %s", questionText, optionLabel))
			return true
		}

		logrus.WithFields(logrus.Fields{
			"request_id":   requestID,
			"tool_name":    pendingReq.ToolName,
			"question":     questionText,
			"option_label": optionLabel,
			"user_id":      senderID,
		}).Info("User selected option")

	default:
		// Look up permission action from shared config
		permOpt := ask.FindPermissionByAction(subAction)
		if permOpt == nil {
			logrus.WithField("action", subAction).Warn("Unknown permission action")
			return true
		}

		if err := prompter.SubmitDecision(requestID, permOpt.Approved, permOpt.Remember, permOpt.Label); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit permission decision")
			send(fmt.Sprintf("Failed to process permission response: %v", err))
			return true
		}
		resultText = fmt.Sprintf("%s %s", permOpt.Icon, permOpt.Label)
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"tool_name":  pendingReq.ToolName,
			"action":     subAction,
			"user_id":    senderID,
		}).Info("User responded to permission request")
	}

	// Send feedback to user
	send(fmt.Sprintf("%s for tool: `%s`", resultText, pendingReq.ToolName))
	return true
}

// HandlePromptTextReply routes a plain-text reply to the prompter's most
// recent pending request for chatID. Returns false when nothing is pending in
// that chat or the text is not a recognizable answer, so the caller can hand
// the message to other handlers.
func HandlePromptTextReply(prompter *imchannel.IMPrompter, send func(string), chatID, senderID, input string) bool {
	// Check if there are pending permission requests for this chat
	pendingReqs := prompter.GetPendingRequestsForChat(chatID)
	if len(pendingReqs) == 0 {
		return false
	}

	// Never consume handoff commands (@cc, @tb, @mock, /cc, /tb, /mock) as permission responses
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "@cc") || strings.HasPrefix(trimmed, "@tb") || strings.HasPrefix(trimmed, "@mock") ||
		strings.HasPrefix(trimmed, "/cc") || strings.HasPrefix(trimmed, "/tb") || strings.HasPrefix(trimmed, "/mock") {
		return false
	}

	// Get the most recent pending request for this chat
	// (usually there's only one at a time)
	latestReq := pendingReqs[0]

	// For AskUserQuestion, try to parse as option selection first
	if latestReq.ToolName == "AskUserQuestion" {
		// Try to submit as a text selection
		if err := prompter.SubmitUserResponse(latestReq.ID, ask.Response{
			Type: "text",
			Data: input,
		}); err == nil {
			send(fmt.Sprintf("✅ Selected: %s", input))
			logrus.WithFields(logrus.Fields{
				"request_id": latestReq.ID,
				"tool_name":  latestReq.ToolName,
				"user_id":    senderID,
				"selection":  input,
			}).Info("User selected option via text")
			return true
		}
	}

	// Try to parse the text as a standard permission response
	approved, remember, isValid := ask.ParseTextResponse(input)
	if !isValid {
		// Not a valid permission response, let other handlers process it
		return false
	}

	// Submit the decision
	if err := prompter.SubmitDecision(latestReq.ID, approved, remember, ""); err != nil {
		logrus.WithError(err).WithField("request_id", latestReq.ID).Error("Failed to submit permission decision")
		send(fmt.Sprintf("Failed to process permission response: %v", err))
		return true
	}

	// Send feedback to user
	var resultText string
	if remember {
		resultText = "🔄 Always allowed"
	} else if approved {
		resultText = "✅ Permission granted"
	} else {
		resultText = "❌ Permission denied"
	}

	send(fmt.Sprintf("%s for tool: `%s`", resultText, latestReq.ToolName))

	logrus.WithFields(logrus.Fields{
		"request_id": latestReq.ID,
		"tool_name":  latestReq.ToolName,
		"user_id":    senderID,
		"approved":   approved,
		"remember":   remember,
	}).Info("User responded to permission request via text")

	return true
}

// ForwardReplyContext copies the inbound message's reply-context token onto
// outbound options. Weixin and WeCom tie each reply to the inbound message it
// answers, and drop or misattribute a reply that arrives without the token.
//
// TODO(phase-4): this should not be the caller's job at all — the bot knows
// which inbound message it is answering.
func ForwardReplyContext(opts *imbot.SendMessageOptions, inbound imbot.Message) {
	if inbound.Metadata == nil {
		return
	}
	token, _ := inbound.Metadata["context_token"].(string)
	if token == "" {
		return
	}
	if opts.Metadata == nil {
		opts.Metadata = make(map[string]interface{})
	}
	opts.Metadata["context_token"] = token
}
