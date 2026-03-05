package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/imbot"
)

// IMPrompterV2 implements ask.Prompter using the new platform-agnostic interaction system
// This replaces IMPrompter with better multi-platform support
type IMPrompterV2 struct {
	mu sync.RWMutex

	// handler is the interaction handler for platform-agnostic interactions
	handler *imbot.InteractionHandler

	// registry is the tool handler registry for customizing prompts and responses
	registry *ask.ToolHandlerRegistry

	// pendingRequests stores requests waiting for user response
	// key: requestID, value: *pendingIMRequestV2
	pendingRequests map[string]*pendingIMRequestV2

	// responseChannels stores channels for sending responses back to waiting goroutines
	// key: requestID, value: channel for result
	responseChannels map[string]chan ask.Result

	// defaultTimeout is the default timeout for requests
	defaultTimeout time.Duration
}

// pendingIMRequestV2 stores a pending request with its context
type pendingIMRequestV2 struct {
	request       ask.Request
	chatID        string
	platform      imbot.Platform
	botUUID       string
	messageID     string
	interactionID string // For interaction response tracking
	createdAt     time.Time
}

// NewIMPrompterV2 creates a new IM-based prompter using the interaction system
func NewIMPrompterV2(handler *imbot.InteractionHandler) *IMPrompterV2 {
	return &IMPrompterV2{
		handler:          handler,
		registry:         ask.NewToolHandlerRegistry(),
		pendingRequests:  make(map[string]*pendingIMRequestV2),
		responseChannels: make(map[string]chan ask.Result),
		defaultTimeout:   5 * time.Minute,
	}
}

// SetDefaultTimeout sets the default timeout for requests
func (p *IMPrompterV2) SetDefaultTimeout(timeout time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.defaultTimeout = timeout
}

// Prompt prompts the user via IM using the new interaction system
// This implements the ask.Prompter interface
func (p *IMPrompterV2) Prompt(ctx context.Context, req ask.Request) (ask.Result, error) {
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

	// Create response channel
	responseChan := make(chan ask.Result, 1)

	p.mu.Lock()
	p.pendingRequests[req.ID] = &pendingIMRequestV2{
		request:   req,
		chatID:    chatID,
		platform:  platform,
		botUUID:   botUUID,
		createdAt: time.Now(),
	}
	p.responseChannels[req.ID] = responseChan
	timeout := p.defaultTimeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}
	p.mu.Unlock()

	// Build prompt text and interactions
	promptText := p.buildPromptText(req)
	interactions := p.buildInteractions(req)

	// Create interaction request using the new system
	interactionReq := imbot.InteractionRequest{
		ID:           p.formatRequestID(req.ID),
		ChatID:       chatID,
		Platform:     platform,
		BotUUID:      botUUID,
		Message:      promptText,
		ParseMode:    imbot.ParseModeMarkdown,
		Mode:         imbot.ModeAuto, // Auto-detect best mode for platform
		Interactions: interactions,
		Timeout:      timeout,
	}

	// Send via interaction handler
	resp, err := p.handler.RequestInteraction(ctx, interactionReq)
	if err != nil {
		p.cleanup(req.ID)
		logrus.WithError(err).WithField("id", req.ID).Error("Failed to send interaction request")
		return ask.Result{ID: req.ID, Approved: false}, fmt.Errorf("failed to send interaction request: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"id":        req.ID,
		"chat_id":   chatID,
		"tool_name": req.ToolName,
	}).Info("Interaction request sent, waiting for response")

	// Process the response
	result := p.processResponse(req, resp)
	p.cleanup(req.ID)

	return result, nil
}

// processResponse processes the interaction response into an ask.Result
func (p *IMPrompterV2) processResponse(req ask.Request, resp *imbot.InteractionResponse) ask.Result {
	if resp == nil {
		return ask.Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "no response received",
		}
	}

	// Check if this is a cancel action
	if resp.IsCancel() {
		return ask.Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "user cancelled",
		}
	}

	// Handle AskUserQuestion option selections
	if req.ToolName == "AskUserQuestion" {
		return p.processAskUserQuestionResponse(req, resp)
	}

	// Handle default allow/deny/always responses
	return p.processDefaultResponse(req, resp)
}

// processDefaultResponse processes allow/deny/always responses
func (p *IMPrompterV2) processDefaultResponse(req ask.Request, resp *imbot.InteractionResponse) ask.Result {
	action := resp.Action.Value

	switch action {
	case "allow":
		return ask.Result{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
		}
	case "deny":
		return ask.Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "user denied",
		}
	case "always":
		return ask.Result{
			ID:           req.ID,
			Approved:     true,
			Remember:     true,
			UpdatedInput: req.Input,
		}
	default:
		logrus.WithField("action", action).Warn("Unknown action in permission response")
		return ask.Result{
			ID:       req.ID,
			Approved: false,
			Reason:   fmt.Sprintf("unknown action: %s", action),
		}
	}
}

