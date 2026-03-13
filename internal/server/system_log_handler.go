package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// SystemLogEntry represents a system log entry for API response
type SystemLogEntry struct {
	Time    time.Time              `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// SystemLogsResponse represents the API response for system logs
type SystemLogsResponse struct {
	Total int              `json:"total"`
	Logs  []SystemLogEntry `json:"logs"`
}

// GetSystemLogs retrieves system logs with optional filtering
// Query parameters:
//   - limit: maximum number of recent entries to return (default: 100, max: 1000)
func (s *Server) GetSystemLogs(c *gin.Context) {
	if s.multiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "System logger not available",
		})
		return
	}

	// Parse query parameters
	// limit - controls how many recent entries to return
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000 // Max limit
	}

	// Read logs from JSON log file (only recent entries, system source only)
	entries, err := s.multiLogger.ReadJSONLogs(limit)
	if err != nil {
		logrus.Errorf("Failed to read system logs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read system logs",
		})
		return
	}

	// Convert to response format
	logs := make([]SystemLogEntry, len(entries))
	for i, entry := range entries {
		logs[i] = SystemLogEntry{
			Time:    entry.Time,
			Level:   entry.Level,
			Message: entry.Message,
			Fields:  entry.Fields,
		}
	}

	c.JSON(http.StatusOK, SystemLogsResponse{
		Total: len(logs),
		Logs:  logs,
	})
}

// GetSystemLogStats returns statistics about the system logs
func (s *Server) GetSystemLogStats(c *gin.Context) {
	if s.multiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "System logger not available",
		})
		return
	}

	// Get log file path
	logPath := s.multiLogger.GetJSONLogPath()

	// Read all logs to calculate stats (with a reasonable limit, system source only)
	entries, err := s.multiLogger.ReadJSONLogs(10000)
	if err != nil {
		logrus.Errorf("Failed to read system logs for stats: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read system logs",
		})
		return
	}

	// Count by level
	levelCounts := make(map[string]int)
	for _, entry := range entries {
		levelCounts[entry.Level]++
	}

	c.JSON(http.StatusOK, gin.H{
		"log_path":     logPath,
		"total":        len(entries),
		"level_counts": levelCounts,
	})
}

// GetSystemLogLevel returns the current system log level
func (s *Server) GetSystemLogLevel(c *gin.Context) {
	if s.multiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "System logger not available",
		})
		return
	}

	level := s.multiLogger.GetLevel()
	c.JSON(http.StatusOK, gin.H{
		"level": level.String(),
	})
}

// SystemLogLevelRequest represents a request to set the log level
type SystemLogLevelRequest struct {
	Level string `json:"level" binding:"required"`
}

// SetSystemLogLevel sets the minimum log level for system logs
func (s *Server) SetSystemLogLevel(c *gin.Context) {
	if s.multiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "System logger not available",
		})
		return
	}

	var req SystemLogLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	level, err := logrus.ParseLevel(req.Level)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid log level, use: debug, info, warn, error, fatal, panic",
		})
		return
	}

	s.multiLogger.SetLevel(level)
	logrus.SetLevel(level)

	c.JSON(http.StatusOK, gin.H{
		"message": "Log level updated",
		"level":   level.String(),
	})
}

// ActionHistoryEntry represents an action history entry for API response
type ActionHistoryEntry struct {
	Time    time.Time              `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Action  string                 `json:"action,omitempty"`
	Success bool                   `json:"success,omitempty"`
	Details interface{}            `json:"details,omitempty"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// ActionHistoryResponse represents the API response for action history
type ActionHistoryResponse struct {
	Total   int                  `json:"total"`
	Actions []ActionHistoryEntry `json:"actions"`
}

// GetActionHistory retrieves user action history from memory
// Query parameters:
//   - limit: maximum number of recent entries to return (default: 100, max: 1000)
func (s *Server) GetActionHistory(c *gin.Context) {
	if s.multiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Logger not available",
		})
		return
	}

	// Parse query parameters
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000 // Max limit
	}

	// Get action scoped logger
	actionLogger := s.multiLogger.WithSource(obs.LogSourceAction)
	entries := actionLogger.GetMemoryLatest(limit)

	// Convert to response format
	actions := make([]ActionHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		actionEntry := ActionHistoryEntry{
			Time:    entry.Time,
			Level:   entry.Level.String(),
			Message: entry.Message,
			Fields:  entry.Data,
		}

		// Extract action-specific fields
		if action, ok := entry.Data["action"].(string); ok {
			actionEntry.Action = action
		}
		if success, ok := entry.Data["success"].(bool); ok {
			actionEntry.Success = success
		}
		if details, ok := entry.Data["details"]; ok {
			actionEntry.Details = details
		}

		actions = append(actions, actionEntry)
	}

	c.JSON(http.StatusOK, ActionHistoryResponse{
		Total:   len(actions),
		Actions: actions,
	})
}

// GetActionStats returns statistics about user actions
func (s *Server) GetActionStats(c *gin.Context) {
	if s.multiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Logger not available",
		})
		return
	}

	// Get all action entries
	actionLogger := s.multiLogger.WithSource(obs.LogSourceAction)
	entries := actionLogger.GetMemoryEntries()

	// Count by action type
	actionCounts := make(map[string]int)
	successCounts := make(map[string]int)

	for _, entry := range entries {
		if action, ok := entry.Data["action"].(string); ok {
			actionCounts[action]++
			if success, ok := entry.Data["success"].(bool); ok && success {
				successCounts[action]++
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":          len(entries),
		"action_counts":  actionCounts,
		"success_counts": successCounts,
	})
}
