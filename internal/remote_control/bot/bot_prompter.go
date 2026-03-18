package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/imbot"
)

// IMPrompter implements ask.Prompter using IM (Telegram, etc.) for user interaction
type IMPrompter struct {
	mu sync.RWMutex

	// manager is the IM bot manager for sending messages
	manager *imbot.Manager

	// registry is the tool handler registry for customizing prompts and responses
	registry *ask.ToolHandlerRegistry

	// pendingRequests stores requests waiting for user response
	// key: requestID, value: *pendingIMRequest
	pendingRequests map[string]*pendingIMRequest

	// responseChannels stores channels for sending responses back to waiting goroutines
	// key: requestID, value: channel for result
	responseChannels map[string]chan ask.Result

	// defaultTimeout is the default timeout for requests
	defaultTimeout time.Duration

	// whitelist stores tools that have been approved with "Always Allow"
	// key: toolName, value: true
	whitelist map[string]bool
}

// pendingIMRequest stores a pending request with its context
type pendingIMRequest struct {
	request   ask.Request
	chatID    string
	platform  imbot.Platform
	messageID string
	createdAt time.Time
}

// NewIMPrompter creates a new IM-based prompter
func NewIMPrompter(manager *imbot.Manager) *IMPrompter {
	return &IMPrompter{
		manager:          manager,
		registry:         ask.NewToolHandlerRegistry(),
		pendingRequests:  make(map[string]*pendingIMRequest),
		responseChannels: make(map[string]chan ask.Result),
		defaultTimeout:   5 * time.Minute,
		whitelist:        make(map[string]bool),
	}
}

// SetDefaultTimeout sets the default timeout for requests
func (p *IMPrompter) SetDefaultTimeout(timeout time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.defaultTimeout = timeout
}

// Prompt prompts the user via IM for response
// This implements the ask.Prompter interface
func (p *IMPrompter) Prompt(ctx context.Context, req ask.Request) (ask.Result, error) {
	// Get the chat context from the request
	chatID, platform := req.ChatID, imbot.Platform(req.Platform)
	botUUID := req.BotUUID
	if chatID == "" {
		// No chat context, auto-approve
		logrus.WithField("id", req.ID).Debug("No chat context for request, auto-approving")
		return ask.Result{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	// Check whitelist for permission requests
	if req.Type == ask.TypePermission && req.ToolName != "" {
		p.mu.RLock()
		if p.whitelist[req.ToolName] {
			p.mu.RUnlock()
			logrus.WithFields(logrus.Fields{
				"tool_name":  req.ToolName,
				"request_id": req.ID,
			}).Info("Tool is whitelisted, auto-approving")
			return ask.Result{
				ID:           req.ID,
				Approved:     true,
				UpdatedInput: req.Input,
				Reason:       "Tool is whitelisted (Always Allow)",
			}, nil
		}
		p.mu.RUnlock()
	}

	// Use UUID to get bot (preferred) or fall back to platform lookup
	var bot imbot.Bot
	bot = p.manager.GetBot(botUUID, platform)
	if bot == nil {
		logrus.WithFields(logrus.Fields{
			"id":       req.ID,
			"platform": platform,
			"botUUID":  botUUID,
		}).Warn("Bot not found for request, auto-approving")
		return ask.Result{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	// Create response channel
	responseChan := make(chan ask.Result, 1)

	p.mu.Lock()
	p.pendingRequests[req.ID] = &pendingIMRequest{
		request:   req,
		chatID:    chatID,
		platform:  platform,
		createdAt: time.Now(),
	}
	p.responseChannels[req.ID] = responseChan
	timeout := p.defaultTimeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}
	p.mu.Unlock()

	// Check if platform supports inline keyboards
	supportsKeyboard := imbot.GetPlatformCapabilities(string(platform)).SupportsInteraction()

	// Build prompt using tool handler (pass platform for text fallback)
	promptText := p.buildPromptText(req, supportsKeyboard)
	keyboard := p.buildKeyboard(req)

	// For platforms without keyboard support, replace "Click a button" with text instructions
	if !supportsKeyboard && req.ToolName == "AskUserQuestion" {
		// Replace the "Click a button" text with text-only instructions
		promptText = strings.Replace(promptText, "*Click a button below to select*",
			p.buildTextSelectionInstructions(req), 1)
	}

	// Send the prompt message
	msg, err := bot.SendMessage(context.Background(), chatID, &imbot.SendMessageOptions{
		Text:      promptText,
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup": imbot.BuildTelegramActionKeyboard(keyboard),
		},
	})
	if err != nil {
		p.cleanup(req.ID)
		logrus.WithError(err).WithField("id", req.ID).Error("Failed to send prompt")
		return ask.Result{ID: req.ID, Approved: false}, fmt.Errorf("failed to send prompt: %w", err)
	}

	// Store message ID for potential editing
	p.mu.Lock()
	if pending, exists := p.pendingRequests[req.ID]; exists {
		pending.messageID = msg.MessageID
	}
	p.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"id":        req.ID,
		"chat_id":   chatID,
		"tool_name": req.ToolName,
	}).Info("Prompt sent, waiting for response")

	// Wait for response or timeout
	select {
	case result := <-responseChan:
		p.cleanup(req.ID)
		// Edit message to show result
		p.editPromptToResult(bot, chatID, msg.MessageID, req, result.Approved)
		return result, nil

	case <-time.After(timeout):
		p.cleanup(req.ID)
		p.editPromptToTimeout(bot, chatID, msg.MessageID, req)
		return ask.Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "request timed out",
		}, fmt.Errorf("request timed out")

	case <-ctx.Done():
		p.cleanup(req.ID)
		return ask.Result{ID: req.ID, Approved: false}, ctx.Err()
	}
}

