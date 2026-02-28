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

// Load balancing threshold defaults
const DefaultRequestThreshold = int64(10)  // Default request threshold for round-robin and hybrid tactics
const DefaultTokenThreshold = int64(10000) // Default token threshold for token-based and hybrid tactics

// Latency-based routing defaults
const (
	DefaultLatencyThresholdMs    = int64(2000)  // Default latency threshold in milliseconds
	DefaultLatencySampleWindow   = 100          // Default number of latency samples to keep
	DefaultLatencyPercentile     = 0.95         // Default percentile for latency comparison (0.95 = p95)
	DefaultLatencyComparisonMode = "avg"        // Default comparison mode: "avg", "p50", "p95", "p99"
)

// Token speed-based routing defaults
const (
	DefaultMinSpeedSamples     = 5     // Minimum samples required before making speed-based decisions
	DefaultSpeedThresholdTps   = 50.0  // Minimum acceptable tokens per second
	DefaultSpeedSampleWindow   = 100   // Default number of speed samples to keep
)

// Adaptive routing defaults
const (
	DefaultLatencyWeight   = 0.4   // Weight for latency in adaptive scoring
	DefaultTokenWeight     = 0.3   // Weight for token usage in adaptive scoring
	DefaultSpeedWeight     = 0.2   // Weight for token speed in adaptive scoring
	DefaultHealthWeight    = 0.1   // Weight for health status in adaptive scoring
	DefaultScoringMode     = "weighted_sum"  // Default scoring mode: "weighted_sum", "multiplicative", "rank_based"
)

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
