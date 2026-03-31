package serverguardrails

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	guardrailsDirName         = "guardrails"
	guardrailsConfigBaseName  = "guardrails"
	guardrailsDBDirName       = "db"
	guardrailsDBFileName      = "guardrails.db"
	guardrailsHistoryFileName = "history.json"
)

func GetGuardrailsDir(configDir string) string {
	return filepath.Join(configDir, guardrailsDirName)
}

func GetGuardrailsConfigPath(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsConfigBaseName+".yaml")
}

func GetGuardrailsHistoryPath(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsHistoryFileName)
}

func GetGuardrailsDBDir(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsDBDirName)
}

func GetGuardrailsDBPath(configDir string) string {
	return filepath.Join(GetGuardrailsDBDir(configDir), guardrailsDBFileName)
}

func guardrailsConfigCandidates(configDir string) []string {
	newDir := GetGuardrailsDir(configDir)
	return []string{
		filepath.Join(newDir, guardrailsConfigBaseName+".yaml"),
		filepath.Join(newDir, guardrailsConfigBaseName+".yml"),
		filepath.Join(newDir, guardrailsConfigBaseName+".json"),
		// Keep the legacy flat-file paths as a fallback while storage moves under
		// the dedicated guardrails directory.
		filepath.Join(configDir, guardrailsConfigBaseName+".yaml"),
		filepath.Join(configDir, guardrailsConfigBaseName+".yml"),
		filepath.Join(configDir, guardrailsConfigBaseName+".json"),
	}
}

func FindGuardrailsConfig(configDir string) (string, error) {
	if configDir == "" {
		return "", fmt.Errorf("config dir is empty")
	}

	for _, path := range guardrailsConfigCandidates(configDir) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no guardrails config in %s", GetGuardrailsDir(configDir))
}
