package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
)

// IMPrompter implements permission.UserPrompter using IM (Telegram, etc.) for user interaction
type IMPrompter struct {
	mu sync.RWMutex

	// manager is the IM bot manager for sending messages
	manager *imbot.Manager

	// pendingRequests stores permission requests waiting for user response
	// key: requestID, value: *pendingPermissionRequest
	pendingRequests map[string]*pendingPermissionRequest

	// responseChannels stores channels for sending responses back to waiting goroutines
	// key: requestID, value: channel for permission response
	responseChannels map[string]chan permissionResponse

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

// permissionResponse represents a user's response to a permission request
type permissionResponse struct {
	approved bool
	remember bool
	reason   string
}

// NewIMPrompter creates a new IM-based permission prompter
func NewIMPrompter(manager *imbot.Manager) *IMPrompter {
	return &IMPrompter{
		manager:          manager,
		pendingRequests:  make(map[string]*pendingPermissionRequest),
		responseChannels: make(map[string]chan permissionResponse),
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
func (p *IMPrompter) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (bool, bool, error) {
	// Get the chat context from the request
	chatID, platform := p.getChatContextFromRequest(req)
	if chatID == "" {
		// No chat context, auto-approve
		logrus.WithField("request_id", req.RequestID).Debug("No chat context for permission request, auto-approving")
		return true, false, nil
	}

	bot := p.manager.GetBot(platform)
	if bot == nil {
		logrus.WithFields(logrus.Fields{
			"request_id": req.RequestID,
			"platform":   platform,
		}).Warn("Bot not found for platform, auto-approving")
		return true, false, nil
	}

	// Create response channel
	responseChan := make(chan permissionResponse, 1)

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

	// Build and send permission prompt message
	promptText := p.buildPromptText(req)
	keyboard := p.buildPermissionKeyboard(req.RequestID)

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
		return false, false, fmt.Errorf("failed to send permission prompt: %w", err)
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
	case response := <-responseChan:
		p.cleanup(req.RequestID)
		return response.approved, response.remember, nil

	case <-time.After(timeout):
		p.cleanup(req.RequestID)
		p.editPromptToTimeout(bot, chatID, msg.MessageID, req)
		return false, false, fmt.Errorf("permission request timed out")

	case <-ctx.Done():
		p.cleanup(req.RequestID)
		return false, false, ctx.Err()
	}
}

// SubmitDecision submits a user's decision for a pending permission request
// This is called when a user clicks a button in response to a permission prompt
func (p *IMPrompter) SubmitDecision(requestID string, approved bool, remember bool, reason string) error {
	// Prepare response before acquiring lock
	resp := permissionResponse{
		approved: approved,
		remember: remember,
		reason:   reason,
	}

	p.mu.Lock()
	responseChan, exists := p.responseChannels[requestID]
	if !exists {
		p.mu.Unlock()
		return fmt.Errorf("permission request not found or expired: %s", requestID)
	}
	// Keep lock held while sending to prevent race with cleanup
	// Channel send with buffer size 1 is non-blocking
	select {
	case responseChan <- resp:
		p.mu.Unlock()
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"approved":   approved,
			"remember":   remember,
		}).Info("Permission decision submitted")
		return nil
	default:
		p.mu.Unlock()
		return fmt.Errorf("response channel full for request: %s", requestID)
	}
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

// buildPromptText builds the permission prompt message text
func (p *IMPrompter) buildPromptText(req agentboot.PermissionRequest) string {
	text := fmt.Sprintf("🔐 *Tool Permission Request*\n\n")
	text += fmt.Sprintf("Tool: `%s`\n", req.ToolName)

	// Show relevant input details
	if cmd, ok := req.Input["command"].(string); ok && cmd != "" {
		text += fmt.Sprintf("Command: `%s`\n", truncateText(cmd, 200))
	} else if filePath, ok := req.Input["file_path"].(string); ok && filePath != "" {
		text += fmt.Sprintf("File: `%s`\n", filePath)
	}

	if req.Reason != "" {
		text += fmt.Sprintf("\nReason: %s\n", req.Reason)
	}

	text += "\n━━━━━━━━━━━━━━━━━━━━\n"
	text += "*Reply to confirm:*\n"
	text += "• `1`, `y`, `yes` → Allow\n"
	text += "• `0`, `n`, `no` → Deny\n"
	text += "• `a`, `always` → Always allow"

	return text
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
func (p *IMPrompter) buildPermissionKeyboard(requestID string) imbot.InlineKeyboardMarkup {
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
