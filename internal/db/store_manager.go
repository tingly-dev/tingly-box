package db

import (
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// StoreManager manages all database stores with a shared GORM DB instance.
// It provides unified initialization, thread-safe access, and lifecycle management.
type StoreManager struct {
	mu      sync.RWMutex
	baseDir string
	db      *gorm.DB // Shared DB instance for all stores

	storeSet
}

// storeSet groups every store StoreManager owns. Keeping them in one struct
// lets Close reset them all with a single zero-value assignment, so the set
// is spelled out only here and in initialized() — it used to be
// hand-enumerated in four places (fields, initStores, Close, HealthCheck)
// and the copies had already drifted.
type storeSet struct {
	statsStore         *StatsStore
	usageStore         *UsageStore
	providerStore      *ProviderStore
	ruleStore          *RuleStore
	imbotSettingsStore *ImBotSettingsStore
	modelStore         *ModelStore
	teamStore          *TeamStore
	apiTokenStore      *APITokenStore
	remoteChatStore    *RemoteChatStore
	remoteSessionStore *RemoteSessionStore
	botAccessStore     *BotAccessStore
}

// initialized reports, per health-report name, whether each store is set.
// Returning bools rather than the stores themselves avoids the
// typed-nil-in-interface trap a map[string]any would reintroduce.
func (s *storeSet) initialized() map[string]bool {
	return map[string]bool{
		"stats":          s.statsStore != nil,
		"usage":          s.usageStore != nil,
		"provider":       s.providerStore != nil,
		"rule":           s.ruleStore != nil,
		"imbotSettings":  s.imbotSettingsStore != nil,
		"model":          s.modelStore != nil,
		"team":           s.teamStore != nil,
		"apiToken":       s.apiTokenStore != nil,
		"remoteChats":    s.remoteChatStore != nil,
		"remoteSessions": s.remoteSessionStore != nil,
		"botAccess":      s.botAccessStore != nil,
	}
}

// StoreManagerConfig holds configuration for StoreManager initialization.
type StoreManagerConfig struct {
	BaseDir     string
	BusyTimeout int // Milliseconds, default 5000
}

// HealthStatus represents the health of all stores.
type HealthStatus struct {
	Healthy         bool              `json:"healthy"`
	TotalStores     int               `json:"total_stores"`
	HealthyStores   int               `json:"healthy_stores"`
	UnhealthyStores int               `json:"unhealthy_stores"`
	StoreStatus     map[string]string `json:"store_status"`
}

// Health status constants
const (
	HealthStatusOK      = "ok"
	HealthStatusError   = "error"
	HealthStatusNotInit = "not_initialized"
)

// NewStoreManager creates a new StoreManager and initializes all stores.
// It opens a single SQLite database connection shared by all stores.
//
// Parameters:
//
//	baseDir - Base directory for database storage
//
// Returns:
//
//	*StoreManager - Initialized store manager
//	error - Error if any store fails to initialize
func NewStoreManager(baseDir string) (*StoreManager, error) {
	return NewStoreManagerWithConfig(StoreManagerConfig{BaseDir: baseDir})
}

// NewStoreManagerWithConfig creates a StoreManager with custom configuration.
// A zero BusyTimeout takes OpenSQLite's default.
func NewStoreManagerWithConfig(config StoreManagerConfig) (*StoreManager, error) {
	if config.BaseDir == "" {
		return nil, errors.New("base directory cannot be empty")
	}

	// Open shared database connection. OpenSQLite creates baseDir/db, and
	// baseDir with it.
	dbPath := constant.GetDBFile(config.BaseDir)
	db, err := OpenSQLite(dbPath, config.BusyTimeout)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("StoreManager: Opened database at %s", dbPath)

	// Create store manager
	sm := &StoreManager{
		baseDir: config.BaseDir,
		db:      db,
	}

	// Initialize all stores
	if err := sm.initStores(); err != nil {
		// Close DB on initialization failure
		sqlDB, _ := db.DB()
		sqlDB.Close()
		return nil, err
	}

	logrus.Debugf("StoreManager: All stores initialized successfully")
	return sm, nil
}

// initStores initializes all individual stores over the shared connection.
// Each newXStore runs that store's schema migration; errors are collected so
// one failing store doesn't hide the rest.
func (sm *StoreManager) initStores() error {
	conn := borrowedConn(sm.db)
	var errs []error
	var err error

	if sm.statsStore, err = newStatsStore(conn); err != nil {
		errs = append(errs, fmt.Errorf("stats store: %w", err))
	}
	if sm.usageStore, err = newUsageStore(conn); err != nil {
		errs = append(errs, fmt.Errorf("usage store: %w", err))
	}
	if sm.providerStore, err = newProviderStore(conn); err != nil {
		errs = append(errs, fmt.Errorf("provider store: %w", err))
	}
	if sm.ruleStore, err = newRuleStore(conn); err != nil {
		errs = append(errs, fmt.Errorf("rule store: %w", err))
	}
	if sm.imbotSettingsStore, err = newImBotSettingsStore(conn); err != nil {
		errs = append(errs, fmt.Errorf("imbot settings store: %w", err))
	}
	if err = sm.dropDeprecatedModelCapabilities(); err != nil {
		errs = append(errs, fmt.Errorf("drop deprecated model_capabilities: %w", err))
	}
	if sm.modelStore, err = newModelStore(conn); err != nil {
		errs = append(errs, fmt.Errorf("model store: %w", err))
	}
	if sm.teamStore, err = newTeamStore(conn); err != nil {
		errs = append(errs, fmt.Errorf("team store: %w", err))
	}
	if sm.apiTokenStore, err = newAPITokenStore(conn, sm.teamStore); err != nil {
		errs = append(errs, fmt.Errorf("api token store: %w", err))
	}
	if err = sm.initRemoteStores(); err != nil {
		errs = append(errs, fmt.Errorf("remote stores: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to initialize stores: %v", errs)
	}

	return nil
}

// dropDeprecatedModelCapabilities removes the model_capabilities table that
// belonged to the now-removed AdaptiveProbe subsystem. Idempotent: harmless
// when the table is already absent (new installs or post-migration restarts).
func (sm *StoreManager) dropDeprecatedModelCapabilities() error {
	return sm.db.Exec("DROP TABLE IF EXISTS model_capabilities").Error
}

// initRemoteStores initializes the remote-control chat and session stores.
// These replace the JSON files the remote subsystem used to keep beside the
// database; see .design/remote-storage.md.
func (sm *StoreManager) initRemoteStores() error {
	if err := migrateBotAccessTables(sm.db); err != nil {
		return fmt.Errorf("migrate bot access tables: %w", err)
	}
	if err := sm.db.AutoMigrate(
		&RemoteChatRecord{},
		&RemoteSessionRecord{},
	); err != nil {
		return err
	}
	// Session transcripts are files, not rows — see session.Transcript.
	transcript, err := session.NewTranscript(constant.GetRemoteTranscriptDir(sm.baseDir))
	if err != nil {
		return fmt.Errorf("open transcript store: %w", err)
	}
	sm.remoteChatStore = NewRemoteChatStore(sm.db)
	sm.remoteSessionStore = NewRemoteSessionStore(sm.db, transcript)
	sm.botAccessStore = NewBotAccessStore(sm.db)

	// Migrating here, rather than from whichever feature happens to construct
	// a store first, is what makes every entry point — server, standalone CLI
	// bot, `remote pair revoke` — see the same migrated data.
	if err := importLegacyRemoteJSON(sm.baseDir, sm.remoteChatStore, sm.remoteSessionStore); err != nil {
		// Best effort: the legacy files are left in place for the next start.
		logrus.WithError(err).Error("Failed to import legacy remote JSON stores; leaving files in place")
	}
	return nil
}

// BotAccess returns the final-state Bot Capability and access-policy store.
func (sm *StoreManager) BotAccess() *BotAccessStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.botAccessStore
}

// RemoteChats returns the RemoteChatStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) RemoteChats() *RemoteChatStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.remoteChatStore
}

