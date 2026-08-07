package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleRecord is the GORM model for persisting a routing rule.
//
// Column vs fat-field split (see .design/rule-storage.md):
//   - Columns: identity (uuid), the routing key (scenario, request_model),
//     list/filter state (active, smart_enabled) and ordering (position).
//     These are what queries actually filter or sort on.
//   - Fat JSON text: services, flags, lb_tactic, smart_routing. They are
//     structurally nested, evolve frequently (flags gain fields every few
//     weeks), and are only ever read as a whole together with the rule.
type RuleRecord struct {
	UUID string `gorm:"primaryKey;column:uuid"`

	// Position preserves the rule-list order that used to be implied by the
	// JSON array in config.json. DefaultRequestID indexes into that order.
	Position int `gorm:"column:position;index"`

	Scenario      string `gorm:"column:scenario;not null;index:idx_rules_scenario_request_model"`
	RequestModel  string `gorm:"column:request_model;not null;index:idx_rules_scenario_request_model"`
	ResponseModel string `gorm:"column:response_model"`
	Description   string `gorm:"column:description"`
	Active        bool   `gorm:"column:active"`
	SmartEnabled  bool   `gorm:"column:smart_enabled"`

	// Fat JSON fields. Same wire format as the legacy config.json entries, so
	// migration is a lossless re-encode of the exact structures.
	Services     string `gorm:"column:services;type:text"`
	Flags        string `gorm:"column:flags;type:text"`
	LBTactic     string `gorm:"column:lb_tactic;type:text"`
	SmartRouting string `gorm:"column:smart_routing;type:text"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (RuleRecord) TableName() string {
	return "rules"
}

// toRule converts a RuleRecord back to the typ.Rule domain model.
// Corrupted fat fields are logged and skipped rather than failing the load:
// a rule with default tactic/flags is repairable from the UI, a server that
// refuses to start is not.
func (r *RuleRecord) toRule() typ.Rule {
	rule := typ.Rule{
		UUID:          r.UUID,
		Scenario:      typ.RuleScenario(r.Scenario),
		RequestModel:  r.RequestModel,
		ResponseModel: r.ResponseModel,
		Description:   r.Description,
		Active:        r.Active,
		SmartEnabled:  r.SmartEnabled,
	}

	if r.Services != "" {
		if err := json.Unmarshal([]byte(r.Services), &rule.Services); err != nil {
			logrus.WithError(err).Warnf("rule %s: failed to decode services JSON", r.UUID)
		}
	}
	if r.Flags != "" {
		if err := json.Unmarshal([]byte(r.Flags), &rule.Flags); err != nil {
			logrus.WithError(err).Warnf("rule %s: failed to decode flags JSON", r.UUID)
		}
	}
	if r.LBTactic != "" {
		if err := json.Unmarshal([]byte(r.LBTactic), &rule.LBTactic); err != nil {
			logrus.WithError(err).Warnf("rule %s: failed to decode lb_tactic JSON", r.UUID)
		}
	}
	if r.SmartRouting != "" {
		if err := json.Unmarshal([]byte(r.SmartRouting), &rule.SmartRouting); err != nil {
			logrus.WithError(err).Warnf("rule %s: failed to decode smart_routing JSON", r.UUID)
		}
	}

	return rule
}

// newRuleRecord converts a typ.Rule to its persistence form at the given
// list position. Timestamps are left zero; SyncAll fills them in.
func newRuleRecord(rule *typ.Rule, position int) *RuleRecord {
	record := &RuleRecord{
		UUID:          rule.UUID,
		Position:      position,
		Scenario:      string(rule.Scenario),
		RequestModel:  rule.RequestModel,
		ResponseModel: rule.ResponseModel,
		Description:   rule.Description,
		Active:        rule.Active,
		SmartEnabled:  rule.SmartEnabled,
	}

	record.Services = marshalRuleField(rule.UUID, "services", rule.Services, len(rule.Services) > 0)
	record.Flags = marshalRuleField(rule.UUID, "flags", rule.Flags, rule.Flags != (typ.RuleFlags{}))
	record.LBTactic = marshalRuleField(rule.UUID, "lb_tactic", rule.LBTactic, rule.LBTactic.Type != 0 || rule.LBTactic.Params != nil)
	record.SmartRouting = marshalRuleField(rule.UUID, "smart_routing", rule.SmartRouting, len(rule.SmartRouting) > 0)

	return record
}

// marshalRuleField JSON-encodes one fat field, storing "" for empty values so
// unset stays distinguishable from explicit zero.
func marshalRuleField(uuid, name string, value interface{}, present bool) string {
	if !present {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		logrus.WithError(err).Warnf("rule %s: failed to encode %s JSON", uuid, name)
		return ""
	}
	return string(data)
}

// payloadEqual reports whether two records carry the same persisted rule
// content (everything except timestamps). Used to skip no-op writes so
// updated_at only moves when the rule actually changed.
func (r *RuleRecord) payloadEqual(other *RuleRecord) bool {
	return r.UUID == other.UUID &&
		r.Position == other.Position &&
		r.Scenario == other.Scenario &&
		r.RequestModel == other.RequestModel &&
		r.ResponseModel == other.ResponseModel &&
		r.Description == other.Description &&
		r.Active == other.Active &&
		r.SmartEnabled == other.SmartEnabled &&
		r.Services == other.Services &&
		r.Flags == other.Flags &&
		r.LBTactic == other.LBTactic &&
		r.SmartRouting == other.SmartRouting
}

// RuleStore persists routing rules in SQLite. It is the durability layer
// behind Config's in-memory rule list: reads at request time stay in memory
// (rules carry hydrated runtime stats), the store is the source of truth
// across restarts.
type RuleStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewRuleStore creates a RuleStore on an existing GORM DB, running schema
// migration. Used by StoreManager (shared connection) and tests.
func NewRuleStore(db *gorm.DB, dbPath string) (*RuleStore, error) {
	if err := db.AutoMigrate(&RuleRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate rules table: %w", err)
	}
	return &RuleStore{db: db, dbPath: dbPath}, nil
}

// List returns all rules ordered by their list position.
func (rs *RuleStore) List() ([]typ.Rule, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var records []RuleRecord
	if err := rs.db.Order("position asc").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}

	rules := make([]typ.Rule, 0, len(records))
	for i := range records {
		rules = append(rules, records[i].toRule())
	}
	return rules, nil
}

// GetByUUID returns a single rule by UUID.
func (rs *RuleStore) GetByUUID(uuid string) (*typ.Rule, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var record RuleRecord
	if err := rs.db.Where("uuid = ?", uuid).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("rule with UUID '%s' not found", uuid)
		}
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}
	rule := record.toRule()
	return &rule, nil
}

// Count returns the total number of stored rules.
func (rs *RuleStore) Count() (int64, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var count int64
	if err := rs.db.Model(&RuleRecord{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count rules: %w", err)
	}
	return count, nil
}

// SyncAll makes the table match the given rule list exactly, in one
// transaction: upserts changed rules, deletes rules no longer present, and
// records each rule's list position. Unchanged rows are not touched, so
// created_at/updated_at stay meaningful. Rules with an empty UUID are skipped
// with a warning — they cannot be keyed, and the config layer assigns UUIDs
// before persisting.
func (rs *RuleStore) SyncAll(rules []typ.Rule) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	now := time.Now()
	return rs.db.Transaction(func(tx *gorm.DB) error {
		var existing []RuleRecord
		if err := tx.Find(&existing).Error; err != nil {
			return fmt.Errorf("failed to load existing rules: %w", err)
		}
		existingByUUID := make(map[string]*RuleRecord, len(existing))
		for i := range existing {
			existingByUUID[existing[i].UUID] = &existing[i]
		}

		keep := make(map[string]bool, len(rules))
		for i := range rules {
			rule := &rules[i]
			if rule.UUID == "" {
				logrus.Warnf("rule store: skipping rule without UUID (request_model=%s scenario=%s)", rule.RequestModel, rule.Scenario)
				continue
			}
			if keep[rule.UUID] {
				logrus.Warnf("rule store: skipping duplicate rule UUID %s", rule.UUID)
				continue
			}
			keep[rule.UUID] = true

			record := newRuleRecord(rule, i)
			if old, ok := existingByUUID[rule.UUID]; ok {
				if record.payloadEqual(old) {
					continue
				}
				record.CreatedAt = old.CreatedAt
				record.UpdatedAt = now
				if err := tx.Save(record).Error; err != nil {
					return fmt.Errorf("failed to update rule %s: %w", rule.UUID, err)
				}
			} else {
				record.CreatedAt = now
				record.UpdatedAt = now
				if err := tx.Create(record).Error; err != nil {
					return fmt.Errorf("failed to create rule %s: %w", rule.UUID, err)
				}
			}
		}

		for uuid := range existingByUUID {
			if keep[uuid] {
				continue
			}
			if err := tx.Where("uuid = ?", uuid).Delete(&RuleRecord{}).Error; err != nil {
				return fmt.Errorf("failed to delete rule %s: %w", uuid, err)
			}
		}

		return nil
	})
}

// GetDB returns the underlying GORM DB instance (for testing/advanced usage)
func (rs *RuleStore) GetDB() *gorm.DB {
	return rs.db
}
