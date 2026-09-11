package visionproxy

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DescribeStore is where describeCache keeps descriptions: the SQLite
// implementation below in production, so a process restart costs a row
// lookup rather than a vision call; memoryDescribeStore for tests and as
// the no-database fallback.
//
// Implementations must be safe for concurrent use. Errors are never
// surfaced to the request path — a failing read is a miss and a failing
// write is dropped; the request itself never fails.
type DescribeStore interface {
	// Get returns the replacement text for key, if any.
	Get(key visionCacheKey) (string, bool)
	// Put upserts the replacement text for key.
	Put(key visionCacheKey, text string)
}

// Retention for the durable tier. There is deliberately no age limit: a
// row is a few hundred bytes of text, and a description already paid for
// only becomes more valuable the longer the conversation it belongs to
// keeps coming back — dropping it by age would throw away exactly the
// prefix stability the cache exists for.
//
// describeStoreMaxRows is the one safety valve, so a long-lived gateway
// cannot grow the table without bound; the least recently used rows go
// first. 100000 rows is on the order of tens of megabytes at the very
// worst and is not reached in normal use.
const (
	describeStoreMaxRows = 100000
	// describeStorePruneEvery throttles how often Put runs the prune query;
	// a prune per write would turn every describe into two extra statements.
	describeStorePruneEvery = time.Hour
	// describeStoreTouchEvery throttles last_used_at bumps on Get: a session
	// with dozens of historical images would otherwise issue dozens of
	// UPDATEs per request just to say "still here". last_used_at only
	// orders the size-ceiling eviction, so hour granularity is plenty.
	describeStoreTouchEvery = time.Hour
)

// VisionDescriptionRecord is the GORM model for one cached description. The
// composite unique index mirrors visionCacheKey exactly, so a Put is a plain
// upsert on conflict.
type VisionDescriptionRecord struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	Session     string    `gorm:"column:session;uniqueIndex:idx_vision_desc_key,priority:1;size:512"`
	Provider    string    `gorm:"column:provider;uniqueIndex:idx_vision_desc_key,priority:2;size:128"`
	Model       string    `gorm:"column:model;uniqueIndex:idx_vision_desc_key,priority:3;size:256"`
	ContentHash string    `gorm:"column:content_hash;uniqueIndex:idx_vision_desc_key,priority:4;size:2048"`
	Text        string    `gorm:"column:text;type:text"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	LastUsedAt  time.Time `gorm:"column:last_used_at;index:idx_vision_desc_last_used"`
}

// TableName specifies the table name for GORM.
func (VisionDescriptionRecord) TableName() string { return "vision_descriptions" }

// sqliteDescribeStore keeps descriptions in tingly's shared SQLite database.
// It borrows the *gorm.DB (owned by db.StoreManager) and never closes it.
type sqliteDescribeStore struct {
	db *gorm.DB

	pruneMu   sync.Mutex
	lastPrune time.Time
	now       func() time.Time
}

// NewSQLiteDescribeStore builds a DescribeStore over an existing GORM handle
// (the StoreManager's shared tingly.db connection) and runs the schema
// migration. The size ceiling is applied once here, then throttled from Put.
func NewSQLiteDescribeStore(db *gorm.DB) (DescribeStore, error) {
	if db == nil {
		return nil, errors.New("vision describe store: nil db")
	}
	if err := db.AutoMigrate(&VisionDescriptionRecord{}); err != nil {
		return nil, fmt.Errorf("vision describe store: migrate: %w", err)
	}
	s := &sqliteDescribeStore{db: db, now: time.Now}
	s.lastPrune = s.now()
	s.prune()
	return s, nil
}

func (s *sqliteDescribeStore) Get(key visionCacheKey) (string, bool) {
	var rec VisionDescriptionRecord
	err := s.db.
		Where("session = ? AND provider = ? AND model = ? AND content_hash = ?",
			key.session, key.provider, key.model, key.content).
		First(&rec).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logrus.WithError(err).WithField("component", "vision_proxy").
				Warn("vision proxy: describe store read failed; treating as miss")
		}
		return "", false
	}
	if now := s.now(); now.Sub(rec.LastUsedAt) > describeStoreTouchEvery {
		if err := s.db.Model(&VisionDescriptionRecord{}).Where("id = ?", rec.ID).
			Update("last_used_at", now).Error; err != nil {
			logrus.WithError(err).WithField("component", "vision_proxy").
				Debug("vision proxy: describe store touch failed")
		}
	}
	return rec.Text, true
}

func (s *sqliteDescribeStore) Put(key visionCacheKey, text string) {
	now := s.now()
	rec := VisionDescriptionRecord{
		Session:     key.session,
		Provider:    key.provider,
		Model:       key.model,
		ContentHash: key.content,
		Text:        text,
		CreatedAt:   now,
		LastUsedAt:  now,
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "session"}, {Name: "provider"}, {Name: "model"}, {Name: "content_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"text", "last_used_at"}),
	}).Create(&rec).Error
	if err != nil {
		logrus.WithError(err).WithField("component", "vision_proxy").
			Warn("vision proxy: describe store write failed; description dropped, image will be re-described")
		return
	}
	s.maybePrune(now)
}

// maybePrune runs prune at most once per describeStorePruneEvery.
func (s *sqliteDescribeStore) maybePrune(now time.Time) {
	s.pruneMu.Lock()
	due := now.Sub(s.lastPrune) >= describeStorePruneEvery
	if due {
		s.lastPrune = now
	}
	s.pruneMu.Unlock()
	if due {
		s.prune()
	}
}

// prune enforces the size ceiling: if the table is over
// describeStoreMaxRows, drop the least recently used rows down to it.
func (s *sqliteDescribeStore) prune() {
	log := logrus.WithField("component", "vision_proxy")
	var count int64
	if err := s.db.Model(&VisionDescriptionRecord{}).Count(&count).Error; err != nil {
		log.WithError(err).Warn("vision proxy: describe store count failed")
		return
	}
	if count <= describeStoreMaxRows {
		return
	}
	excess := count - describeStoreMaxRows
	// Delete the `excess` least-recently-used rows by id (SQLite DELETE has
	// no ORDER BY/LIMIT without a compile flag, so go through a subquery).
	sub := s.db.Model(&VisionDescriptionRecord{}).Select("id").
		Order("last_used_at ASC").Limit(int(excess))
	if err := s.db.Where("id IN (?)", sub).Delete(&VisionDescriptionRecord{}).Error; err != nil {
		log.WithError(err).Warn("vision proxy: describe store size prune failed")
	}
}
