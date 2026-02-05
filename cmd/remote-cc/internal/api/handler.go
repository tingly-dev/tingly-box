package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/audit"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/launcher"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/session"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/summarizer"
)

// Handler handles API requests
type Handler struct {
	sessionMgr   *session.Manager
	claude       *launcher.ClaudeCodeLauncher
	summarizer   *summarizer.Engine
	auditLogger  *audit.Logger
}

// NewHandler creates a new API handler
func NewHandler(sessionMgr *session.Manager, claude *launcher.ClaudeCodeLauncher, summary *summarizer.Engine, auditLogger *audit.Logger) *Handler {
	return &Handler{
		sessionMgr:  sessionMgr,
		claude:      claude,
		summarizer:  summary,
		auditLogger: auditLogger,
	}
}

// HandshakeRequest represents the handshake request body
type HandshakeRequest struct {
	// Empty for now, may include client info in future
}

// HandshakeResponse represents the handshake response
type HandshakeResponse struct {
	SessionID string `json:"session_id"`
	Status   string `json:"status"`
	ExpiresAt string `json:"expires_at"`
}

// ExecuteRequest represents the execute request body
type ExecuteRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	Request   string `json:"request" binding:"required"`
}

// ExecuteResponse represents the execute response
type ExecuteResponse struct {
	SessionID string `json:"session_id"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
	Error    string `json:"error,omitempty"`
}

// StatusResponse represents the status response
type StatusResponse struct {
	SessionID   string `json:"session_id"`
	Status     string `json:"status"`
	Request    string `json:"request,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Error     string `json:"error,omitempty"`
	CreatedAt  string `json:"created_at"`
	LastActivity string `json:"last_activity"`
	ExpiresAt  string `json:"expires_at"`
}

// CloseRequest represents the close request body
type CloseRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

