package db

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// ManagedAgentStore implements the four index stores of
// managedagent.Stores over the shared SQLite handle. Reads go to SQLite
// directly: nothing here sits on the per-request LLM path, so the
// write-through cache the provider/token stores need (.design/db.md) would
// be complexity without a benchmark behind it.
type ManagedAgentStore struct {
	db *gorm.DB
}

var (
	_ managedagent.FolderStore  = (*ManagedAgentStore)(nil)
	_ managedagent.SessionStore = (*ManagedAgentStore)(nil)
)

// NewManagedAgentStore migrates the two tables and returns the store.
func NewManagedAgentStore(db *gorm.DB) (*ManagedAgentStore, error) {
	if err := db.AutoMigrate(&AgentFolderRecord{}, &AgentSessionRecord{}); err != nil {
		return nil, fmt.Errorf("migrate managed agent tables: %w", err)
	}
	return &ManagedAgentStore{db: db}, nil
}

// Stores bundles this store with a file event log into managedagent.Stores.
func (s *ManagedAgentStore) Stores(events managedagent.EventStore) managedagent.Stores {
	return managedagent.Stores{Folders: s, Sessions: s, Events: events}
}

func wrapNotFound(err error, entity, id string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s %s: %w", entity, id, managedagent.ErrNotFound)
	}
	return fmt.Errorf("%s %s: %w", entity, id, err)
}

// deleteOne runs a delete by primary key and maps "no rows" to ErrNotFound.
func (s *ManagedAgentStore) deleteOne(ctx context.Context, model any, entity, id string) error {
	res := s.db.WithContext(ctx).Where("id = ?", id).Delete(model)
	if res.Error != nil {
		return fmt.Errorf("delete %s %s: %w", entity, id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%s %s: %w", entity, id, managedagent.ErrNotFound)
	}
	return nil
}

// updateOne saves all columns and maps "no rows" to ErrNotFound.
func (s *ManagedAgentStore) updateOne(ctx context.Context, rec any, entity, id string) error {
	res := s.db.WithContext(ctx).Model(rec).Select("*").Where("id = ?", id).Updates(rec)
	if res.Error != nil {
		return fmt.Errorf("update %s %s: %w", entity, id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%s %s: %w", entity, id, managedagent.ErrNotFound)
	}
	return nil
}

// ---------- sources ----------

// defaultSessionListLimit caps a list page when the caller asks for no
// specific limit.
const defaultSessionListLimit = 200

// ---------- folders ----------

func (s *ManagedAgentStore) CreateFolder(ctx context.Context, f *managedagent.Folder) error {
	if err := s.db.WithContext(ctx).Create(toAgentFolderRecord(f)).Error; err != nil {
		return fmt.Errorf("create folder: %w", err)
	}
	return nil
}

func (s *ManagedAgentStore) GetFolder(ctx context.Context, id string) (*managedagent.Folder, error) {
	var rec AgentFolderRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		return nil, wrapNotFound(err, "folder", id)
	}
	f := fromAgentFolderRecord(&rec)
	return &f, nil
}

func (s *ManagedAgentStore) GetFolderByPath(ctx context.Context, path string) (*managedagent.Folder, error) {
	var rec AgentFolderRecord
	if err := s.db.WithContext(ctx).First(&rec, "path = ?", path).Error; err != nil {
		return nil, wrapNotFound(err, "folder", path)
	}
	f := fromAgentFolderRecord(&rec)
	return &f, nil
}

// ListFolders orders by most recently used: that is the order the composer
// and the picker offer them in.
func (s *ManagedAgentStore) ListFolders(ctx context.Context) ([]managedagent.Folder, error) {
	var recs []AgentFolderRecord
	if err := s.db.WithContext(ctx).Order("last_used_at DESC").Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	out := make([]managedagent.Folder, 0, len(recs))
	for i := range recs {
		out = append(out, fromAgentFolderRecord(&recs[i]))
	}
	return out, nil
}

func (s *ManagedAgentStore) UpdateFolder(ctx context.Context, f *managedagent.Folder) error {
	return s.updateOne(ctx, toAgentFolderRecord(f), "folder", f.ID)
}

func (s *ManagedAgentStore) DeleteFolder(ctx context.Context, id string) error {
	return s.deleteOne(ctx, &AgentFolderRecord{}, "folder", id)
}

// ---------- sessions ----------

func (s *ManagedAgentStore) CreateSession(ctx context.Context, sess *managedagent.Session) error {
	if err := s.db.WithContext(ctx).Create(toAgentSessionRecord(sess)).Error; err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *ManagedAgentStore) GetSession(ctx context.Context, id string) (*managedagent.Session, error) {
	var rec AgentSessionRecord
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&rec).Error; err != nil {
		return nil, wrapNotFound(err, "session", id)
	}
	out := fromAgentSessionRecord(&rec)
	return &out, nil
}

func (s *ManagedAgentStore) ListSessions(ctx context.Context, f managedagent.SessionFilter) ([]managedagent.Session, error) {
	q := s.db.WithContext(ctx).Model(&AgentSessionRecord{})
	if f.FolderID != "" {
		q = q.Where("folder_id = ?", f.FolderID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", string(f.Status))
	}
	if f.Active {
		q = q.Where("status IN ?", []string{
			string(managedagent.SessionQueued), string(managedagent.SessionRunning),
			string(managedagent.SessionWaitingInput), string(managedagent.SessionIdle),
		})
	}
	limit := f.Limit
	if limit <= 0 {
		limit = defaultSessionListLimit
	}
	var recs []AgentSessionRecord
	if err := q.Order("last_active_at DESC").Limit(limit).Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	out := make([]managedagent.Session, 0, len(recs))
	for i := range recs {
		out = append(out, fromAgentSessionRecord(&recs[i]))
	}
	return out, nil
}

func (s *ManagedAgentStore) UpdateSession(ctx context.Context, sess *managedagent.Session) error {
	return s.updateOne(ctx, toAgentSessionRecord(sess), "session", sess.ID)
}
