package constant

import (
	"path/filepath"

	"tingly-box/internal/util"
)

const (
	// ConfigDirName is the main configuration directory name

	// ModelsDirName is the subdirectory for provider model configurations

	LogDirName = "log"

	// DebugLogFileName is the name of the debug log file
	DebugLogFileName = "bad_requests.log"

	// DefaultRequestTimeout is the default timeout for HTTP requests in seconds
	DefaultRequestTimeout = 1800
	// DefaultMaxTimeout in seconds
	DefaultMaxTimeout = 30 * 60

	// DefaultMaxTokens is the default max_tokens value for API requests
	DefaultMaxTokens = 8192

	// Template cache constants

)

const StateDirName = "state"

const StatsDBFileName = "stats.db"   // SQLite database file
const ModelsDBFileName = "models.db" // SQLite database file for provider models

// Load balancing threshold defaults
const DefaultRequestThreshold = int64(10)  // Default request threshold for round-robin and hybrid tactics
const DefaultTokenThreshold = int64(10000) // Default token threshold for token-based and hybrid tactics

const ConfigDirName = ".tingly-box"

const ModelsDirName = "models"

const MemoryDirName = "memory"

// GetStateDir returns the directory used for persisted runtime state
func GetStateDir() string {
	return filepath.Join(GetTinglyConfDir(), StateDirName)
}

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

// GetMemoryDir returns the memory directory path
func GetMemoryDir() string {
	return filepath.Join(GetTinglyConfDir(), MemoryDirName)
}

// GetLogDir returns the log directory path
func GetLogDir() string {
	return filepath.Join(GetTinglyConfDir(), LogDirName)
}
