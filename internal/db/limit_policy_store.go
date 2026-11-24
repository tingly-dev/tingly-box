package db

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/ai"
)

// LimitPolicyRecord persists one limit policy over a person, team or model
// (see .design/routing-limits.md §10). (kind, ref) is the identity.
type LimitPolicyRecord struct {
	Kind                string     `gorm:"primaryKey;column:kind;size:16"`
	Ref                 string     `gorm:"primaryKey;column:ref;size:256"`
	Limits              *ai.Limits `gorm:"column:limits;type:text;serializer:json"`
	Overflow            string     `gorm:"column:overflow;size:16"`
	QueueTimeoutSeconds int        `gorm:"column:queue_timeout_seconds"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM.
func (LimitPolicyRecord) TableName() string { return "limit_policies" }

func (r LimitPolicyRecord) policy() ai.LimitPolicy {
	p := ai.LimitPolicy{
		Kind:                ai.LimitPolicyKind(r.Kind),
		Ref:                 r.Ref,
		Overflow:            r.Overflow,
		QueueTimeoutSeconds: r.QueueTimeoutSeconds,
	}
	if r.Limits != nil {
		p.Limits = *r.Limits
	}
	return p
}

// LimitPolicyStore persists policies and mirrors them in memory: resolution
// runs on every model request, so reads never touch SQLite and a write is
// visible to the next request as soon as Upsert/Delete returns.
type LimitPolicyStore struct {
	storeConn
	mu    sync.RWMutex
	cache map[string]ai.LimitPolicy // key: kind + "\x00" + ref
}

func policyCacheKey(kind ai.LimitPolicyKind, ref string) string {
	return string(kind) + "\x00" + ref
}

func newLimitPolicyStore(conn storeConn) (*LimitPolicyStore, error) {
	if err := conn.db.AutoMigrate(&LimitPolicyRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate limit policies: %w", err)
	}
	s := &LimitPolicyStore{storeConn: conn, cache: map[string]ai.LimitPolicy{}}
	var records []LimitPolicyRecord
	if err := s.db.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to load limit policies: %w", err)
	}
	for _, r := range records {
		p := r.policy()
		s.cache[policyCacheKey(p.Kind, p.Ref)] = p
	}
	return s, nil
}

// NewLimitPolicyStore opens a standalone store (tests, tools); the server
// uses the one owned by StoreManager.
func NewLimitPolicyStore(baseDir string) (*LimitPolicyStore, error) {
	db, err := openTinglyDB(baseDir)
	if err != nil {
		return nil, fmt.Errorf("limit policy store: %w", err)
	}
	return newLimitPolicyStore(ownedConn(db))
}

// GetLimitPolicy returns the policy for (kind, ref). It satisfies the routing
// layer's PolicyResolver.
func (s *LimitPolicyStore) GetLimitPolicy(kind ai.LimitPolicyKind, ref string) (ai.LimitPolicy, bool) {
	if s == nil {
		return ai.LimitPolicy{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.cache[policyCacheKey(kind, ref)]
	return p, ok
}

// List returns all policies, optionally narrowed to one kind and/or ref,
// ordered by kind then ref.
func (s *LimitPolicyStore) List(kind ai.LimitPolicyKind, ref string) []ai.LimitPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ai.LimitPolicy, 0, len(s.cache))
	for _, p := range s.cache {
		if (kind == "" || p.Kind == kind) && (ref == "" || p.Ref == ref) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Ref < out[j].Ref
	})
	return out
}

// Upsert validates, normalizes and stores a policy. A policy that holds no
// limit after normalization is deleted instead (saving an emptied editor
// clears the policy). The returned bool reports whether a policy now exists.
func (s *LimitPolicyStore) Upsert(p ai.LimitPolicy) (ai.LimitPolicy, bool, error) {
	if !p.Kind.IsValid() {
		return p, false, fmt.Errorf("unknown limit policy kind %q", p.Kind)
	}
	if p.Ref == "" {
		return p, false, errors.New("limit policy ref cannot be empty")
	}
	norm, ok := ai.NormalizePolicy(p)
	if !ok {
		return p, false, s.Delete(p.Kind, p.Ref)
	}
	limits := norm.Limits
	rec := LimitPolicyRecord{
		Kind: string(norm.Kind), Ref: norm.Ref, Limits: &limits,
		Overflow: norm.Overflow, QueueTimeoutSeconds: norm.QueueTimeoutSeconds, UpdatedAt: time.Now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.Save(&rec).Error; err != nil {
		return norm, false, fmt.Errorf("failed to save limit policy: %w", err)
	}
	s.cache[policyCacheKey(norm.Kind, norm.Ref)] = norm
	return norm, true, nil
}

// Delete removes a policy (no error when it does not exist).
func (s *LimitPolicyStore) Delete(kind ai.LimitPolicyKind, ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.Where("kind = ? AND ref = ?", string(kind), ref).Delete(&LimitPolicyRecord{}).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to delete limit policy: %w", err)
	}
	delete(s.cache, policyCacheKey(kind, ref))
	return nil
}
