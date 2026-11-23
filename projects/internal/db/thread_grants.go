package db

import (
	"time"

	"gorm.io/gorm"
)

// Grant scopes.
const (
	// GrantOnce is for this turn: a command grant is used up by a call that
	// runs it without an error, and whatever is left is dropped when the turn
	// ends.
	GrantOnce = "once"
	// GrantThread lasts until the thread is closed, or until it expires.
	GrantThread = "thread"
)

// ThreadGrant is one permission the user gave a thread on a request_permission
// card. Command is an exact command line the thread runs with its own Bash
// tool; Rule is a Claude Code permission rule with a pattern; at most one is
// set, and with neither it is a record of the user's yes and enforces nothing.
// See projects/.design/thread-permissions.md.
type ThreadGrant struct {
	// ID is the request's id, which the card and the thread's note quote.
	ID        string     `json:"id"`
	Action    string     `json:"action"`
	Command   string     `json:"command,omitempty"`
	Rule      string     `json:"rule,omitempty"`
	Scope     string     `json:"scope"`
	GrantedAt time.Time  `json:"granted_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Live is whether g still holds at now.
func (g ThreadGrant) Live(now time.Time) bool {
	return g.ExpiresAt == nil || now.Before(*g.ExpiresAt)
}

// UpdateThreadGrants changes a thread's grants in one transaction: fn gets
// them as stored and returns what to store. It is the one writer of the column
// (SaveThread leaves it out), so a grant used up here is not brought back by a
// stale copy of the thread.
func (s *Store) UpdateThreadGrants(threadID string, fn func([]ThreadGrant) []ThreadGrant) ([]ThreadGrant, error) {
	var out []ThreadGrant
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var t Thread
		if err := tx.Select("id", "grants").First(&t, "id = ?", threadID).Error; err != nil {
			return notFound(err)
		}
		t.Grants = fn(t.Grants)
		out = t.Grants
		return tx.Model(&Thread{}).Where("id = ?", threadID).Select("Grants").UpdateColumns(&t).Error
	})
	return out, err
}
