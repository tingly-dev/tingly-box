package obs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"tingly-box/internal/util"
)

// ActionType represents the type of action performed
type ActionType string

const (
	ActionAddProvider    ActionType = "add_provider"
	ActionDeleteProvider ActionType = "delete_provider"
	ActionUpdateProvider ActionType = "update_provider"
	ActionStartServer    ActionType = "start_server"
	ActionStopServer     ActionType = "stop_server"
	ActionRestartServer  ActionType = "restart_server"
	ActionGenerateToken  ActionType = "generate_token"
	ActionUpdateDefaults ActionType = "update_defaults"
	ActionFetchModels    ActionType = "fetch_models"
)

// HistoryEntry represents a single history entry
type HistoryEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	Action    ActionType  `json:"action"`
	Details   interface{} `json:"details"`
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
}

// ServerStatus represents server status information
type ServerStatus struct {
	Timestamp    time.Time `json:"timestamp"`
	Running      bool      `json:"running"`
	Port         int       `json:"port"`
	Uptime       string    `json:"uptime"`
	RequestCount int       `json:"request_count"`
}

// MemoryLogger manages logging and history
type MemoryLogger struct {
	historyFile    string
	statusFile     string
	historyEntries []HistoryEntry
	currentStatus  *ServerStatus
	mu             sync.RWMutex
}

// NewMemoryLogger creates a new memory logger
func NewMemoryLogger() (*MemoryLogger, error) {
	home, _ := util.GetUserPath()
	memoryDir := filepath.Join(home, ".tingly-box", "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create memory directory: %w", err)
	}

	logger := &MemoryLogger{
		historyFile: filepath.Join(memoryDir, "history.json"),
		statusFile:  filepath.Join(memoryDir, "status.json"),
	}

	// Load existing history
	if err := logger.loadHistory(); err != nil {
		fmt.Printf("Warning: Failed to load history: %v\n", err)
	}

	// Load current status
	if err := logger.loadStatus(); err != nil {
		fmt.Printf("Warning: Failed to load status: %v\n", err)
	}

	return logger, nil
}

// LogAction logs an action to history
func (ml *MemoryLogger) LogAction(action ActionType, details interface{}, success bool, message string) error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	entry := HistoryEntry{
		Timestamp: time.Now(),
		Action:    action,
		Details:   details,
		Success:   success,
		Message:   message,
	}

	ml.historyEntries = append(ml.historyEntries, entry)

	// Keep only last 100 entries
	if len(ml.historyEntries) > 100 {
		ml.historyEntries = ml.historyEntries[len(ml.historyEntries)-100:]
	}

	return ml.saveHistory()
}

// UpdateServerStatus updates server status
func (ml *MemoryLogger) UpdateServerStatus(running bool, port int, uptime string, requestCount int) error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	ml.currentStatus = &ServerStatus{
		Timestamp:    time.Now(),
		Running:      running,
		Port:         port,
		Uptime:       uptime,
		RequestCount: requestCount,
	}

	return ml.saveStatus()
}

// GetHistory returns recent history entries
func (ml *MemoryLogger) GetHistory(limit int) []HistoryEntry {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	if limit <= 0 || limit > len(ml.historyEntries) {
		limit = len(ml.historyEntries)
	}

	start := len(ml.historyEntries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]HistoryEntry, limit)
	copy(result, ml.historyEntries[start:])
	return result
}

// GetCurrentStatus returns current server status
func (ml *MemoryLogger) GetCurrentStatus() *ServerStatus {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	if ml.currentStatus == nil {
		return &ServerStatus{
			Timestamp: time.Now(),
			Running:   false,
			Port:      0,
		}
	}

	// Return a copy to prevent concurrent modification
	statusCopy := *ml.currentStatus
	return &statusCopy
}

// GetActionStats returns statistics for different action types
func (ml *MemoryLogger) GetActionStats() map[ActionType]int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	stats := make(map[ActionType]int)
	for _, entry := range ml.historyEntries {
		stats[entry.Action]++
	}
	return stats
}

// loadHistory loads history from file
func (ml *MemoryLogger) loadHistory() error {
	data, err := os.ReadFile(ml.historyFile)
	if os.IsNotExist(err) {
		ml.historyEntries = []HistoryEntry{}
		return nil
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &ml.historyEntries)
}

// saveHistory saves history to file
func (ml *MemoryLogger) saveHistory() error {
	data, err := json.MarshalIndent(ml.historyEntries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ml.historyFile, data, 0644)
}

// loadStatus loads status from file
func (ml *MemoryLogger) loadStatus() error {
	data, err := os.ReadFile(ml.statusFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &ml.currentStatus)
}

// saveStatus saves status to file
func (ml *MemoryLogger) saveStatus() error {
	data, err := json.MarshalIndent(ml.currentStatus, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ml.statusFile, data, 0644)
}

// ClearHistory clears all history entries
func (ml *MemoryLogger) ClearHistory() error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	ml.historyEntries = []HistoryEntry{}
	return ml.saveHistory()
}
