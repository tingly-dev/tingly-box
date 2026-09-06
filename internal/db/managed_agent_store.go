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
	_ managedagent.SourceStore      = (*ManagedAgentStore)(nil)
	_ managedagent.EnvironmentStore = (*ManagedAgentStore)(nil)
	_ managedagent.WorkspaceStore   = (*ManagedAgentStore)(nil)
	_ managedagent.SessionStore     = (*ManagedAgentStore)(nil)
)

// NewManagedAgentStore migrates the four tables and returns the store.
func NewManagedAgentStore(db *gorm.DB) (*ManagedAgentStore, error) {
	if err := db.AutoMigrate(
		&AgentSourceRecord{},
		&AgentEnvironmentRecord{},
		&AgentWorkspaceRecord{},
		&AgentSessionRecord{},
	); err != nil {
		return nil, fmt.Errorf("migrate managed agent tables: %w", err)
	}
	return &ManagedAgentStore{db: db}, nil
}

// Stores bundles this store with a file event log into managedagent.Stores.
func (s *ManagedAgentStore) Stores(events managedagent.EventStore) managedagent.Stores {
	return managedagent.Stores{Sources: s, Environments: s, Workspaces: s, Sessions: s, Events: events}
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

func (s *ManagedAgentStore) CreateSource(ctx context.Context, src *managedagent.Source) error {
	if err := s.db.WithContext(ctx).Create(toAgentSourceRecord(src)).Error; err != nil {
		return fmt.Errorf("create source: %w", err)
	}
	return nil
}

func (s *ManagedAgentStore) GetSource(ctx context.Context, id string) (*managedagent.Source, error) {
	var rec AgentSourceRecord
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&rec).Error; err != nil {
		return nil, wrapNotFound(err, "source", id)
	}
	out := fromAgentSourceRecord(&rec)
	return &out, nil
}

func (s *ManagedAgentStore) ListSources(ctx context.Context) ([]managedagent.Source, error) {
	var recs []AgentSourceRecord
	if err := s.db.WithContext(ctx).Order("created_at ASC").Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	out := make([]managedagent.Source, 0, len(recs))
	for i := range recs {
		out = append(out, fromAgentSourceRecord(&recs[i]))
	}
	return out, nil
}

func (s *ManagedAgentStore) UpdateSource(ctx context.Context, src *managedagent.Source) error {
	return s.updateOne(ctx, toAgentSourceRecord(src), "source", src.ID)
}

func (s *ManagedAgentStore) DeleteSource(ctx context.Context, id string) error {
	return s.deleteOne(ctx, &AgentSourceRecord{}, "source", id)
}

// ---------- environments ----------

func (s *ManagedAgentStore) CreateEnvironment(ctx context.Context, e *managedagent.Environment) error {
	if err := s.db.WithContext(ctx).Create(toAgentEnvironmentRecord(e)).Error; err != nil {
		return fmt.Errorf("create environment: %w", err)
	}
	return nil
}

func (s *ManagedAgentStore) GetEnvironment(ctx context.Context, id string) (*managedagent.Environment, error) {
	var rec AgentEnvironmentRecord
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&rec).Error; err != nil {
		return nil, wrapNotFound(err, "environment", id)
	}
	out := fromAgentEnvironmentRecord(&rec)
	return &out, nil
}

func (s *ManagedAgentStore) ListEnvironments(ctx context.Context) ([]managedagent.Environment, error) {
	var recs []AgentEnvironmentRecord
	if err := s.db.WithContext(ctx).Order("is_default DESC, created_at ASC").Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	out := make([]managedagent.Environment, 0, len(recs))
	for i := range recs {
		out = append(out, fromAgentEnvironmentRecord(&recs[i]))
	}
	return out, nil
}

func (s *ManagedAgentStore) UpdateEnvironment(ctx context.Context, e *managedagent.Environment) error {
	return s.updateOne(ctx, toAgentEnvironmentRecord(e), "environment", e.ID)
}

func (s *ManagedAgentStore) DeleteEnvironment(ctx context.Context, id string) error {
	return s.deleteOne(ctx, &AgentEnvironmentRecord{}, "environment", id)
}

// ---------- workspaces ----------

func (s *ManagedAgentStore) CreateWorkspace(ctx context.Context, w *managedagent.Workspace) error {
	if err := s.db.WithContext(ctx).Create(toAgentWorkspaceRecord(w)).Error; err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	return nil
}

func (s *ManagedAgentStore) GetWorkspace(ctx context.Context, id string) (*managedagent.Workspace, error) {
	var rec AgentWorkspaceRecord
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&rec).Error; err != nil {
		return nil, wrapNotFound(err, "workspace", id)
	}
	out := fromAgentWorkspaceRecord(&rec)
	return &out, nil
}

func (s *ManagedAgentStore) ListWorkspaces(ctx context.Context, f managedagent.WorkspaceFilter) ([]managedagent.Workspace, error) {
	q := s.db.WithContext(ctx).Model(&AgentWorkspaceRecord{})
	if f.SourceID != "" {
		q = q.Where("source_id = ?", f.SourceID)
	}
	if f.EnvironmentID != "" {
		q = q.Where("environment_id = ?", f.EnvironmentID)
	}
	if f.State != "" {
		q = q.Where("state = ?", string(f.State))
	}
	var recs []AgentWorkspaceRecord
	if err := q.Order("last_active_at DESC").Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	out := make([]managedagent.Workspace, 0, len(recs))
	for i := range recs {
		out = append(out, fromAgentWorkspaceRecord(&recs[i]))
	}
	return out, nil
}

func (s *ManagedAgentStore) UpdateWorkspace(ctx context.Context, w *managedagent.Workspace) error {
	return s.updateOne(ctx, toAgentWorkspaceRecord(w), "workspace", w.ID)
}

// ---------- sessions ----------

// defaultSessionListLimit caps an unbounded ListSessions: the sessions page
// wants the recent tail, not the whole history.
const defaultSessionListLimit = 200

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
	if f.WorkspaceID != "" {
		q = q.Where("workspace_id = ?", f.WorkspaceID)
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
