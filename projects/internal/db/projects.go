package db

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ---- Projects ----

// ListProjects returns active or archived projects of every type whose name
// or goal match q.
func (s *Store) ListProjects(q string, archived bool) ([]Project, error) {
	var out []Project
	tx := s.db.Model(&Project{})
	if archived {
		tx = tx.Where("archived_at IS NOT NULL")
	} else {
		tx = tx.Where("archived_at IS NULL")
	}
	if q = strings.TrimSpace(q); q != "" {
		p := likePattern(q)
		tx = tx.Where(`name LIKE ? ESCAPE '\' OR goal LIKE ? ESCAPE '\' OR path LIKE ? ESCAPE '\'`, p, p, p)
	}
	err := tx.Order("starred DESC, updated_at DESC").Find(&out).Error
	return out, err
}

// CreateProject inserts a project.
func (s *Store) CreateProject(p *Project) error {
	p.ID = newID()
	if p.Type == "" {
		p.Type = ProjectProject
	}
	return s.db.Create(p).Error
}

// CreateProjectWithMain inserts a project together with its main thread
// (agent llm; its cwd is the project directory under projectsDir) and points
// the project at it.
func (s *Store) CreateProjectWithMain(p *Project, main *Thread, projectsDir string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		p.ID = newID()
		p.Type = ProjectProject
		main.ID = newID()
		main.ProjectID, main.Agent = p.ID, AgentLLM
		if main.Title == "" {
			main.Title = p.Name
		}
		if main.Status == "" {
			main.Status = ThreadIdle
		}
		p.MainThreadID = main.ID
		if main.Cwd == "" && projectsDir != "" {
			main.Cwd = filepath.Join(projectsDir, p.ID)
		}
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		return tx.Create(main).Error
	})
}

// GetProject loads a project.
func (s *Store) GetProject(id string) (*Project, error) {
	var p Project
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &p, nil
}

// SaveProject writes all fields of p.
func (s *Store) SaveProject(p *Project) error {
	return s.db.Save(p).Error
}

// TouchProject bumps a project's UpdatedAt (recent activity).
func (s *Store) TouchProject(id string) error {
	return s.db.Model(&Project{}).Where("id = ?", id).UpdateColumn("updated_at", time.Now()).Error
}

// ThreadCounts is how many threads a project has in each Overview group.
type ThreadCounts struct {
	Working  int `json:"working"`
	Waiting  int `json:"waiting"`
	Ready    int `json:"ready"`
	Idle     int `json:"idle"`
	Resolved int `json:"resolved"`
}

// CountsOf turns a status → count map into ThreadCounts.
func CountsOf(m map[string]int) ThreadCounts {
	return ThreadCounts{Working: m[ThreadWorking], Waiting: m[ThreadWaiting], Ready: m[ThreadReady], Idle: m[ThreadIdle], Resolved: m[ThreadResolved]}
}

// ProjectThreadCounts is ThreadCounts for one project.
func (s *Store) ProjectThreadCounts(projectID string) (ThreadCounts, error) {
	var rows []struct {
		Status string
		N      int
	}
	if err := s.db.Model(&Thread{}).Select("status, COUNT(*) AS n").Where("project_id = ? AND agent <> ?", projectID, AgentLLM).Group("status").Scan(&rows).Error; err != nil {
		return ThreadCounts{}, err
	}
	m := map[string]int{}
	for _, r := range rows {
		m[r.Status] = r.N
	}
	return CountsOf(m), nil
}

