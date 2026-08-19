package db

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// DefaultTeamID is stable across upgrades so legacy sharing keys can be
	// backfilled deterministically before user-created teams exist.
	DefaultTeamID   = "00000000-0000-0000-0000-000000000001"
	DefaultTeamSlug = "default"
	DefaultTeamName = "Default Team"
)

// TeamRecord is an authorization and routing boundary for sharing keys.
// Human-readable names and slugs may change; ID is the durable identity stored
// on tokens, rules, and usage records.
type TeamRecord struct {
	ID        string    `gorm:"primaryKey;column:id;size:36" json:"id"`
	Slug      string    `gorm:"uniqueIndex;column:slug;not null;size:64" json:"slug"`
	Name      string    `gorm:"column:name;not null;size:128" json:"name"`
	Enabled   bool      `gorm:"column:enabled;not null;default:true" json:"enabled"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (TeamRecord) TableName() string { return "teams" }

// TeamStore persists teams and mirrors them in memory because every sharing
// request must confirm that its owning team still exists and is enabled.
type TeamStore struct {
	storeConn
	mu    sync.RWMutex
	cache map[string]*TeamRecord
}

func NewTeamStore(baseDir string) (*TeamStore, error) {
	db, err := openTinglyDB(baseDir)
	if err != nil {
		return nil, fmt.Errorf("team store: %w", err)
	}
	return newTeamStore(ownedConn(db))
}

func newTeamStore(conn storeConn) (*TeamStore, error) {
	if err := conn.db.AutoMigrate(&TeamRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate teams: %w", err)
	}

	defaultTeam := TeamRecord{
		ID:      DefaultTeamID,
		Slug:    DefaultTeamSlug,
		Name:    DefaultTeamName,
		Enabled: true,
	}
	if err := conn.db.Where("id = ?", DefaultTeamID).FirstOrCreate(&defaultTeam).Error; err != nil {
		return nil, fmt.Errorf("failed to ensure default team: %w", err)
	}

	store := &TeamStore{storeConn: conn, cache: make(map[string]*TeamRecord)}
	if err := store.loadCache(); err != nil {
		return nil, fmt.Errorf("failed to load team cache: %w", err)
	}
	return store, nil
}

func (s *TeamStore) loadCache() error {
	var records []TeamRecord
	if err := s.db.Find(&records).Error; err != nil {
		return err
	}
	s.cache = make(map[string]*TeamRecord, len(records))
	for i := range records {
		record := records[i]
		s.cache[record.ID] = &record
	}
	return nil
}

func validateTeamFields(name, slug string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("team name cannot be empty")
	}
	if slug == "" || len(slug) > 64 {
		return errors.New("team slug must be between 1 and 64 characters")
	}
	for i := 0; i < len(slug); i++ {
		ch := slug[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return errors.New("team slug may contain only lowercase letters, digits, '-' and '_'")
	}
	return nil
}

func cloneTeam(record *TeamRecord) *TeamRecord {
	if record == nil {
		return nil
	}
	clone := *record
	return &clone
}

func (s *TeamStore) Create(name, slug string) (*TeamRecord, error) {
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	if err := validateTeamFields(name, slug); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	record := &TeamRecord{ID: uuid.NewString(), Name: name, Slug: slug, Enabled: true}
	if err := s.db.Create(record).Error; err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}
	s.cache[record.ID] = record
	return cloneTeam(record), nil
}

func (s *TeamStore) Get(id string) (*TeamRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.cache[id]
	if !ok {
		return nil, fmt.Errorf("team '%s' not found", id)
	}
	return cloneTeam(record), nil
}

func (s *TeamStore) List() []TeamRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := make([]TeamRecord, 0, len(s.cache))
	for _, record := range s.cache {
		records = append(records, *record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records
}

func (s *TeamStore) Update(id, name, slug string) (*TeamRecord, error) {
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	if err := validateTeamFields(name, slug); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.cache[id]
	if !ok {
		return nil, fmt.Errorf("team '%s' not found", id)
	}
	if err := s.db.Model(&TeamRecord{}).Where("id = ?", id).Updates(map[string]any{
		"name": name,
		"slug": slug,
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update team: %w", err)
	}
	record.Name = name
	record.Slug = slug
	record.UpdatedAt = time.Now()
	return cloneTeam(record), nil
}

func (s *TeamStore) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.cache[id]
	if !ok {
		return fmt.Errorf("team '%s' not found", id)
	}
	if err := s.db.Model(&TeamRecord{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		return fmt.Errorf("failed to update team enabled state: %w", err)
	}
	record.Enabled = enabled
	record.UpdatedAt = time.Now()
	return nil
}

func (s *TeamStore) Delete(id string) error {
	if id == DefaultTeamID {
		return errors.New("default team cannot be deleted")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cache[id]; !ok {
		return fmt.Errorf("team '%s' not found", id)
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasTable(&APITokenRecord{}) {
			var tokenCount int64
			if err := tx.Model(&APITokenRecord{}).Where("team_id = ?", id).Count(&tokenCount).Error; err != nil {
				return err
			}
			if tokenCount > 0 {
				return fmt.Errorf("team still owns %d sharing key(s)", tokenCount)
			}
		}
		result := tx.Where("id = ?", id).Delete(&TeamRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("team '%s' not found", id)
		}
		return nil
	}); err != nil {
		return err
	}
	delete(s.cache, id)
	return nil
}
