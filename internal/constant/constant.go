package constant

import (
	"path/filepath"

	"github.com/tingly-dev/tingly-box/pkg/fs"
)

const (
	// Default authentication tokens
	DefaultUserToken         = "tingly-box-user-token"
	DefaultModelToken        = "tingly-box-model-token"
	DefaultVirtualModelToken = "tingly-virtual-model-token"
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
	// ModelFetchTimeout is the timeout for fetching models from provider API in seconds
	ModelFetchTimeout = 30

	// DefaultMaxTokens is the default max_tokens value for API requests
	DefaultMaxTokens = 8192

	// Template cache constants

)

const DBFileName = "tingly.db" // Unified SQLite database file

// Enterprise database file (separate from community edition)
const EnterpriseDBFileName = "tingly_enterprise.db"

// Load balancing threshold defaults
const DefaultRequestThreshold = int64(10)  // Default request threshold for round-robin and hybrid tactics
const DefaultTokenThreshold = int64(10000) // Default token threshold for token-based and hybrid tactics

const ConfigDirName = ".tingly-box"

const DBDirName = "db"

const MemoryDirName = "memory"

// GetTinglyConfDir returns the config directory path (default: ~/.tingly-box)
func GetTinglyConfDir() string {
	homeDir, err := fs.GetUserPath()
	if err != nil {
		// Fallback to current directory if home directory is not accessible
		return ConfigDirName
	}
	return filepath.Join(homeDir, ConfigDirName)
}

// GetMemoryDir returns the memory directory path
func GetMemoryDir(baseDir string) string {
	return filepath.Join(baseDir, MemoryDirName)
}

// GetLogDir returns the log directory path
func GetLogDir(baseDir string) string {
	return filepath.Join(baseDir, LogDirName)
}

func GetDBDir(baseDir string) string {
	return filepath.Join(baseDir, DBDirName)
}

func GetDBFile(baseDir string) string {
	return filepath.Join(baseDir, DBDirName, DBFileName)
}

// GetEnterpriseDBFile returns the enterprise database file path (separate from community db)
func GetEnterpriseDBFile(baseDir string) string {
	return filepath.Join(baseDir, DBDirName, EnterpriseDBFileName)
}