// CloseResponse represents the close response
type CloseResponse struct {
	SessionID string `json:"session_id"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

// getClientIP extracts client IP from context
func getClientIP(c *gin.Context) string {
	// Check for forwarded header first
	if forwarded := c.GetHeader("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}
	if realIP := c.GetHeader("X-Real-IP"); realIP != "" {
		return realIP
	}
	return c.ClientIP()
}

// getUserID extracts user ID from context (set by auth middleware)
func getUserID(c *gin.Context) string {
	if userID, exists := c.Get("client_id"); exists {
		return userID.(string)
	}
	return "unknown"
}

// getRequestID generates or extracts request ID
func getRequestID(c *gin.Context) string {
	if reqID, exists := c.Get("request_id"); exists {
		return reqID.(string)
	}
	return uuid.New().String()[:8]
}

// Handshake handles POST /opsx/handshake
func (h *Handler) Handshake(c *gin.Context) {
	start := time.Now()
	requestID := getRequestID(c)
	clientIP := getClientIP(c)
	userID := getUserID(c)

	var req HandshakeRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		h.auditLogger.LogRequest("handshake", userID, clientIP, "", requestID, false, time.Since(start), map[string]interface{}{
			"error": err.Error(),
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: " + err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Create new session
	s := h.sessionMgr.Create()

	response := HandshakeResponse{
		SessionID: s.ID,
		Status:   string(s.Status),
		ExpiresAt: s.ExpiresAt.Format(time.RFC3339),
	}

	logrus.Infof("New handshake: session_id=%s", s.ID)

	// Audit log
	h.auditLogger.LogRequest("handshake", userID, clientIP, s.ID, requestID, true, time.Since(start), map[string]interface{}{
		"expires_at": s.ExpiresAt.Format(time.RFC3339),
	})

	c.JSON(http.StatusOK, response)
}

// Execute handles POST /opsx/execute
func (h *Handler) Execute(c *gin.Context) {
	start := time.Now()
	requestID := getRequestID(c)
	clientIP := getClientIP(c)
	userID := getUserID(c)

	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.auditLogger.LogRequest("execute", userID, clientIP, "", requestID, false, time.Since(start), map[string]interface{}{
			"error": "missing required fields",
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request: session_id and request are required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Get session
	s, exists := h.sessionMgr.Get(req.SessionID)
	if !exists {
		h.auditLogger.LogRequest("execute", userID, clientIP, req.SessionID, requestID, false, time.Since(start), map[string]interface{}{
			"error": "session not found",
		})

		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Session not found",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Check if session is still valid
	if s.Status == session.StatusClosed || s.Status == session.StatusExpired {
		h.auditLogger.LogRequest("execute", userID, clientIP, req.SessionID, requestID, false, time.Since(start), map[string]interface{}{
			"error":    "session no longer active",
			"status":   string(s.Status),
		})

		c.JSON(http.StatusGone, gin.H{
			"error": gin.H{
				"message": "Session is no longer active",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Store request
	h.sessionMgr.SetRequest(req.SessionID, req.Request)

	// Update status to running
	h.sessionMgr.SetRunning(req.SessionID)

	// Execute Claude Code
	result, err := h.claude.Execute(c.Request.Context(), req.Request)

	response := ExecuteResponse{
		SessionID: req.SessionID,
		Status:   string(session.StatusRunning),
	}

	if err != nil {
		// Execution failed
		h.sessionMgr.SetFailed(req.SessionID, err.Error())
		response.Status = string(session.StatusFailed)
		response.Error = err.Error()

		h.auditLogger.LogRequest("execute", userID, clientIP, req.SessionID, requestID, false, time.Since(start), map[string]interface{}{
			"error":      err.Error(),
			"request_len": len(req.Request),
		})

		c.JSON(http.StatusOK, response)
		return
	}

	// Generate summary
	summary := h.summarizer.Summarize(result.Output)
	h.sessionMgr.SetCompleted(req.SessionID, summary)

	response.Status = string(session.StatusCompleted)
	response.Summary = summary

	logrus.Infof("Execute completed: session_id=%s, duration=%v", req.SessionID, result.Duration)

	// Audit log
	h.auditLogger.LogRequest("execute", userID, clientIP, req.SessionID, requestID, true, time.Since(start), map[string]interface{}{
		"duration_ms":   result.Duration.Milliseconds(),
		"output_len":    len(result.Output),
		"summary_len":  len(summary),
	})

	c.JSON(http.StatusOK, response)
}

// Status handles GET /opsx/status/:session_id
func (h *Handler) Status(c *gin.Context) {
	start := time.Now()
	requestID := getRequestID(c)
	clientIP := getClientIP(c)
	userID := getUserID(c)

	sessionID := c.Param("session_id")

	s, exists := h.sessionMgr.Get(sessionID)
	if !exists {
		h.auditLogger.LogRequest("status", userID, clientIP, sessionID, requestID, false, time.Since(start), map[string]interface{}{
			"error": "session not found",
		})

		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Session not found",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	response := StatusResponse{
		SessionID:    s.ID,
		Status:      string(s.Status),
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
		LastActivity: s.LastActivity.Format(time.RFC3339),
		ExpiresAt:  s.ExpiresAt.Format(time.RFC3339),
	}

	// Include request if available
	if req, ok := h.sessionMgr.GetRequest(sessionID); ok {
		response.Request = req
	}

	// Include summary if available
	if s.Response != "" {
		response.Summary = s.Response
	}

	// Include error if failed
	if s.Error != "" {
		response.Error = s.Error
	}

	// Audit log
	h.auditLogger.LogRequest("status", userID, clientIP, sessionID, requestID, true, time.Since(start), map[string]interface{}{
		"status": string(s.Status),
	})

	c.JSON(http.StatusOK, response)
}

// Close handles POST /opsx/close
func (h *Handler) Close(c *gin.Context) {
	start := time.Now()
	requestID := getRequestID(c)
	clientIP := getClientIP(c)
	userID := getUserID(c)

	var req CloseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.auditLogger.LogRequest("close", userID, clientIP, "", requestID, false, time.Since(start), map[string]interface{}{
			"error": "missing session_id",
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request: session_id is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Check if session exists
	_, exists := h.sessionMgr.Get(req.SessionID)
	if !exists {
		h.auditLogger.LogRequest("close", userID, clientIP, req.SessionID, requestID, false, time.Since(start), map[string]interface{}{
			"error": "session not found",
		})

		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Session not found",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Close session
	h.sessionMgr.Close(req.SessionID)

	response := CloseResponse{
		SessionID: req.SessionID,
		Status:   string(session.StatusClosed),
		Message:  "Session closed successfully",
	}

	logrus.Infof("Session closed: session_id=%s", req.SessionID)

	// Audit log
	h.auditLogger.LogRequest("close", userID, clientIP, req.SessionID, requestID, true, time.Since(start), nil)

	c.JSON(http.StatusOK, response)
}
