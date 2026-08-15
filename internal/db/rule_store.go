package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleRecord is the GORM model for persisting a routing rule.
//
// Column vs fat-field split (see .design/rule-storage.md):
//   - Columns: identity (uuid), the routing key (scenario, request_model),
//     list/filter state (active, smart_enabled) and ordering (position).
//     These are what queries actually filter or sort on.
//   - JSON-backed columns (serializer:json): services, flags, lb_tactic,
//     smart_routing. They are structurally nested, evolve frequently (flags
//     gain fields every few weeks), and are only ever read as a whole
//     together with the rule. Same wire format the legacy config.json used,
//     so migration is a lossless re-encode.
//
// Unlike ProviderStore, RuleStore does not cache records: every read
// allocates fresh values through the serializer, so no clone-at-boundary
// helpers are needed (see provider_store.go for the contrast).
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

	Services     []*loadbalance.Service      `gorm:"column:services;type:text;serializer:json"`
	Flags        typ.RuleFlags               `gorm:"column:flags;type:text;serializer:json"`
	LBTactic     typ.Tactic                  `gorm:"column:lb_tactic;type:text;serializer:json"`
	SmartRouting []smartrouting.SmartRouting `gorm:"column:smart_routing;type:text;serializer:json"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (RuleRecord) TableName() string {
	return "rules"
}

// toRule converts a RuleRecord back to the typ.Rule domain model.
func (r *RuleRecord) toRule() typ.Rule {
	return typ.Rule{
		UUID:          r.UUID,
		Scenario:      typ.RuleScenario(r.Scenario),
		RequestModel:  r.RequestModel,
		ResponseModel: r.ResponseModel,
		Description:   r.Description,
		Active:        r.Active,
		SmartEnabled:  r.SmartEnabled,
		Services:      r.Services,
		Flags:         r.Flags,
		LBTactic:      r.LBTactic,
		SmartRouting:  r.SmartRouting,
	}
}

// newRuleRecord converts a typ.Rule to its persistence form at the given
// list position. Timestamps are left zero; SyncAll fills them in.
func newRuleRecord(rule *typ.Rule, position int) *RuleRecord {
	return &RuleRecord{
		UUID:          rule.UUID,
		Position:      position,
		Scenario:      string(rule.Scenario),
		RequestModel:  rule.RequestModel,
		ResponseModel: rule.ResponseModel,
		Description:   rule.Description,
		Active:        rule.Active,
		SmartEnabled:  rule.SmartEnabled,
		Services:      rule.Services,
		Flags:         rule.Flags,
		LBTactic:      rule.LBTactic,
		SmartRouting:  rule.SmartRouting,
	}
}

// payloadEqual reports whether two records carry the same persisted rule
// content (everything except timestamps). Used to skip no-op writes so
// updated_at only moves when the rule actually changed. The records contain
// slices and interface-typed tactic params, so the comparison goes through
// canonical JSON on timestamp-zeroed shallow copies; future columns are
// included automatically.
func (r RuleRecord) payloadEqual(other RuleRecord) bool {
	a, errA := canonicalRecordJSON(r)
	b, errB := canonicalRecordJSON(other)
	if errA != nil || errB != nil {
		return false // undecidable: treat as changed so the write happens
	}
	return a == b
}

// canonicalRecordJSON renders a record as round-trip-stable JSON: one
// marshal→unmarshal→marshal pass, so encodings that decoding normalizes
// compare equal. Example: a zero typ.Tactic marshals params as null, but
// reading a stored row back turns that into RandomParams ("{}") via
// Tactic.UnmarshalJSON — a record fresh from memory and its own stored form
// must not be reported as different for it. Timestamps are excluded (zeroed
// on the value copy).
func canonicalRecordJSON(r RuleRecord) (string, error) {
	r.CreatedAt, r.UpdatedAt = time.Time{}, time.Time{}
	first, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	var rt RuleRecord
	if err := json.Unmarshal(first, &rt); err != nil {
		return "", err
	}
	second, err := json.Marshal(rt)
	if err != nil {
		return "", err
	}
	return string(second), nil
}

// RuleStore persists routing rules in SQLite. It is the durability layer
// behind Config's in-memory rule list: reads at request time stay in memory
// (rules carry hydrated runtime stats), the store is the source of truth
// across restarts.
type RuleStore struct {
	storeConn
	mu sync.Mutex
}

// newRuleStore finishes setting up a RuleStore (schema migration) over an
// already-open connection — see newProviderStore.
func newRuleStore(conn storeConn) (*RuleStore, error) {
	if err := conn.db.AutoMigrate(&RuleRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate rules table: %w", err)
	}
	return &RuleStore{storeConn: conn}, nil
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
// created_at/updated_at stay meaningful. Rules with an empty or duplicate
// UUID are skipped with a warning — they cannot be keyed; the config layer's
// ensureRuleUUIDs repairs both cases before persisting, so this is pure
// defense that should never fire.
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
				if record.payloadEqual(*old) {
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
