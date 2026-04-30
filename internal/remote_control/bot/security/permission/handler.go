// Package permission provides permission handling for bot commands and agent execution.
// It encapsulates the logic for requesting, tracking, and resolving user permissions.
package permission

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/imbot"
)

// Request represents a permission request
type Request struct {
	ID           string
	ToolName     string
	Input        map[string]interface{}
	Reason       string
	SessionID    string
	ChatID       string
	BotUUID      string
	Platform     imbot.Platform
	RequestedAt  time.Time
	ExpiresAt    time.Time
}

// Response represents a permission response
type Response struct {
	Approved     bool
	Remember     bool
	Reason       string
	RespondedAt  time.Time
}

// Handler handles permission requests and responses
type Handler struct {
	mu sync.RWMutex

	// manager is the IM bot manager for sending messages
	manager *imbot.Manager

	// registry is the tool handler registry for customizing prompts and responses
	registry *ask.ToolHandlerRegistry

	// pendingRequests stores requests waiting for user response
	pendingRequests map[string]*pendingRequest

	// responseChannels stores channels for sending responses back to waiting goroutines
	responseChannels map[string]chan Response

	// defaultTimeout is the default timeout for requests
	defaultTimeout time.Duration

	// whitelist stores tools that have been approved with "Always Allow"
	whitelist map[string]bool
}

// pendingRequest stores a pending request with its context
type pendingRequest struct {
	request        Request
	chatID         string
	platform       imbot.Platform
	messageID      string
	createdAt      time.Time
	partialAnswers map[string]string // question text -> selected label (for multi-question AskUserQuestion)
	totalQuestions int               // total number of questions (set when request is created)
}

// NewHandler creates a new permission handler
func NewHandler(manager *imbot.Manager) *Handler {
	return &Handler{
		manager:          manager,
		registry:         ask.NewToolHandlerRegistry(),
		pendingRequests:  make(map[string]*pendingRequest),
		responseChannels: make(map[string]chan Response),
		defaultTimeout:   5 * time.Minute,
		whitelist:        make(map[string]bool),
	}
}

// SetDefaultTimeout sets the default timeout for requests
func (h *Handler) SetDefaultTimeout(timeout time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.defaultTimeout = timeout
}

// Prompt prompts the user via IM for permission response
func (h *Handler) Prompt(ctx context.Context, req Request) (Response, error) {
	if req.ChatID == "" {
		logrus.WithField("id", req.ID).Debug("No chat context for request, auto-approving")
		return Response{
			Approved:    true,
			RespondedAt: time.Now(),
		}, nil
	}

	// Check whitelist for permission requests
	h.mu.RLock()
	if h.whitelist[req.ToolName] {
		h.mu.RUnlock()
		logrus.WithFields(logrus.Fields{
			"tool_name":  req.ToolName,
			"request_id": req.ID,
		}).Info("Tool is whitelisted, auto-approving")
		return Response{
			Approved:    true,
			Reason:      "Tool is whitelisted (Always Allow)",
			RespondedAt: time.Now(),
		}, nil
	}
	h.mu.RUnlock()

	// Get bot for sending messages
	bot := h.manager.GetBot(req.BotUUID, req.Platform)
	if bot == nil {
		logrus.WithFields(logrus.Fields{
			"id":       req.ID,
			"platform": req.Platform,
			"botUUID":  req.BotUUID,
		}).Warn("Bot not found for request, auto-approving")
		return Response{
			Approved:    true,
			RespondedAt: time.Now(),
		}, nil
	}

	// Create response channel
	responseChan := make(chan Response, 1)

	// Count questions for AskUserQuestion requests
	totalQuestions := 0
	if req.ToolName == "AskUserQuestion" {
		if questions, ok := req.Input["questions"].([]interface{}); ok {
			totalQuestions = len(questions)
		}
	}

	h.mu.Lock()
	h.pendingRequests[req.ID] = &pendingRequest{
		request:        req,
		chatID:         req.ChatID,
		platform:       req.Platform,
		createdAt:      time.Now(),
		partialAnswers: make(map[string]string),
		totalQuestions: totalQuestions,
	}
	h.responseChannels[req.ID] = responseChan
	timeout := h.defaultTimeout
	if req.ExpiresAt.After(time.Now()) {
		timeout = time.Until(req.ExpiresAt)
	}
	h.mu.Unlock()

	// Check if platform supports inline keyboards
	supportsKeyboard := imbot.GetPlatformCapabilities(string(req.Platform)).SupportsInteraction()

	// Build prompt
	promptText := h.buildPromptText(req, supportsKeyboard)
	keyboard := h.buildKeyboard(req)

	// Send the prompt message
	opts := &imbot.SendMessageOptions{
		Text:      promptText,
		ParseMode: imbot.ParseModeMarkdown,
	}
	if supportsKeyboard {
		opts.Metadata = map[string]interface{}{
			"replyMarkup": imbot.BuildTelegramActionKeyboard(keyboard),
		}
	}
	msg, err := bot.SendMessage(context.Background(), req.ChatID, opts)
	if err != nil {
		h.cleanup(req.ID)
		logrus.WithError(err).WithField("id", req.ID).Error("Failed to send prompt")
		return Response{Approved: false}, fmt.Errorf("failed to send prompt: %w", err)
	}

	// Store message ID for potential editing
	h.mu.Lock()
	if pending, exists := h.pendingRequests[req.ID]; exists {
		pending.messageID = msg.MessageID
	}
	h.mu.Unlock()

	// Wait for response or timeout
	select {
	case resp := <-responseChan:
		h.cleanup(req.ID)
		h.editPromptToResult(bot, req.ChatID, msg.MessageID, req, resp.Approved)
		return resp, nil

	case <-time.After(timeout):
		h.cleanup(req.ID)
		h.editPromptToTimeout(bot, req.ChatID, msg.MessageID, req)

		// For AskUserQuestion: auto-select first option
		// For permission/approval requests: deny on timeout
		if req.ToolName == "AskUserQuestion" {
			return h.buildTimeoutQuestionResult(req), nil
		}
		return Response{
			Approved:    false,
			Reason:      "request timed out",
			RespondedAt: time.Now(),
		}, fmt.Errorf("request timed out")

	case <-ctx.Done():
		h.cleanup(req.ID)
		return Response{Approved: false, RespondedAt: time.Now()}, ctx.Err()
	}
}

