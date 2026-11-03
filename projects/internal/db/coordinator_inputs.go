package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

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
	res := s.db.Model(&CoordinatorInput{}).Where("project_id = ? AND id = ? AND state = ?", projectID, id, CoordinatorUncertain).Update("state", state)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errors.New("coordinator input is not uncertain in this project")
	}
	return nil
}
