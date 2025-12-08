package config

import "path/filepath"

const (
	// ConfigDirName is the main configuration directory name
	ConfigDirName = ".tingly-box"

	// ModelsDirName is the subdirectory for provider model configurations
	ModelsDirName = "models"
)

// GetTinglyConfDir returns the config directory path
func GetTinglyConfDir() string {
	return ConfigDirName
}

// GetModelsDir returns the models directory path
func GetModelsDir() string {
	return filepath.Join(ConfigDirName, ModelsDirName)
}
