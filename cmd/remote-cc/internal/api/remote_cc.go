package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/audit"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/config"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/launcher"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/session"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/summarizer"
)

// RemoteCCHandler handles remote Claude Code requests
type RemoteCCHandler struct {
	sessionMgr    *session.Manager
	claudeLauncher *launcher.ClaudeCodeLauncher
	summaryEngine *summarizer.Engine
	auditLogger   *audit.Logger
	config        *config.Config
}

// NewRemoteCCHandler creates a new remote-cc handler
func NewRemoteCCHandler(sessionMgr *session.Manager, claudeLauncher *launcher.ClaudeCodeLauncher, summaryEngine *summarizer.Engine, auditLogger *audit.Logger, cfg *config.Config) *RemoteCCHandler {
	return &RemoteCCHandler{
		sessionMgr:     sessionMgr,
		claudeLauncher: claudeLauncher,
		summaryEngine:  summaryEngine,
		auditLogger:   auditLogger,
		config:        cfg,
	}
}

// RemoteSession represents a remote session for API response
type RemoteSession struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Request      string    `json:"request,omitempty"`
	Response     string    `json:"response,omitempty"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    string    `json:"created_at"`
	LastActivity string    `json:"last_activity"`
	ExpiresAt    string    `json:"expires_at"`
}

// RemoteChatRequest represents a chat request to Claude Code
type RemoteChatRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message" binding:"required"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// RemoteChatResponse represents a chat response from Claude Code
type RemoteChatResponse struct {
	SessionID  string `json:"session_id"`
	Message    string `json:"message"`
	Summary    string `json:"summary"` // Chopped/summarized response
	FullResponse string `json:"full_response,omitempty"` // Full response (if requested)
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

// RemoteChatMessage represents a chat message for API response
type RemoteChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Summary   string `json:"summary,omitempty"`
	Timestamp string `json:"timestamp"`
}

// GetSessions handles GET /remote-cc/sessions
func (h *RemoteCCHandler) GetSessions(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	// Parse query params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")

	if limit > 100 {
		limit = 100
	}
	if page < 1 {
		page = 1
	}

	// Get all sessions
	stats := h.sessionMgr.GetStats()
	allSessions := h.getAllSessions()

	// Filter by status
	var filteredSessions []*session.Session
	for _, s := range allSessions {
		if status == "" || string(s.Status) == status {
			filteredSessions = append(filteredSessions, s)
		}
	}

	// Paginate
	total := len(filteredSessions)
	startIdx := (page - 1) * limit
	endIdx := startIdx + limit

	var paginatedSessions []*session.Session
	if startIdx < total {
		if endIdx > total {
			paginatedSessions = filteredSessions[startIdx:]
		} else {
			paginatedSessions = filteredSessions[startIdx:endIdx]
		}
	}

	// Convert to response format
	entries := make([]RemoteSession, len(paginatedSessions))
	for i, s := range paginatedSessions {
		entries[i] = RemoteSession{
			ID:           s.ID,
			Status:       string(s.Status),
			Request:      s.Request,
			Response:     s.Response,
			Error:        s.Error,
			CreatedAt:    s.CreatedAt.Format(time.RFC3339),
			LastActivity: s.LastActivity.Format(time.RFC3339),
			ExpiresAt:    s.ExpiresAt.Format(time.RFC3339),
		}
	}

	logrus.Debugf("Remote-cc sessions request: page=%d, limit=%d, status=%s, total=%d", page, limit, status, total)

	h.auditLogger.LogRequest("remote_cc_sessions", userID, clientIP, "", getRequestID(c), true, time.Since(start), map[string]interface{}{
		"page":   page,
		"limit":  limit,
		"status": status,
		"total":  total,
	})

	c.JSON(http.StatusOK, gin.H{
		"sessions":   entries,
		"page":       page,
		"limit":      limit,
		"total":      total,
		"total_pages": (total + limit - 1) / limit,
		"stats":      stats,
	})
}

// GetSession handles GET /remote-cc/sessions/:id
func (h *RemoteCCHandler) GetSession(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)
	sessionID := c.Param("id")

	session, exists := h.sessionMgr.Get(sessionID)
	if !exists {
		h.auditLogger.LogRequest("remote_cc_session_get", userID, clientIP, sessionID, getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": "session not found",
		})

		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Session not found",
				"type":    "not_found_error",
			},
		})
		return
	}

	logrus.Debugf("Remote-cc session get: id=%s, status=%s", sessionID, session.Status)

	h.auditLogger.LogRequest("remote_cc_session_get", userID, clientIP, sessionID, getRequestID(c), true, time.Since(start), nil)

	c.JSON(http.StatusOK, RemoteSession{
		ID:           session.ID,
		Status:       string(session.Status),
		Request:      session.Request,
		Response:     session.Response,
		Error:        session.Error,
		CreatedAt:    session.CreatedAt.Format(time.RFC3339),
		LastActivity: session.LastActivity.Format(time.RFC3339),
		ExpiresAt:    session.ExpiresAt.Format(time.RFC3339),
	})
}

// Chat handles POST /remote-cc/chat
func (h *RemoteCCHandler) Chat(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	var req RemoteChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.auditLogger.LogRequest("remote_cc_chat", userID, clientIP, "", getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": "invalid request body",
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: message is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	var sessionID string
	var session *session.Session
	var exists bool

	// If session ID provided, use existing session
	if req.SessionID != "" {
		session, exists = h.sessionMgr.Get(req.SessionID)
		if !exists {
			h.auditLogger.LogRequest("remote_cc_chat", userID, clientIP, req.SessionID, getRequestID(c), false, time.Since(start), map[string]interface{}{
				"error": "session not found",
			})

			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "Session not found",
					"type":    "not_found_error",
				},
			})
			return
		}
		sessionID = req.SessionID
	} else {
		// Create new session
		session = h.sessionMgr.Create()
		sessionID = session.ID

		// Set initial request
		h.sessionMgr.SetRequest(sessionID, req.Message)
	}

	// Append user message to history
	h.sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now(),
	})

	// Update session status
	h.sessionMgr.SetRunning(sessionID)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	// Execute Claude Code
	result, err := h.claudeLauncher.Execute(ctx, req.Message)
	response := result.Output
	if err != nil && result.Error != "" {
		response = result.Error
	}

	if err != nil {
		h.sessionMgr.SetFailed(sessionID, result.Error)

		logrus.Errorf("Claude Code execution failed: %v", result.Error)

		h.auditLogger.LogRequest("remote_cc_chat", userID, clientIP, sessionID, getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": result.Error,
		})

		// Return error with summary (still return something useful)
		c.JSON(http.StatusOK, RemoteChatResponse{
			SessionID: sessionID,
			Success:   false,
			Error:     result.Error,
			Summary:   "Failed to get response from Claude Code",
		})
		return
	}

	// Set completed with response
	h.sessionMgr.SetCompleted(sessionID, response)

	// Generate summary (chopped response)
	summary := h.summaryEngine.Summarize(response)

	// Append assistant message to history
	h.sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "assistant",
		Content:   response,
		Summary:   summary,
		Timestamp: time.Now(),
	})

	logrus.Debugf("Remote-cc chat completed: session=%s, response_length=%d, summary_length=%d",
		sessionID, len(response), len(summary))

	h.auditLogger.LogRequest("remote_cc_chat", userID, clientIP, sessionID, getRequestID(c), true, time.Since(start), map[string]interface{}{
		"response_length": len(response),
		"summary_length": len(summary),
	})

	c.JSON(http.StatusOK, RemoteChatResponse{
		SessionID:     sessionID,
		Message:       req.Message,
		Summary:       summary,
		FullResponse: response,
		Success:     true,
	})
}

// GetSessionMessages handles GET /remote-cc/sessions/:id/messages
func (h *RemoteCCHandler) GetSessionMessages(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)
	sessionID := c.Param("id")

	messages, exists := h.sessionMgr.GetMessages(sessionID)
	if !exists {
		h.auditLogger.LogRequest("remote_cc_session_messages", userID, clientIP, sessionID, getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": "session not found",
		})

		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Session not found",
				"type":    "not_found_error",
			},
		})
		return
	}

	resp := make([]RemoteChatMessage, 0, len(messages))
	for _, msg := range messages {
		resp = append(resp, RemoteChatMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Summary:   msg.Summary,
			Timestamp: msg.Timestamp.Format(time.RFC3339),
		})
	}

	h.auditLogger.LogRequest("remote_cc_session_messages", userID, clientIP, sessionID, getRequestID(c), true, time.Since(start), map[string]interface{}{
		"count": len(resp),
	})

	c.JSON(http.StatusOK, gin.H{
		"messages": resp,
	})
}

// ClearSessions handles POST /remote-cc/sessions/clear
func (h *RemoteCCHandler) ClearSessions(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	cleared := h.sessionMgr.Clear()

	h.auditLogger.LogRequest("remote_cc_sessions_clear", userID, clientIP, "", getRequestID(c), true, time.Since(start), map[string]interface{}{
		"cleared": cleared,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"cleared": cleared,
	})
}

// getAllSessions returns all sessions (helper function)
func (h *RemoteCCHandler) getAllSessions() []*session.Session {
	return h.sessionMgr.List()
}