// ThreadCounts maps project ID → thread status → count. A main thread is the
// project's conversation, not one of its threads, and is not counted.
func (s *Store) ThreadCounts() (map[string]map[string]int, error) {
	var rows []struct {
		ProjectID string
		Status    string
		N         int
	}
	if err := s.db.Model(&Thread{}).Select("project_id, status, COUNT(*) AS n").Where("agent <> ?", AgentLLM).Group("project_id, status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]map[string]int{}
	for _, r := range rows {
		if out[r.ProjectID] == nil {
			out[r.ProjectID] = map[string]int{}
		}
		out[r.ProjectID][r.Status] = r.N
	}
	return out, nil
}

// ---- Folders ----

// FindFolder returns the folder project at path, matching spellings of the
// same folder (trailing slash, symlink, case on case-insensitive systems).
func (s *Store) FindFolder(path string) (*Project, error) {
	key := PathKey(path)
	var folders []Project
	if err := s.db.Where("type = ?", ProjectFolder).Find(&folders).Error; err != nil {
		return nil, err
	}
	for i := range folders {
		if PathKey(folders[i].Path) == key {
			return &folders[i], nil
		}
	}
	return nil, ErrNotFound
}

// OpenFolder returns the folder project for path, creating it when there is
// none. An archived folder is brought back: asking for it is using it.
func (s *Store) OpenFolder(path, name string) (*Project, error) {
	path = CanonicalPath(path)
	p, err := s.FindFolder(path)
	switch {
	case err == nil:
		if p.ArchivedAt != nil {
			p.ArchivedAt = nil
			if err := s.SaveProject(p); err != nil {
				return nil, err
			}
		}
		return p, nil
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}
	if name = strings.TrimSpace(name); name == "" {
		name = FolderName(path)
	}
	p = &Project{Type: ProjectFolder, Path: path, Name: name}
	if err := s.CreateProject(p); err != nil {
		return nil, err
	}
	return p, nil
}

// ---- Threads ----

// GetMainThread loads a project's main thread.
func (s *Store) GetMainThread(projectID string) (*Thread, error) {
	var t Thread
	if err := s.db.Where("project_id = ? AND agent = ?", projectID, AgentLLM).First(&t).Error; err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

// CreateThread inserts a thread. A thread that is not a main thread gets the
// next number in its project, which is never reused.
func (s *Store) CreateThread(t *Thread) error {
	t.ID = newID()
	if t.Agent == "" {
		t.Agent = AgentClaudeCode
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if t.Agent != AgentLLM && t.Seq == 0 {
			var max int
			if err := tx.Model(&Thread{}).Where("project_id = ?", t.ProjectID).Select("COALESCE(MAX(seq), 0)").Scan(&max).Error; err != nil {
				return err
			}
			t.Seq = max + 1
		}
		return tx.Create(t).Error
	})
}

// GetThread loads a thread.
func (s *Store) GetThread(id string) (*Thread, error) {
	var t Thread
	if err := s.db.First(&t, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

// SaveThread writes all fields of t.
func (s *Store) SaveThread(t *Thread) error {
	return s.db.Save(t).Error
}

// ListThreads returns a project's threads, most recently updated first. The
// main thread is the project's conversation and is not listed.
func (s *Store) ListThreads(projectID string) ([]Thread, error) {
	var out []Thread
	err := s.db.Where("project_id = ? AND agent <> ?", projectID, AgentLLM).Order("updated_at DESC").Find(&out).Error
	return out, err
}

// ListThreadsByStatus returns threads in any of the given statuses across
// all projects.
func (s *Store) ListThreadsByStatus(statuses ...string) ([]Thread, error) {
	var out []Thread
	err := s.db.Where("status IN ? AND agent <> ?", statuses, AgentLLM).Find(&out).Error
	return out, err
}

// Lineage returns a thread and its ancestors, the thread first.
func (s *Store) Lineage(id string) ([]Thread, error) {
	var out []Thread
	for i := 0; id != "" && i < 16; i++ {
		t, err := s.GetThread(id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				break
			}
			return nil, err
		}
		out = append(out, *t)
		id = t.ParentID
	}
	return out, nil
}

// ---- Thread events ----

// AddThreadEvent appends to a thread's transcript.
func (s *Store) AddThreadEvent(e *ThreadEvent) error {
	e.ID = newID()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return s.db.Create(e).Error
}

// UpdateThreadEvent rewrites an event's kind, content and payload (e.g. an
// approval once answered).
func (s *Store) UpdateThreadEvent(e *ThreadEvent) error {
	return s.db.Model(&ThreadEvent{}).Where("id = ?", e.ID).
		UpdateColumns(map[string]any{"kind": e.Kind, "content": e.Content, "payload": e.Payload}).Error
}

// GetThreadEvent loads one event.
func (s *Store) GetThreadEvent(id string) (*ThreadEvent, error) {
	var e ThreadEvent
	if err := s.db.First(&e, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &e, nil
}

// ListThreadEvents returns a thread's transcript in order.
func (s *Store) ListThreadEvents(threadID string) ([]ThreadEvent, error) {
	return s.ListRecentThreadEvents(threadID, 0)
}

// ListRecentThreadEvents returns the latest limit events of a thread in
// chronological order (all when limit <= 0).
func (s *Store) ListRecentThreadEvents(threadID string, limit int) ([]ThreadEvent, error) {
	var out []ThreadEvent
	if limit <= 0 {
		err := s.db.Where("thread_id = ?", threadID).Order("created_at ASC, rowid ASC").Find(&out).Error
		return out, err
	}
	if err := s.db.Where("thread_id = ?", threadID).Order("created_at DESC, rowid DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ProjectUsage sums thread token usage by model for a project.
func (s *Store) ProjectUsage(projectID string) ([]ModelUsage, error) {
	var out []ModelUsage
	err := s.db.Model(&Thread{}).
		Select("model, COUNT(*) AS threads, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens").
		Where("project_id = ? AND agent <> ?", projectID, AgentLLM).Group("model").Scan(&out).Error
	return out, err
}

// ModelUsage is token use aggregated per model.
type ModelUsage struct {
	Model        string `json:"model"`
	Threads      int    `json:"threads"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}
