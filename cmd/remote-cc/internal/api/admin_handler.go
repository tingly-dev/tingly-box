package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/audit"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/config"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/middleware"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/session"
)

// AdminHandler handles admin API requests
type AdminHandler struct {
	sessionMgr  *session.Manager
	auditLogger *audit.Logger
	rateLimiter *middleware.RateLimiter
	config      *config.Config
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(sessionMgr *session.Manager, auditLogger *audit.Logger, rateLimiter *middleware.RateLimiter, cfg *config.Config) *AdminHandler {
	return &AdminHandler{
		sessionMgr:  sessionMgr,
		auditLogger: auditLogger,
		rateLimiter: rateLimiter,
		config:      cfg,
	}
}

// AuditLogEntry represents an audit log entry for API response
type AuditLogEntry struct {
	Timestamp   string                 `json:"timestamp"`
	Level       string                 `json:"level"`
	Action      string                 `json:"action"`
	UserID      string                 `json:"user_id"`
	ClientIP    string                 `json:"client_ip"`
	SessionID   string                 `json:"session_id"`
	RequestID   string                 `json:"request_id"`
	Success     bool                   `json:"success"`
	DurationMs  int64                  `json:"duration_ms"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// GetAuditLogs handles GET /admin/logs
// Query params: page, limit, action, user_id, start_date, end_date
func (h *AdminHandler) GetAuditLogs(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	// Parse pagination params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit > 100 {
		limit = 100
	}
	if page < 1 {
		page = 1
	}

	// Parse filters
	action := c.Query("action")
	userIDFilter := c.Query("user_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// Get logs from audit logger
	logs := h.auditLogger.GetLogs()

	// Apply filters
	var filteredLogs []*audit.Entry
	for _, entry := range logs {
		// Filter by action
		if action != "" && entry.Action != action {
			continue
		}
		// Filter by user ID
		if userIDFilter != "" && entry.UserID != userIDFilter {
			continue
		}
		// Filter by date range
		if startDate != "" {
			if entry.Timestamp.Before(parseDate(startDate)) {
				continue
			}
		}
		if endDate != "" {
			if entry.Timestamp.After(parseDate(endDate)) {
				continue
			}
		}
		filteredLogs = append(filteredLogs, entry)
	}

	// Paginate
	total := len(filteredLogs)
	startIdx := (page - 1) * limit
	endIdx := startIdx + limit
	if startIdx >= total {
		filteredLogs = []*audit.Entry{}
	} else if endIdx > total {
		filteredLogs = filteredLogs[startIdx:]
	} else {
		filteredLogs = filteredLogs[startIdx:endIdx]
	}

	// Convert to response format
	entries := make([]AuditLogEntry, len(filteredLogs))
	for i, entry := range filteredLogs {
		entries[i] = AuditLogEntry{
			Timestamp:   entry.Timestamp.Format(time.RFC3339),
			Level:       entry.Level.String(),
			Action:      entry.Action,
			UserID:      entry.UserID,
			ClientIP:    entry.ClientIP,
			SessionID:   entry.SessionID,
			RequestID:   entry.RequestID,
			Success:     entry.Success,
			DurationMs:  entry.DurationMs,
			Details:     entry.Details,
		}
	}

	logrus.Debugf("Admin audit logs request: page=%d, limit=%d, filtered=%d", page, limit, total)

	h.auditLogger.LogRequest("admin_logs", userID, clientIP, "", getRequestID(c), true, time.Since(start), map[string]interface{}{
		"page":   page,
		"limit":  limit,
		"total":  total,
		"action": action,
	})

	c.JSON(http.StatusOK, gin.H{
		"logs":        entries,
		"page":        page,
		"limit":       limit,
		"total":       total,
		"total_pages": (total + limit - 1) / limit,
	})
}

// StatsResponse represents the stats response
type StatsResponse struct {
	TotalSessions     int                    `json:"total_sessions"`
	ActiveSessions    int                    `json:"active_sessions"`
	CompletedSessions int                    `json:"completed_sessions"`
	FailedSessions    int                    `json:"failed_sessions"`
	ClosedSessions    int                    `json:"closed_sessions"`
	RecentActions     map[string]int         `json:"recent_actions"`
	Uptime            string                 `json:"uptime"`
	RateLimitStats    map[string]interface{} `json:"rate_limit_stats"`
}

// GetStats handles GET /admin/stats
func (h *AdminHandler) GetStats(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	// Get session stats
	stats := h.sessionMgr.GetStats()

	// Get rate limit stats
	rateLimitStats := h.rateLimiter.GetStats()

	response := StatsResponse{
		TotalSessions:     stats["total"].(int),
		ActiveSessions:    stats["active"].(int),
		CompletedSessions: stats["completed"].(int),
		FailedSessions:    stats["failed"].(int),
		ClosedSessions:    stats["closed"].(int),
		RecentActions:     stats["recent_actions"].(map[string]int),
		Uptime:            stats["uptime"].(string),
		RateLimitStats:    rateLimitStats,
	}

	logrus.Debugf("Admin stats request completed")

	h.auditLogger.LogRequest("admin_stats", userID, clientIP, "", getRequestID(c), true, time.Since(start), nil)

	c.JSON(http.StatusOK, response)
}

// GetRateLimitStats handles GET /admin/ratelimit/stats
func (h *AdminHandler) GetRateLimitStats(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	stats := h.rateLimiter.GetStats()

	h.auditLogger.LogRequest("admin_ratelimit_stats", userID, clientIP, "", getRequestID(c), true, time.Since(start), nil)

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
	})
}

// ResetRateLimitRequest represents the request body for resetting rate limit
type ResetRateLimitRequest struct {
	IP string `json:"ip"`
}

// ResetRateLimit handles POST /admin/ratelimit/reset
func (h *AdminHandler) ResetRateLimit(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	var req ResetRateLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.auditLogger.LogRequest("admin_ratelimit_reset", userID, clientIP, "", getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": "invalid request body",
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: ip is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if req.IP == "" {
		h.auditLogger.LogRequest("admin_ratelimit_reset", userID, clientIP, "", getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": "ip is required",
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "IP is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	h.rateLimiter.ResetIP(req.IP)

	logrus.Infof("Rate limit reset for IP: %s by admin %s", req.IP, userID)

	h.auditLogger.LogRequest("admin_ratelimit_reset", userID, clientIP, "", getRequestID(c), true, time.Since(start), map[string]interface{}{
		"ip": req.IP,
	})

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Rate limit reset for IP: " + req.IP,
	})
}

// parseDate parses a date string in RFC3339 or YYYY-MM-DD format
func parseDate(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try YYYY-MM-DD format
		t, err = time.Parse("2006-01-02", s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}

// TokenInfo represents token information for response
type TokenInfo struct {
	Token    string `json:"token"`
	ClientID string `json:"client_id"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// GenerateTokenRequest represents the request body for generating a token
type GenerateTokenRequest struct {
	ClientID string `json:"client_id" binding:"required"`
	Description string `json:"description,omitempty"`
	ExpiryHours int `json:"expiry_hours,omitempty"` // 0 = no expiry
}

// GenerateToken handles POST /admin/tokens/generate
func (h *AdminHandler) GenerateToken(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	var req GenerateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.auditLogger.LogRequest("admin_token_generate", userID, clientIP, "", getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": "invalid request body",
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: client_id is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Generate token
	token, err := h.config.GenerateToken(req.ClientID, req.ExpiryHours)
	if err != nil {
		h.auditLogger.LogRequest("admin_token_generate", userID, clientIP, "", getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error":      err.Error(),
			"client_id":  req.ClientID,
		})

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to generate token: " + err.Error(),
				"type":    "server_error",
			},
		})
		return
	}

	logrus.Infof("Admin generated token for client: %s", req.ClientID)

	h.auditLogger.LogRequest("admin_token_generate", userID, clientIP, "", getRequestID(c), true, time.Since(start), map[string]interface{}{
		"client_id":  req.ClientID,
		"expiry_hours": req.ExpiryHours,
	})

	response := TokenInfo{
		Token:     token,
		ClientID:  req.ClientID,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	if req.ExpiryHours > 0 {
		response.ExpiresAt = time.Now().Add(time.Duration(req.ExpiryHours) * time.Hour).Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, gin.H{
		"token": response,
		"status": "success",
		"message": "Token generated successfully. Save this token - it cannot be retrieved again.",
	})
}

