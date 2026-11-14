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
// the model said. A thread's links are kept in its own row (Thread.Links, a
// JSON column), written only by the methods below, never by SaveThread.
type ThreadLink struct {
	Kind string `json:"kind"`
	// Repo is owner/name on github.com.
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Role   string `json:"role"`
	State  string `json:"state"`
	// Draft, CI and MergeState describe a PR. CI is none, pending, passing or
	// failing. MergeState is ready, conflicts, blocked, behind or unknown.
	Draft bool `json:"draft,omitempty"`
	// Head and Base are the branches of a PR as GitHub last said (the base
	// moves when an `update` retargets it). They tell the PRs of a stack apart.
	Head       string `json:"head,omitempty"`
	Base       string `json:"base,omitempty"`
	CI         string `json:"ci,omitempty"`
	MergeState string `json:"merge_state,omitempty"`
	// RefreshedAt is when GitHub last answered about it. AttemptedAt is the
	// last try, answered or not. Failures counts the refreshes that failed
	// since then; LastError says why the last one did.
	RefreshedAt time.Time `json:"refreshed_at"`
	AttemptedAt time.Time `json:"attempted_at"`
	Failures    int       `json:"failures,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Is reports whether l is the item with this repository, kind and number.
func (l ThreadLink) Is(repo, kind string, number int) bool {
	return l.Repo == repo && l.Kind == kind && l.Number == number
}

// UpdateThreadLinks reads a thread's links, lets fn change them and writes them
// back, in one transaction and without touching the rest of the row: a caller
// holding an older copy of the thread cannot clobber them (SaveThread leaves
// the column out), nor they its other columns.
func (s *Store) UpdateThreadLinks(threadID string, fn func([]ThreadLink) []ThreadLink) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var t Thread
		if err := tx.Select("id", "links").First(&t, "id = ?", threadID).Error; err != nil {
			return notFound(err)
		}
		t.Links = fn(t.Links)
		return tx.Model(&Thread{}).Where("id = ?", threadID).Select("Links").UpdateColumns(&t).Error
	})
}

// UpsertThreadLink records l on the thread, keeping the creation time of an
// item it already has and its role if it opened it: opening beats commenting.
// It returns what is stored.
func (s *Store) UpsertThreadLink(threadID string, l ThreadLink) (ThreadLink, error) {
	err := s.UpdateThreadLinks(threadID, func(links []ThreadLink) []ThreadLink {
		for i, old := range links {
			if old.Is(l.Repo, l.Kind, l.Number) {
				l.CreatedAt = old.CreatedAt
				if old.Role == LinkOpened {
					l.Role = LinkOpened
				}
				links[i] = l
				return links
			}
		}
		if l.CreatedAt.IsZero() {
			l.CreatedAt = time.Now()
		}
		return append(links, l)
	})
	return l, err
}

// SaveThreadLinkReading updates what a refresh learned (or its failure) on one
// of the thread's links, nothing else about it.
func (s *Store) SaveThreadLinkReading(threadID string, k ThreadLink) error {
	return s.UpdateThreadLinks(threadID, func(links []ThreadLink) []ThreadLink {
		for i := range links {
			if links[i].Is(k.Repo, k.Kind, k.Number) {
				o := &links[i]
				o.Title, o.URL, o.State, o.Draft, o.CI, o.MergeState = k.Title, k.URL, k.State, k.Draft, k.CI, k.MergeState
				o.RefreshedAt, o.AttemptedAt, o.Failures, o.LastError = k.RefreshedAt, k.AttemptedAt, k.Failures, k.LastError
			}
		}
		return links
	})
}

// ThreadLinks are one thread's links, oldest first.
func (s *Store) ThreadLinks(threadID string) ([]ThreadLink, error) {
	var t Thread
	if err := s.db.Select("id", "links").First(&t, "id = ?", threadID).Error; err != nil {
		return nil, notFound(err)
	}
	return t.Links, nil
}

// FindThreadLink is the thread's link to the issue or PR with this number in
// repo (an issue and a PR of one repository never share a number); ErrNotFound
// when it has none.
func (s *Store) FindThreadLink(threadID, repo string, number int) (*ThreadLink, error) {
	links, err := s.ThreadLinks(threadID)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if l.Repo == repo && l.Number == number {
			return &l, nil
		}
	}
	return nil, ErrNotFound
}

// LinkedThreads are the threads of projects that are not archived that have
// links, with only their id, project and links filled in: what a refresh sweep
// needs.
func (s *Store) LinkedThreads() ([]Thread, error) {
	var out []Thread
	err := s.db.Select("threads.id", "threads.project_id", "threads.links").
		Joins("JOIN projects ON projects.id = threads.project_id AND projects.archived_at IS NULL").
		Where("threads.links IS NOT NULL AND threads.links NOT IN ('', 'null', '[]')").
		Find(&out).Error
	return out, err
}
