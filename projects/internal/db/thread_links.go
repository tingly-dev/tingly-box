package db

import (
	"time"

	"gorm.io/gorm"
)

// Link kinds, roles and states. See projects/.design/github-tools.md.
const (
	LinkIssue = "issue"
	LinkPR    = "pr"

	// LinkOpened: the thread created the issue or PR. LinkCommented: it only
	// commented on one.
	LinkOpened    = "opened"
	LinkCommented = "commented"

	LinkStateOpen   = "open"
	LinkStateClosed = "closed"
	LinkStateMerged = "merged"
)

// ThreadLink is an issue or pull request a thread touched through the
// Projects tools. It is a fact the tool read back from GitHub, not something
// the model said.
type ThreadLink struct {
	ID        uint   `gorm:"primaryKey" json:"-"`
	ThreadID  string `gorm:"size:36;not null;uniqueIndex:idx_thread_link,priority:1;index" json:"thread_id"`
	ProjectID string `gorm:"size:36;not null;index" json:"project_id"`
	Kind      string `gorm:"size:10;not null;uniqueIndex:idx_thread_link,priority:3" json:"kind"`
	// Repo is owner/name on the host that served it (host/owner/name off github.com).
	Repo   string `gorm:"size:300;not null;uniqueIndex:idx_thread_link,priority:2" json:"repo"`
	Number int    `gorm:"not null;uniqueIndex:idx_thread_link,priority:4" json:"number"`
	URL    string `gorm:"size:500" json:"url"`
	Title  string `gorm:"size:500" json:"title"`
	Role   string `gorm:"size:12" json:"role"`
	State  string `gorm:"size:10;index" json:"state"`
	// Draft, CI and MergeState describe a PR. CI is none, pending, passing or
	// failing. MergeState is ready, conflicts, blocked, behind or unknown.
	Draft      bool   `json:"draft,omitempty"`
	CI         string `gorm:"size:10" json:"ci,omitempty"`
	MergeState string `gorm:"size:10" json:"merge_state,omitempty"`
	// RefreshedAt is when GitHub last answered about it. AttemptedAt is the
	// last try, answered or not. Failures counts the refreshes that failed
	// since then; LastError says why the last one did.
	RefreshedAt time.Time `json:"refreshed_at"`
	AttemptedAt time.Time `json:"-"`
	Failures    int       `json:"-"`
	LastError   string    `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UpsertThreadLink records l, keeping the role and creation time of a row that
// already exists unless it was only a comment: opening beats commenting.
func (s *Store) UpsertThreadLink(l *ThreadLink) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var old ThreadLink
		err := tx.Where("thread_id = ? AND repo = ? AND kind = ? AND number = ?", l.ThreadID, l.Repo, l.Kind, l.Number).First(&old).Error
		if err == nil {
			l.ID, l.CreatedAt = old.ID, old.CreatedAt
			if old.Role == LinkOpened {
				l.Role = LinkOpened
			}
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
		return tx.Save(l).Error
	})
}

// ThreadLinks are one thread's links, oldest first.
func (s *Store) ThreadLinks(threadID string) ([]ThreadLink, error) {
	var out []ThreadLink
	err := s.db.Where("thread_id = ?", threadID).Order("created_at, id").Find(&out).Error
	return out, err
}

// ThreadLinksOf are the links of the given threads, by thread id.
func (s *Store) ThreadLinksOf(threadIDs []string) (map[string][]ThreadLink, error) {
	out := map[string][]ThreadLink{}
	if len(threadIDs) == 0 {
		return out, nil
	}
	var rows []ThreadLink
	if err := s.db.Where("thread_id IN ?", threadIDs).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ThreadID] = append(out[r.ThreadID], r)
	}
	return out, nil
}

// GetThreadLink is the link of a thread to an issue or PR.
func (s *Store) GetThreadLink(threadID, kind string, number int) (*ThreadLink, error) {
	var l ThreadLink
	err := s.db.Where("thread_id = ? AND kind = ? AND number = ?", threadID, kind, number).Order("id").First(&l).Error
	if err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

// OpenThreadLinks are the links still open in projects that are not archived,
// least recently refreshed first: the ones a refresh sweep may need.
func (s *Store) OpenThreadLinks() ([]ThreadLink, error) {
	var out []ThreadLink
	err := s.db.Joins("JOIN projects ON projects.id = thread_links.project_id AND projects.archived_at IS NULL").
		Where("thread_links.state = ?", LinkStateOpen).Order("thread_links.refreshed_at, thread_links.id").Find(&out).Error
	return out, err
}

// SaveThreadLinkReading updates what a refresh learned (or its failure) on the
// row, nothing else.
func (s *Store) SaveThreadLinkReading(l *ThreadLink) error {
	return s.db.Model(&ThreadLink{}).Where("id = ?", l.ID).Select("Title", "URL", "State", "Draft", "CI", "MergeState", "RefreshedAt", "AttemptedAt", "Failures", "LastError").Updates(l).Error
}