// ListTokensRequest represents the request body for listing tokens
type ListTokensRequest struct {
	ActiveOnly bool `json:"active_only,omitempty"`
}

// TokenValidationResult represents the result of token validation
type TokenValidationResult struct {
	Valid    bool   `json:"valid"`
	ClientID string `json:"client_id,omitempty"`
	Message  string `json:"message,omitempty"`
}

// ValidateToken handles POST /admin/tokens/validate
func (h *AdminHandler) ValidateToken(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.auditLogger.LogRequest("admin_token_validate", userID, clientIP, "", getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": "invalid request body",
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: token is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Validate token using JWT manager
	claims, err := h.config.ValidateToken(req.Token)
	if err != nil {
		h.auditLogger.LogRequest("admin_token_validate", userID, clientIP, "", getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": err.Error(),
		})

		c.JSON(http.StatusOK, gin.H{
			"result": TokenValidationResult{
				Valid:   false,
				Message: err.Error(),
			},
		})
		return
	}

	h.auditLogger.LogRequest("admin_token_validate", userID, clientIP, "", getRequestID(c), true, time.Since(start), map[string]interface{}{
		"client_id": claims.ClientID,
	})

	c.JSON(http.StatusOK, gin.H{
		"result": TokenValidationResult{
			Valid:    true,
			ClientID: claims.ClientID,
			Message:  "Token is valid",
		},
	})
}

// RevokeTokenRequest represents the request body for revoking a token
type RevokeTokenRequest struct {
	ClientID string `json:"client_id" binding:"required"`
}

// RevokeToken handles POST /admin/tokens/revoke
func (h *AdminHandler) RevokeToken(c *gin.Context) {
	start := time.Now()
	clientIP := c.ClientIP()
	userID := getUserID(c)

	var req RevokeTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.auditLogger.LogRequest("admin_token_revoke", userID, clientIP, "", getRequestID(c), false, time.Since(start), map[string]interface{}{
			"error": "invalid request body",
		})

		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: client_id is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Note: In a production system, you would maintain a revocation list
	// For now, we just log the revocation request
	logrus.Infof("Admin requested token revocation for client: %s", req.ClientID)

	h.auditLogger.LogRequest("admin_token_revoke", userID, clientIP, "", getRequestID(c), true, time.Since(start), map[string]interface{}{
		"client_id": req.ClientID,
	})

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Token revocation request recorded. Note: JWT tokens cannot be immediately revoked without a token blacklist.",
	})
}