// processAskUserQuestionResponse processes AskUserQuestion option selections
func (p *IMPrompterV2) processAskUserQuestionResponse(req ask.Request, resp *imbot.InteractionResponse) ask.Result {
	action := resp.Action.Value

	// Check if it's an option selection (format: "option:123")
	if len(action) > 7 && action[:7] == "option:" {
		optionIndex := -1
		if _, err := fmt.Sscanf(action, "option:%d", &optionIndex); err == nil && optionIndex >= 0 {
			// Get the options from the request
			questions, ok := req.Input["questions"].([]interface{})
			if ok && len(questions) > 0 {
				if question, ok := questions[0].(map[string]interface{}); ok {
					if options, ok := question["options"].([]interface{}); ok && optionIndex < len(options) {
						if option, ok := options[optionIndex].(map[string]interface{}); ok {
							// Extract option value
							selection := make(map[string]interface{})
							selection["index"] = optionIndex
							if v, ok := option["value"]; ok {
								selection["value"] = v
							}
							if v, ok := option["label"]; ok {
								selection["label"] = v
							}

							// Response as string
							responseStr := ""
							if label, ok := option["label"].(string); ok {
								responseStr = label
							} else if val, ok := option["value"].(string); ok {
								responseStr = val
							}

							return ask.Result{
								ID:        req.ID,
								Approved:  true,
								Selection: selection,
								Response:  responseStr,
							}
						}
					}
				}
			}
		}
	}

	// Handle allow/deny/always for AskUserQuestion too
	switch action {
	case "allow":
		return ask.Result{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
		}
	case "deny", "cancel":
		return ask.Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "user cancelled",
		}
	default:
		logrus.WithField("action", action).Warn("Unknown action in AskUserQuestion response")
		return ask.Result{
			ID:       req.ID,
			Approved: false,
			Reason:   fmt.Sprintf("unknown action: %s", action),
		}
	}
}

// SubmitResult submits a result for a pending request
func (p *IMPrompterV2) SubmitResult(requestID string, result ask.Result) error {
	p.mu.Lock()
	responseChan, exists := p.responseChannels[requestID]
	if !exists {
		p.mu.Unlock()
		return fmt.Errorf("request not found or expired: %s", requestID)
	}
	// Keep lock held while sending to prevent race with cleanup
	select {
	case responseChan <- result:
		p.mu.Unlock()
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"approved":   result.Approved,
		}).Info("Result submitted")
		return nil
	default:
		p.mu.Unlock()
		return fmt.Errorf("response channel full for request: %s", requestID)
	}
}

// GetPendingRequest returns a pending request by ID
func (p *IMPrompterV2) GetPendingRequest(requestID string) (*ask.Request, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if pending, exists := p.pendingRequests[requestID]; exists {
		return &pending.request, true
	}
	return nil, false
}

// GetPendingRequestsForChat returns all pending requests for a specific chat
func (p *IMPrompterV2) GetPendingRequestsForChat(chatID string) []ask.Request {
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
func (p *IMPrompterV2) buildPromptText(req ask.Request) string {
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

// buildInteractions builds platform-agnostic interactions for prompts
func (p *IMPrompterV2) buildInteractions(req ask.Request) []imbot.Interaction {
	// Check if this is AskUserQuestion - build option interactions
	if req.ToolName == "AskUserQuestion" {
		return p.buildAskUserQuestionInteractions(req)
	}

	// Default interactions: Approve/Deny/Always
	return p.buildDefaultInteractions(req.ID)
}

// buildDefaultInteractions builds the default allow/deny interactions
func (p *IMPrompterV2) buildDefaultInteractions(requestID string) []imbot.Interaction {
	builder := imbot.NewInteractionBuilder()
	builder.AddAllowDeny(requestID)
	builder.AddButton("always", "🔄 Always Allow", "always")
	return builder.Build()
}

// buildAskUserQuestionInteractions builds interactions with options for AskUserQuestion
func (p *IMPrompterV2) buildAskUserQuestionInteractions(req ask.Request) []imbot.Interaction {
	builder := imbot.NewInteractionBuilder()

	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return p.buildDefaultInteractions(req.ID)
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
						// Use value with option index
						value := fmt.Sprintf("option:%d", i)
						builder.AddButton(fmt.Sprintf("opt-%d", i), buttonText, value)
					}
				}
			}
		}
	}

	// Add cancel button
	builder.AddCancel("cancel")

	return builder.Build()
}

// cleanup removes a pending request and its response channel
func (p *IMPrompterV2) cleanup(requestID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.pendingRequests, requestID)
	delete(p.responseChannels, requestID)
}

// formatRequestID formats a request ID for use in interaction requests
func (p *IMPrompterV2) formatRequestID(requestID string) string {
	return fmt.Sprintf("perm-%s", requestID)
}

// Legacy compatibility methods

// OnApproval implements agentboot.ApprovalHandler.
// It handles permission confirmation requests via IM using the new interaction system.
func (p *IMPrompterV2) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	askReq := ask.FromPermissionRequest(req)
	result, err := p.Prompt(ctx, *askReq)
	if err != nil {
		return agentboot.PermissionResult{}, err
	}
	return result.ToPermissionResult(), nil
}

// OnAsk implements agentboot.AskHandler.
// It handles user questions/selections via IM using the new interaction system.
func (p *IMPrompterV2) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
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
// Deprecated: Use SubmitResult instead
func (p *IMPrompterV2) SubmitDecision(requestID string, approved bool, remember bool, reason string) error {
	result := ask.Result{
		ID:       requestID,
		Approved: approved,
		Remember: remember,
		Reason:   reason,
	}
	return p.SubmitResult(requestID, result)
}

// PromptPermission implements the legacy agentboot.UserPrompter interface
// Deprecated: Use OnApproval instead
func (p *IMPrompterV2) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	return p.OnApproval(ctx, req)
}
