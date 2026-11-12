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
		if thread.Agent == AgentLLM {
			return errors.New("cannot clear the project coordinator session")
		}
		updates := map[string]any{
			"session_id": newID, "turns": 0, "context_tokens": 0, "context_window": 0,
			"last_turn_ms": 0, "last_turn_at": nil, "reported_model": "",
		}
		if thread.Status == ThreadWaiting || thread.Status == ThreadFailed {
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
		payload, err := json.Marshal(map[string]string{"name": "clear", "state": "ok", "previous_session_id": expectedID, "session_id": newID})
		if err != nil {
			return err
		}
		event.ThreadID, event.Kind, event.Content, event.Payload = threadID, EvCommand, "clear", string(payload)
		return (&Store{db: tx}).AddThreadEvent(event)
	})
}

// HasCoordinatorSessionBoundary distinguishes an explicit fresh session from
// the first session created while migrating an existing conversation.
func (s *Store) HasCoordinatorSessionBoundary(threadID, sessionID string) (bool, error) {
	var count int64
	// Retain recognition of boundaries written by the earlier /new version.
	err := s.db.Model(&ThreadEvent{}).Where("thread_id = ? AND kind = ? AND content IN ? AND json_extract(payload, '$.session_id') = ?", threadID, EvCommand, []string{"clear", "new"}, sessionID).Count(&count).Error
	return count != 0, err
}
