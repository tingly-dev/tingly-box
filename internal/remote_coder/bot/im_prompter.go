package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/permission"
	"github.com/tingly-dev/tingly-box/imbot"
)

// IMPrompter implements permission.UserPrompter using IM (Telegram, etc.) for user interaction
type IMPrompter struct {
	mu sync.RWMutex

	// manager is the IM bot manager for sending messages
	manager *imbot.Manager

	// registry is the tool handler registry for customizing prompts and responses
	registry *permission.ToolHandlerRegistry

	// pendingRequests stores permission requests waiting for user response
	// key: requestID, value: *pendingPermissionRequest
	pendingRequests map[string]*pendingPermissionRequest

	// responseChannels stores channels for sending responses back to waiting goroutines
	// key: requestID, value: channel for permission result
	responseChannels map[string]chan agentboot.PermissionResult

	// defaultTimeout is the default timeout for permission requests
	defaultTimeout time.Duration
}

// pendingPermissionRequest stores a pending permission request with its context
type pendingPermissionRequest struct {
	request   agentboot.PermissionRequest
	chatID    string
	platform  imbot.Platform
	messageID string
	createdAt time.Time
}

// NewIMPrompter creates a new IM-based permission prompter
func NewIMPrompter(manager *imbot.Manager) *IMPrompter {
	return &IMPrompter{
		manager:          manager,
		registry:         permission.NewToolHandlerRegistry(),
		pendingRequests:  make(map[string]*pendingPermissionRequest),
		responseChannels: make(map[string]chan agentboot.PermissionResult),
		defaultTimeout:   5 * time.Minute,
	}
}

// SetDefaultTimeout sets the default timeout for permission requests
func (p *IMPrompter) SetDefaultTimeout(timeout time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.defaultTimeout = timeout
}

// PromptPermission prompts the user via IM for permission decision
// This implements the permission.UserPrompter interface
func (p *IMPrompter) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	// Get the chat context from the request
	chatID, platform := p.getChatContextFromRequest(req)
	if chatID == "" {
		// No chat context, auto-approve
		logrus.WithField("request_id", req.RequestID).Debug("No chat context for permission request, auto-approving")
		return agentboot.PermissionResult{
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	bot := p.manager.GetBot(platform)
	if bot == nil {
		logrus.WithFields(logrus.Fields{
			"request_id": req.RequestID,
			"platform":   platform,
		}).Warn("Bot not found for platform, auto-approving")
		return agentboot.PermissionResult{
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	// Create response channel
	responseChan := make(chan agentboot.PermissionResult, 1)

	p.mu.Lock()
	p.pendingRequests[req.RequestID] = &pendingPermissionRequest{
		request:   req,
		chatID:    chatID,
		platform:  platform,
		createdAt: time.Now(),
	}
	p.responseChannels[req.RequestID] = responseChan
	timeout := p.defaultTimeout
	p.mu.Unlock()

	// Build prompt using tool handler
	promptText := p.buildPromptText(req)
	keyboard := p.buildPermissionKeyboard(req)

	// Send the prompt message
	msg, err := bot.SendMessage(context.Background(), chatID, &imbot.SendMessageOptions{
		Text:      promptText,
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup": convertActionKeyboardToTelegram(keyboard),
		},
	})
	if err != nil {
		p.cleanup(req.RequestID)
		logrus.WithError(err).WithField("request_id", req.RequestID).Error("Failed to send permission prompt")
		return agentboot.PermissionResult{Approved: false}, fmt.Errorf("failed to send permission prompt: %w", err)
	}

	// Store message ID for potential editing
	p.mu.Lock()
	if pending, exists := p.pendingRequests[req.RequestID]; exists {
		pending.messageID = msg.MessageID
	}
	p.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"request_id": req.RequestID,
		"chat_id":    chatID,
		"tool_name":  req.ToolName,
	}).Info("Permission prompt sent, waiting for response")

	// Wait for response or timeout
	select {
	case result := <-responseChan:
		p.cleanup(req.RequestID)
		return result, nil

	case <-time.After(timeout):
		p.cleanup(req.RequestID)
		p.editPromptToTimeout(bot, chatID, msg.MessageID, req)
		return agentboot.PermissionResult{
			Approved: false,
			Reason:   "permission request timed out",
		}, fmt.Errorf("permission request timed out")

	case <-ctx.Done():
		p.cleanup(req.RequestID)
		return agentboot.PermissionResult{Approved: false}, ctx.Err()
	}
}

