package ask

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Handler is the main interface for handling user prompts
type Handler interface {
	// Ask sends a request to the user and waits for response
	Ask(ctx context.Context, req Request) (Result, error)

	// SetMode sets the mode for a session/scope
	SetMode(scopeID string, mode Mode) error

	// GetMode gets the current mode for a session/scope
	GetMode(scopeID string) (Mode, error)

	// SubmitResult submits a result for a pending request (for async handlers)
	SubmitResult(requestID string, result Result) error

	// GetPendingRequests returns all pending requests
	GetPendingRequests() []Request

	// SetPrompter sets the prompter for user interaction
	SetPrompter(p Prompter)
}

// DefaultHandler implements the Handler interface
type DefaultHandler struct {
	mu               sync.RWMutex
	config           Config
	modePerScope     map[string]Mode
	pendingRequests  map[string]*pendingRequest
	decisionChannels map[string]chan Result
	prompter         Prompter
}

type pendingRequest struct {
	request   Request
	createdAt time.Time
	timeout   time.Duration
}

type cachedDecision struct {
	result    Result
	expiresAt time.Time
}

// NewHandler creates a new ask handler with the given config
func NewHandler(config Config) *DefaultHandler {
	if config.DefaultMode == "" {
		config.DefaultMode = ModeAuto
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}
	if config.DecisionDuration == 0 {
		config.DecisionDuration = 24 * time.Hour
	}

	return &DefaultHandler{
		config:           config,
		modePerScope:     make(map[string]Mode),
		pendingRequests:  make(map[string]*pendingRequest),
		decisionChannels: make(map[string]chan Result),
	}
}

// Ask handles user prompt requests
func (h *DefaultHandler) Ask(ctx context.Context, req Request) (Result, error) {
	logrus.WithFields(logrus.Fields{
		"type":       req.Type,
		"tool_name":  req.ToolName,
		"session_id": req.SessionID,
		"id":         req.ID,
	}).Info("Ask called")

	// For permission requests, check blacklist/whitelist
	if req.Type == TypePermission && req.ToolName != "" {
		if h.isBlacklisted(req.ToolName) {
			return Result{
				ID:       req.ID,
				Approved: false,
				Reason:   fmt.Sprintf("Tool '%s' is blacklisted", req.ToolName),
			}, nil
		}

		if h.isWhitelisted(req.ToolName) {
			logrus.WithField("tool_name", req.ToolName).Info("Tool is whitelisted, auto-approving")
			return Result{
				ID:       req.ID,
				Approved: true,
				Reason:   fmt.Sprintf("Tool '%s' is whitelisted", req.ToolName),
			}, nil
		}
	}

	// Check for cached decision
	if h.config.RememberDecisions {
		if result := h.getCachedDecision(req); result != nil {
			logrus.Debugf("Using cached decision for %s: %v", req.ToolName, result.Approved)
			return *result, nil
		}
	}

	// Get mode for scope
	mode := h.getModeForScope(req.SessionID)
	logrus.WithFields(logrus.Fields{
		"session_id":   req.SessionID,
		"mode":         mode,
		"has_prompter": h.prompter != nil,
	}).Info("Ask mode check")

	switch mode {
	case ModeAuto:
		return Result{ID: req.ID, Approved: true}, nil

	case ModeSkip:
		return Result{ID: req.ID, Approved: true, Reason: "Mode: skip"}, nil

	case ModeManual:
		return h.handleManualAsk(ctx, req)
	}

	return Result{ID: req.ID, Approved: false}, fmt.Errorf("unknown mode: %s", mode)
}

