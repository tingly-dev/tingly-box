package config

import (
	"path/filepath"
	"time"
	"tingly-box/internal/util"
)

const (
	// ConfigDirName is the main configuration directory name
	ConfigDirName = ".tingly-box"

	// ModelsDirName is the subdirectory for provider model configurations
	ModelsDirName = "models"

	LogDirName      = "log"
	StateDirName    = "state"
	StatsDBFileName = "stats.db" // SQLite database file

	// DebugLogFileName is the name of the debug log file
	DebugLogFileName = "bad_requests.log"

	// DefaultRequestTimeout is the default timeout for HTTP requests in seconds
	DefaultRequestTimeout = 1800 * time.Second

	// DefaultMaxTokens is the default max_tokens value for API requests
	DefaultMaxTokens = 8192

	// Load balancing threshold defaults
	DefaultRequestThreshold = int64(100)   // Default request threshold for round-robin and hybrid tactics
	DefaultTokenThreshold   = int64(10000) // Default token threshold for token-based and hybrid tactics
)

// GetTinglyConfDir returns the config directory path (default: ~/.tingly-box)
func GetTinglyConfDir() string {
	homeDir, err := util.GetUserPath()
	if err != nil {
		// Fallback to current directory if home directory is not accessible
		return ConfigDirName
	}
	return filepath.Join(homeDir, ConfigDirName)
}

// GetModelsDir returns the models directory path
func GetModelsDir() string {
	return filepath.Join(GetTinglyConfDir(), ModelsDirName)
}

// GetStateDir returns the directory used for persisted runtime state
func GetStateDir() string {
	return filepath.Join(GetTinglyConfDir(), StateDirName)
}
