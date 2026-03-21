package server

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	guardrailsDirName         = "guardrails"
	guardrailsConfigBaseName  = "guardrails"
	guardrailsCredentialsFile = "credentials.json"
	guardrailsHistoryFileName = "history.json"
)

func getGuardrailsDir(configDir string) string {
	return filepath.Join(configDir, guardrailsDirName)
}

func getGuardrailsConfigPath(configDir string) string {
	return filepath.Join(getGuardrailsDir(configDir), guardrailsConfigBaseName+".yaml")
}

func getGuardrailsHistoryPath(configDir string) string {
	return filepath.Join(getGuardrailsDir(configDir), guardrailsHistoryFileName)
}

func getGuardrailsCredentialsPath(configDir string) string {
	return filepath.Join(getGuardrailsDir(configDir), guardrailsCredentialsFile)
}

func guardrailsConfigCandidates(configDir string) []string {
	newDir := getGuardrailsDir(configDir)
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

func findGuardrailsConfig(configDir string) (string, error) {
	if configDir == "" {
		return "", fmt.Errorf("config dir is empty")
	}

	for _, path := range guardrailsConfigCandidates(configDir) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no guardrails config in %s", getGuardrailsDir(configDir))
}
