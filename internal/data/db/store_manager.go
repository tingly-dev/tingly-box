package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// StoreManager manages all database stores with a shared GORM DB instance.
// It provides unified initialization, thread-safe access, and lifecycle management.
type StoreManager struct {
	mu      sync.RWMutex
	baseDir string
	db      *gorm.DB // Shared DB instance for all stores

	// Individual stores
	statsStore         *StatsStore
	usageStore         *UsageStore
	providerStore      *ProviderStore
	toolConfigStore    *ToolConfigStore
	imbotSettingsStore *ImBotSettingsStore
	modelStore         *ModelStore
	apiTokenStore      *APITokenStore
	taskStore          *TaskStore
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
	config := StoreManagerConfig{
		BaseDir:     baseDir,
		BusyTimeout: 5000,
	}
	return NewStoreManagerWithConfig(config)
}

// NewStoreManagerWithConfig creates a StoreManager with custom configuration.
func NewStoreManagerWithConfig(config StoreManagerConfig) (*StoreManager, error) {
	if config.BaseDir == "" {
		return nil, errors.New("base directory cannot be empty")
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(config.BaseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Set default busy timeout
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5000
	}

	// Get database path
	dbPath := constant.GetDBFile(config.BaseDir)
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// Open shared database connection
	dsn := fmt.Sprintf("%s?_busy_timeout=%d&_journal_mode=WAL&_foreign_keys=1",
		dbPath, config.BusyTimeout)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
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

// initStores initializes all individual stores.
func (sm *StoreManager) initStores() error {
	var errs []error

	// Initialize each store with its schema migration
	if err := sm.initStatsStore(); err != nil {
		errs = append(errs, fmt.Errorf("stats store: %w", err))
	}
	if err := sm.initUsageStore(); err != nil {
		errs = append(errs, fmt.Errorf("usage store: %w", err))
	}
	if err := sm.initProviderStore(); err != nil {
		errs = append(errs, fmt.Errorf("provider store: %w", err))
	}
	if err := sm.initToolConfigStore(); err != nil {
		errs = append(errs, fmt.Errorf("tool config store: %w", err))
	}
	if err := sm.initImBotSettingsStore(); err != nil {
		errs = append(errs, fmt.Errorf("imbot settings store: %w", err))
	}
	if err := sm.dropDeprecatedModelCapabilities(); err != nil {
		errs = append(errs, fmt.Errorf("drop deprecated model_capabilities: %w", err))
	}
	if err := sm.initModelStore(); err != nil {
		errs = append(errs, fmt.Errorf("model store: %w", err))
	}
	if err := sm.initAPITokenStore(); err != nil {
		errs = append(errs, fmt.Errorf("api token store: %w", err))
	}
	if err := sm.initTaskStore(); err != nil {
		errs = append(errs, fmt.Errorf("task store: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to initialize stores: %v", errs)
	}

	return nil
}

// initStatsStore initializes the StatsStore.
func (sm *StoreManager) initStatsStore() error {
	if err := sm.db.AutoMigrate(&ServiceStatsRecord{}); err != nil {
		return err
	}
	sm.statsStore = &StatsStore{
		db:     sm.db,
		dbPath: constant.GetDBFile(sm.baseDir),
	}
	return nil
}

// initUsageStore initializes the UsageStore.
func (sm *StoreManager) initUsageStore() error {
	if err := migrateUsageTables(sm.db); err != nil {
		return err
	}
	sm.usageStore = &UsageStore{
		db:     sm.db,
		dbPath: constant.GetDBFile(sm.baseDir),
	}
	return nil
}

// initProviderStore initializes the ProviderStore.
func (sm *StoreManager) initProviderStore() error {
	if err := sm.db.AutoMigrate(&ProviderRecord{}); err != nil {
		return err
	}
	sm.providerStore = &ProviderStore{
		db:     sm.db,
		dbPath: constant.GetDBFile(sm.baseDir),
	}
	return nil
}

// initToolConfigStore initializes the ToolConfigStore.
func (sm *StoreManager) initToolConfigStore() error {
	if err := sm.db.AutoMigrate(&ToolConfigRecord{}); err != nil {
		return err
	}
	sm.toolConfigStore = &ToolConfigStore{
		db:     sm.db,
		dbPath: constant.GetDBFile(sm.baseDir),
	}
	return nil
}

// initImBotSettingsStore initializes the ImBotSettingsStore.
func (sm *StoreManager) initImBotSettingsStore() error {
	if err := sm.db.AutoMigrate(&ImBotSettingsRecord{}); err != nil {
		return err
	}
	sm.imbotSettingsStore = &ImBotSettingsStore{
		db:     sm.db,
		dbPath: constant.GetDBFile(sm.baseDir),
	}
	return nil
}

// dropDeprecatedModelCapabilities removes the model_capabilities table that
// belonged to the now-removed AdaptiveProbe subsystem. Idempotent: harmless
// when the table is already absent (new installs or post-migration restarts).
func (sm *StoreManager) dropDeprecatedModelCapabilities() error {
	return sm.db.Exec("DROP TABLE IF EXISTS model_capabilities").Error
}

// initModelStore initializes the ModelStore.
func (sm *StoreManager) initModelStore() error {
	if err := sm.db.AutoMigrate(&ProviderModelRecord{}); err != nil {
		return err
	}
	sm.modelStore = &ModelStore{
		db:     sm.db,
		dbPath: constant.GetDBFile(sm.baseDir),
	}
	return nil
}

// initAPITokenStore initializes the APITokenStore.
func (sm *StoreManager) initAPITokenStore() error {
	if err := sm.db.AutoMigrate(&APITokenRecord{}); err != nil {
		return err
	}
	sm.apiTokenStore = &APITokenStore{
		db:     sm.db,
		dbPath: constant.GetDBFile(sm.baseDir),
	}
	return nil
}

// initTaskStore initializes the TaskStore.
func (sm *StoreManager) initTaskStore() error {
	if err := sm.db.AutoMigrate(&TaskRecord{}); err != nil {
		return err
	}
	// Belt-and-suspenders: ensure composite indices exist regardless of GORM tag behaviour.
	sm.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_status_scheduled ON tasks(status, scheduled_at)`)
	sm.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_key_status ON tasks(serialization_key, status, created_at)`)
	sm.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_owner ON tasks(owner_type, owner_id, created_at)`)
	sm.taskStore = &TaskStore{
		db:     sm.db,
		dbPath: constant.GetDBFile(sm.baseDir),
	}
	return nil
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

// ToolConfig returns the ToolConfigStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) ToolConfig() *ToolConfigStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.toolConfigStore
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

// APIToken returns the APITokenStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) APIToken() *APITokenStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.apiTokenStore
}

// Tasks returns the TaskStore (thread-safe).
// Returns nil if the store is not initialized or after Close() has been called.
func (sm *StoreManager) Tasks() *TaskStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.taskStore
}

// BaseDir returns the base directory for this StoreManager.
func (sm *StoreManager) BaseDir() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.baseDir
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
	sm.statsStore = nil
	sm.usageStore = nil
	sm.providerStore = nil
	sm.toolConfigStore = nil
	sm.imbotSettingsStore = nil
	sm.modelStore = nil
	sm.apiTokenStore = nil
	sm.taskStore = nil
	sm.db = nil

	logrus.Info("StoreManager: Closed all stores")
	return nil
}

// HealthCheck checks the health of all stores.
// Returns a HealthStatus with the state of each store.
func (sm *StoreManager) HealthCheck() (*HealthStatus, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	status := &HealthStatus{
		TotalStores: 8,
		StoreStatus: make(map[string]string),
	}

	// Check each store
	stores := map[string]interface{}{
		"stats":         sm.statsStore,
		"usage":         sm.usageStore,
		"provider":      sm.providerStore,
		"toolConfig":    sm.toolConfigStore,
		"imbotSettings": sm.imbotSettingsStore,
		"model":         sm.modelStore,
		"apiToken":      sm.apiTokenStore,
		"tasks":         sm.taskStore,
	}

	for name, store := range stores {
		if store == nil {
			status.StoreStatus[name] = HealthStatusNotInit
			status.UnhealthyStores++
		} else {
			// Try to ping the database
			if sm.db != nil {
				sqlDB, err := sm.db.DB()
				if err != nil {
					status.StoreStatus[name] = HealthStatusError
					status.UnhealthyStores++
				} else if err := sqlDB.Ping(); err != nil {
					status.StoreStatus[name] = HealthStatusError
					status.UnhealthyStores++
				} else {
					status.StoreStatus[name] = HealthStatusOK
					status.HealthyStores++
				}
			} else {
				status.StoreStatus[name] = HealthStatusNotInit
				status.UnhealthyStores++
			}
		}
	}

	status.Healthy = status.UnhealthyStores == 0
	return status, nil
}