// SubmitDecision submits a user's decision for a pending permission request
// This is called when a user clicks a button in response to a permission prompt
// Deprecated: Use SubmitResult instead for structured responses
func (p *IMPrompter) SubmitDecision(requestID string, approved bool, remember bool, reason string) error {
	result := agentboot.PermissionResult{
		Approved: approved,
		Remember: remember,
		Reason:   reason,
	}
	return p.SubmitResult(requestID, result)
}

// SubmitResult submits a permission result for a pending permission request
// This is the preferred method for structured responses
func (p *IMPrompter) SubmitResult(requestID string, result agentboot.PermissionResult) error {
	p.mu.Lock()
	responseChan, exists := p.responseChannels[requestID]
	if !exists {
		p.mu.Unlock()
		return fmt.Errorf("permission request not found or expired: %s", requestID)
	}
	// Keep lock held while sending to prevent race with cleanup
	// Channel send with buffer size 1 is non-blocking
	select {
	case responseChan <- result:
		p.mu.Unlock()
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"approved":   result.Approved,
			"remember":   result.Remember,
		}).Info("Permission result submitted")
		return nil
	default:
		p.mu.Unlock()
		return fmt.Errorf("response channel full for request: %s", requestID)
	}
}

// SubmitUserResponse submits a user response for a pending permission request
// This parses the response using the appropriate tool handler
func (p *IMPrompter) SubmitUserResponse(requestID string, response permission.UserResponse) error {
	// Get the pending request
	p.mu.RLock()
	pending, exists := p.pendingRequests[requestID]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("permission request not found or expired: %s", requestID)
	}

	// Find the appropriate handler and parse the response
	parser := p.registry.FindResponseParser(pending.request.ToolName, pending.request.Input)
	if parser == nil {
		return fmt.Errorf("no handler found for tool: %s", pending.request.ToolName)
	}

	result, err := parser.ParseResponse(pending.request, response)
	if err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return p.SubmitResult(requestID, result)
}

// GetPendingRequest returns a pending permission request by ID
func (p *IMPrompter) GetPendingRequest(requestID string) (*agentboot.PermissionRequest, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if pending, exists := p.pendingRequests[requestID]; exists {
		return &pending.request, true
	}
	return nil, false
}

// GetPendingRequestsForChat returns all pending requests for a specific chat
func (p *IMPrompter) GetPendingRequestsForChat(chatID string) []agentboot.PermissionRequest {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var requests []agentboot.PermissionRequest
	for _, pending := range p.pendingRequests {
		if pending.chatID == chatID {
			requests = append(requests, pending.request)
		}
	}
	return requests
}

// buildPromptText builds the permission prompt message text using tool handlers
func (p *IMPrompter) buildPromptText(req agentboot.PermissionRequest) string {
	// Try to use tool-specific prompt builder
	builder := p.registry.FindPromptBuilder(req.ToolName, req.Input)
	if builder != nil {
		return builder.BuildPrompt(req)
	}

	// Fallback to default prompt
	return fmt.Sprintf("🔐 *Tool Permission Request*\n\nTool: `%s`", req.ToolName)
}

// ParseTextResponse parses user text input as a permission response
// Returns: (approved, remember, isValid)
func ParseTextResponse(text string) (approved bool, remember bool, isValid bool) {
	// Normalize input
	text = normalizeText(text)

	switch text {
	case "1", "y", "yes":
		return true, false, true
	case "0", "n", "no":
		return false, false, true
	case "a", "always":
		return true, true, true
	default:
		return false, false, false
	}
}

// normalizeText normalizes user input for comparison
func normalizeText(text string) string {
	// Trim whitespace and convert to lowercase
	text = strings.TrimSpace(text)
	text = strings.ToLower(text)
	return text
}