// SubmitResponse submits a response for a pending request
func (h *Handler) SubmitResponse(requestID string, resp Response) error {
	h.mu.Lock()
	responseChan, exists := h.responseChannels[requestID]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("request not found or expired: %s", requestID)
	}

	// Get pending request for tool name (needed for whitelist)
	pending, hasPending := h.pendingRequests[requestID]

	// If remember is true and approved, add tool to whitelist
	if resp.Remember && resp.Approved && hasPending && pending.request.ToolName != "" {
		h.whitelist[pending.request.ToolName] = true
		logrus.WithFields(logrus.Fields{
			"tool_name":  pending.request.ToolName,
			"request_id": requestID,
		}).Info("Tool added to whitelist (Always Allow)")
	}

	// Mark response time
	resp.RespondedAt = time.Now()

	// Keep lock held while sending to prevent race with cleanup
	select {
	case responseChan <- resp:
		h.mu.Unlock()
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"approved":   resp.Approved,
			"remember":   resp.Remember,
		}).Info("Response submitted")
		return nil
	default:
		h.mu.Unlock()
		return fmt.Errorf("response channel full for request: %s", requestID)
	}
}

// GetPendingRequest returns a pending request by ID
func (h *Handler) GetPendingRequest(requestID string) (*Request, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if pending, exists := h.pendingRequests[requestID]; exists {
		return &pending.request, true
	}
	return nil, false
}

// GetPendingRequestsForChat returns all pending requests for a specific chat
func (h *Handler) GetPendingRequestsForChat(chatID string) []Request {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var requests []Request
	for _, pending := range h.pendingRequests {
		if pending.request.ChatID == chatID {
			requests = append(requests, pending.request)
		}
	}
	return requests
}

// GetWhitelist returns the list of whitelisted tools
func (h *Handler) GetWhitelist() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tools := make([]string, 0, len(h.whitelist))
	for tool := range h.whitelist {
		tools = append(tools, tool)
	}
	return tools
}

// AddToWhitelist adds a tool to the whitelist
func (h *Handler) AddToWhitelist(toolName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.whitelist[toolName] = true
	logrus.WithField("tool_name", toolName).Info("Tool added to whitelist")
}

// RemoveFromWhitelist removes a tool from the whitelist
func (h *Handler) RemoveFromWhitelist(toolName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.whitelist, toolName)
	logrus.WithField("tool_name", toolName).Info("Tool removed from whitelist")
}

// ClearWhitelist clears all tools from the whitelist
func (h *Handler) ClearWhitelist() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.whitelist = make(map[string]bool)
	logrus.Info("Whitelist cleared")
}

// IsWhitelisted checks if a tool is in the whitelist
func (h *Handler) IsWhitelisted(toolName string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.whitelist[toolName]
}

// cleanup removes a pending request and its response channel
func (h *Handler) cleanup(requestID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.pendingRequests, requestID)
	delete(h.responseChannels, requestID)
}

// buildPromptText builds the prompt message text
func (h *Handler) buildPromptText(req Request, supportsKeyboard bool) string {
	// Try to use tool-specific prompt builder
	builder := h.registry.FindPromptBuilder(req.ToolName, req.Input)
	if builder != nil {
		prompt := builder.BuildPrompt(ask.Request{
			ID:       req.ID,
			ToolName: req.ToolName,
			Input:    req.Input,
		})
		logrus.WithFields(logrus.Fields{
			"tool_name":  req.ToolName,
			"prompt_len": len(prompt),
		}).Debug("Built prompt using tool-specific builder")
		return prompt
	}

	// Fallback to default prompt
	logrus.WithField("tool_name", req.ToolName).Debug("No specific builder found, using default prompt")
	return fmt.Sprintf("🔐 *Tool Permission Request*\n\nTool: `%s`", req.ToolName)
}

