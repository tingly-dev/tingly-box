package config

import (
	"path/filepath"
	"time"
)

const (
	// ConfigDirName is the main configuration directory name
	ConfigDirName = ".tingly-box"

	// ModelsDirName is the subdirectory for provider model configurations
	ModelsDirName = "models"

	LogDirName = "log"

	// DebugLogFileName is the name of the debug log file
	DebugLogFileName = "bad_requests.log"

	// RequestTimeout is the default timeout for HTTP requests in seconds
	RequestTimeout = 60 * time.Second
)

// GetTinglyConfDir returns the config directory path
func GetTinglyConfDir() string {
	return ConfigDirName
}

// GetModelsDir returns the models directory path
func GetModelsDir() string {
	return filepath.Join(ConfigDirName, ModelsDirName)
}