// buildPermissionKeyboard builds the inline keyboard for permission prompts
// This uses tool-specific keyboards when available
func (p *IMPrompter) buildPermissionKeyboard(req agentboot.PermissionRequest) imbot.InlineKeyboardMarkup {
	// Check if this is AskUserQuestion - build option keyboard
	if req.ToolName == "AskUserQuestion" {
		return p.buildAskUserQuestionKeyboard(req)
	}

	// Default keyboard: Approve/Deny/Always
	return p.buildDefaultKeyboard(req.RequestID)
}

// buildDefaultKeyboard builds the default allow/deny keyboard
func (p *IMPrompter) buildDefaultKeyboard(requestID string) imbot.InlineKeyboardMarkup {
	kb := imbot.NewKeyboardBuilder()

	// First row: Approve and Deny buttons
	kb.AddRow(
		imbot.CallbackButton("✅ Allow", imbot.FormatCallbackData("perm", "allow", requestID)),
		imbot.CallbackButton("❌ Deny", imbot.FormatCallbackData("perm", "deny", requestID)),
	)

	// Second row: Always allow (remember decision)
	kb.AddRow(
		imbot.CallbackButton("🔄 Always Allow", imbot.FormatCallbackData("perm", "always", requestID)),
	)

	return kb.Build()
}

// buildAskUserQuestionKeyboard builds a keyboard with options for AskUserQuestion
func (p *IMPrompter) buildAskUserQuestionKeyboard(req agentboot.PermissionRequest) imbot.InlineKeyboardMarkup {
	kb := imbot.NewKeyboardBuilder()

	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return p.buildDefaultKeyboard(req.RequestID)
	}

	// Build buttons for each option in the first question
	if question, ok := questions[0].(map[string]interface{}); ok {
		if options, ok := question["options"].([]interface{}); ok {
			for i, opt := range options {
				if option, ok := opt.(map[string]interface{}); ok {
					label, _ := option["label"].(string)
					if label != "" {
						// Use option index as callback data
						buttonText := fmt.Sprintf("%d️⃣ %s", i+1, label)
						callbackData := imbot.FormatCallbackData("perm", "option", req.RequestID, fmt.Sprintf("%d", i))
						kb.AddRow(imbot.CallbackButton(buttonText, callbackData))
					}
				}
			}
		}
	}

	// Add cancel button
	kb.AddRow(imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("perm", "deny", req.RequestID)))

	return kb.Build()
}

// editPromptToResult edits the permission prompt message to show the result
func (p *IMPrompter) editPromptToResult(bot imbot.Bot, chatID, messageID string, req agentboot.PermissionRequest, approved bool) {
	resultText := p.buildPromptText(req)
	if approved {
		resultText += "\n\n✅ *Approved*"
	} else {
		resultText += "\n\n❌ *Denied*"
	}

	// Edit message to remove keyboard and show result
	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		// Use Telegram-specific editing with empty keyboard to remove buttons
		_ = tgBot.EditMessageWithKeyboard(context.Background(), chatID, messageID, resultText, nil)
	} else {
		// Fallback: send a new message with the result
		_, _ = bot.SendMessage(context.Background(), chatID, &imbot.SendMessageOptions{
			Text:      resultText,
			ParseMode: imbot.ParseModeMarkdown,
		})
	}
}

// editPromptToTimeout edits the permission prompt message to show timeout
func (p *IMPrompter) editPromptToTimeout(bot imbot.Bot, chatID, messageID string, req agentboot.PermissionRequest) {
	resultText := p.buildPromptText(req)
	resultText += "\n\n⏰ *Timed Out*"

	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		_ = tgBot.EditMessageWithKeyboard(context.Background(), chatID, messageID, resultText, nil)
	}
}

// getChatContextFromRequest extracts chat context from a permission request
func (p *IMPrompter) getChatContextFromRequest(req agentboot.PermissionRequest) (string, imbot.Platform) {
	// Try to get chat context from the request's metadata
	if chatID, ok := req.Input["_chat_id"].(string); ok {
		platform := imbot.PlatformTelegram
		if p, ok := req.Input["_platform"].(string); ok {
			platform = imbot.Platform(p)
		}
		return chatID, platform
	}
	return "", ""
}

// cleanup removes a pending request and its response channel
func (p *IMPrompter) cleanup(requestID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.pendingRequests, requestID)
	delete(p.responseChannels, requestID)
}

// truncateText truncates text to maxLen with ellipsis
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}
