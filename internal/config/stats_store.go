package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const defaultServiceTimeWindow = 300

// ServiceStatsRecord is the GORM model for persisting service statistics
type ServiceStatsRecord struct {
	// Composite primary key: provider + model (stats are global, not per-rule)
	Provider             string    `gorm:"primaryKey;column:provider"`
	Model                string    `gorm:"primaryKey;column:model"`
	ServiceID            string    `gorm:"column:service_id"`
	RequestCount         int64     `gorm:"column:request_count"`
	LastUsed             time.Time `gorm:"column:last_used"`
	WindowStart          time.Time `gorm:"column:window_start"`
	WindowRequestCount   int64     `gorm:"column:window_request_count"`
	WindowTokensConsumed int64     `gorm:"column:window_tokens_consumed"`
	WindowInputTokens    int64     `gorm:"column:window_input_tokens"`
	WindowOutputTokens   int64     `gorm:"column:window_output_tokens"`
	TimeWindow           int       `gorm:"column:time_window"`
}

// TableName specifies the table name for GORM
func (ServiceStatsRecord) TableName() string {
	return "service_stats"
}

// StatsStore persists service usage statistics in SQLite using GORM.
type StatsStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewStatsStore creates or loads a stats store using SQLite database.
func NewStatsStore(baseDir string) (*StatsStore, error) {
	log.Printf("Initializing stats store in directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create stats store directory: %w", err)
	}

	dbPath := filepath.Join(baseDir, StatsDBFileName)
	log.Printf("Opening SQLite database: %s", dbPath)
	// Configure SQLite with busy timeout and other settings to prevent hangs
	// Use pure Go driver by ensuring modernc.org/sqlite is used
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Disable verbose logging for now
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open stats database: %w", err)
	}
	log.Printf("SQLite database opened successfully")

	store := &StatsStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema, if we add column it would create or update the database table to match the struct definition
	if err := db.AutoMigrate(&ServiceStatsRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate stats database: %w", err)
	}
	log.Printf("Stats store initialization completed")

	return store, nil
}

// ServiceKey builds a unique key for a provider/model combination.
func (ss *StatsStore) ServiceKey(provider, model string) string {
	return fmt.Sprintf("%s:%s", provider, model)
}

// Snapshot returns a copy of all stats keyed by provider:model.
func (ss *StatsStore) Snapshot() map[string]ServiceStats {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	var records []ServiceStatsRecord
	if err := ss.db.Find(&records).Error; err != nil {
		return make(map[string]ServiceStats)
	}

	snapshot := make(map[string]ServiceStats, len(records))
	for _, record := range records {
		key := ss.ServiceKey(record.Provider, record.Model)
		snapshot[key] = record.toServiceStats()
	}

	return snapshot
}

// Get returns stats for a specific provider/model combination.
func (ss *StatsStore) Get(provider, model string) (ServiceStats, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	var record ServiceStatsRecord
	err := ss.db.Where("provider = ? AND model = ?", provider, model).
		First(&record).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ServiceStats{}, false
	}
	if err != nil {
		return ServiceStats{}, false
	}

	return record.toServiceStats(), true
}

