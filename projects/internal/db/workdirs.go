package db

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---- Working folders and folder sessions ----

// SaveWorkdir keeps a folder in the sidebar and marks it used now.
func (s *Store) SaveWorkdir(path string) error {
	now := time.Now()
	w := Workdir{Path: path, CreatedAt: now, LastUsedAt: now}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "path"}},
		DoUpdates: clause.Assignments(map[string]any{"last_used_at": now}),
	}).Create(&w).Error
}

// SaveWorkdirAt keeps a folder in the sidebar as last used at (an import's
// own time), never moving a later use back.
func (s *Store) SaveWorkdirAt(path string, at time.Time) error {
	w := Workdir{Path: path, CreatedAt: at, LastUsedAt: at}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "path"}},
		DoUpdates: clause.Assignments(map[string]any{"last_used_at": gorm.Expr("MAX(last_used_at, excluded.last_used_at)")}),
	}).Create(&w).Error
}

// RemoveWorkdir drops a folder from the sidebar. Its sessions stay, and a
// folder that has any is listed regardless.
func (s *Store) RemoveWorkdir(path string) error {
	return s.db.Delete(&Workdir{}, "path = ?", path).Error
}

// ListWorkdirs returns the kept folders, most recently used first.
func (s *Store) ListWorkdirs() ([]Workdir, error) {
	var out []Workdir
	err := s.db.Order("last_used_at DESC").Find(&out).Error
	return out, err
}

// ListSessions returns folder sessions, most recently updated first. A
// non-empty folder limits them to that folder.
func (s *Store) ListSessions(folder string) ([]Thread, error) {
	var out []Thread
	q := s.db.Where("project_id = ?", "")
	if folder != "" {
		q = q.Where("worktree = ?", folder)
	}
	err := q.Order("updated_at DESC").Find(&out).Error
	return out, err
}

// GetSession loads a folder session; a thread of a project is not one.
func (s *Store) GetSession(id string) (*Thread, error) {
	t, err := s.GetThread(id)
	if err != nil {
		return nil, err
	}
	if t.ProjectID != "" {
		return nil, ErrNotFound
	}
	return t, nil
}

// SessionCounts maps folder → session status → count.
func (s *Store) SessionCounts() (map[string]map[string]int, error) {
	var rows []struct {
		Worktree string
		Status   string
		N        int
	}
	err := s.db.Model(&Thread{}).Select("worktree, status, COUNT(*) AS n").
		Where("project_id = ?", "").Group("worktree, status").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]int{}
	for _, r := range rows {
		if out[r.Worktree] == nil {
			out[r.Worktree] = map[string]int{}
		}
		out[r.Worktree][r.Status] = r.N
	}
	return out, nil
}

// HasSession reports whether any thread runs Claude Code session sid.
func (s *Store) HasSession(sid string) (bool, error) {
	var n int64
	err := s.db.Model(&Thread{}).Where("session_id = ?", sid).Count(&n).Error
	return n > 0, err
}

// ImportThread creates a thread with its whole transcript, or nothing. A
// thread that already carries an id keeps it.
func (s *Store) ImportThread(t *Thread, events []*ThreadEvent) error {
	if t.ID == "" {
		t.ID = newID()
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		for _, e := range events {
			e.ID, e.ThreadID = newID(), t.ID
			if err := tx.Create(e).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