// handleManualAsk handles manual mode requests
func (h *DefaultHandler) handleManualAsk(ctx context.Context, req Request) (Result, error) {
	logrus.WithFields(logrus.Fields{
		"type":        req.Type,
		"tool_name":   req.ToolName,
		"id":          req.ID,
		"has_prompter": h.prompter != nil,
	}).Info("handleManualAsk called")

	// If prompter is set, use it for interactive prompt
	if h.prompter != nil {
		result, err := h.prompter.Prompt(ctx, req)
		if err != nil {
			return Result{
				ID:       req.ID,
				Approved: false,
				Reason:   fmt.Sprintf("Prompt failed: %v", err),
			}, nil
		}

		// If remember is true, add to whitelist (for permission type)
		if result.Remember && result.Approved && h.config.EnableWhitelist && req.Type == TypePermission {
			h.mu.Lock()
			h.config.Whitelist = append(h.config.Whitelist, req.ToolName)
			h.mu.Unlock()
		}

		// Cache decision if enabled
		if h.config.RememberDecisions {
			h.cacheDecision(req, result)
		}

		return result, nil
	}

	// Fallback to channel-based decision waiting
	responseChan := make(chan Result, 1)

	h.mu.Lock()
	h.pendingRequests[req.ID] = &pendingRequest{
		request:   req,
		createdAt: time.Now(),
		timeout:   h.config.Timeout,
	}
	h.decisionChannels[req.ID] = responseChan
	h.mu.Unlock()

	logrus.Infof("Ask request pending: %s - %s", req.Type, req.ID)

	// Wait for response or timeout
	timeout := h.config.Timeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	select {
	case result := <-responseChan:
		// Cache the decision if enabled
		if h.config.RememberDecisions {
			h.cacheDecision(req, result)
		}

		// Clean up
		h.mu.Lock()
		delete(h.pendingRequests, req.ID)
		delete(h.decisionChannels, req.ID)
		h.mu.Unlock()

		return result, nil

	case <-time.After(timeout):
		h.mu.Lock()
		delete(h.pendingRequests, req.ID)
		delete(h.decisionChannels, req.ID)
		h.mu.Unlock()

		return Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "Request timed out",
		}, fmt.Errorf("request timed out")

	case <-ctx.Done():
		return Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "Context cancelled",
		}, ctx.Err()
	}
}

// SubmitResult submits a result for a pending request
func (h *DefaultHandler) SubmitResult(requestID string, result Result) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch, exists := h.decisionChannels[requestID]
	if !exists {
		return fmt.Errorf("request not found: %s", requestID)
	}

	result.ID = requestID

	select {
	case ch <- result:
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"approved":   result.Approved,
		}).Info("Result submitted")
		return nil
	default:
		return fmt.Errorf("response channel closed for request: %s", requestID)
	}
}

// GetPendingRequests returns all pending requests
func (h *DefaultHandler) GetPendingRequests() []Request {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var requests []Request
	for _, pr := range h.pendingRequests {
		requests = append(requests, pr.request)
	}
	return requests
}

// SetMode sets the mode for a scope
func (h *DefaultHandler) SetMode(scopeID string, mode Mode) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.modePerScope[scopeID] = mode
	logrus.Infof("Mode for %s set to: %s", scopeID, mode)
	return nil
}

// GetMode gets the current mode for a scope
func (h *DefaultHandler) GetMode(scopeID string) (Mode, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if mode, exists := h.modePerScope[scopeID]; exists {
		return mode, nil
	}
	return h.config.DefaultMode, nil
}

// SetPrompter sets the prompter for user interaction
func (h *DefaultHandler) SetPrompter(p Prompter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prompter = p
}

// GetPrompter returns the current prompter
func (h *DefaultHandler) GetPrompter() Prompter {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.prompter
}

func (h *DefaultHandler) getModeForScope(scopeID string) Mode {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if mode, exists := h.modePerScope[scopeID]; exists {
		return mode
	}
	return h.config.DefaultMode
}

func (h *DefaultHandler) isBlacklisted(toolName string) bool {
	for _, blacklisted := range h.config.Blacklist {
		if toolName == blacklisted {
			return true
		}
	}
	return false
}

func (h *DefaultHandler) isWhitelisted(toolName string) bool {
	for _, whitelisted := range h.config.Whitelist {
		if toolName == whitelisted {
			return true
		}
	}
	return false
}

// Decision cache (optional, can be implemented later)
type decisionCache struct {
	mu        sync.RWMutex
	decisions map[string]cachedDecision
}

func newDecisionCache() *decisionCache {
	return &decisionCache{
		decisions: make(map[string]cachedDecision),
	}
}

func (h *DefaultHandler) getCachedDecision(req Request) *Result {
	// TODO: Implement decision caching
	return nil
}

func (h *DefaultHandler) cacheDecision(req Request, result Result) {
	// TODO: Implement decision caching
}