// RemoteSessions returns the RemoteSessionStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) RemoteSessions() *RemoteSessionStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.remoteSessionStore
}

// Stats returns the StatsStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) Stats() *StatsStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.statsStore
}

// Usage returns the UsageStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) Usage() *UsageStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.usageStore
}

// Provider returns the ProviderStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) Provider() *ProviderStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.providerStore
}

// Rules returns the RuleStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) Rules() *RuleStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.ruleStore
}

// ImBotSettings returns the ImBotSettingsStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) ImBotSettings() *ImBotSettingsStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.imbotSettingsStore
}

// Model returns the ModelStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) Model() *ModelStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.modelStore
}

// Team returns the TeamStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) Team() *TeamStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.teamStore
}

// APIToken returns the APITokenStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) APIToken() *APITokenStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.apiTokenStore
}

// BaseDir returns the base directory for this StoreManager.
func (sm *StoreManager) BaseDir() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.baseDir
}

// DB returns the shared *gorm.DB every store runs on. Subsystems that keep
// their own record types on the shared tingly.db borrow this handle instead
// of opening a second connection to the same file — the process should hold
// exactly one connection pool per database. Returns nil after Close().
func (sm *StoreManager) DB() *gorm.DB {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.db
}

// Close closes all database connections and cleans up resources.
// After Close() is called, all accessor methods will return nil.
func (sm *StoreManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.db == nil {
		return nil // Already closed
	}

	// Close the shared database connection
	sqlDB, err := sm.db.DB()
	if err != nil {
		logrus.Warnf("StoreManager: Failed to get database instance for closing: %v", err)
	} else {
		if err := sqlDB.Close(); err != nil {
			logrus.Warnf("StoreManager: Error closing database: %v", err)
		}
	}

	// Clear all store references
	sm.storeSet = storeSet{}
	sm.db = nil

	logrus.Info("StoreManager: Closed all stores")
	return nil
}

// HealthCheck checks the health of all stores.
// Returns a HealthStatus with the state of each store.
func (sm *StoreManager) HealthCheck() (*HealthStatus, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stores := sm.initialized()

	status := &HealthStatus{
		TotalStores: len(stores),
		StoreStatus: make(map[string]string),
	}

	// Every store runs on the one shared connection, so ping it once rather
	// than once per store (this used to issue ten identical pings).
	dbOK := false
	if sm.db != nil {
		if sqlDB, err := sm.db.DB(); err == nil && sqlDB.Ping() == nil {
			dbOK = true
		}
	}

	for name, inited := range stores {
		switch {
		case !inited || sm.db == nil:
			status.StoreStatus[name] = HealthStatusNotInit
			status.UnhealthyStores++
		case !dbOK:
			status.StoreStatus[name] = HealthStatusError
			status.UnhealthyStores++
		default:
			status.StoreStatus[name] = HealthStatusOK
			status.HealthyStores++
		}
	}

	status.Healthy = status.UnhealthyStores == 0
	return status, nil
}
