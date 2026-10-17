package db

import (
	"time"

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
