package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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
func (s *Server) GetLogs(c *gin.Context) {
	if s.memoryLogMW == nil {
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
		entries = s.memoryLogMW.GetEntriesByLevel(level)
	} else if sinceStr != "" {
		// Filter by time if specified
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid since timestamp, use RFC3339 format",
			})
			return
		}
		entries = s.memoryLogMW.GetEntriesSince(since)
	} else {
		// Get latest entries
		entries = s.memoryLogMW.GetLatest(limit)
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
func (s *Server) ClearLogs(c *gin.Context) {
	if s.memoryLogMW == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Memory log middleware not available",
		})
		return
	}

	s.memoryLogMW.Clear()
	c.JSON(http.StatusOK, gin.H{
		"message": "Logs cleared successfully",
	})
}

// GetLogStats returns statistics about the logs
func (s *Server) GetLogStats(c *gin.Context) {
	if s.memoryLogMW == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Memory log middleware not available",
		})
		return
	}

	// Get all entries to calculate stats
	entries := s.memoryLogMW.GetEntries()

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
