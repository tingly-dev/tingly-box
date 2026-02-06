package db

import (
	"errors"
	"fmt"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleServiceRecord stores the current service for each rule using provider+model as ID
// This is persisted to SQLite to avoid frequent config.json writes
type RuleServiceRecord struct {
	RuleUUID         string `gorm:"primaryKey;column:rule_uuid"`
	CurrentServiceID string `gorm:"column:current_service_id"` // provider:model format
}

// TableName specifies the table name for GORM
func (RuleServiceRecord) TableName() string {
	return "rule_service_index"
}

// RuleStateStore manages rule state persistence separately from config
type RuleStateStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewRuleStateStore creates or loads a rule state store using SQLite database.
func NewRuleStateStore(baseDir string) (*RuleStateStore, error) {
	dbPath := constant.GetDBFile(baseDir)
	// Configure SQLite with busy timeout and other settings to prevent hangs
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open rule state database: %w", err)
	}

	store := &RuleStateStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&RuleServiceRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate rule state database: %w", err)
	}

	return store, nil
}

// GetServiceID retrieves the current service ID for a rule
func (rs *RuleStateStore) GetServiceID(ruleUUID string) (string, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var record RuleServiceRecord
	err := rs.db.Where("rule_uuid = ?", ruleUUID).First(&record).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Return empty string as default if not found
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return record.CurrentServiceID, nil
}

// SetServiceID sets the current service ID for a rule
func (rs *RuleStateStore) SetServiceID(ruleUUID string, serviceID string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	record := RuleServiceRecord{
		RuleUUID:         ruleUUID,
		CurrentServiceID: serviceID,
	}

	return rs.db.Save(&record).Error
}

// HydrateRules loads service IDs from SQLite into the provided rules
func (rs *RuleStateStore) HydrateRules(rules []typ.Rule) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var records []RuleServiceRecord
	if err := rs.db.Find(&records).Error; err != nil {
		return err
	}

	// Build lookup map by rule UUID
	serviceIDMap := make(map[string]string)
	for _, record := range records {
		serviceIDMap[record.RuleUUID] = record.CurrentServiceID
	}

	// Update rules with loaded service IDs
	for i := range rules {
		rule := &rules[i]
		serviceID, found := serviceIDMap[rule.UUID]
		if !found || serviceID == "" {
			continue
		}

		// Find the service with matching provider:model and set it as current
		for j := range rule.Services {
			svc := &rule.Services[j]
			svcID := svc.Provider + ":" + svc.Model
			if svcID == serviceID {
				rule.SetCurrentServiceID(serviceID)
				break
			}
		}
	}

	return nil
}

// ClearAll removes all persisted rule state
func (rs *RuleStateStore) ClearAll() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	return rs.db.Exec("DELETE FROM rule_service_index").Error
}