// buildKeyboard builds the inline keyboard for prompts
func (h *Handler) buildKeyboard(req Request) imbot.InlineKeyboardMarkup {
	// Check if this is AskUserQuestion - build option keyboard
	if req.ToolName == "AskUserQuestion" {
		return h.buildAskUserQuestionKeyboard(req)
	}

	// Default keyboard: Approve/Deny/Always
	return h.buildDefaultKeyboard(req.ID)
}

// buildDefaultKeyboard builds the permission keyboard
func (h *Handler) buildDefaultKeyboard(requestID string) imbot.InlineKeyboardMarkup {
	kb := imbot.NewKeyboardBuilder()

	for _, opt := range ask.PermissionOptions {
		buttonText := opt.Icon + " " + opt.Label
		callbackData := imbot.FormatCallbackData("perm", opt.Action, requestID)
		kb.AddRow(imbot.CallbackButton(buttonText, callbackData))
	}

	return kb.Build()
}

// buildAskUserQuestionKeyboard builds a keyboard with options for AskUserQuestion
func (h *Handler) buildAskUserQuestionKeyboard(req Request) imbot.InlineKeyboardMarkup {
	kb := imbot.NewKeyboardBuilder()

	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return h.buildDefaultKeyboard(req.ID)
	}

	// Build buttons for each question and its options
	for qIdx, q := range questions {
		question, ok := q.(map[string]interface{})
		if !ok {
			continue
		}
		options, ok := question["options"].([]interface{})
		if !ok || len(options) == 0 {
			continue
		}
		// Add a label row for the question if there are multiple questions
		if len(questions) > 1 {
			questionText, _ := question["question"].(string)
			kb.AddRow(imbot.CallbackButton(fmt.Sprintf("Q%d: %s", qIdx+1, questionText), imbot.FormatCallbackData("perm", "noop", req.ID)))
		}
		for optIdx, opt := range options {
			if option, ok := opt.(map[string]interface{}); ok {
				label, _ := option["label"].(string)
				if label != "" {
					buttonText := fmt.Sprintf("%d. %s", optIdx+1, label)
					callbackData := imbot.FormatCallbackData("perm", "option", req.ID, fmt.Sprintf("%d", qIdx), fmt.Sprintf("%d", optIdx))
					kb.AddRow(imbot.CallbackButton(buttonText, callbackData))
				}
			}
		}
	}

	// Add cancel button
	kb.AddRow(imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("perm", "deny", req.ID)))

	return kb.Build()
}

// buildTimeoutQuestionResult builds a result that auto-selects the first (recommended) option
func (h *Handler) buildTimeoutQuestionResult(req Request) Response {
	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return Response{
			Approved:    false,
			Reason:      "request timed out",
			RespondedAt: time.Now(),
		}
	}

	// For each question, select its first option as the recommended default
	answers := make(map[string]interface{})
	for _, q := range questions {
		question, ok := q.(map[string]interface{})
		if !ok {
			continue
		}
		questionText, ok := question["question"].(string)
		if !ok {
			continue
		}
		options, ok := question["options"].([]interface{})
		if !ok || len(options) == 0 {
			continue
		}
		if opt, ok := options[0].(map[string]interface{}); ok {
			if label, ok := opt["label"].(string); ok {
				answers[questionText] = label
			}
		}
	}

	// Note: This returns a Response, but the caller needs to convert to ask.Result
	return Response{
		Approved:    true,
		Reason:      "timed out - auto-selected recommended option",
		RespondedAt: time.Now(),
	}
}

// editPromptToResult edits the prompt message to show the result
func (h *Handler) editPromptToResult(bot imbot.Bot, chatID, messageID string, req Request, approved bool) {
	resultText := h.buildPromptText(req, true)
	if approved {
		resultText += "\n\n✅ *Approved*"
	} else {
		resultText += "\n\n❌ *Denied*"
	}

	// Edit message to remove keyboard and show result
	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		_ = tgBot.EditMessageWithKeyboard(context.Background(), chatID, messageID, resultText, nil)
	} else {
		// Fallback: send a new message with the result
		_, _ = bot.SendMessage(context.Background(), chatID, &imbot.SendMessageOptions{
			Text:      resultText,
			ParseMode: imbot.ParseModeMarkdown,
		})
	}
}

// editPromptToTimeout edits the prompt message to show timeout
func (h *Handler) editPromptToTimeout(bot imbot.Bot, chatID, messageID string, req Request) {
	resultText := h.buildPromptText(req, true)
	resultText += "\n\n⏰ *Timed Out*"

	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		_ = tgBot.EditMessageWithKeyboard(context.Background(), chatID, messageID, resultText, nil)
	}
}
