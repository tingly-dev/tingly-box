package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GitHubAppRecord is the GitHub App this tingly-box created for its user
// through the App Manifest flow (.design/github.md §3). There is at most
// one; the private key never leaves this table.
type GitHubAppRecord struct {
	AppID         int64     `gorm:"primaryKey;autoIncrement:false;column:app_id"`
	Slug          string    `gorm:"column:slug;size:128;not null"`
	Owner         string    `gorm:"column:owner;size:128"`
	OwnerType     string    `gorm:"column:owner_type;size:16"` // User | Organization
	ClientID      string    `gorm:"column:client_id;size:128"`
	ClientSecret  string    `gorm:"column:client_secret;size:256"`
	WebhookSecret string    `gorm:"column:webhook_secret;size:256"`
	PrivateKey    string    `gorm:"column:private_key;type:text"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (GitHubAppRecord) TableName() string { return "github_apps" }

// GitHubInstallationRecord is one account the App is installed on.
type GitHubInstallationRecord struct {
	InstallationID      int64     `gorm:"primaryKey;autoIncrement:false;column:installation_id"`
	AppID               int64     `gorm:"index;column:app_id"`
	Account             string    `gorm:"column:account;size:128"`
	AccountType         string    `gorm:"column:account_type;size:16"`
	RepositorySelection string    `gorm:"column:repository_selection;size:16"` // all | selected
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (GitHubInstallationRecord) TableName() string { return "github_installations" }

// GitHubPATRecord is the fallback fine-grained personal access token for
// users who cannot create an App (.design/github.md §1.4). At most one.
type GitHubPATRecord struct {
	ID        uint      `gorm:"primaryKey;column:id"` // always 1
	Token     string    `gorm:"column:token;type:text;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (GitHubPATRecord) TableName() string { return "github_pats" }

// GitHubSessionPRRecord remembers which branch and PR a session opened, so
// the write tools only touch their own (.design/github.md §5).
type GitHubSessionPRRecord struct {
	SessionID string    `gorm:"primaryKey;column:session_id;size:128"`
	Repo      string    `gorm:"primaryKey;column:repo;size:256"` // owner/name
	Branch    string    `gorm:"column:branch;size:256"`
	PRNumber  int       `gorm:"column:pr_number"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (GitHubSessionPRRecord) TableName() string { return "github_session_prs" }

// GitHubStore persists the GitHub integration's state.
type GitHubStore struct {
	storeConn
}

func newGitHubStore(conn storeConn) (*GitHubStore, error) {
	if err := conn.db.AutoMigrate(&GitHubAppRecord{}, &GitHubInstallationRecord{}, &GitHubPATRecord{}, &GitHubSessionPRRecord{}); err != nil {
		return nil, fmt.Errorf("migrate github tables: %w", err)
	}
	return &GitHubStore{storeConn: conn}, nil
}

// App returns the stored App, or (nil, nil) when none is connected.
func (s *GitHubStore) App() (*GitHubAppRecord, error) {
	var rec GitHubAppRecord
	err := s.db.Order("created_at DESC").First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// SaveApp replaces any previous App (and drops its installations): a
// tingly-box has one App at a time.
func (s *GitHubStore) SaveApp(rec GitHubAppRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&GitHubInstallationRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&GitHubAppRecord{}).Error; err != nil {
			return err
		}
		return tx.Create(&rec).Error
	})
}

// DeleteApp removes the App, its installations and session PR records.
func (s *GitHubStore) DeleteApp() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, m := range []any{&GitHubInstallationRecord{}, &GitHubAppRecord{}, &GitHubSessionPRRecord{}} {
			if err := tx.Where("1 = 1").Delete(m).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SaveInstallation upserts one installation.
func (s *GitHubStore) SaveInstallation(rec GitHubInstallationRecord) error {
	rec.UpdatedAt = time.Now()
	return s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rec).Error
}

// Installations lists the stored installations.
func (s *GitHubStore) Installations() ([]GitHubInstallationRecord, error) {
	var out []GitHubInstallationRecord
	err := s.db.Order("account").Find(&out).Error
	return out, err
}

// ReplaceInstallations makes the stored set exactly recs (used after a sync
// with GitHub, so an uninstalled account disappears).
func (s *GitHubStore) ReplaceInstallations(recs []GitHubInstallationRecord) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&GitHubInstallationRecord{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for i := range recs {
			recs[i].UpdatedAt = now
			if err := tx.Create(&recs[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// PAT returns the stored token, or "" when none.
func (s *GitHubStore) PAT() (string, error) {
	var rec GitHubPATRecord
	err := s.db.First(&rec, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return rec.Token, err
}

// SetPAT stores token; an empty token deletes it.
func (s *GitHubStore) SetPAT(token string) error {
	if token == "" {
		return s.db.Where("1 = 1").Delete(&GitHubPATRecord{}).Error
	}
	return s.db.Save(&GitHubPATRecord{ID: 1, Token: token, CreatedAt: time.Now()}).Error
}

// SessionPR returns what a session opened in repo, or (nil, nil).
func (s *GitHubStore) SessionPR(sessionID, repo string) (*GitHubSessionPRRecord, error) {
	var rec GitHubSessionPRRecord
	err := s.db.Where("session_id = ? AND repo = ?", sessionID, repo).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// SaveSessionPR upserts the (session, repo) record.
func (s *GitHubStore) SaveSessionPR(rec GitHubSessionPRRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	return s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rec).Error
}
