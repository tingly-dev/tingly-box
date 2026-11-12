package db

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

// RotateThreadSession changes only session-specific state and records a durable
// boundary. The old transcript and the thread's accumulated usage remain.
func (s *Store) RotateThreadSession(threadID, expectedID, newID string, event *ThreadEvent) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var thread Thread
		if err := tx.Select("agent", "status", "commits_ahead", "merged_at").First(&thread, "id = ?", threadID).Error; err != nil {
			return notFound(err)
		}
		updates := map[string]any{
			"session_id": newID, "turns": 0, "context_tokens": 0, "context_window": 0,
			"last_turn_ms": 0, "last_turn_at": nil, "reported_model": "",
		}
		if thread.Agent != AgentLLM && (thread.Status == ThreadWaiting || thread.Status == ThreadFailed) {
			updates["status"], updates["wait_reason"] = ThreadIdle, ""
			if thread.CommitsAhead > 0 && thread.MergedAt == nil {
				updates["status"] = ThreadReady
			}
		}
		res := tx.Model(&Thread{}).Where("id = ? AND COALESCE(session_id, '') = ?", threadID, expectedID).UpdateColumns(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("session changed while starting a new session")
		}
		if err := tx.Where("thread_id = ?", threadID).Delete(&CoordinatorSettingsDelivery{}).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]string{"name": "new", "state": "ok", "previous_session_id": expectedID, "session_id": newID})
		if err != nil {
			return err
		}
		event.ThreadID, event.Kind, event.Content, event.Payload = threadID, EvCommand, "new", string(payload)
		return (&Store{db: tx}).AddThreadEvent(event)
	})
}

// HasCoordinatorSessionBoundary distinguishes an explicit fresh session from
// the first session created while migrating an existing conversation.
func (s *Store) HasCoordinatorSessionBoundary(threadID, sessionID string) (bool, error) {
	var count int64
	err := s.db.Model(&ThreadEvent{}).Where("thread_id = ? AND kind = ? AND content = ? AND json_extract(payload, '$.session_id') = ?", threadID, EvCommand, "new", sessionID).Count(&count).Error
	return count != 0, err
}

// HasOpenCoordinatorInputs includes deliveries whose outcome is uncertain.
func (s *Store) HasOpenCoordinatorInputs(projectID string) (bool, error) {
	var count int64
	err := s.db.Model(&CoordinatorInput{}).Where("project_id = ? AND state IN ?", projectID, []string{CoordinatorPending, CoordinatorRunning, CoordinatorUncertain}).Count(&count).Error
	return count != 0, err
}
