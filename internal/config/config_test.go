package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestConfig_DebugVerbose_DefaultValues tests that Debug and Verbose default to false
func TestConfig_DebugVerbose_DefaultValues(t *testing.T) {
	configDir := t.TempDir()
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Test default values
	if cfg.Debug {
		t.Error("Expected Debug to default to false, got true")
	}
	if cfg.Verbose {
		t.Error("Expected Verbose to default to false, got true")
	}
}

// TestConfig_SetDebug tests setting and getting Debug field
func TestConfig_SetDebug(t *testing.T) {
	configDir := t.TempDir()
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Test setting Debug to true
	err = cfg.SetDebug(true)
	if err != nil {
		t.Fatalf("Failed to set Debug: %v", err)
	}

	if !cfg.GetDebug() {
		t.Error("Expected Debug to be true after SetDebug(true)")
	}

	// Test setting Debug to false
	err = cfg.SetDebug(false)
	if err != nil {
		t.Fatalf("Failed to set Debug: %v", err)
	}

	if cfg.GetDebug() {
		t.Error("Expected Debug to be false after SetDebug(false)")
	}
}

// TestConfig_SetVerbose tests setting and getting Verbose field
func TestConfig_SetVerbose(t *testing.T) {
	configDir := t.TempDir()
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Test setting Verbose to true
	err = cfg.SetVerbose(true)
	if err != nil {
		t.Fatalf("Failed to set Verbose: %v", err)
	}

	if !cfg.GetVerbose() {
		t.Error("Expected Verbose to be true after SetVerbose(true)")
	}

	// Test setting Verbose to false
	err = cfg.SetVerbose(false)
	if err != nil {
		t.Fatalf("Failed to set Verbose: %v", err)
	}

	if cfg.GetVerbose() {
		t.Error("Expected Verbose to be false after SetVerbose(false)")
	}
}

// TestConfig_DebugVerbose_Persistence tests that Debug and Verbose are persisted to JSON
func TestConfig_DebugVerbose_Persistence(t *testing.T) {
	configDir := t.TempDir()

	// Create initial config and set Debug/Verbose
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	err = cfg.SetDebug(true)
	if err != nil {
		t.Fatalf("Failed to set Debug: %v", err)
	}

	err = cfg.SetVerbose(true)
	if err != nil {
		t.Fatalf("Failed to set Verbose: %v", err)
	}

	// Read the config file to verify JSON serialization
	configFile := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var jsonConfig map[string]interface{}
	if err := json.Unmarshal(data, &jsonConfig); err != nil {
		t.Fatalf("Failed to unmarshal config JSON: %v", err)
	}

	// Verify Debug is in JSON and set to true
	debugVal, ok := jsonConfig["debug"]
	if !ok {
		t.Error("Debug field not found in JSON config")
	} else if debugVal != true {
		t.Errorf("Expected debug to be true in JSON, got %v", debugVal)
	}

	// Verify Verbose is in JSON and set to true
	verboseVal, ok := jsonConfig["verbose"]
	if !ok {
		t.Error("Verbose field not found in JSON config")
	} else if verboseVal != true {
		t.Errorf("Expected verbose to be true in JSON, got %v", verboseVal)
	}
}

// TestConfig_DebugVerbose_Load tests that Debug and Verbose are loaded from JSON
func TestConfig_DebugVerbose_Load(t *testing.T) {
	configDir := t.TempDir()

	// Create a config file with Debug and Verbose set
	configFile := filepath.Join(configDir, "config.json")
	testConfig := map[string]interface{}{
		"debug":              true,
		"verbose":            true,
		"server_port":        12580,
		"jwt_secret":         "test-secret",
		"user_token":         "test-user-token",
		"model_token":        "test-model-token",
		"providers_v2":       []interface{}{},
		"providers_v1":       map[string]interface{}{},
		"rules":              []interface{}{},
		"default_request_id": 0,
		"encrypt_providers":  false,
		"default_max_tokens": 4096,
	}

	data, err := json.MarshalIndent(testConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load the config
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify Debug and Verbose are loaded correctly
	if !cfg.GetDebug() {
		t.Error("Expected Debug to be loaded as true from JSON")
	}
	if !cfg.GetVerbose() {
		t.Error("Expected Verbose to be loaded as true from JSON")
	}
}

// TestConfig_DebugVerbose_FalseValuesInJSON tests that false values are persisted correctly
func TestConfig_DebugVerbose_FalseValuesInJSON(t *testing.T) {
	configDir := t.TempDir()

	// Create config with Debug and Verbose explicitly set to false
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	err = cfg.SetDebug(false)
	if err != nil {
		t.Fatalf("Failed to set Debug: %v", err)
	}

	err = cfg.SetVerbose(false)
	if err != nil {
		t.Fatalf("Failed to set Verbose: %v", err)
	}

	// Read the config file to verify JSON serialization
	configFile := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var jsonConfig map[string]interface{}
	if err := json.Unmarshal(data, &jsonConfig); err != nil {
		t.Fatalf("Failed to unmarshal config JSON: %v", err)
	}

	// Verify Debug is false in JSON
	debugVal, ok := jsonConfig["debug"]
	if !ok {
		t.Error("Debug field not found in JSON config")
	} else if debugVal != false {
		t.Errorf("Expected debug to be false in JSON, got %v", debugVal)
	}

	// Verify Verbose is false in JSON
	verboseVal, ok := jsonConfig["verbose"]
	if !ok {
		t.Error("Verbose field not found in JSON config")
	} else if verboseVal != false {
		t.Errorf("Expected verbose to be false in JSON, got %v", verboseVal)
	}
}

// TestAppConfig_DebugVerbose_Delegation tests that AppConfig properly delegates to Config
func TestAppConfig_DebugVerbose_Delegation(t *testing.T) {
	configDir := t.TempDir()
	appCfg, err := NewAppConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create AppConfig: %v", err)
	}

	// Test default values
	if appCfg.GetDebug() {
		t.Error("Expected AppConfig Debug to default to false")
	}
	if appCfg.GetVerbose() {
		t.Error("Expected AppConfig Verbose to default to false")
	}

	// Test setting via AppConfig
	err = appCfg.SetDebug(true)
	if err != nil {
		t.Fatalf("Failed to set Debug via AppConfig: %v", err)
	}

	err = appCfg.SetVerbose(true)
	if err != nil {
		t.Fatalf("Failed to set Verbose via AppConfig: %v", err)
	}

	// Verify values are set
	if !appCfg.GetDebug() {
		t.Error("Expected AppConfig Debug to be true")
	}
	if !appCfg.GetVerbose() {
		t.Error("Expected AppConfig Verbose to be true")
	}

	// Verify the underlying Config has the same values
	globalCfg := appCfg.GetGlobalConfig()
	if !globalCfg.GetDebug() {
		t.Error("Expected underlying Config Debug to be true")
	}
	if !globalCfg.GetVerbose() {
		t.Error("Expected underlying Config Verbose to be true")
	}
}

