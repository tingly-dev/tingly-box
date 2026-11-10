package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CoordinatorSettingsDelivery records only a digest of settings confirmed in a session.
// An empty digest means a prior delivery may have reached the transcript.
type CoordinatorSettingsDelivery struct {
	ThreadID  string `gorm:"primaryKey;size:36"`
	SessionID string `gorm:"size:64;not null"`
	Digest    string `gorm:"size:64;not null"`
}

// BeginCoordinatorSettings reads the last confirmation and invalidates it before
// dispatch. A crash or skipped uncertain input must not leave a stale marker.
func (s *Store) BeginCoordinatorSettings(threadID, sessionID string) (string, error) {
	var digest string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var thread Thread
		if err := tx.Select("id", "session_id").First(&thread, "id = ? AND session_id = ?", threadID, sessionID).Error; err != nil {
			return notFound(err)
		}
		var delivery CoordinatorSettingsDelivery
		err := tx.First(&delivery, "thread_id = ?", threadID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && delivery.SessionID == sessionID {
			digest = delivery.Digest
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&CoordinatorSettingsDelivery{ThreadID: threadID, SessionID: sessionID}).Error
	})
	return digest, err
}

// InvalidateCoordinatorSettings ensures compaction or uncertain delivery is
// followed by a fresh settings snapshot, including across service restarts.
func (s *Store) InvalidateCoordinatorSettings(threadID string) error {
	return s.db.Where("thread_id = ?", threadID).Delete(&CoordinatorSettingsDelivery{}).Error
}

// FinishCoordinatorTurn commits the successful input and its settings snapshot
// together. Neither a changed session nor a failed input advances the marker.
func (s *Store) FinishCoordinatorTurn(inputID, threadID, sessionID, digest string, turns int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Thread{}).Where("id = ? AND session_id = ?", threadID, sessionID).UpdateColumn("turns", turns)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("coordinator session changed")
		}
		res = tx.Model(&CoordinatorSettingsDelivery{}).Where("thread_id = ? AND session_id = ?", threadID, sessionID).UpdateColumn("digest", digest)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("coordinator settings delivery missing")
		}
		return (&Store{db: tx}).FinishCoordinatorInput(inputID, true)
	})
}

// CoordinatorInput tracks the delivery of a conversation event to the
// coordinator session. Running is intentionally not retried after a crash: the
// session may already have executed tools before the process disappeared.
type CoordinatorInput struct {
	ID        string `gorm:"primaryKey;size:36"`
	ProjectID string `gorm:"size:36;index;not null"`
	EventID   string `gorm:"size:36;uniqueIndex;not null"`
	State     string `gorm:"size:20;index;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	CoordinatorPending   = "pending"
	CoordinatorRunning   = "running"
	CoordinatorDone      = "done"
	CoordinatorUncertain = "uncertain"
)

// AddCoordinatorInput atomically saves the visible event and its delivery.
func (s *Store) AddCoordinatorInput(projectID string, e *ThreadEvent) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var project Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return notFound(err)
		}
		if project.MainThreadID == "" || e.ThreadID != project.MainThreadID {
			return errors.New("event is not in the project's main thread")
		}
		if err := (&Store{db: tx}).AddThreadEvent(e); err != nil {
			return err
		}
		return tx.Create(&CoordinatorInput{ID: newID(), ProjectID: projectID, EventID: e.ID, State: CoordinatorPending}).Error
	})
}

// SaveProjectWithCoordinatorInput atomically saves settings and their notice.
func (s *Store) SaveProjectWithCoordinatorInput(p *Project, e *ThreadEvent) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(p).Error; err != nil {
			return err
		}
		if e == nil {
			return nil
		}
		return (&Store{db: tx}).AddCoordinatorInput(p.ID, e)
	})
}

func (s *Store) CoordinatorProjectsWithInputs() ([]string, error) {
	var ids []string
	err := s.db.Model(&CoordinatorInput{}).Distinct("project_id").Where("state IN ?", []string{CoordinatorPending, CoordinatorUncertain}).Pluck("project_id", &ids).Error
	return ids, err
}

// PendingCoordinatorInputs counts the inputs waiting behind the running turn.
func (s *Store) PendingCoordinatorInputs(projectID string) (int, error) {
	var n int64
	err := s.db.Model(&CoordinatorInput{}).Where("project_id = ? AND state = ?", projectID, CoordinatorPending).Count(&n).Error
	return int(n), err
}

func (s *Store) UncertainCoordinatorInput(projectID string) (string, error) {
	var input CoordinatorInput
	err := s.db.Where("project_id = ? AND state = ?", projectID, CoordinatorUncertain).Order("created_at ASC, rowid ASC").First(&input).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return input.ID, err
}

// MarkCoordinatorSessionStarted records Claude's session init before it can
// execute tools, so a later restart resumes rather than re-creating its ID.
func (s *Store) MarkCoordinatorSessionStarted(threadID, sessionID string) error {
	res := s.db.Model(&Thread{}).Where("id = ? AND session_id = ? AND turns = 0", threadID, sessionID).UpdateColumn("turns", 1)
	return res.Error
}

// ClaimCoordinatorInput claims the oldest pending input. An earlier ambiguous
// or running delivery blocks later inputs so they cannot overtake it.
func (s *Store) ClaimCoordinatorInput(projectID string) (*CoordinatorInput, error) {
	var input CoordinatorInput
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ? AND state IN ?", projectID, []string{CoordinatorPending, CoordinatorRunning, CoordinatorUncertain}).Order("created_at ASC, rowid ASC").First(&input).Error; err != nil {
			return notFound(err)
		}
		if input.State != CoordinatorPending {
			return fmt.Errorf("earlier coordinator input %s needs recovery", input.ID)
		}
		res := tx.Model(&CoordinatorInput{}).Where("id = ? AND state = ?", input.ID, CoordinatorPending).Update("state", CoordinatorRunning)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("coordinator input was claimed concurrently")
		}
		input.State = CoordinatorRunning
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &input, nil
}

// FinishCoordinatorInput records a successful turn, or an ambiguous failure.
func (s *Store) FinishCoordinatorInput(id string, success bool) error {
	state := CoordinatorUncertain
	if success {
		state = CoordinatorDone
	}
	res := s.db.Model(&CoordinatorInput{}).Where("id = ? AND state = ?", id, CoordinatorRunning).Update("state", state)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errors.New("coordinator input is not running")
	}
	return nil
}

// RecoverCoordinatorInputs marks in-flight deliveries as uncertain at startup.
func (s *Store) RecoverCoordinatorInputs() error {
	return s.db.Model(&CoordinatorInput{}).Where("state = ?", CoordinatorRunning).Update("state", CoordinatorUncertain).Error
}

// ResolveCoordinatorInput records an explicit recovery decision for an
// uncertain delivery. Retrying may repeat side effects and is never automatic.
func (s *Store) ResolveCoordinatorInput(projectID, id string, retry bool) error {
	state := CoordinatorDone
	if retry {
		state = CoordinatorPending
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&CoordinatorInput{}).Where("project_id = ? AND id = ? AND state = ?", projectID, id, CoordinatorUncertain).Update("state", state)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("coordinator input is not uncertain in this project")
		}
		// Older versions could have sent settings in an uncertain notice.
		return tx.Where("thread_id IN (SELECT main_thread_id FROM projects WHERE id = ?)", projectID).Delete(&CoordinatorSettingsDelivery{}).Error
	})
}