// SubmitResult submits a result for a pending request
func (p *IMPrompter) SubmitResult(requestID string, result ask.Result) error {
	p.mu.Lock()
	responseChan, exists := p.responseChannels[requestID]
	if !exists {
		p.mu.Unlock()
		return fmt.Errorf("request not found or expired: %s", requestID)
	}

	// Get pending request for tool name (needed for whitelist)
	pending, hasPending := p.pendingRequests[requestID]

	// If remember is true and approved, add tool to whitelist
	if result.Remember && result.Approved && hasPending && pending.request.ToolName != "" {
		p.whitelist[pending.request.ToolName] = true
		logrus.WithFields(logrus.Fields{
			"tool_name":  pending.request.ToolName,
			"request_id": requestID,
		}).Info("Tool added to whitelist (Always Allow)")
	}

	// Keep lock held while sending to prevent race with cleanup
	select {
	case responseChan <- result:
		p.mu.Unlock()
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"approved":   result.Approved,
			"remember":   result.Remember,
		}).Info("Result submitted")
		return nil
	default:
		p.mu.Unlock()
		return fmt.Errorf("response channel full for request: %s", requestID)
	}
}

// SubmitUserResponse submits a user response for a pending request
// This parses the response using the appropriate tool handler
func (p *IMPrompter) SubmitUserResponse(requestID string, response ask.Response) error {
	// Get the pending request
	p.mu.RLock()
	pending, exists := p.pendingRequests[requestID]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("request not found or expired: %s", requestID)
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

// GetPendingRequest returns a pending request by ID
func (p *IMPrompter) GetPendingRequest(requestID string) (*ask.Request, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if pending, exists := p.pendingRequests[requestID]; exists {
		return &pending.request, true
	}
	return nil, false
}

// GetPendingRequestsForChat returns all pending requests for a specific chat
func (p *IMPrompter) GetPendingRequestsForChat(chatID string) []ask.Request {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var requests []ask.Request
	for _, pending := range p.pendingRequests {
		if pending.chatID == chatID {
			requests = append(requests, pending.request)
		}
	}
	return requests
}

// buildPromptText builds the prompt message text using tool handlers
func (p *IMPrompter) buildPromptText(req ask.Request, supportsKeyboard bool) string {
	// Try to use tool-specific prompt builder
	builder := p.registry.FindPromptBuilder(req.ToolName, req.Input)
	if builder != nil {
		prompt := builder.BuildPrompt(req)
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
func (p *IMPrompter) buildKeyboard(req ask.Request) imbot.InlineKeyboardMarkup {
	// Check if this is AskUserQuestion - build option keyboard
	if req.ToolName == "AskUserQuestion" {
		return p.buildAskUserQuestionKeyboard(req)
	}

	// Default keyboard: Approve/Deny/Always
	return p.buildDefaultKeyboard(req.ID)
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
func (p *IMPrompter) buildAskUserQuestionKeyboard(req ask.Request) imbot.InlineKeyboardMarkup {
	kb := imbot.NewKeyboardBuilder()

	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return p.buildDefaultKeyboard(req.ID)
	}

	// Build buttons for each option in the first question
	if question, ok := questions[0].(map[string]interface{}); ok {
		if options, ok := question["options"].([]interface{}); ok {
			for i, opt := range options {
				if option, ok := opt.(map[string]interface{}); ok {
					label, _ := option["label"].(string)
					if label != "" {
						// Use simple button text with just the number
						buttonText := fmt.Sprintf("Option %d", i+1)
						// Use only the index in callback data to keep it short
						callbackData := imbot.FormatCallbackData("perm", "option", req.ID, fmt.Sprintf("%d", i))
						kb.AddRow(imbot.CallbackButton(buttonText, callbackData))
					}
				}
			}
		}
	}

	// Add cancel button
	kb.AddRow(imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("perm", "deny", req.ID)))

	return kb.Build()
}

// buildTextSelectionInstructions builds text instructions for platforms without keyboard support
func (p *IMPrompter) buildTextSelectionInstructions(req ask.Request) string {
	var text strings.Builder

	text.WriteString("*To select an option, reply with the number:*\n\n")

	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		// For permission prompts (not AskUserQuestion)
		text.WriteString("• `1` - Allow\n")
		text.WriteString("• `0` - Deny\n")
		text.WriteString("• `a` - Always Allow")
		return text.String()
	}

	// For AskUserQuestion - list the options
	if question, ok := questions[0].(map[string]interface{}); ok {
		if options, ok := question["options"].([]interface{}); ok {
			for i, opt := range options {
				if option, ok := opt.(map[string]interface{}); ok {
					label, _ := option["label"].(string)
					desc, hasDesc := option["description"].(string)
					if hasDesc && desc != "" {
						text.WriteString(fmt.Sprintf("• `%d` - %s - %s\n", i+1, label, desc))
					} else {
						text.WriteString(fmt.Sprintf("• `%d` - %s\n", i+1, label))
					}
				}
			}
		}
	}

	text.WriteString("\n_Just type the number to reply_")

	return text.String()
}

// editPromptToResult edits the prompt message to show the result
func (p *IMPrompter) editPromptToResult(bot imbot.Bot, chatID, messageID string, req ask.Request, approved bool) {
	resultText := p.buildPromptText(req, true) // supportsKeyboard doesn't matter for result
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
func (p *IMPrompter) editPromptToTimeout(bot imbot.Bot, chatID, messageID string, req ask.Request) {
	resultText := p.buildPromptText(req, true) // supportsKeyboard doesn't matter for timeout
	resultText += "\n\n⏰ *Timed Out*"

	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		_ = tgBot.EditMessageWithKeyboard(context.Background(), chatID, messageID, resultText, nil)
	}
}

// cleanup removes a pending request and its response channel
func (p *IMPrompter) cleanup(requestID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.pendingRequests, requestID)
	delete(p.responseChannels, requestID)
}