// TestConfig_OpenBrowser_DefaultValue tests that OpenBrowser defaults to false (zero value)
func TestConfig_OpenBrowser_DefaultValue(t *testing.T) {
	configDir := t.TempDir()
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// OpenBrowser defaults to true for CLI mode (runtime-only setting)
	if !cfg.GetOpenBrowser() {
		t.Error("Expected OpenBrowser to default to true, got false")
	}
}

// TestConfig_SetOpenBrowser tests setting and getting OpenBrowser field
func TestConfig_SetOpenBrowser(t *testing.T) {
	configDir := t.TempDir()
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Test setting OpenBrowser to false (disable browser)
	err = cfg.SetOpenBrowser(false)
	if err != nil {
		t.Fatalf("Failed to set OpenBrowser: %v", err)
	}

	if cfg.GetOpenBrowser() {
		t.Error("Expected OpenBrowser to be false after SetOpenBrowser(false)")
	}

	// Test setting OpenBrowser back to true
	err = cfg.SetOpenBrowser(true)
	if err != nil {
		t.Fatalf("Failed to set OpenBrowser: %v", err)
	}

	if !cfg.GetOpenBrowser() {
		t.Error("Expected OpenBrowser to be true after SetOpenBrowser(true)")
	}
}

// TestConfig_OpenBrowser_Persistence tests that OpenBrowser is NOT persisted to JSON
func TestConfig_OpenBrowser_Persistence(t *testing.T) {
	configDir := t.TempDir()

	// Create initial config and set OpenBrowser to false
	cfg, err := NewConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	err = cfg.SetOpenBrowser(false)
	if err != nil {
		t.Fatalf("Failed to set OpenBrowser: %v", err)
	}

	// Read the config file to verify JSON serialization
	configFile := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var jsonConfig map[string]interface{}
	if err := json.Unmarshal(data, &jsonConfig); err != nil {
		t.Fatalf("Failed to unmarshal config JSON: %v", err)
	}

	// Verify OpenBrowser is NOT in JSON (it has json:"-" tag)
	if _, ok := jsonConfig["open_browser"]; ok {
		t.Error("OpenBrowser field should NOT be in JSON config (it has json:\"-\" tag)")
	}

	// Also check for other possible key names
	if _, ok := jsonConfig["OpenBrowser"]; ok {
		t.Error("OpenBrowser field should NOT be in JSON config (it has json:\"-\" tag)")
	}
}

// TestAppConfig_OpenBrowser_Delegation tests that AppConfig properly delegates OpenBrowser
func TestAppConfig_OpenBrowser_Delegation(t *testing.T) {
	configDir := t.TempDir()
	appCfg, err := NewAppConfigWithDir(configDir)
	if err != nil {
		t.Fatalf("Failed to create AppConfig: %v", err)
	}

	// OpenBrowser defaults to true (runtime-only setting)
	if !appCfg.GetOpenBrowser() {
		t.Error("Expected AppConfig OpenBrowser to default to true")
	}

	// Test setting via AppConfig
	err = appCfg.SetOpenBrowser(true)
	if err != nil {
		t.Fatalf("Failed to set OpenBrowser via AppConfig: %v", err)
	}

	// Verify value is set
	if !appCfg.GetOpenBrowser() {
		t.Error("Expected AppConfig OpenBrowser to be true")
	}

	// Verify the underlying Config has the same value
	globalCfg := appCfg.GetGlobalConfig()
	if !globalCfg.GetOpenBrowser() {
		t.Error("Expected underlying Config OpenBrowser to be true")
	}
}
