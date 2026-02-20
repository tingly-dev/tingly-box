package permission

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// DefaultHandler implements the Handler interface
type DefaultHandler struct {
	mu                sync.RWMutex
	config            Config
	modePerScope       map[string]agentboot.PermissionMode // scope -> mode
	pendingRequests   map[string]*pendingRequest
	decisions         map[string]*cachedDecision
	decisionChannels  map[string]chan agentboot.PermissionResponse
}

type pendingRequest struct {
	request   agentboot.PermissionRequest
	createdAt time.Time
	timeout   time.Duration
}

type cachedDecision struct {
	result    agentboot.PermissionResult
	expiresAt time.Time
}

// NewDefaultHandler creates a new permission handler
func NewDefaultHandler(config Config) *DefaultHandler {
	if config.DefaultMode == "" {
		config.DefaultMode = agentboot.PermissionModeAuto
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute // 5 minutes default
	}
	if config.DecisionDuration == 0 {
		config.DecisionDuration = 24 * time.Hour // 24 hours default
	}

	return &DefaultHandler{
		config:            config,
		modePerScope:      make(map[string]agentboot.PermissionMode),
		pendingRequests:   make(map[string]*pendingRequest),
		decisions:         make(map[string]*cachedDecision),
		decisionChannels:  make(map[string]chan agentboot.PermissionResponse),
	}
}

// CanUseTool handles permission requests
func (h *DefaultHandler) CanUseTool(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	// Check blacklist first
	if h.isBlacklisted(req.ToolName) {
		return agentboot.PermissionResult{
			Approved: false,
			Reason:   fmt.Sprintf("Tool '%s' is blacklisted", req.ToolName),
		}, nil
	}

	// Check whitelist
	if h.isWhitelisted(req.ToolName) {
		return agentboot.PermissionResult{
			Approved: true,
			Reason:   fmt.Sprintf("Tool '%s' is whitelisted", req.ToolName),
		}, nil
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

	switch mode {
	case agentboot.PermissionModeAuto:
		return agentboot.PermissionResult{Approved: true}, nil

	case agentboot.PermissionModeSkip:
		return agentboot.PermissionResult{Approved: true, Reason: "Permission mode: skip"}, nil

	case agentboot.PermissionModeManual:
		return h.handleManualPermission(ctx, req)
	}

	return agentboot.PermissionResult{Approved: false}, fmt.Errorf("unknown permission mode: %s", mode)
}

// handleManualPermission handles manual permission requests
func (h *DefaultHandler) handleManualPermission(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	// Create channel for response
	responseChan := make(chan agentboot.PermissionResponse, 1)

	h.mu.Lock()
	h.pendingRequests[req.RequestID] = &pendingRequest{
		request:   req,
		createdAt: time.Now(),
		timeout:   h.config.Timeout,
	}
	h.decisionChannels[req.RequestID] = responseChan
	h.mu.Unlock()

	// Log the pending request
	logrus.Infof("Permission request pending: %s - %s", req.ToolName, req.RequestID)

	// Wait for response or timeout
	select {
	case response := <-responseChan:
		// Cache the decision if enabled
		if h.config.RememberDecisions {
			h.cacheDecision(req, response)
		}

		// Clean up
		h.mu.Lock()
		delete(h.pendingRequests, req.RequestID)
		delete(h.decisionChannels, req.RequestID)
		h.mu.Unlock()

		return agentboot.PermissionResult{
			Approved: response.Approved,
			Reason:   response.Reason,
		}, nil

	case <-time.After(h.config.Timeout):
		// Timeout
		h.mu.Lock()
		delete(h.pendingRequests, req.RequestID)
		delete(h.decisionChannels, req.RequestID)
		h.mu.Unlock()

		return agentboot.PermissionResult{
			Approved: false,
			Reason:   "Permission request timed out",
		}, nil

	case <-ctx.Done():
		return agentboot.PermissionResult{
			Approved: false,
			Reason:   "Context cancelled",
		}, ctx.Err()
	}
}

// SubmitDecision submits a permission decision
func (h *DefaultHandler) SubmitDecision(requestID string, approved bool, reason string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch, exists := h.decisionChannels[requestID]
	if !exists {
		return fmt.Errorf("permission request not found: %s", requestID)
	}

	response := agentboot.PermissionResponse{
		RequestID: requestID,
		Approved:  approved,
		Reason:    reason,
		Timestamp: time.Now(),
	}

	select {
	case ch <- response:
		return nil
	default:
		return fmt.Errorf("response channel closed for request: %s", requestID)
	}
}

// GetPendingRequests returns all pending permission requests
func (h *DefaultHandler) GetPendingRequests() []agentboot.PermissionRequest {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var requests []agentboot.PermissionRequest
	for _, pr := range h.pendingRequests {
		requests = append(requests, pr.request)
	}
	return requests
}

// SetMode sets the permission mode for a scope
func (h *DefaultHandler) SetMode(scopeID string, mode agentboot.PermissionMode) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.modePerScope[scopeID] = mode
	logrus.Infof("Permission mode for %s set to: %s", scopeID, mode)
	return nil
}

// GetMode gets the current permission mode
func (h *DefaultHandler) GetMode(scopeID string) (agentboot.PermissionMode, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if mode, exists := h.modePerScope[scopeID]; exists {
		return mode, nil
	}
	return h.config.DefaultMode, nil
}

// RecordDecision records a permission decision for learning
func (h *DefaultHandler) RecordDecision(req agentboot.PermissionRequest, response agentboot.PermissionResponse) error {
	// Store in database for analytics and learning
	// Implementation depends on storage backend
	logrus.Infof("Recorded permission decision: %s -> %v", req.ToolName, response.Approved)
	return nil
}

func (h *DefaultHandler) getModeForScope(scopeID string) agentboot.PermissionMode {
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

func (h *DefaultHandler) getCachedDecision(req agentboot.PermissionRequest) *agentboot.PermissionResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := h.decisionKey(req)
	cached, exists := h.decisions[key]
	if !exists {
		return nil
	}

	if time.Now().After(cached.expiresAt) {
		delete(h.decisions, key)
		return nil
	}

	return &cached.result
}

func (h *DefaultHandler) cacheDecision(req agentboot.PermissionRequest, response agentboot.PermissionResponse) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := h.decisionKey(req)
	h.decisions[key] = &cachedDecision{
		result: agentboot.PermissionResult{
			Approved: response.Approved,
			Reason:   response.Reason,
		},
		expiresAt: time.Now().Add(h.config.DecisionDuration),
	}
}

func (h *DefaultHandler) decisionKey(req agentboot.PermissionRequest) string {
	return fmt.Sprintf("%s:%s:%s", req.AgentType, req.SessionID, req.ToolName)
}