// ParseTextResponse parses user text input as a permission response
// Returns: (approved, remember, isValid)
func ParseTextResponse(text string) (approved bool, remember bool, isValid bool) {
	return ask.ParseTextResponse(text)
}

// GetWhitelist returns the list of whitelisted tools
func (p *IMPrompter) GetWhitelist() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tools := make([]string, 0, len(p.whitelist))
	for tool := range p.whitelist {
		tools = append(tools, tool)
	}
	return tools
}

// AddToWhitelist adds a tool to the whitelist
func (p *IMPrompter) AddToWhitelist(toolName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.whitelist[toolName] = true
	logrus.WithField("tool_name", toolName).Info("Tool added to whitelist")
}

// RemoveFromWhitelist removes a tool from the whitelist
func (p *IMPrompter) RemoveFromWhitelist(toolName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.whitelist, toolName)
	logrus.WithField("tool_name", toolName).Info("Tool removed from whitelist")
}

// ClearWhitelist clears all tools from the whitelist
func (p *IMPrompter) ClearWhitelist() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.whitelist = make(map[string]bool)
	logrus.Info("Whitelist cleared")
}

// IsWhitelisted checks if a tool is in the whitelist
func (p *IMPrompter) IsWhitelisted(toolName string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.whitelist[toolName]
}

// Legacy compatibility methods

// OnApproval implements agentboot.ApprovalHandler.
// It handles permission confirmation requests via IM.
func (p *IMPrompter) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	askReq := ask.FromPermissionRequest(req)
	result, err := p.Prompt(ctx, *askReq)
	if err != nil {
		return agentboot.PermissionResult{}, err
	}
	return result.ToPermissionResult(), nil
}

// OnAsk implements agentboot.AskHandler.
// It handles user questions/selections via IM.
func (p *IMPrompter) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	// Convert AskRequest to ask.Request
	askReq := ask.Request{
		ID:        req.ID,
		Type:      ask.Type(req.Type),
		ChatID:    req.ChatID,
		Platform:  req.Platform,
		BotUUID:   req.BotUUID,
		SessionID: req.SessionID,
		AgentType: req.AgentType,
		ToolName:  req.ToolName,
		Input:     req.Input,
		Message:   req.Message,
		Reason:    req.Reason,
		Metadata:  req.Metadata,
	}

	result, err := p.Prompt(ctx, askReq)
	if err != nil {
		return agentboot.AskResult{}, err
	}

	// Convert back to AskResult
	return agentboot.AskResult{
		ID:           result.ID,
		Approved:     result.Approved,
		Response:     result.Response,
		Selection:    result.Selection,
		Remember:     result.Remember,
		Reason:       result.Reason,
		UpdatedInput: result.UpdatedInput,
	}, nil
}

// SubmitDecision submits a user's decision for a pending request
func (p *IMPrompter) SubmitDecision(requestID string, approved bool, remember bool, reason string) error {
	result := ask.Result{
		ID:       requestID,
		Approved: approved,
		Remember: remember,
		Reason:   reason,
	}
	return p.SubmitResult(requestID, result)
}

// PromptPermission implements the legacy agentboot.UserPrompter interface
func (p *IMPrompter) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	return p.OnApproval(ctx, req)
}