// UpdateFromService stores the current stats from a service into the store.
func (ss *StatsStore) UpdateFromService(service *Service) error {
	if service == nil {
		return nil
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	service.InitializeStats()
	stat := service.Stats.GetStats()

	record := ServiceStatsRecord{
		Provider:             service.Provider,
		Model:                service.Model,
		ServiceID:            stat.ServiceID,
		RequestCount:         stat.RequestCount,
		LastUsed:             stat.LastUsed,
		WindowStart:          stat.WindowStart,
		WindowRequestCount:   stat.WindowRequestCount,
		WindowTokensConsumed: stat.WindowTokensConsumed,
		WindowInputTokens:    stat.WindowInputTokens,
		WindowOutputTokens:   stat.WindowOutputTokens,
		TimeWindow:           stat.TimeWindow,
	}

	// Normalize time window if needed
	if record.TimeWindow == 0 {
		if service.TimeWindow > 0 {
			record.TimeWindow = service.TimeWindow
		} else {
			record.TimeWindow = defaultServiceTimeWindow
		}
	}
	if record.ServiceID == "" {
		record.ServiceID = service.ServiceID()
	}
	if record.WindowStart.IsZero() {
		record.WindowStart = time.Now()
	}

	return ss.db.Save(&record).Error
}

// RecordUsage records usage for a service and persists the updated stats.
func (ss *StatsStore) RecordUsage(service *Service, inputTokens, outputTokens int) (ServiceStats, error) {
	if service == nil {
		return ServiceStats{}, nil
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Get or create record
	var record ServiceStatsRecord
	err := ss.db.Where("provider = ? AND model = ?", service.Provider, service.Model).
		First(&record).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new record
		record = ServiceStatsRecord{
			Provider:  service.Provider,
			Model:     service.Model,
			ServiceID: service.ServiceID(),
			TimeWindow: func() int {
				if service.TimeWindow > 0 {
					return service.TimeWindow
				}
				return defaultServiceTimeWindow
			}(),
			WindowStart: time.Now(),
		}
	} else if err != nil {
		return ServiceStats{}, err
	}

	// Update stats
	now := time.Now()
	if now.Sub(record.WindowStart) >= time.Duration(record.TimeWindow)*time.Second {
		record.WindowStart = now
		record.WindowRequestCount = 0
		record.WindowTokensConsumed = 0
		record.WindowInputTokens = 0
		record.WindowOutputTokens = 0
	}

	record.RequestCount++
	record.WindowRequestCount++
	record.WindowInputTokens += int64(inputTokens)
	record.WindowOutputTokens += int64(outputTokens)
	record.WindowTokensConsumed += int64(inputTokens + outputTokens)
	record.LastUsed = now

	if err := ss.db.Save(&record).Error; err != nil {
		return ServiceStats{}, err
	}

	return record.toServiceStats(), nil
}

// HydrateRules injects stored stats into the provided rules and initializes missing entries.
func (ss *StatsStore) HydrateRules(rules []Rule) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	var records []ServiceStatsRecord
	if err := ss.db.Find(&records).Error; err != nil {
		return err
	}

	// Build lookup map by provider:model
	statsMap := make(map[string]*ServiceStatsRecord)
	for i := range records {
		record := &records[i]
		key := ss.ServiceKey(record.Provider, record.Model)
		statsMap[key] = record
	}

	for i := range rules {
		rule := &rules[i]
		for j := range rule.Services {
			service := &rule.Services[j]
			key := ss.ServiceKey(service.Provider, service.Model)

			if record, ok := statsMap[key]; ok {
				service.Stats = record.toServiceStats()
			} else {
				service.InitializeStats()
				statCopy := service.Stats.GetStats()
				record := &ServiceStatsRecord{
					Provider:             service.Provider,
					Model:                service.Model,
					ServiceID:            statCopy.ServiceID,
					RequestCount:         statCopy.RequestCount,
					LastUsed:             statCopy.LastUsed,
					WindowStart:          statCopy.WindowStart,
					WindowRequestCount:   statCopy.WindowRequestCount,
					WindowTokensConsumed: statCopy.WindowTokensConsumed,
					WindowInputTokens:    statCopy.WindowInputTokens,
					WindowOutputTokens:   statCopy.WindowOutputTokens,
					TimeWindow:           statCopy.TimeWindow,
				}
				if record.TimeWindow == 0 {
					if service.TimeWindow > 0 {
						record.TimeWindow = service.TimeWindow
					} else {
						record.TimeWindow = defaultServiceTimeWindow
					}
				}
				if record.ServiceID == "" {
					record.ServiceID = service.ServiceID()
				}
				if record.WindowStart.IsZero() {
					record.WindowStart = time.Now()
				}
				if err := ss.db.Create(record).Error; err != nil {
					return err
				}
				// Add to statsMap so other services with same provider:model find it
				statsMap[key] = record
			}
		}
	}

	return nil
}

// ClearAll removes all persisted stats.
func (ss *StatsStore) ClearAll() error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	return ss.db.Exec("DELETE FROM service_stats").Error
}

// toServiceStats converts a ServiceStatsRecord to ServiceStats.
func (r *ServiceStatsRecord) toServiceStats() ServiceStats {
	return ServiceStats{
		ServiceID:            r.ServiceID,
		RequestCount:         r.RequestCount,
		LastUsed:             r.LastUsed,
		WindowStart:          r.WindowStart,
		WindowRequestCount:   r.WindowRequestCount,
		WindowTokensConsumed: r.WindowTokensConsumed,
		WindowInputTokens:    r.WindowInputTokens,
		WindowOutputTokens:   r.WindowOutputTokens,
		TimeWindow:           r.TimeWindow,
	}
}
