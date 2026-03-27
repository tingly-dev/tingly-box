package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// Watcher monitors configuration changes and triggers reloads
type Watcher struct {
	config       *Config
	watcher      *fsnotify.Watcher
	callbacks    []func(*Config)
	stopCh       chan struct{}
	mu           sync.RWMutex
	running      bool
	paused       bool                 // Paused flag to prevent reload loops
	watchFiles   []string             // List of files being watched
	fileModTimes map[string]time.Time // ModTime for each watched file
}

// NewConfigWatcher creates a new configuration watcher
func NewConfigWatcher(config *Config) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cw := &Watcher{
		config:       config,
		watcher:      watcher,
		stopCh:       make(chan struct{}),
		watchFiles:   []string{}, // Empty by default, must be configured explicitly
		fileModTimes: make(map[string]time.Time),
	}

	return cw, nil
}

// AddCallback adds a callback function to be called when configuration changes
func (cw *Watcher) AddCallback(callback func(*Config)) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	cw.callbacks = append(cw.callbacks, callback)
}

// Start starts watching for configuration changes
func (cw *Watcher) Start() error {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	if cw.running {
		return fmt.Errorf("watcher is already running")
	}

	// Add all watch files to watcher and record initial ModTime
	for _, file := range cw.watchFiles {
		if err := cw.watcher.Add(file); err != nil {
			logrus.Warnf("Failed to watch file %s: %v", file, err)
			continue
		}

		// Record initial modification time
		if stat, err := os.Stat(file); err == nil {
			cw.fileModTimes[file] = stat.ModTime()
		}
	}

	cw.running = true

	// Start watching in goroutine
	go cw.watchLoop()

	return nil
}

// Stop stops the configuration watcher
func (cw *Watcher) Stop() error {
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
func (cw *Watcher) watchLoop() {
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

			// Debounce rapid file changes (1s to handle vim/editor multi-write)
			debounceTimer.Stop()
			debounceTimer = time.AfterFunc(1*time.Second, func() {
				cw.handleConfigChange(event)
			})

		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return
			}
			logrus.Debugf("Config watcher error: %v", err)

		case <-cw.stopCh:
			return
		}
	}
}

// isConfigEvent checks if an event is related to our config files
func (cw *Watcher) isConfigEvent(event fsnotify.Event) bool {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	// Ignore events if watcher is paused (during reload)
	if cw.paused {
		return false
	}

	// Check if event file is in our watch list
	for _, watchFile := range cw.watchFiles {
		if event.Name == watchFile {
			return event.Op&(fsnotify.Write|fsnotify.Create) != 0
		}
	}

	return false
}

// handleConfigChange processes configuration changes
func (cw *Watcher) handleConfigChange(event fsnotify.Event) {
	changedFile := event.Name

	// Pause watcher to prevent reload loop (migration may call Save())
	cw.mu.Lock()
	cw.paused = true
	cw.mu.Unlock()

	defer func() {
		// Update ModTime for all watched files after reload
		// This prevents migration-triggered Save() from causing another reload
		cw.mu.Lock()
		for _, file := range cw.watchFiles {
			if stat, err := os.Stat(file); err == nil {
				cw.fileModTimes[file] = stat.ModTime()
			}
		}
		cw.paused = false
		cw.mu.Unlock()

		logrus.Debugf("Configuration reloaded successfully (triggered by %s)", changedFile)
	}()

	// Check if file actually changed via ModTime
	cw.mu.Lock()
	lastMod, exists := cw.fileModTimes[changedFile]
	cw.mu.Unlock()

	if stat, err := os.Stat(changedFile); err == nil {
		if exists && !stat.ModTime().After(lastMod) {
			return // No actual change
		}
	} else {
		// File doesn't exist, skip reload
		return
	}

	// Reload configuration
	if err := cw.config.load(); err != nil {
		logrus.Debugf("Failed to reload configuration: %v", err)
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
}

// TriggerReload manually triggers a configuration reload
func (cw *Watcher) TriggerReload() error {
	return cw.config.load()
}

// AddWatchFile adds a file to the watch list
// If the watcher is already running, the file will be added immediately
func (cw *Watcher) AddWatchFile(filepath string) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	// Check if already watching
	for _, f := range cw.watchFiles {
		if f == filepath {
			return nil // Already watching
		}
	}

	// Add to watch list
	cw.watchFiles = append(cw.watchFiles, filepath)

	// If watcher is running, add to fsnotify immediately
	if cw.running {
		if err := cw.watcher.Add(filepath); err != nil {
			// Remove from watch list if add fails
			cw.watchFiles = cw.watchFiles[:len(cw.watchFiles)-1]
			return fmt.Errorf("failed to watch file %s: %w", filepath, err)
		}

		// Record initial modification time
		if stat, err := os.Stat(filepath); err == nil {
			cw.fileModTimes[filepath] = stat.ModTime()
		}

		logrus.Debugf("Added watch file: %s", filepath)
	}

	return nil
}

// RemoveWatchFile removes a file from the watch list
func (cw *Watcher) RemoveWatchFile(filepath string) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	// Find and remove from watch list
	found := false
	for i, f := range cw.watchFiles {
		if f == filepath {
			cw.watchFiles = append(cw.watchFiles[:i], cw.watchFiles[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("file %s is not being watched", filepath)
	}

	// If watcher is running, remove from fsnotify
	if cw.running {
		if err := cw.watcher.Remove(filepath); err != nil {
			logrus.Warnf("Failed to remove watch for %s: %v", filepath, err)
		}
		delete(cw.fileModTimes, filepath)
		logrus.Debugf("Removed watch file: %s", filepath)
	}

	return nil
}

// GetWatchFiles returns a copy of the current watch file list
func (cw *Watcher) GetWatchFiles() []string {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	files := make([]string, len(cw.watchFiles))
	copy(files, cw.watchFiles)
	return files
}
