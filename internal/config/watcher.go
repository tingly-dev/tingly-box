package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ConfigWatcher monitors configuration changes and triggers reloads
type ConfigWatcher struct {
	config      *Config
	watcher     *fsnotify.Watcher
	callbacks   []func(*Config)
	stopCh      chan struct{}
	mu          sync.RWMutex
	running     bool
	lastModTime time.Time
}

// NewConfigWatcher creates a new configuration watcher
func NewConfigWatcher(config *Config) (*ConfigWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cw := &ConfigWatcher{
		config:  config,
		watcher: watcher,
		stopCh:  make(chan struct{}),
	}

	return cw, nil
}

// AddCallback adds a callback function to be called when configuration changes
func (cw *ConfigWatcher) AddCallback(callback func(*Config)) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	cw.callbacks = append(cw.callbacks, callback)
}

// Start starts watching for configuration changes
func (cw *ConfigWatcher) Start() error {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	if cw.running {
		return fmt.Errorf("watcher is already running")
	}

	// Get config file path (Config uses ConfigFile)
	configFile := cw.config.ConfigFile

	// Get initial modification time
	if stat, err := os.Stat(configFile); err == nil {
		cw.lastModTime = stat.ModTime()
	}

	// Add Config file to watcher
	if err := cw.watcher.Add(configFile); err != nil {
		return fmt.Errorf("failed to watch global config file: %w", err)
	}

	cw.running = true

	// Start watching in goroutine
	go cw.watchLoop()

	return nil
}

// Stop stops the configuration watcher
func (cw *ConfigWatcher) Stop() error {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	if !cw.running {
		return nil
	}

	cw.running = false
	close(cw.stopCh)

	return cw.watcher.Close()
}

// watchLoop monitors file system events
func (cw *ConfigWatcher) watchLoop() {
	debounceTimer := time.NewTimer(0)
	<-debounceTimer.C // Stop the initial timer

	for {
		select {
		case event, ok := <-cw.watcher.Events:
			if !ok {
				return
			}

			// Filter for events related to our config file
			if !cw.isConfigEvent(event) {
				continue
			}

			// Debounce rapid file changes
			debounceTimer.Stop()
			debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
				cw.handleConfigChange(event)
			})

		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Config watcher error: %v", err)

		case <-cw.stopCh:
			return
		}
	}
}

// isConfigEvent checks if an event is related to our config files
func (cw *ConfigWatcher) isConfigEvent(event fsnotify.Event) bool {
	configFile := cw.config.ConfigFile
	configDir := filepath.Dir(configFile)
	providerConfigFile := filepath.Join(configDir, "config.json")

	// Direct config file events (Config)
	if event.Name == configFile {
		return event.Op&(fsnotify.Write|fsnotify.Create) != 0
	}

	// Provider config file events
	if event.Name == providerConfigFile {
		return event.Op&(fsnotify.Write|fsnotify.Create) != 0
	}

	// Check if it's a create/rename event in the config directory
	if filepath.Dir(event.Name) == configDir {
		return event.Op&(fsnotify.Create|fsnotify.Rename) != 0
	}

	return false
}

// handleConfigChange processes configuration changes
func (cw *ConfigWatcher) handleConfigChange(event fsnotify.Event) {
	configFile := cw.config.ConfigFile
	configDir := filepath.Dir(configFile)
	providerConfigFile := filepath.Join(configDir, "config.json")

	// Determine which file changed and check if it actually changed
	var checkFile string
	if event.Name == configFile || event.Name == providerConfigFile {
		checkFile = event.Name
	} else {
		// Directory event, check the main config file
		checkFile = configFile
	}

	if stat, err := os.Stat(checkFile); err == nil {
		if !stat.ModTime().After(cw.lastModTime) {
			return
		}
		cw.lastModTime = stat.ModTime()
	} else {
		// File doesn't exist, skip reload
		return
	}

	// Reload configuration (reload Config)
	if err := cw.config.load(); err != nil {
		log.Printf("Failed to reload configuration: %v", err)
		return
	}

	// Create a Config struct for callbacks (for backward compatibility)
	config := &Config{
		Providers:  cw.config.Providers,
		ServerPort: cw.config.ServerPort,
		JWTSecret:  cw.config.JWTSecret,
	}

	// Notify callbacks
	cw.mu.RLock()
	callbacks := make([]func(*Config), len(cw.callbacks))
	copy(callbacks, cw.callbacks)
	cw.mu.RUnlock()

	for _, callback := range callbacks {
		callback(config)
	}

	log.Println("Configuration reloaded successfully")
}

// TriggerReload manually triggers a configuration reload
func (cw *ConfigWatcher) TriggerReload() error {
	return cw.config.load()
}
