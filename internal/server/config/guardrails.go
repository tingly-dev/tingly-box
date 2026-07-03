// Package guardrailspath holds filesystem-layout helpers for the guardrails
// config/storage directory. It is a dedicated leaf package (rather than
// living in root server or webui) because both the WebUI Management API's
// admin handlers (internal/server/webui) and the AI Model API's runtime
// evaluation (internal/server/aimodel, root server today) need it, and those
// two cannot import each other.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
	"gopkg.in/yaml.v3"
)

const (
	guardrailsDirName               = "guardrails"
	guardrailsConfigBaseName        = "guardrails"
	guardrailsBuiltinDirName        = "builtin"
	guardrailsCacheDirName          = "cache"
	guardrailsCustomDirName         = "custom"
	guardrailsRemoteDirName         = "remote"
	guardrailsDBDirName             = "db"
	guardrailsDBFileName            = "guardrails.db"
	guardrailsHistoryFileName       = "history.json"
	guardrailsRegistryCacheFileName = "registry_index.json"
)

func Dir(configDir string) string {
	return filepath.Join(configDir, guardrailsDirName)
}

func ConfigPath(configDir string) string {
	return filepath.Join(Dir(configDir), guardrailsConfigBaseName+".yaml")
}

func HistoryPath(configDir string) string {
	return filepath.Join(Dir(configDir), guardrailsHistoryFileName)
}

func DBDir(configDir string) string {
	return filepath.Join(Dir(configDir), guardrailsDBDirName)
}

func CustomDir(configDir string) string {
	return filepath.Join(Dir(configDir), guardrailsCustomDirName)
}

func BuiltinDir(configDir string) string {
	return filepath.Join(Dir(configDir), guardrailsBuiltinDirName)
}

func CacheDir(configDir string) string {
	return filepath.Join(Dir(configDir), guardrailsCacheDirName)
}

func RemoteDir(configDir string) string {
	return filepath.Join(Dir(configDir), guardrailsRemoteDirName)
}

func DBPath(configDir string) string {
	return filepath.Join(DBDir(configDir), guardrailsDBFileName)
}

func RegistryCachePath(configDir string) string {
	return filepath.Join(CacheDir(configDir), guardrailsRegistryCacheFileName)
}

// Prefer the new nested guardrails directory, but keep the legacy flat-file
// locations readable during the transition.
func configCandidates(configDir string) []string {
	newDir := Dir(configDir)
	return []string{
		filepath.Join(newDir, guardrailsConfigBaseName+".yaml"),
		filepath.Join(newDir, guardrailsConfigBaseName+".yml"),
		filepath.Join(newDir, guardrailsConfigBaseName+".json"),
		filepath.Join(configDir, guardrailsConfigBaseName+".yaml"),
		filepath.Join(configDir, guardrailsConfigBaseName+".yml"),
		filepath.Join(configDir, guardrailsConfigBaseName+".json"),
	}
}

// FindConfig locates the guardrails config file, checking the nested
// directory first and falling back to legacy flat-file locations.
func FindConfig(configDir string) (string, error) {
	if configDir == "" {
		return "", fmt.Errorf("config dir is empty")
	}

	for _, path := range configCandidates(configDir) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no guardrails config in %s", Dir(configDir))
}

// EnsurePath returns the guardrails config path, lazily choosing the default
// location when no config has been written yet. Used by the editor-style
// endpoints, which are allowed to create the default file on first write.
func EnsurePath(configDir string) (string, error) {
	path, err := FindConfig(configDir)
	if err == nil {
		return path, nil
	}
	if strings.Contains(err.Error(), "no guardrails config") || errors.Is(err, os.ErrNotExist) {
		return ConfigPath(configDir), nil
	}
	return "", err
}

// MarshalConfig serializes a guardrails config to its on-disk YAML form.
func MarshalConfig(cfg guardrailscore.Config) ([]byte, error) {
	return yaml.Marshal(guardrailsevaluate.StorageConfig(cfg))
}

// WriteFileAtomic keeps config/history writes crash-safe: it writes to a
// temp file in the same directory then renames it into place.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CredentialStore opens the protected-credential sqlite store shared by
// request-time masking (AI Model API) and the admin credential CRUD (WebUI
// Management API).
func CredentialStore(configDir string) (*guardrailsutils.ProtectedCredentialStore, error) {
	if configDir == "" {
		return nil, errors.New("config directory not set")
	}
	return guardrailsutils.NewProtectedCredentialStore(DBPath(configDir)), nil
}
