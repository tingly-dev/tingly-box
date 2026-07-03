package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// LogEntry represents a log entry for API response
type LogEntry struct {
	Time    time.Time              `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// LogsResponse represents the API response for logs
type LogsResponse struct {
	Total int        `json:"total"`
	Logs  []LogEntry `json:"logs"`
}

// convertLogrusEntry converts a logrus.Entry to LogEntry for API response
func convertLogrusEntry(entry *logrus.Entry) LogEntry {
	data := make(map[string]interface{})
	for k, v := range entry.Data {
		data[k] = v
	}

	// Extract standard HTTP fields for easier access
	fields := make(map[string]interface{})
	if status, ok := data["status"].(int); ok {
		fields["status"] = status
	}
	if latency, ok := data["latency"].(time.Duration); ok {
		fields["latency_ms"] = latency.Milliseconds()
	}
	if clientIP, ok := data["client_ip"].(string); ok {
		fields["client_ip"] = clientIP
	}
	if method, ok := data["method"].(string); ok {
		fields["method"] = method
	}
	if path, ok := data["path"].(string); ok {
		fields["path"] = path
	}
	if bodySize, ok := data["body_size"].(int); ok {
		fields["body_size"] = bodySize
	}
	if userAgent, ok := data["user_agent"].(string); ok {
		fields["user_agent"] = userAgent
	}

	return LogEntry{
		Time:    entry.Time,
		Level:   entry.Level.String(),
		Message: entry.Message,
		Data:    data,
		Fields:  fields,
	}
}

// GetLogs retrieves logs with optional filtering
// Query parameters:
//   - limit: maximum number of entries to return (default: 100)
//   - level: filter by log level (debug, info, warn, error)
//   - since: RFC3339 timestamp to filter entries after this time
func (h *WebHandler) GetLogs(c *gin.Context) {
	if h.deps.MemoryLogMW == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Memory log middleware not available",
		})
		return
	}

	// Parse query parameters
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500 // Max limit
	}

	levelStr := c.Query("level")
	sinceStr := c.Query("since")

	var entries []*logrus.Entry

	// Filter by level if specified
	if levelStr != "" {
		level, err := logrus.ParseLevel(levelStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid log level",
			})
			return
		}
		entries = h.deps.MemoryLogMW.GetEntriesByLevel(level)
	} else if sinceStr != "" {
		// Filter by time if specified
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid since timestamp, use RFC3339 format",
			})
			return
		}
		entries = h.deps.MemoryLogMW.GetEntriesSince(since)
	} else {
		// Get latest entries
		entries = h.deps.MemoryLogMW.GetLatestEntries(limit)
	}

	// Convert entries and apply limit
	result := make([]LogEntry, 0, len(entries))
	for i, entry := range entries {
		if i >= limit {
			break
		}
		result = append(result, convertLogrusEntry(entry))
	}

	c.JSON(http.StatusOK, LogsResponse{
		Total: len(result),
		Logs:  result,
	})
}

// ClearLogs clears all log entries
func (h *WebHandler) ClearLogs(c *gin.Context) {
	if h.deps.MemoryLogMW == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Memory log middleware not available",
		})
		return
	}

	h.deps.MemoryLogMW.Clear()
	c.JSON(http.StatusOK, gin.H{
		"message": "Logs cleared successfully",
	})
}

// GetLogStats returns statistics about the logs
func (h *WebHandler) GetLogStats(c *gin.Context) {
	if h.deps.MemoryLogMW == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Memory log middleware not available",
		})
		return
	}

	// Get all entries to calculate stats
	entries := h.deps.MemoryLogMW.GetEntries()

	// Count by level
	levelCounts := make(map[string]int)
	for _, entry := range entries {
		levelCounts[entry.Level.String()]++
	}

	c.JSON(http.StatusOK, gin.H{
		"total":        len(entries),
		"level_counts": levelCounts,
		"capacity":     500, // maxEntries configured in NewServer
	})
}

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
func (h *WebHandler) GetSystemLogs(c *gin.Context) {
	if h.deps.MultiLogger == nil {
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

	// Read logs from JSON log file, keeping system logs and HTTP access logs.
	// AI model endpoint requests are included here; their detailed timeline
	// is available in the AI Logs view (correlated by request_id).
	entries, err := h.deps.MultiLogger.ReadJSONLogsBySource(limit, obs.LogSourceSystem, obs.LogSourceAction, obs.LogSourceUnknown, obs.LogSourceHTTP)
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
func (h *WebHandler) GetSystemLogStats(c *gin.Context) {
	if h.deps.MultiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "System logger not available",
		})
		return
	}

	// Get log file path
	logPath := h.deps.MultiLogger.GetJSONLogPath()

	// Read all logs to calculate stats (with a reasonable limit, system source only)
	entries, err := h.deps.MultiLogger.ReadJSONLogs(500)
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
func (h *WebHandler) GetSystemLogLevel(c *gin.Context) {
	if h.deps.MultiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "System logger not available",
		})
		return
	}

	level := h.deps.MultiLogger.GetLevel()
	c.JSON(http.StatusOK, gin.H{
		"level": level.String(),
	})
}

// SystemLogLevelRequest represents a request to set the log level
type SystemLogLevelRequest struct {
	Level string `json:"level" binding:"required"`
}

// SetSystemLogLevel sets the minimum log level for system logs
func (h *WebHandler) SetSystemLogLevel(c *gin.Context) {
	if h.deps.MultiLogger == nil {
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

	h.deps.MultiLogger.SetLevel(level)
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
func (h *WebHandler) GetActionHistory(c *gin.Context) {
	if h.deps.MultiLogger == nil {
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
	actionLogger := h.deps.MultiLogger.WithSource(obs.LogSourceAction)
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
func (h *WebHandler) GetActionStats(c *gin.Context) {
	if h.deps.MultiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Logger not available",
		})
		return
	}

	// Get all action entries
	actionLogger := h.deps.MultiLogger.WithSource(obs.LogSourceAction)
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
